package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/platform-management/audit"
)

// Spec 7.1 rule 9 and 7.7: owner and admin set the is_guest flag, only on a
// member or viewer. The flag changes in the same transaction as the role and
// is audited.

// guestTestMember adds a member with the given role and guest flag and returns
// the user id and the member id.
func guestTestMember(t *testing.T, role string, guest bool) (userID, memberID string) {
	t.Helper()
	userID = createTestUserAndMember(t, role)
	if guest {
		setMemberGuest(t, userID, true)
	}
	if err := testPool.QueryRow(context.Background(),
		`SELECT id FROM member WHERE workspace_id = $1 AND user_id = $2`,
		parseUUID(testWorkspaceID), parseUUID(userID),
	).Scan(&memberID); err != nil {
		t.Fatalf("read member id: %v", err)
	}
	return userID, memberID
}

func setMemberGuest(t *testing.T, userID string, guest bool) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`UPDATE member SET is_guest = $3 WHERE workspace_id = $1 AND user_id = $2`,
		parseUUID(testWorkspaceID), parseUUID(userID), guest,
	); err != nil {
		t.Fatalf("set guest flag: %v", err)
	}
}

func memberRoleAndGuest(t *testing.T, userID string) (role string, guest bool) {
	t.Helper()
	if err := testPool.QueryRow(context.Background(),
		`SELECT role, is_guest FROM member WHERE workspace_id = $1 AND user_id = $2`,
		parseUUID(testWorkspaceID), parseUUID(userID),
	).Scan(&role, &guest); err != nil {
		t.Fatalf("read member: %v", err)
	}
	return role, guest
}

// updateMemberAs calls UpdateMember as callerID.
func updateMemberAs(t *testing.T, callerID, memberID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	req := newRequest("PATCH", "/api/workspaces/"+testWorkspaceID+"/members/"+memberID, body)
	req.Header.Set("X-User-ID", callerID)
	req = withURLParams(req, "id", testWorkspaceID, "memberId", memberID)
	w := httptest.NewRecorder()
	testHandler.UpdateMember(w, req)
	return w
}

func guestErrCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return body.Code
}

func TestUpdateMemberGuestToggle(t *testing.T) {
	adminID := createTestUserAndMember(t, "admin")
	for name, caller := range map[string]string{"owner": testUserID, "admin": adminID} {
		t.Run(name, func(t *testing.T) {
			userID, memberID := guestTestMember(t, "member", false)

			w := updateMemberAs(t, caller, memberID, map[string]any{"role": "member", "is_guest": true})
			if w.Code != http.StatusOK {
				t.Fatalf("set guest: status %d, want 200 (body %q)", w.Code, w.Body.String())
			}
			if role, guest := memberRoleAndGuest(t, userID); role != "member" || !guest {
				t.Fatalf("after set: role=%q guest=%v, want member and true", role, guest)
			}

			// Without is_guest the current flag is kept.
			w = updateMemberAs(t, caller, memberID, map[string]any{"role": "viewer"})
			if w.Code != http.StatusOK {
				t.Fatalf("role only: status %d, want 200 (body %q)", w.Code, w.Body.String())
			}
			if role, guest := memberRoleAndGuest(t, userID); role != "viewer" || !guest {
				t.Fatalf("after role only: role=%q guest=%v, want viewer and true", role, guest)
			}

			w = updateMemberAs(t, caller, memberID, map[string]any{"role": "viewer", "is_guest": false})
			if w.Code != http.StatusOK {
				t.Fatalf("unset guest: status %d, want 200 (body %q)", w.Code, w.Body.String())
			}
			if role, guest := memberRoleAndGuest(t, userID); role != "viewer" || guest {
				t.Fatalf("after unset: role=%q guest=%v, want viewer and false", role, guest)
			}
		})
	}
}

func TestUpdateMemberGuestRefusedForRoleOutsideMemberViewer(t *testing.T) {
	for _, role := range []string{"admin", "owner"} {
		userID, memberID := guestTestMember(t, "member", false)
		w := updateMemberAs(t, testUserID, memberID, map[string]any{"role": role, "is_guest": true})
		if w.Code != http.StatusBadRequest || guestErrCode(t, w) != "project_member_invalid" {
			t.Errorf("is_guest with role %s: status %d code %q, want 400 project_member_invalid (body %q)",
				role, w.Code, guestErrCode(t, w), w.Body.String())
		}
		if r, g := memberRoleAndGuest(t, userID); r != "member" || g {
			t.Errorf("refused %s change altered the row: role=%q guest=%v", role, r, g)
		}
	}

	// A guest promoted without being un-guested in the same request.
	userID, memberID := guestTestMember(t, "member", true)
	w := updateMemberAs(t, testUserID, memberID, map[string]any{"role": "admin"})
	if w.Code != http.StatusBadRequest || guestErrCode(t, w) != "project_member_invalid" {
		t.Errorf("guest promoted to admin: status %d code %q, want 400 project_member_invalid (body %q)",
			w.Code, guestErrCode(t, w), w.Body.String())
	}
	if r, g := memberRoleAndGuest(t, userID); r != "member" || !g {
		t.Errorf("refused promotion altered the row: role=%q guest=%v", r, g)
	}

	// Un-guesting in the same request lets the promotion through.
	w = updateMemberAs(t, testUserID, memberID, map[string]any{"role": "admin", "is_guest": false})
	if w.Code != http.StatusOK {
		t.Fatalf("un-guest and promote: status %d, want 200 (body %q)", w.Code, w.Body.String())
	}
	if r, g := memberRoleAndGuest(t, userID); r != "admin" || g {
		t.Errorf("after un-guest and promote: role=%q guest=%v, want admin and false", r, g)
	}
}

func TestUpdateMemberGuestNeedsManagerRole(t *testing.T) {
	callerID := createTestUserAndMember(t, "member")
	userID, memberID := guestTestMember(t, "member", false)

	w := updateMemberAs(t, callerID, memberID, map[string]any{"role": "member", "is_guest": true})
	if w.Code != http.StatusForbidden {
		t.Fatalf("member sets guest: status %d, want 403 (body %q)", w.Code, w.Body.String())
	}
	if _, g := memberRoleAndGuest(t, userID); g {
		t.Errorf("a refused call set the guest flag")
	}
}

func TestGuestFlagAudited(t *testing.T) {
	userID, memberID := guestTestMember(t, "member", false)

	call := func(prefix string, guest bool) string {
		req := newRequest("PATCH", "/api/workspaces/"+testWorkspaceID+"/members/"+memberID,
			map[string]any{"role": "member", "is_guest": guest})
		req = withURLParams(req, "id", testWorkspaceID, "memberId", memberID)
		req, id := auditRequest(t, req, prefix)
		w := httptest.NewRecorder()
		testHandler.UpdateMember(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("is_guest=%v: status %d (body %q)", guest, w.Code, w.Body.String())
		}
		return id
	}

	setID := call("guest-set-", true)
	unsetID := call("guest-unset-", false)

	for _, c := range []struct {
		id, action string
		from, to   bool
	}{
		{setID, audit.ActionMemberGuestSet, false, true},
		{unsetID, audit.ActionMemberGuestUnset, true, false},
	} {
		rows := auditEvents(t, c.id, c.action)
		if len(rows) != 1 {
			t.Fatalf("%s rows = %d, want 1", c.action, len(rows))
		}
		r := rows[0]
		if r.TargetType != "member" || r.TargetID != userID {
			t.Errorf("%s target = %s/%s, want member/%s", c.action, r.TargetType, r.TargetID, userID)
		}
		if r.Detail["from"] != c.from || r.Detail["to"] != c.to {
			t.Errorf("%s detail = %v, want from %v to %v", c.action, r.Detail, c.from, c.to)
		}
		if n := len(auditEvents(t, c.id, audit.ActionMemberRoleChanged)); n != 1 {
			t.Errorf("%s request wrote %d role_changed rows, want 1 (the role call is unchanged)", c.action, n)
		}
	}
	// Each request wrote only its own guest action.
	if n := len(auditEvents(t, setID, audit.ActionMemberGuestUnset)); n != 0 {
		t.Errorf("set request wrote %d guest_unset rows", n)
	}
	if n := len(auditEvents(t, unsetID, audit.ActionMemberGuestSet)); n != 0 {
		t.Errorf("unset request wrote %d guest_set rows", n)
	}
}

func TestGuestMemberListHasNoEmail(t *testing.T) {
	guestUserID, _ := guestTestMember(t, "viewer", true)

	list := func(userID string) []MemberWithUserResponse {
		req := newRequest("GET", "/api/workspaces/"+testWorkspaceID+"/members", nil)
		req.Header.Set("X-User-ID", userID)
		req = withURLParam(req, "id", testWorkspaceID)
		w := httptest.NewRecorder()
		testHandler.ListMembersWithUser(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("list as %s: status %d (body %q)", userID, w.Code, w.Body.String())
		}
		var rows []MemberWithUserResponse
		if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		var raw []map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &raw)
		for _, row := range raw {
			if _, ok := row["email"]; !ok {
				t.Errorf("row without an email key: the JSON shape must stay")
			}
		}
		return rows
	}

	rows := list(guestUserID)
	if len(rows) < 2 {
		t.Fatalf("guest sees %d rows, want the workspace members", len(rows))
	}
	for _, r := range rows {
		if r.Email != "" {
			t.Errorf("guest sees email %q for member %s", r.Email, r.UserID)
		}
		if r.Name == "" {
			t.Errorf("guest sees no name for member %s", r.UserID)
		}
	}

	withEmail := 0
	for _, r := range list(testUserID) {
		if r.Email != "" {
			withEmail++
		}
	}
	if withEmail == 0 {
		t.Errorf("owner sees no emails in the member list")
	}
}
