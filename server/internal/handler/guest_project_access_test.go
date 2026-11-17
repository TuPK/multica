package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/projectaccess"
	"github.com/multica-ai/multica/server/internal/seatcapacity"
)

// A guest sees a project only while a project_member row grants it. Removing
// the grant takes effect on the next request: the project is a 404.
func TestGuestLosesAccessAtOnce(t *testing.T) {
	projectID := dbfx.Project(t, "guest-access-"+uuid.NewString())
	guestUserID, _ := guestTestMember(t, "member", true)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM project_member WHERE project_id = $1`, parseUUID(projectID))
	})
	dbfx.Exec(t, `INSERT INTO project_member (project_id, workspace_id, user_id) VALUES ($1, $2, $3)`,
		projectID, testWorkspaceID, guestUserID)

	getProject := func() *httptest.ResponseRecorder {
		req := newRequest("GET", "/api/projects/"+projectID, nil)
		req.Header.Set("X-User-ID", guestUserID)
		req = withURLParam(req, "id", projectID)
		w := httptest.NewRecorder()
		testHandler.GetProject(w, req)
		return w
	}
	if w := getProject(); w.Code != http.StatusOK {
		t.Fatalf("granted guest GET project: status %d, want 200 (body %q)", w.Code, w.Body.String())
	}

	access := projectaccess.NewHandler(testHandler.Queries, testPool)
	req := newRequest("DELETE", "/api/projects/"+projectID+"/members/"+guestUserID, nil)
	req = withURLParams(req, "id", projectID, "userId", guestUserID)
	w := httptest.NewRecorder()
	access.RemoveMember(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("remove grant: status %d, want 204 (body %q)", w.Code, w.Body.String())
	}

	w = getProject()
	if w.Code != http.StatusNotFound {
		t.Fatalf("guest GET project after removal: status %d, want 404 (body %q)", w.Code, w.Body.String())
	}

	// GetProject (project.go, owned by P41.3) answers without a code, so the
	// code is read from the project access route, which shares the same scope.
	req = newRequest("GET", "/api/projects/"+projectID+"/members", nil)
	req.Header.Set("X-User-ID", guestUserID)
	req = withURLParam(req, "id", projectID)
	w = httptest.NewRecorder()
	access.ListMembers(w, req)
	if w.Code != http.StatusNotFound || guestErrCode(t, w) != "project_not_found" {
		t.Errorf("guest GET project members after removal: status %d code %q, want 404 project_not_found (body %q)",
			w.Code, guestErrCode(t, w), w.Body.String())
	}
}

// O-R4-2: a guest takes a seat like any member. A guest invitation reserves
// one seat when it is created and consumes it when it is accepted, the same
// calls a normal member invitation makes.
func TestGuestCountsAsSeat(t *testing.T) {
	clearInvitationsForTestWorkspace(t)
	allowAllInvitationLimits(t)

	run := func(guest bool) (reserve, consume int) {
		capacity := &stubSeatCapacity{reserveDecision: seatcapacity.Decision{Managed: true, Allowed: true}}
		useSeatCapacity(t, capacity)
		email := "seat-invitee-" + uuid.NewString() + "@multica.ai"
		userID := dbfx.User(t, "Seat Invitee", email)
		body := map[string]any{"email": email, "role": "member"}
		if guest {
			body["is_guest"] = true
			body["guest_project_ids"] = []string{}
		}
		w := guestInviteRequest(t, body)
		if w.Code != http.StatusCreated {
			t.Fatalf("invite (guest=%v): status %d (body %q)", guest, w.Code, w.Body.String())
		}
		var inv InvitationResponse
		if err := json.Unmarshal(w.Body.Bytes(), &inv); err != nil {
			t.Fatalf("decode invitation: %v", err)
		}

		req := newRequest("POST", "/api/invitations/"+inv.ID+"/accept", nil)
		req.Header.Set("X-User-ID", userID)
		req = withURLParam(req, "id", inv.ID)
		rec := httptest.NewRecorder()
		testHandler.AcceptInvitation(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("accept (guest=%v): status %d (body %q)", guest, rec.Code, rec.Body.String())
		}
		if _, g := memberRoleAndGuest(t, userID); g != guest {
			t.Fatalf("accepted member is_guest=%v, want %v", g, guest)
		}
		return capacity.reserveCalls, capacity.consumeCalls
	}

	normalReserve, normalConsume := run(false)
	guestReserve, guestConsume := run(true)
	if guestReserve != 1 || guestConsume != 1 {
		t.Errorf("guest invitation: reserve=%d consume=%d, want 1 and 1", guestReserve, guestConsume)
	}
	if guestReserve != normalReserve || guestConsume != normalConsume {
		t.Errorf("guest seat calls %d/%d differ from a member's %d/%d", guestReserve, guestConsume, normalReserve, normalConsume)
	}
}
