package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/auditaction"
	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Tests of the account group routes (platform spec 7b.2, rules 1 to 6 and 12
// to 14). They run against the dedicated *_test database that TestMain checked,
// and need no other variable.

// memberGroupRouter mounts the routes with the middleware router.go uses.
func memberGroupRouter() http.Handler {
	q := testHandler.Queries
	r := chi.NewRouter()
	r.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(RequireHumanActor)
			r.Use(middleware.RequireWorkspaceMemberFromURL(q, "id"))
			r.Get("/permissions/me", testHandler.GetMyPermissions)
		})
		r.Group(func(r chi.Router) {
			r.Use(RequireHumanActor)
			r.Use(middleware.RequireWorkspaceRoleFromURL(q, "id", "owner", "admin"))
			r.Get("/permission-catalogue", testHandler.GetPermissionCatalogue)
			r.Get("/groups", testHandler.ListMemberGroups)
			r.Post("/groups", testHandler.CreateMemberGroup)
			r.Get("/groups/{groupId}", testHandler.GetMemberGroup)
			r.Patch("/groups/{groupId}", testHandler.UpdateMemberGroup)
			r.Delete("/groups/{groupId}", testHandler.DeleteMemberGroup)
			r.Put("/groups/{groupId}/permissions", testHandler.SetMemberGroupPermissions)
			r.Post("/groups/{groupId}/members", testHandler.AddMemberGroupMembers)
			r.Delete("/groups/{groupId}/members/{userId}", testHandler.RemoveMemberGroupMember)
		})
	})
	return r
}

type mgEnv struct {
	t      *testing.T
	ws     string
	router http.Handler
	// owner is the suite's user; the others are members of the workspace with
	// that role.
	owner, admin, member, viewer string
}

func newMGEnv(t *testing.T) *mgEnv {
	t.Helper()
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ws := planLimitWorkspace(t)
	e := &mgEnv{t: t, ws: ws, router: memberGroupRouter(), owner: testUserID}
	e.admin = e.newMember("admin")
	e.member = e.newMember("member")
	e.viewer = e.newMember("viewer")
	// Registered after the workspace, so they run before it is deleted.
	for _, q := range []string{
		`DELETE FROM member_group_member WHERE workspace_id = $1`,
		`DELETE FROM member_group WHERE workspace_id = $1`,
		`DELETE FROM audit_event WHERE workspace_id = $1`,
	} {
		dbfx.Cleanup(t, q, ws)
	}
	return e
}

func (e *mgEnv) newMember(role string) string {
	e.t.Helper()
	user := dbfx.User(e.t, "Group "+role, testutil.UniqueEmail("mg-"+role))
	dbfx.Member(e.t, e.ws, user, role)
	return user
}

func (e *mgEnv) path(suffix string) string { return "/api/workspaces/" + e.ws + suffix }

func (e *mgEnv) req(method, suffix, user string, body any) *http.Request {
	req := newRequest(method, e.path(suffix), body)
	req.Header.Set("X-User-ID", user)
	req.Header.Set("X-Workspace-ID", e.ws)
	return req
}

func (e *mgEnv) do(method, suffix, user string, body any) *testutil.Response {
	e.t.Helper()
	return testutil.Call(e.t, e.router.ServeHTTP, e.req(method, suffix, user, body))
}

// doAudited is do with a request id, returned so the audit rows can be found.
func (e *mgEnv) doAudited(method, suffix, user string, body any) (*testutil.Response, string) {
	e.t.Helper()
	req, id := auditRequest(e.t, e.req(method, suffix, user, body), "audit-mg-")
	return testutil.Call(e.t, e.router.ServeHTTP, req), id
}

// doFailing is do with an audit write that raises.
func (e *mgEnv) doFailing(method, suffix, user string, body any) *testutil.Response {
	e.t.Helper()
	return testutil.Call(e.t, e.router.ServeHTTP, r2FailingRequest(e.t, e.req(method, suffix, user, body)))
}

type mgGroup struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Permissions []string   `json:"permissions"`
	MemberCount int        `json:"member_count"`
	CreatedBy   *string    `json:"created_by"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
	Members     []mgMember `json:"members"`
}

type mgMember struct {
	UserID  string  `json:"user_id"`
	AddedBy *string `json:"added_by"`
	AddedAt string  `json:"added_at"`
}

type mgError struct {
	Error      string `json:"error"`
	Code       string `json:"code"`
	Permission string `json:"permission"`
	Limit      *int64 `json:"limit"`
	Used       *int64 `json:"used"`
	Gate       string `json:"gate"`
	Feature    string `json:"feature"`
}

func wantMGError(t *testing.T, resp *testutil.Response, status int, code string) mgError {
	t.Helper()
	resp.Want(status)
	var body mgError
	resp.JSON(&body)
	if body.Code != code || body.Error == "" {
		t.Fatalf("body = %s, want code %q with a message", resp.Text(), code)
	}
	return body
}

func (e *mgEnv) create(name string) mgGroup {
	e.t.Helper()
	var g mgGroup
	e.do(http.MethodPost, "/groups", e.owner, map[string]any{"name": name}).Want(http.StatusCreated).JSON(&g)
	return g
}

func (e *mgEnv) get(id string) mgGroup {
	e.t.Helper()
	var g mgGroup
	e.do(http.MethodGet, "/groups/"+id, e.owner, nil).Want(http.StatusOK).JSON(&g)
	return g
}

func (e *mgEnv) setPermissions(id string, keys []string) {
	e.t.Helper()
	e.do(http.MethodPut, "/groups/"+id+"/permissions", e.owner, map[string]any{"permissions": keys}).Want(http.StatusOK)
}

func (e *mgEnv) addMembers(id string, users ...string) *testutil.Response {
	e.t.Helper()
	return e.do(http.MethodPost, "/groups/"+id+"/members", e.owner, map[string]any{"user_ids": users})
}

type permissionsMe struct {
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
}

func (e *mgEnv) me(user string) permissionsMe {
	e.t.Helper()
	var out permissionsMe
	e.do(http.MethodGet, "/permissions/me", user, nil).Want(http.StatusOK).JSON(&out)
	return out
}

func (e *mgEnv) groupCount() int {
	e.t.Helper()
	return dbfx.Count(e.t, `SELECT count(*) FROM member_group WHERE workspace_id = $1`, e.ws)
}

// ---- routes and roles ---------------------------------------------------------

type mgRoute struct{ method, suffix string }

func (e *mgEnv) adminRoutes(groupID string) []mgRoute {
	other := uuid.NewString()
	return []mgRoute{
		{http.MethodGet, "/permission-catalogue"},
		{http.MethodGet, "/groups"},
		{http.MethodPost, "/groups"},
		{http.MethodGet, "/groups/" + groupID},
		{http.MethodPatch, "/groups/" + groupID},
		{http.MethodDelete, "/groups/" + groupID},
		{http.MethodPut, "/groups/" + groupID + "/permissions"},
		{http.MethodPost, "/groups/" + groupID + "/members"},
		{http.MethodDelete, "/groups/" + groupID + "/members/" + other},
	}
}

// Rule 6: managing groups is for owner and admin. A member, a viewer and a
// stranger are refused on every group route, and nothing is written.
func TestMemberGroupRoutesAreForOwnerAndAdmin(t *testing.T) {
	e := newMGEnv(t)
	g := e.create("Guarded")
	e.setPermissions(g.ID, []string{"audit.read", "billing.manage", "integrations.manage", "members.invite", "runtime_profiles.manage"})
	// Even a member of every key-holding group is refused.
	e.addMembers(g.ID, e.member, e.viewer).Want(http.StatusOK)
	stranger := dbfx.User(t, "Stranger", testutil.UniqueEmail("mg-stranger"))

	for _, route := range e.adminRoutes(g.ID) {
		for name, user := range map[string]string{"member": e.member, "viewer": e.viewer} {
			body := map[string]any{"name": "x", "permissions": []string{}, "user_ids": []string{e.admin}}
			if got := e.do(route.method, route.suffix, user, body); got.Code != http.StatusForbidden {
				t.Errorf("%s on %s %s = %d (%s), want 403", name, route.method, route.suffix, got.Code, strings.TrimSpace(got.Text()))
			}
		}
		if got := e.do(route.method, route.suffix, stranger, map[string]any{"name": "x"}).Code; got != http.StatusNotFound {
			t.Errorf("a non-member on %s %s = %d, want 404", route.method, route.suffix, got)
		}
	}
	if e.groupCount() != 1 {
		t.Fatalf("%d groups, want the 1 created before the refused calls", e.groupCount())
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM member_group_member WHERE workspace_id = $1`, e.ws); n != 2 {
		t.Fatalf("%d member rows, want 2", n)
	}

	// An admin is not refused on any of them.
	var list []mgGroup
	e.do(http.MethodGet, "/groups", e.admin, nil).Want(http.StatusOK).JSON(&list)
	if len(list) != 1 {
		t.Fatalf("an admin lists %d groups, want 1", len(list))
	}
}

// A machine credential never reaches a group route or permissions/me.
func TestMemberGroupRoutesRefuseMachineCredentials(t *testing.T) {
	e := newMGEnv(t)
	g := e.create("Humans only")
	for _, route := range append(e.adminRoutes(g.ID), mgRoute{http.MethodGet, "/permissions/me"}) {
		for _, source := range []string{"task_token", "cloud_pat"} {
			req := e.req(route.method, route.suffix, e.owner, map[string]any{"name": "x"})
			req.Header.Set("X-Actor-Source", source)
			if got := testutil.Call(t, e.router.ServeHTTP, req).Code; got != http.StatusForbidden {
				t.Errorf("%s on %s %s = %d, want 403", source, route.method, route.suffix, got)
			}
		}
	}
	if e.groupCount() != 1 {
		t.Fatalf("%d groups after machine calls, want 1", e.groupCount())
	}
}

func TestMemberGroupCatalogueRoute(t *testing.T) {
	e := newMGEnv(t)
	var body struct {
		Permissions []string `json:"permissions"`
	}
	e.do(http.MethodGet, "/permission-catalogue", e.admin, nil).Want(http.StatusOK).JSON(&body)
	want := []string{"members.invite", "audit.read", "billing.view", "billing.manage", "integrations.manage", "runtime_profiles.manage"}
	if !reflect.DeepEqual(body.Permissions, want) {
		t.Fatalf("catalogue = %v, want %v", body.Permissions, want)
	}
}

// ---- create, read, update, delete -------------------------------------------------

func TestMemberGroupCreateReadUpdateDelete(t *testing.T) {
	e := newMGEnv(t)
	var created mgGroup
	e.do(http.MethodPost, "/groups", e.admin, map[string]any{"name": "  Accounting ", "description": "the accountants"}).
		Want(http.StatusCreated).JSON(&created)
	if created.ID == "" || created.WorkspaceID != e.ws || created.Name != "Accounting" || created.Description != "the accountants" ||
		created.Permissions == nil || len(created.Permissions) != 0 || created.MemberCount != 0 ||
		created.CreatedBy == nil || *created.CreatedBy != e.admin || created.CreatedAt == "" || created.UpdatedAt == "" {
		t.Fatalf("created = %+v", created)
	}

	e.addMembers(created.ID, e.member).Want(http.StatusOK)
	var one mgGroup
	e.do(http.MethodGet, "/groups/"+created.ID, e.owner, nil).Want(http.StatusOK).JSON(&one)
	if one.MemberCount != 1 || len(one.Members) != 1 || one.Members[0].UserID != e.member ||
		one.Members[0].AddedBy == nil || *one.Members[0].AddedBy != e.owner || one.Members[0].AddedAt == "" {
		t.Fatalf("group = %+v", one)
	}
	var list []mgGroup
	e.do(http.MethodGet, "/groups", e.owner, nil).Want(http.StatusOK).JSON(&list)
	if len(list) != 1 || list[0].ID != created.ID || list[0].MemberCount != 1 {
		t.Fatalf("list = %+v", list)
	}

	var patched mgGroup
	e.do(http.MethodPatch, "/groups/"+created.ID, e.owner, map[string]any{"name": "Finance"}).Want(http.StatusOK).JSON(&patched)
	if patched.Name != "Finance" || patched.Description != "the accountants" {
		t.Fatalf("patched = %+v; an omitted description must stay", patched)
	}
	e.do(http.MethodPatch, "/groups/"+created.ID, e.owner, map[string]any{"description": ""}).Want(http.StatusOK).JSON(&patched)
	if patched.Name != "Finance" || patched.Description != "" {
		t.Fatalf("patched = %+v; an empty description clears it", patched)
	}

	e.do(http.MethodDelete, "/groups/"+created.ID, e.owner, nil).Want(http.StatusNoContent)
	wantMGError(t, e.do(http.MethodGet, "/groups/"+created.ID, e.owner, nil), http.StatusNotFound, "group_not_found")
}

// Rule 1: name and description rules.
func TestMemberGroupNameAndDescriptionRules(t *testing.T) {
	e := newMGEnv(t)
	for name, body := range map[string]map[string]any{
		"no name":          {},
		"blank name":       {"name": "   "},
		"65 characters":    {"name": strings.Repeat("n", 65)},
		"long description": {"name": "ok", "description": strings.Repeat("d", 281)},
	} {
		if got := e.do(http.MethodPost, "/groups", e.owner, body); got.Code != http.StatusBadRequest {
			t.Errorf("%s = %d (%s), want 400", name, got.Code, strings.TrimSpace(got.Text()))
		}
	}
	e.do(http.MethodPost, "/groups", e.owner, map[string]any{"name": strings.Repeat("n", 64), "description": strings.Repeat("d", 280)}).Want(http.StatusCreated)

	e.create("Finance")
	for _, name := range []string{"Finance", "FINANCE", " finance "} {
		wantMGError(t, e.do(http.MethodPost, "/groups", e.owner, map[string]any{"name": name}), http.StatusConflict, "group_name_taken")
	}
	other := e.create("Support")
	wantMGError(t, e.do(http.MethodPatch, "/groups/"+other.ID, e.owner, map[string]any{"name": "fINANCE"}), http.StatusConflict, "group_name_taken")
	if got := e.get(other.ID).Name; got != "Support" {
		t.Fatalf("a refused rename changed the name to %q", got)
	}
	if got := e.do(http.MethodPatch, "/groups/"+other.ID, e.owner, map[string]any{"name": ""}).Code; got != http.StatusBadRequest {
		t.Fatalf("an empty name on PATCH = %d, want 400", got)
	}
	if e.groupCount() != 3 {
		t.Fatalf("%d groups, want 3", e.groupCount())
	}
}

func TestMemberGroupNamesAreUniquePerWorkspace(t *testing.T) {
	a, b := newMGEnv(t), newMGEnv(t)
	a.create("Shared name")
	b.create("Shared name")
}

// ---- members ---------------------------------------------------------------------

// Rule 2.
func TestMemberGroupMembersMustBeWorkspaceMembers(t *testing.T) {
	e := newMGEnv(t)
	g := e.create("Team")
	stranger := dbfx.User(t, "Stranger", testutil.UniqueEmail("mg-stranger"))

	wantMGError(t, e.addMembers(g.ID, e.member, stranger), http.StatusConflict, "not_a_member")
	if got := e.get(g.ID); got.MemberCount != 0 {
		t.Fatalf("a refused batch left %d members", got.MemberCount)
	}

	e.addMembers(g.ID, e.member, e.viewer, e.member).Want(http.StatusOK)
	if got := e.get(g.ID); got.MemberCount != 2 {
		t.Fatalf("members = %d, want 2 (a repeated id counts once)", got.MemberCount)
	}
	// A repeat is not an error.
	e.addMembers(g.ID, e.member).Want(http.StatusOK)
	if got := e.get(g.ID); got.MemberCount != 2 {
		t.Fatalf("members = %d after a repeat, want 2", got.MemberCount)
	}

	e.do(http.MethodDelete, "/groups/"+g.ID+"/members/"+e.member, e.owner, nil).Want(http.StatusNoContent)
	if got := e.get(g.ID); got.MemberCount != 1 || got.Members[0].UserID != e.viewer {
		t.Fatalf("after removal: %+v", got)
	}
	// Removing someone who is not in the group changes nothing.
	e.do(http.MethodDelete, "/groups/"+g.ID+"/members/"+e.member, e.owner, nil).Want(http.StatusNoContent)
}

func TestMemberGroupAddValidatesTheRequest(t *testing.T) {
	e := newMGEnv(t)
	g := e.create("Team")

	var ids []string
	for i := 0; i < 101; i++ {
		ids = append(ids, uuid.NewString())
	}
	wantMGError(t, e.addMembers(g.ID, ids...), http.StatusBadRequest, "too_many_user_ids")
	if got := e.addMembers(g.ID); got.Code != http.StatusBadRequest {
		t.Errorf("an empty list = %d, want 400", got.Code)
	}
	if got := e.addMembers(g.ID, "not-a-uuid"); got.Code != http.StatusBadRequest {
		t.Errorf("a malformed id = %d, want 400", got.Code)
	}
	if got := e.do(http.MethodPost, "/groups/"+g.ID+"/members", e.owner, map[string]any{"user_ids": nil}); got.Code != http.StatusBadRequest {
		t.Errorf("a null list = %d, want 400", got.Code)
	}
	if got := e.do(http.MethodDelete, "/groups/"+g.ID+"/members/not-a-uuid", e.owner, nil); got.Code != http.StatusBadRequest {
		t.Errorf("a malformed user id on DELETE = %d, want 400", got.Code)
	}
	if got := e.do(http.MethodGet, "/groups/not-a-uuid", e.owner, nil); got.Code != http.StatusBadRequest {
		t.Errorf("a malformed group id = %d, want 400", got.Code)
	}
}

// 100 ids is the most one request takes.
func TestMemberGroupAddTakesOneHundredMembers(t *testing.T) {
	e := newMGEnv(t)
	g := e.create("Hundred")
	var users []string
	for i := 0; i < 100; i++ {
		users = append(users, e.newMember("member"))
	}
	e.addMembers(g.ID, users...).Want(http.StatusOK)
	if got := e.get(g.ID); got.MemberCount != 100 {
		t.Fatalf("members = %d, want 100", got.MemberCount)
	}
}

// Rule 2: a group holds at most 1000 members.
func TestMemberGroupMemberLimit(t *testing.T) {
	e := newMGEnv(t)
	g := e.create("Full")
	dbfx.Exec(t, `INSERT INTO member_group_member (workspace_id, group_id, user_id)
		SELECT $1, $2, gen_random_uuid() FROM generate_series(1, 1000)`, e.ws, g.ID)
	body := wantMGError(t, e.addMembers(g.ID, e.member), http.StatusConflict, "group_member_limit")
	if body.Limit == nil || *body.Limit != 1000 {
		t.Fatalf("limit = %v, want 1000 in %s", body.Limit, "the body")
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM member_group_member WHERE group_id = $1`, g.ID); n != 1000 {
		t.Fatalf("%d member rows, want 1000", n)
	}
}

// ---- permissions -----------------------------------------------------------------

func TestMemberGroupSetPermissions(t *testing.T) {
	e := newMGEnv(t)
	g := e.create("Billing")

	var got mgGroup
	e.do(http.MethodPut, "/groups/"+g.ID+"/permissions", e.owner,
		map[string]any{"permissions": []string{"billing.manage", "audit.read", "audit.read"}}).Want(http.StatusOK).JSON(&got)
	if !reflect.DeepEqual(got.Permissions, []string{"audit.read", "billing.manage"}) {
		t.Fatalf("permissions = %v", got.Permissions)
	}
	if !reflect.DeepEqual(e.get(g.ID).Permissions, []string{"audit.read", "billing.manage"}) {
		t.Fatal("the stored permissions differ from the response")
	}
	// An empty list clears the permissions.
	e.do(http.MethodPut, "/groups/"+g.ID+"/permissions", e.owner, map[string]any{"permissions": []string{}}).Want(http.StatusOK).JSON(&got)
	if got.Permissions == nil || len(got.Permissions) != 0 {
		t.Fatalf("permissions = %#v, want []", got.Permissions)
	}
	if r := e.do(http.MethodPut, "/groups/"+g.ID+"/permissions", e.owner, map[string]any{}); r.Code != http.StatusBadRequest {
		t.Fatalf("a body without permissions = %d, want 400", r.Code)
	}
}

// Rule 3: an unknown key is a 400 that names it, and nothing is stored.
func TestMemberGroupUnknownPermissionIsRefused(t *testing.T) {
	e := newMGEnv(t)
	g := e.create("Strict")
	e.setPermissions(g.ID, []string{"audit.read"})
	failAuditWrites(t)

	for _, bad := range []string{"groups.manage", "Audit.Read", "", "billing.view "} {
		resp, reqID := e.doAudited(http.MethodPut, "/groups/"+g.ID+"/permissions", e.owner,
			map[string]any{"permissions": []string{"billing.view", bad}})
		body := wantMGError(t, resp, http.StatusBadRequest, "permission_unknown")
		if body.Permission != bad {
			t.Errorf("permission = %q, want %q", body.Permission, bad)
		}
		noAuditRow(t, reqID, audit.ActionGroupPermissionsChanged)
	}
	if got := e.get(g.ID).Permissions; !reflect.DeepEqual(got, []string{"audit.read"}) {
		t.Fatalf("permissions = %v after refused changes, want [audit.read]", got)
	}
}

// ---- permissions/me ---------------------------------------------------------------

func TestPermissionsMeOfTheRoles(t *testing.T) {
	e := newMGEnv(t)
	all := []string{"audit.read", "billing.manage", "billing.view", "integrations.manage", "members.invite", "runtime_profiles.manage"}
	for role, user := range map[string]string{"owner": e.owner, "admin": e.admin} {
		if got := e.me(user); got.Role != role || !reflect.DeepEqual(got.Permissions, all) {
			t.Errorf("%s = %+v, want every key", role, got)
		}
	}
	for role, user := range map[string]string{"member": e.member, "viewer": e.viewer} {
		got := e.me(user)
		if got.Role != role || got.Permissions == nil || len(got.Permissions) != 0 {
			t.Errorf("%s = %+v, want no keys and an empty array", role, got)
		}
	}
	stranger := dbfx.User(t, "Stranger", testutil.UniqueEmail("mg-stranger"))
	if got := e.do(http.MethodGet, "/permissions/me", stranger, nil).Code; got != http.StatusNotFound {
		t.Errorf("a non-member = %d, want 404", got)
	}
}

// Rules 4, 5 and 11: the union of the groups, billing.manage implies
// billing.view, and a removal shows on the next request.
func TestPermissionsMeFollowsTheGroups(t *testing.T) {
	e := newMGEnv(t)
	billing, audits := e.create("Billing"), e.create("Auditors")
	e.setPermissions(billing.ID, []string{"billing.manage"})
	e.setPermissions(audits.ID, []string{"audit.read", "members.invite"})

	if got := e.me(e.member); len(got.Permissions) != 0 {
		t.Fatalf("before joining: %v", got.Permissions)
	}
	e.addMembers(billing.ID, e.member, e.viewer).Want(http.StatusOK)
	if got := e.me(e.member); !reflect.DeepEqual(got.Permissions, []string{"billing.manage", "billing.view"}) {
		t.Fatalf("billing group: %v", got.Permissions)
	}
	e.addMembers(audits.ID, e.member).Want(http.StatusOK)
	if got := e.me(e.member); got.Role != "member" ||
		!reflect.DeepEqual(got.Permissions, []string{"audit.read", "billing.manage", "billing.view", "members.invite"}) {
		t.Fatalf("both groups: %+v", got)
	}
	// A viewer reads its keys like a member; the write ban is the guard's.
	if got := e.me(e.viewer); got.Role != "viewer" || !reflect.DeepEqual(got.Permissions, []string{"billing.manage", "billing.view"}) {
		t.Fatalf("viewer: %+v", got)
	}

	e.do(http.MethodDelete, "/groups/"+billing.ID+"/members/"+e.member, e.owner, nil).Want(http.StatusNoContent)
	if got := e.me(e.member); !reflect.DeepEqual(got.Permissions, []string{"audit.read", "members.invite"}) {
		t.Fatalf("after leaving the billing group: %v", got.Permissions)
	}
	e.setPermissions(audits.ID, []string{})
	if got := e.me(e.member); len(got.Permissions) != 0 {
		t.Fatalf("after the audit group lost its keys: %v", got.Permissions)
	}
	e.do(http.MethodDelete, "/groups/"+audits.ID, e.owner, nil).Want(http.StatusNoContent)
	if got := e.me(e.member); len(got.Permissions) != 0 {
		t.Fatalf("after deleting the group: %v", got.Permissions)
	}
}

func TestPermissionsMeIgnoresStoredUnknownKeys(t *testing.T) {
	e := newMGEnv(t)
	g := e.create("Future")
	dbfx.Exec(t, `UPDATE member_group SET permissions = '["members.invite","groups.manage","from.the.future"]'::jsonb WHERE id = $1`, g.ID)
	e.addMembers(g.ID, e.member).Want(http.StatusOK)
	if got := e.me(e.member); !reflect.DeepEqual(got.Permissions, []string{"members.invite"}) {
		t.Fatalf("permissions = %v, want only members.invite", got.Permissions)
	}
	// Rewriting the group's permissions drops the stored unknown keys, and the
	// audit row records them as part of the old value.
	e.setPermissions(g.ID, []string{"members.invite"})
	if got := e.get(g.ID).Permissions; !reflect.DeepEqual(got, []string{"members.invite"}) {
		t.Fatalf("stored = %v", got)
	}
}

// ---- the plan gate -----------------------------------------------------------------

type featureBody struct {
	Code    string `json:"code"`
	Feature string `json:"feature"`
	Gate    string `json:"gate"`
	Error   string `json:"error"`
}

// Rule 12: a limit of 0 means the plan has no groups.
func TestMemberGroupGateLimitZeroIsFeatureUnavailable(t *testing.T) {
	e := newMGEnv(t)
	setPlanLimitGates(t, e.ws, map[entitlement.GateName]int{entitlement.GateMemberGroups: 0})

	resp := e.do(http.MethodPost, "/groups", e.owner, map[string]any{"name": "Nope"}).Want(http.StatusPaymentRequired)
	var body featureBody
	resp.JSON(&body)
	if body.Code != "plan_feature_unavailable" || body.Feature != "member_groups" || body.Gate != "member_groups" || body.Error == "" {
		t.Fatalf("body = %s", resp.Text())
	}
	if e.groupCount() != 0 {
		t.Fatalf("%d groups after a refused creation", e.groupCount())
	}
}

func TestMemberGroupGateOverTheLimitIsPlanLimitReached(t *testing.T) {
	e := newMGEnv(t)
	setPlanLimitGates(t, e.ws, map[entitlement.GateName]int{entitlement.GateMemberGroups: 2})
	e.create("One")
	e.create("Two")
	wantPlanLimit(t, e.do(http.MethodPost, "/groups", e.owner, map[string]any{"name": "Three"}), "member_groups", 2, 2)
	if e.groupCount() != 2 {
		t.Fatalf("%d groups, want 2: a refused creation leaves no row", e.groupCount())
	}
	// At the limit the gate answers before the name is looked at.
	wantPlanLimit(t, e.do(http.MethodPost, "/groups", e.owner, map[string]any{"name": "One"}), "member_groups", 2, 2)
}

// Two creations at the last slot: one succeeds, the other is refused.
func TestMemberGroupGateTwoConcurrentCreationsAtTheLimit(t *testing.T) {
	e := newMGEnv(t)
	setPlanLimitGates(t, e.ws, map[entitlement.GateName]int{entitlement.GateMemberGroups: 1})

	const racers = 4
	codes := make([]int, racers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			rec := httptest.NewRecorder()
			e.router.ServeHTTP(rec, e.req(http.MethodPost, "/groups", e.owner, map[string]any{"name": "Racer " + string(rune('A'+i))}))
			codes[i] = rec.Code
		}(i)
	}
	close(start)
	wg.Wait()

	created, refused := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusPaymentRequired:
			refused++
		}
	}
	if created != 1 || refused != racers-1 {
		t.Fatalf("status codes = %v, want one 201 and %d 402", codes, racers-1)
	}
	if e.groupCount() != 1 {
		t.Fatalf("%d groups, want 1", e.groupCount())
	}
}

// Lowering the plan keeps the groups and their permissions; only a new group is refused.
func TestMemberGroupGateLoweringThePlanKeepsExistingGroups(t *testing.T) {
	e := newMGEnv(t)
	g := e.create("Before the downgrade")
	e.setPermissions(g.ID, []string{"billing.manage"})
	e.addMembers(g.ID, e.member).Want(http.StatusOK)
	e.create("Second")

	setPlanLimitGates(t, e.ws, map[entitlement.GateName]int{entitlement.GateMemberGroups: 0})
	wantMGError(t, e.do(http.MethodPost, "/groups", e.owner, map[string]any{"name": "New"}), http.StatusPaymentRequired, "plan_feature_unavailable")

	if got := e.me(e.member); !reflect.DeepEqual(got.Permissions, []string{"billing.manage", "billing.view"}) {
		t.Fatalf("permissions after the downgrade = %v", got.Permissions)
	}
	var list []mgGroup
	e.do(http.MethodGet, "/groups", e.owner, nil).Want(http.StatusOK).JSON(&list)
	if len(list) != 2 {
		t.Fatalf("%d groups listed, want 2", len(list))
	}
	// Every other change stays open.
	e.do(http.MethodPatch, "/groups/"+g.ID, e.owner, map[string]any{"description": "kept"}).Want(http.StatusOK)
	e.setPermissions(g.ID, []string{"audit.read"})
	e.addMembers(g.ID, e.viewer).Want(http.StatusOK)
	e.do(http.MethodDelete, "/groups/"+g.ID, e.owner, nil).Want(http.StatusNoContent)
}

// With no entitlement provider (MULTICA_CLOUD_URL empty) the gate is off.
func TestMemberGroupGateOffWithoutAnEntitlementProvider(t *testing.T) {
	e := newMGEnv(t)
	prior := testHandler.Entitlements
	testHandler.Entitlements = nil
	t.Cleanup(func() { testHandler.Entitlements = prior })
	for i := 0; i < 5; i++ {
		e.create("Group " + string(rune('A'+i)))
	}
	if e.groupCount() != 5 {
		t.Fatalf("%d groups, want 5", e.groupCount())
	}
}

// An absent gate (a policy without member_groups) is off.
func TestMemberGroupGateOffWhenThePolicyOmitsIt(t *testing.T) {
	e := newMGEnv(t)
	setPlanLimitGates(t, e.ws, map[entitlement.GateName]int{entitlement.GateAgents: 1})
	e.create("One")
	e.create("Two")
}

// ---- audit -------------------------------------------------------------------------

func TestMemberGroupAuditRowsOnePerChange(t *testing.T) {
	e := newMGEnv(t)
	owner := e.owner
	ownerEmail := auditTestUserEmail(t, owner)

	// group.created
	resp, reqID := e.doAudited(http.MethodPost, "/groups", owner, map[string]any{"name": "Audited", "description": "d"})
	var g mgGroup
	resp.Want(http.StatusCreated).JSON(&g)
	wantOneAudit(t, reqID, auditWant{
		action: audit.ActionGroupCreated, workspaceID: e.ws, actorID: owner, actorLabel: ownerEmail,
		targetType: "group", targetID: g.ID,
	})

	// group.updated records only the names of the fields that changed.
	resp, reqID = e.doAudited(http.MethodPatch, "/groups/"+g.ID, owner, map[string]any{"name": "Audited", "description": "changed"})
	resp.Want(http.StatusOK)
	wantOneAudit(t, reqID, auditWant{
		action: audit.ActionGroupUpdated, workspaceID: e.ws, actorID: owner,
		targetType: "group", targetID: g.ID,
		detail: map[string]any{"fields": []any{"description"}},
	})
	resp, reqID = e.doAudited(http.MethodPatch, "/groups/"+g.ID, owner, map[string]any{"name": "Renamed", "description": "changed"})
	resp.Want(http.StatusOK)
	wantOneAudit(t, reqID, auditWant{
		action: audit.ActionGroupUpdated, workspaceID: e.ws, actorID: owner,
		targetType: "group", targetID: g.ID,
		detail: map[string]any{"fields": []any{"name"}},
	})
	// An update that changes nothing writes nothing.
	resp, reqID = e.doAudited(http.MethodPatch, "/groups/"+g.ID, owner, map[string]any{"name": "Renamed"})
	resp.Want(http.StatusOK)
	noAuditRow(t, reqID, audit.ActionGroupUpdated)

	// group.permissions_changed records the old and the new array.
	resp, reqID = e.doAudited(http.MethodPut, "/groups/"+g.ID+"/permissions", owner, map[string]any{"permissions": []string{"billing.manage", "audit.read"}})
	resp.Want(http.StatusOK)
	wantOneAudit(t, reqID, auditWant{
		action: audit.ActionGroupPermissionsChanged, workspaceID: e.ws, actorID: owner,
		targetType: "group", targetID: g.ID,
		detail: map[string]any{"old": []any{}, "new": []any{"audit.read", "billing.manage"}},
	})
	resp, reqID = e.doAudited(http.MethodPut, "/groups/"+g.ID+"/permissions", owner, map[string]any{"permissions": []string{"audit.read"}})
	resp.Want(http.StatusOK)
	wantOneAudit(t, reqID, auditWant{
		action: audit.ActionGroupPermissionsChanged, workspaceID: e.ws, actorID: owner,
		targetType: "group", targetID: g.ID,
		detail: map[string]any{"old": []any{"audit.read", "billing.manage"}, "new": []any{"audit.read"}},
	})
	// The same set again is not a change.
	resp, reqID = e.doAudited(http.MethodPut, "/groups/"+g.ID+"/permissions", owner, map[string]any{"permissions": []string{"audit.read"}})
	resp.Want(http.StatusOK)
	noAuditRow(t, reqID, audit.ActionGroupPermissionsChanged)

	// group.member_added: one row for each person actually added.
	resp, reqID = e.doAudited(http.MethodPost, "/groups/"+g.ID+"/members", owner, map[string]any{"user_ids": []string{e.member, e.viewer}})
	resp.Want(http.StatusOK)
	rows := auditEvents(t, reqID, audit.ActionGroupMemberAdded)
	if len(rows) != 2 {
		t.Fatalf("%d member_added rows, want 2", len(rows))
	}
	added := map[string]bool{}
	for _, row := range rows {
		if row.TargetType != "group" || row.TargetID != g.ID || row.WorkspaceID == nil || *row.WorkspaceID != e.ws {
			t.Errorf("member_added target = %s/%s workspace %v", row.TargetType, row.TargetID, row.WorkspaceID)
		}
		added[row.Detail["user_id"].(string)] = true
	}
	if !added[e.member] || !added[e.viewer] || len(added) != 2 {
		t.Fatalf("rows name %v", added)
	}
	resp, reqID = e.doAudited(http.MethodPost, "/groups/"+g.ID+"/members", owner, map[string]any{"user_ids": []string{e.member}})
	resp.Want(http.StatusOK)
	noAuditRow(t, reqID, audit.ActionGroupMemberAdded)

	// group.member_removed
	resp, reqID = e.doAudited(http.MethodDelete, "/groups/"+g.ID+"/members/"+e.member, owner, nil)
	resp.Want(http.StatusNoContent)
	wantOneAudit(t, reqID, auditWant{
		action: audit.ActionGroupMemberRemoved, workspaceID: e.ws, actorID: owner,
		targetType: "group", targetID: g.ID,
		detail: map[string]any{"user_id": e.member},
	})
	resp, reqID = e.doAudited(http.MethodDelete, "/groups/"+g.ID+"/members/"+e.member, owner, nil)
	resp.Want(http.StatusNoContent)
	noAuditRow(t, reqID, audit.ActionGroupMemberRemoved)

	// group.deleted
	resp, reqID = e.doAudited(http.MethodDelete, "/groups/"+g.ID, owner, nil)
	resp.Want(http.StatusNoContent)
	wantOneAudit(t, reqID, auditWant{
		action: audit.ActionGroupDeleted, workspaceID: e.ws, actorID: owner,
		targetType: "group", targetID: g.ID,
		detail: map[string]any{"member_count": float64(1)},
	})
}

// Spec 7b.2: a group name never reaches the audit log. The whole row is
// searched, so a name in any column or in the detail payload fails the test.
func TestMemberGroupAuditRowsCarryNoGroupName(t *testing.T) {
	e := newMGEnv(t)
	owner := e.owner
	const first, second = "Zq-Finance-Secret", "Zq-Treasury-Secret"

	resp, createID := e.doAudited(http.MethodPost, "/groups", owner, map[string]any{"name": first})
	var g mgGroup
	resp.Want(http.StatusCreated).JSON(&g)
	resp, renameID := e.doAudited(http.MethodPatch, "/groups/"+g.ID, owner, map[string]any{"name": second})
	resp.Want(http.StatusOK)
	resp, deleteID := e.doAudited(http.MethodDelete, "/groups/"+g.ID, owner, nil)
	resp.Want(http.StatusNoContent)

	for action, reqID := range map[string]string{
		audit.ActionGroupCreated: createID, audit.ActionGroupUpdated: renameID, audit.ActionGroupDeleted: deleteID,
	} {
		var row string
		if err := testPool.QueryRow(context.Background(),
			`SELECT row_to_json(a)::text FROM audit_event a WHERE request_id = $1 AND action = $2`,
			reqID, action).Scan(&row); err != nil {
			t.Fatalf("%s: read audit row: %v", action, err)
		}
		for _, name := range []string{first, second} {
			if strings.Contains(row, name) {
				t.Errorf("%s: audit row contains group name %q: %s", action, name, row)
			}
		}
	}
}

// A refused request writes no audit row.
func TestMemberGroupAuditNothingForRefusedRequests(t *testing.T) {
	e := newMGEnv(t)
	e.create("Taken")
	_, reqID := e.doAudited(http.MethodPost, "/groups", e.owner, map[string]any{"name": "taken"})
	noAuditRow(t, reqID, audit.ActionGroupCreated)
	resp, reqID := e.doAudited(http.MethodPost, "/groups", e.member, map[string]any{"name": "Member made"})
	resp.Want(http.StatusForbidden)
	noAuditRow(t, reqID, audit.ActionGroupCreated)
}

// Rule 13: when the audit write fails the change does not persist.
func TestMemberGroupAuditFailureRollsTheChangeBack(t *testing.T) {
	e := newMGEnv(t)
	g := e.create("Stable")
	e.setPermissions(g.ID, []string{"audit.read"})
	e.addMembers(g.ID, e.member).Want(http.StatusOK)
	failAuditWrites(t)

	t.Run("create", func(t *testing.T) {
		before := e.groupCount()
		e.doFailing(http.MethodPost, "/groups", e.owner, map[string]any{"name": "Ghost"}).Want(http.StatusInternalServerError)
		if e.groupCount() != before {
			t.Fatalf("%d groups, want %d", e.groupCount(), before)
		}
	})
	t.Run("update", func(t *testing.T) {
		e.doFailing(http.MethodPatch, "/groups/"+g.ID, e.owner, map[string]any{"name": "Changed"}).Want(http.StatusInternalServerError)
		if got := e.get(g.ID).Name; got != "Stable" {
			t.Fatalf("name = %q", got)
		}
	})
	t.Run("permissions", func(t *testing.T) {
		e.doFailing(http.MethodPut, "/groups/"+g.ID+"/permissions", e.owner, map[string]any{"permissions": []string{"billing.manage"}}).Want(http.StatusInternalServerError)
		if got := e.get(g.ID).Permissions; !reflect.DeepEqual(got, []string{"audit.read"}) {
			t.Fatalf("permissions = %v", got)
		}
	})
	t.Run("add members", func(t *testing.T) {
		e.doFailing(http.MethodPost, "/groups/"+g.ID+"/members", e.owner, map[string]any{"user_ids": []string{e.viewer}}).Want(http.StatusInternalServerError)
		if got := e.get(g.ID).MemberCount; got != 1 {
			t.Fatalf("members = %d, want 1", got)
		}
	})
	t.Run("remove member", func(t *testing.T) {
		e.doFailing(http.MethodDelete, "/groups/"+g.ID+"/members/"+e.member, e.owner, nil).Want(http.StatusInternalServerError)
		if got := e.get(g.ID).MemberCount; got != 1 {
			t.Fatalf("members = %d, want 1", got)
		}
	})
	t.Run("delete", func(t *testing.T) {
		e.doFailing(http.MethodDelete, "/groups/"+g.ID, e.owner, nil).Want(http.StatusInternalServerError)
		if got := e.get(g.ID); got.MemberCount != 1 || got.Name != "Stable" {
			t.Fatalf("group = %+v after a failed delete", got)
		}
	})
}

// Rule 10, second sentence: the delete removes the member rows with the group.
func TestMemberGroupDeleteRemovesTheMemberRows(t *testing.T) {
	e := newMGEnv(t)
	g, keep := e.create("Doomed"), e.create("Kept")
	e.addMembers(g.ID, e.member, e.viewer).Want(http.StatusOK)
	e.addMembers(keep.ID, e.member).Want(http.StatusOK)
	e.do(http.MethodDelete, "/groups/"+g.ID, e.owner, nil).Want(http.StatusNoContent)
	if n := dbfx.Count(t, `SELECT count(*) FROM member_group_member WHERE group_id = $1`, g.ID); n != 0 {
		t.Fatalf("%d member rows of a deleted group", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM member_group_member WHERE group_id = $1`, keep.ID); n != 1 {
		t.Fatalf("%d rows of the other group, want 1", n)
	}
	wantMGError(t, e.do(http.MethodDelete, "/groups/"+g.ID, e.owner, nil), http.StatusNotFound, "group_not_found")
}

// ---- other workspaces ----------------------------------------------------------------

// The id of a group of another workspace is a 404 group_not_found on every
// route, and the group is not touched.
func TestMemberGroupOfAnotherWorkspaceIsNotFound(t *testing.T) {
	mine, theirs := newMGEnv(t), newMGEnv(t)
	foreign := theirs.create("Theirs")
	theirs.setPermissions(foreign.ID, []string{"audit.read"})
	theirs.addMembers(foreign.ID, theirs.member).Want(http.StatusOK)

	for _, route := range []struct {
		method, suffix string
		body           any
	}{
		{http.MethodGet, "/groups/" + foreign.ID, nil},
		{http.MethodPatch, "/groups/" + foreign.ID, map[string]any{"name": "Hijacked"}},
		{http.MethodDelete, "/groups/" + foreign.ID, nil},
		{http.MethodPut, "/groups/" + foreign.ID + "/permissions", map[string]any{"permissions": []string{"billing.manage"}}},
		{http.MethodPost, "/groups/" + foreign.ID + "/members", map[string]any{"user_ids": []string{mine.member}}},
		{http.MethodDelete, "/groups/" + foreign.ID + "/members/" + theirs.member, nil},
	} {
		wantMGError(t, mine.do(route.method, route.suffix, mine.owner, route.body), http.StatusNotFound, "group_not_found")
	}
	if got := theirs.get(foreign.ID); got.Name != "Theirs" || got.MemberCount != 1 || !reflect.DeepEqual(got.Permissions, []string{"audit.read"}) {
		t.Fatalf("the foreign group changed: %+v", got)
	}
	// The owner of the other workspace is not an admin of this one.
	elsewhere := dbfx.User(t, "Elsewhere", testutil.UniqueEmail("mg-elsewhere"))
	if got := mine.do(http.MethodGet, "/groups", elsewhere, nil).Code; got != http.StatusNotFound {
		t.Fatalf("a non-member listing groups = %d, want 404", got)
	}
	var list []mgGroup
	mine.do(http.MethodGet, "/groups", mine.owner, nil).Want(http.StatusOK).JSON(&list)
	if len(list) != 0 {
		t.Fatalf("the list shows %d groups of another workspace", len(list))
	}
}

// ---- the audit catalogue ----------------------------------------------------------------

func TestMemberGroupAuditActionsAreFilterable(t *testing.T) {
	for _, a := range []string{
		audit.ActionGroupCreated, audit.ActionGroupUpdated, audit.ActionGroupDeleted,
		audit.ActionGroupMemberAdded, audit.ActionGroupMemberRemoved, audit.ActionGroupPermissionsChanged,
	} {
		if !auditaction.Is(a) {
			t.Errorf("%q is not an action the audit log filter accepts", a)
		}
	}
}
