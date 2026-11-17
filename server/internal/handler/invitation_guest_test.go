package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// Spec 7.1 rule 9 (D53): a guest is a member with the is_guest flag, invited
// as a member or viewer only. An invitation for a guest names the projects the
// guest may see.

func guestInviteRequest(t *testing.T, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	req := newRequest("POST", "/api/workspaces/"+testWorkspaceID+"/members", body)
	req = withURLParam(req, "id", testWorkspaceID)
	w := httptest.NewRecorder()
	testHandler.CreateInvitation(w, req)
	return w
}

func allowAllInvitationLimits(t *testing.T) {
	t.Helper()
	useInvitationRateLimiters(t, InvitationRateLimiters{
		Actor:     &stubInvitationRateLimiter{allowed: true},
		Workspace: &stubInvitationRateLimiter{allowed: true},
		Recipient: &stubInvitationRateLimiter{allowed: true},
	})
}

// A guest is never an owner or an admin: the invitation is refused with
// project_member_invalid before any row is written.
func TestGuestCannotBeOwnerOrAdmin(t *testing.T) {
	clearInvitationsForTestWorkspace(t)
	allowAllInvitationLimits(t)

	for _, role := range []string{"admin", "owner"} {
		w := guestInviteRequest(t, map[string]any{
			"email":             invitationTestEmail,
			"role":              role,
			"is_guest":          true,
			"guest_project_ids": []string{},
		})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("guest %s invitation: status %d, want 400 (body %q)", role, w.Code, w.Body.String())
		}
		var body struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Code != "project_member_invalid" {
			t.Errorf("guest %s invitation: code %q, want project_member_invalid (body %q)", role, body.Code, w.Body.String())
		}
	}
	if n := dbfxCountInvitations(t); n != 0 {
		t.Fatalf("refused guest invitations left %d rows", n)
	}
}

// The invitation stores the guest flag and the granted projects. Accepting it
// creates the member row with is_guest and the project grants in one
// transaction: a commit that fails leaves neither behind, and the invitation
// stays pending.
func TestGuestInviteCreatesMemberFlagAndGrantsInOneTx(t *testing.T) {
	clearInvitationsForTestWorkspace(t)
	allowAllInvitationLimits(t)
	ctx := context.Background()

	projectID := dbfx.Project(t, "guest-visible-"+uuid.NewString())
	email := "guest-invitee-" + uuid.NewString() + "@multica.ai"
	userID := dbfx.User(t, "Guest Invitee", email)

	w := guestInviteRequest(t, map[string]any{
		"email":             email,
		"role":              "viewer",
		"is_guest":          true,
		"guest_project_ids": []string{projectID},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("guest invitation: status %d, want 201 (body %q)", w.Code, w.Body.String())
	}
	var inv InvitationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &inv); err != nil {
		t.Fatalf("decode invitation: %v", err)
	}
	var isGuest bool
	var grantedProjects int
	if err := testPool.QueryRow(ctx,
		`SELECT is_guest, cardinality(guest_project_ids) FROM workspace_invitation WHERE id = $1`, inv.ID,
	).Scan(&isGuest, &grantedProjects); err != nil {
		t.Fatalf("read invitation: %v", err)
	}
	if !isGuest || grantedProjects != 1 {
		t.Fatalf("invitation is_guest=%v projects=%d, want true and 1", isGuest, grantedProjects)
	}

	// Force the commit of the accept transaction to fail: nothing may be left.
	previous := testHandler.TxStarter
	testHandler.TxStarter = rollbackOnCommitTxStarter{pool: testPool}
	accept := func() *httptest.ResponseRecorder {
		req := newRequest("POST", "/api/invitations/"+inv.ID+"/accept", nil)
		req.Header.Set("X-User-ID", userID)
		req = withURLParam(req, "id", inv.ID)
		rec := httptest.NewRecorder()
		testHandler.AcceptInvitation(rec, req)
		return rec
	}
	if rec := accept(); rec.Code == http.StatusOK {
		t.Fatalf("accept with a failing commit returned 200")
	}
	if n := dbfxCountMembers(t, userID); n != 0 {
		t.Fatalf("a failed accept left %d member rows", n)
	}
	if n := dbfxCountGrants(t, projectID, userID); n != 0 {
		t.Fatalf("a failed accept left %d project grants", n)
	}
	var status string
	if err := testPool.QueryRow(ctx, `SELECT status FROM workspace_invitation WHERE id = $1`, inv.ID).Scan(&status); err != nil || status != "pending" {
		t.Fatalf("invitation status after a failed accept = %q (%v), want pending", status, err)
	}

	testHandler.TxStarter = previous
	if rec := accept(); rec.Code != http.StatusOK {
		t.Fatalf("accept: status %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	var memberIsGuest bool
	if err := testPool.QueryRow(ctx,
		`SELECT is_guest FROM member WHERE workspace_id = $1 AND user_id = $2`, testWorkspaceID, userID,
	).Scan(&memberIsGuest); err != nil {
		t.Fatalf("read member: %v", err)
	}
	if !memberIsGuest {
		t.Errorf("accepted guest member has is_guest = false, want true")
	}
	if n := dbfxCountGrants(t, projectID, userID); n != 1 {
		t.Errorf("accepted guest has %d project grants, want 1", n)
	}
}

func dbfxCountInvitations(t *testing.T) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM workspace_invitation WHERE workspace_id = $1 AND invitee_email = $2`,
		parseUUID(testWorkspaceID), invitationTestEmail,
	).Scan(&n); err != nil {
		t.Fatalf("count invitations: %v", err)
	}
	return n
}

func dbfxCountMembers(t *testing.T, userID string) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM member WHERE workspace_id = $1 AND user_id = $2`,
		parseUUID(testWorkspaceID), parseUUID(userID),
	).Scan(&n); err != nil {
		t.Fatalf("count members: %v", err)
	}
	return n
}

func dbfxCountGrants(t *testing.T, projectID, userID string) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM project_member WHERE project_id = $1 AND user_id = $2`,
		parseUUID(projectID), parseUUID(userID),
	).Scan(&n); err != nil {
		t.Fatalf("count grants: %v", err)
	}
	return n
}
