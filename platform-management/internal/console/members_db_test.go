package console

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/memberrequest"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

type noSeats struct{}

func (noSeats) Release(context.Context, string, string) error { return nil }

// memberWorld is the two-tenant scope world with the real member request
// service behind the Handler.
func newMemberWorld(t *testing.T) *scopeWorld {
	t.Helper()
	w := newScopeWorld(t)
	w.deps.Members = memberrequest.New(memberrequest.Deps{
		Runner: memberrequest.NewPgxRunner(w.pool), Reader: memberrequest.NewPgxReader(w.pool), Seats: noSeats{},
	})
	w.h = New(w.deps)
	t.Cleanup(func() {
		for _, ws := range []string{w.ws1, w.ws2} {
			w.pool.Exec(context.Background(), `DELETE FROM platform_member_request WHERE workspace_id = $1`, ws)
		}
	})
	return w
}

func memberBody(email string) string {
	return `{"email":"` + email + `","role":"member"}`
}

func (w *scopeWorld) addMember(user, ws, email string) string {
	w.t.Helper()
	rec := w.do(user, "POST", "/api/platform/workspaces/"+ws+"/members", memberBody(email))
	wantStatus(w.t, rec, http.StatusAccepted)
	id, _ := decode(w.t, rec)["id"].(string)
	if id == "" {
		w.t.Fatalf("no id in %s", rec.Body.String())
	}
	return id
}

func TestMemberRequestAdminInOwnTenantSucceedsWithStaffAuditAndTenantID(t *testing.T) {
	w := newMemberWorld(t)
	email := "mr-own-" + randomHex(t) + "@example.test"
	id := w.addMember(w.admin1, w.ws1, email)
	wantStatus(t, w.do(w.admin1, "POST", "/api/platform/member-requests/"+id+"/cancel", ""), 200)

	rows, err := w.pool.Query(context.Background(), `
		SELECT action, actor_type, COALESCE(actor_id, ''), COALESCE(workspace_id::text, ''), COALESCE(tenant_id::text, ''), target_id
		FROM audit_event WHERE workspace_id = $1 AND action LIKE 'member.staff_%' ORDER BY created_at, id`, w.ws1)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var actions []string
	for rows.Next() {
		var action, actorType, actorID, ws, tenant, target string
		if err := rows.Scan(&action, &actorType, &actorID, &ws, &tenant, &target); err != nil {
			t.Fatal(err)
		}
		if actorType != "staff" || actorID != w.admin1 || ws != w.ws1 || tenant != w.t1 || target != id {
			t.Errorf("%s audit row = %s %s %s %s %s", action, actorType, actorID, ws, tenant, target)
		}
		actions = append(actions, action)
	}
	if strings.Join(actions, ",") != "member.staff_add_requested,member.staff_add_canceled" {
		t.Errorf("audit actions = %v, want requested then canceled", actions)
	}
}

func TestMemberRequestAdminOutsideTenantIsNotFoundAndWritesNothing(t *testing.T) {
	w := newMemberWorld(t)
	const missing = "99999999-0000-4000-8000-000000000004"
	foreign := w.addMember(w.system, w.ws2, "mr-far-"+randomHex(t)+"@example.test")
	auditBefore := w.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action LIKE 'member.staff_%'`, w.ws2)
	email := "mr-out-" + randomHex(t) + "@example.test"

	for _, c := range []struct{ method, foreign, missing, body string }{
		{"POST", "/workspaces/" + w.ws2 + "/members", "/workspaces/" + missing + "/members", memberBody(email)},
		{"GET", "/workspaces/" + w.ws2 + "/member-requests", "/workspaces/" + missing + "/member-requests", ""},
		{"GET", "/member-requests/" + foreign, "/member-requests/" + missing, ""},
		{"POST", "/member-requests/" + foreign + "/cancel", "/member-requests/" + missing + "/cancel", ""},
	} {
		a := w.do(w.admin1, c.method, "/api/platform"+c.foreign, c.body)
		b := w.do(w.admin1, c.method, "/api/platform"+c.missing, c.body)
		if a.Code != 404 || b.Code != 404 || a.Body.String() != b.Body.String() {
			t.Errorf("%s %s: foreign = %d %q, missing = %d %q; want the same 404 body", c.method, c.foreign, a.Code, a.Body.String(), b.Code, b.Body.String())
		}
	}
	if n := w.count(`SELECT count(*) FROM platform_member_request WHERE email = $1`, email); n != 0 {
		t.Errorf("a refused request wrote %d rows", n)
	}
	var state string
	if err := w.pool.QueryRow(context.Background(), `SELECT state FROM platform_member_request WHERE id = $1`, foreign).Scan(&state); err != nil || state != "pending" {
		t.Errorf("the foreign request is %q (%v) after a refused cancel, want pending", state, err)
	}
	if n := w.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action LIKE 'member.staff_%'`, w.ws2); n != auditBefore {
		t.Errorf("audit rows of ws2 went from %d to %d", auditBefore, n)
	}
}

func TestMemberRequestSystemAdminAnyWorkspace(t *testing.T) {
	w := newMemberWorld(t)
	for _, ws := range []string{w.ws1, w.ws2} {
		id := w.addMember(w.system, ws, "mr-sys-"+randomHex(t)+"@example.test")
		wantStatus(t, w.do(w.system, "GET", "/api/platform/member-requests/"+id, ""), 200)
		wantStatus(t, w.do(w.system, "GET", "/api/platform/workspaces/"+ws+"/member-requests", ""), 200)
	}
	// An unknown workspace is a 404, with no row written.
	const missing = "99999999-0000-4000-8000-000000000005"
	wantCode(t, w.do(w.system, "POST", "/api/platform/workspaces/"+missing+"/members", memberBody("mr-none@example.test")), 404, "not_found")
	wantCode(t, w.do(w.system, "GET", "/api/platform/workspaces/"+missing+"/member-requests", ""), 404, "not_found")
}

func TestMemberRequestAdminListIsFilteredToOwnTenant(t *testing.T) {
	w := newMemberWorld(t)
	own := w.addMember(w.system, w.ws1, "mr-list1-"+randomHex(t)+"@example.test")
	foreign := w.addMember(w.system, w.ws2, "mr-list2-"+randomHex(t)+"@example.test")

	rec := w.do(w.admin1, "GET", "/api/platform/workspaces/"+w.ws1+"/member-requests", "")
	wantStatus(t, rec, 200)
	list, _ := decode(t, rec)["requests"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["id"] != own {
		t.Errorf("admin of t1 lists %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), foreign) {
		t.Errorf("the list holds a request of the other tenant")
	}
	wantStatus(t, w.do(w.admin1, "GET", "/api/platform/member-requests/"+own, ""), 200)
	wantStatus(t, w.do(w.admin1, "GET", "/api/platform/member-requests/"+foreign, ""), 404)
}

func TestMemberRequestRoutesMatchMatrixRows(t *testing.T) {
	w := newMemberWorld(t)
	type route struct{ method, path, body string }
	routes := func(req string) []route {
		return []route{
			{"POST", "/api/platform/workspaces/" + w.ws1 + "/members", memberBody("mr-matrix-" + randomHex(t) + "@example.test")}, // M21
			{"GET", "/api/platform/workspaces/" + w.ws1 + "/member-requests", ""},                                                 // M22
			{"GET", "/api/platform/member-requests/" + req, ""},                                                                   // M22
			{"POST", "/api/platform/member-requests/" + req + "/cancel", ""},                                                      // M22
		}
	}
	for name, who := range map[string]struct {
		user    string
		allowed bool
	}{
		"system_admin": {w.system, true},
		"admin (own)":  {w.admin1, true},
		"support":      {w.support, false},
		"billing":      {w.billing, false},
		"no session":   {"", false},
	} {
		req := w.addMember(w.system, w.ws1, "mr-seed-"+randomHex(t)+"@example.test")
		for _, r := range routes(req) {
			rec := w.do(who.user, r.method, r.path, r.body)
			if who.allowed {
				if rec.Code != 200 && rec.Code != 202 {
					t.Errorf("%s %s %s = %d, want success; %s", name, r.method, r.path, rec.Code, rec.Body.String())
				}
				continue
			}
			if rec.Code != 404 || rec.Body.String() != "404 page not found\n" {
				t.Errorf("%s %s %s = %d %q, want the flat 404", name, r.method, r.path, rec.Code, rec.Body.String())
			}
		}
	}
	// Roles and actions of the rows, by the matrix itself.
	for action, want := range map[string]string{
		staff.ActionMembersAdd:     "system_admin,admin",
		staff.ActionMemberRequests: "system_admin,admin",
	} {
		if got := strings.Join(roleNames(staff.RolesScoped(action)), ","); got != want {
			t.Errorf("roles of %s = %s, want %s", action, got, want)
		}
	}
}
