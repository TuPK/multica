package handler

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/membergroup"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// members.invite on the real invitation, member-role and member-removal
// handlers (spec 7b.2 rules 9 and 11, AC26). The router mounts POST /members
// under the key and the role and removal routes under owner and admin.
func inviteLimitsRouter(e *mgEnv) http.Handler {
	q := testHandler.Queries
	r := chi.NewRouter()
	r.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWorkspacePermissionFromURL(q, "id", membergroup.KeyMembersInvite))
			r.Post("/members", testHandler.CreateInvitation)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWorkspaceRoleFromURL(q, "id", "owner", "admin"))
			r.Patch("/members/{memberId}", testHandler.UpdateMember)
			r.Delete("/members/{memberId}", testHandler.DeleteMember)
		})
	})
	return r
}

func (e *mgEnv) invitationCount(email string) int {
	e.t.Helper()
	return dbfx.Count(e.t, `SELECT count(*) FROM workspace_invitation WHERE workspace_id = $1 AND lower(invitee_email) = lower($2)`, e.ws, email)
}

// A member in a group with members.invite invites a member and a viewer, is
// refused an admin invitation and is refused changing a role or removing
// anyone; nothing is written by a refusal.
func TestMembersInviteGroupMemberInviteAndRoleLimits(t *testing.T) {
	e := newMGEnv(t)
	e.router = inviteLimitsRouter(e)
	e.groupWith([]string{membergroup.KeyMembersInvite}, e.member)

	invite := func(role string) (string, *testutil.Response) {
		email := testutil.UniqueEmail("ac26-" + role)
		return email, e.do(http.MethodPost, "/members", e.member, map[string]any{"email": email, "role": role})
	}
	for _, role := range []string{"member", "viewer"} {
		email, got := invite(role)
		got.Want(http.StatusCreated)
		if n := e.invitationCount(email); n != 1 {
			t.Errorf("%s invitation rows = %d, want 1", role, n)
		}
	}
	email, got := invite("admin")
	wantMGError(t, got, http.StatusForbidden, "invite_role_not_assignable")
	var refusal struct {
		Role string `json:"role"`
	}
	got.JSON(&refusal)
	if refusal.Role != "admin" {
		t.Errorf("refusal names role %q, want admin", refusal.Role)
	}
	if n := e.invitationCount(email); n != 0 {
		t.Errorf("a refused admin invitation left %d rows", n)
	}

	// Another member's role and membership are out of reach, whatever the
	// target: a peer, a viewer, an admin and the caller.
	for name, user := range map[string]string{"peer": e.newMember("member"), "viewer": e.viewer, "admin": e.admin, "self": e.member} {
		id := e.memberRowID(user)
		var roleBefore string
		dbfx.QueryRow(t, `SELECT role FROM member WHERE id = $1`, id).Scan(&roleBefore)
		e.do(http.MethodPatch, "/members/"+id, e.member, map[string]any{"role": "admin"}).Want(http.StatusForbidden)
		e.do(http.MethodDelete, "/members/"+id, e.member, nil).Want(http.StatusForbidden)
		var roleAfter string
		dbfx.QueryRow(t, `SELECT role FROM member WHERE id = $1`, id).Scan(&roleAfter)
		if roleAfter != roleBefore {
			t.Errorf("%s: role %q became %q", name, roleBefore, roleAfter)
		}
	}
}

// After the person is taken out of the group, the next invitation request is
// refused with permission_denied and writes nothing.
func TestMembersInviteRemovalFromTheGroupRefusesTheNextInvitation(t *testing.T) {
	e := newMGEnv(t)
	e.router = inviteLimitsRouter(e)
	gid := e.groupWith([]string{membergroup.KeyMembersInvite}, e.member)

	first := testutil.UniqueEmail("ac26-before")
	e.do(http.MethodPost, "/members", e.member, map[string]any{"email": first, "role": "member"}).Want(http.StatusCreated)

	remove := e.req(http.MethodDelete, "/groups/"+gid+"/members/"+e.member, e.owner, nil)
	if got := testutil.Call(t, memberGroupRouter().ServeHTTP, remove); got.Code/100 != 2 {
		t.Fatalf("remove from group = %d (%s), want 2xx", got.Code, got.Text())
	}

	second := testutil.UniqueEmail("ac26-after")
	got := e.do(http.MethodPost, "/members", e.member, map[string]any{"email": second, "role": "member"})
	body := wantMGError(t, got, http.StatusForbidden, "permission_denied")
	if body.Permission != membergroup.KeyMembersInvite {
		t.Errorf("permission = %q, want %q", body.Permission, membergroup.KeyMembersInvite)
	}
	if n := e.invitationCount(second); n != 0 {
		t.Errorf("an invitation was written after the removal: %d rows", n)
	}
}

// The router keeps the role and removal routes in an owner and admin group, so
// the routers above are not a copy that drifted.
func TestMemberRoleAndRemovalRoutesAreInTheOwnerAdminGroupInTheRouter(t *testing.T) {
	raw, err := os.ReadFile("../../cmd/server/router.go")
	if err != nil {
		t.Fatalf("read router: %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	route := -1
	for i, l := range lines {
		if strings.Contains(l, `r.Route("/members/{memberId}"`) {
			route = i
			break
		}
	}
	if route < 0 {
		t.Fatal(`router.go has no r.Route("/members/{memberId}")`)
	}
	for i := route; i >= 0; i-- {
		if strings.Contains(lines[i], "r.Group(func(r chi.Router) {") {
			if !strings.Contains(lines[i+1], `RequireWorkspaceRoleFromURL(queries, "id", "owner", "admin")`) {
				t.Fatalf("/members/{memberId} is in a group that starts %q, want the owner and admin role middleware", strings.TrimSpace(lines[i+1]))
			}
			return
		}
	}
	t.Fatal("/members/{memberId} is not inside a group")
}
