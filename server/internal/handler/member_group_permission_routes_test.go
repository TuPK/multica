package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/multica-ai/multica/platform-management/workspacestate"
	"github.com/multica-ai/multica/server/internal/membergroup"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/workspaceguard"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Permission-key routes (platform spec 7b.2, catalogue and rules 7 to 11). The
// matrix below is the catalogue as the spec lists it. Each route is served by a
// sentinel handler in keyGateRouter, so the tests judge only the gate, and the
// handlers that matter (invitations, member removal) are mounted for real in
// their own tests.

// keyedRoute is one route of the catalogue. A pattern that starts with /api/ is
// absolute; any other is relative to /api/workspaces/{id}.
type keyedRoute struct {
	key     string
	method  string
	pattern string
	handler string
}

func (rt keyedRoute) absolute() bool { return strings.HasPrefix(rt.pattern, "/api/") }

func (rt keyedRoute) isWrite() bool { return rt.method != http.MethodGet }

// keyedRoutes is spec 7b.2, the key catalogue table, one row per route.
var keyedRoutes = []keyedRoute{
	// members.invite
	{membergroup.KeyMembersInvite, http.MethodPost, "/members", "CreateInvitation"},
	{membergroup.KeyMembersInvite, http.MethodGet, "/invitations", "ListWorkspaceInvitations"},
	{membergroup.KeyMembersInvite, http.MethodDelete, "/invitations/{invitationId}", "RevokeInvitation"},
	{membergroup.KeyMembersInvite, http.MethodGet, "/share-links", "ListShareLinks"},
	{membergroup.KeyMembersInvite, http.MethodPost, "/share-links", "CreateShareLink"},
	{membergroup.KeyMembersInvite, http.MethodDelete, "/share-links/{linkId}", "RevokeShareLink"},
	// audit.read
	{membergroup.KeyAuditRead, http.MethodGet, "/audit-events", "ListWorkspaceAuditEvents"},
	// billing.view
	{membergroup.KeyBillingView, http.MethodGet, "/orders", "ListWorkspaceOrders"},
	{membergroup.KeyBillingView, http.MethodGet, "/orders/{orderId}", "GetWorkspaceOrder"},
	{membergroup.KeyBillingView, http.MethodGet, "/orders/{orderId}/usage", "GetWorkspaceOrderUsage"},
	{membergroup.KeyBillingView, http.MethodGet, "/api/cloud-subscriptions/usage", "GetCloudWorkspaceSubscriptionUsage"},
	{membergroup.KeyBillingView, http.MethodGet, "/api/cloud-subscriptions/billing-profile", "GetCloudWorkspaceBillingProfile"},
	// billing.manage: every write of /api/cloud-subscriptions, and the buyer route
	{membergroup.KeyBillingManage, http.MethodPut, "/orders/{orderId}/buyer", "UpdateWorkspaceOrderBuyer"},
	{membergroup.KeyBillingManage, http.MethodPost, "/api/cloud-subscriptions/checkout-sessions", "CreateCloudWorkspaceSubscriptionCheckout"},
	{membergroup.KeyBillingManage, http.MethodPost, "/api/cloud-subscriptions/seats/purchase-preview", "PreviewCloudWorkspaceSubscriptionSeatPurchase"},
	{membergroup.KeyBillingManage, http.MethodPost, "/api/cloud-subscriptions/seats/purchases", "PurchaseCloudWorkspaceSubscriptionSeats"},
	{membergroup.KeyBillingManage, http.MethodPost, "/api/cloud-subscriptions/seats/reconcile", "ReconcileCloudWorkspaceSubscriptionSeats"},
	{membergroup.KeyBillingManage, http.MethodPost, "/api/cloud-subscriptions/portal-sessions", "CreateCloudWorkspaceSubscriptionPortal"},
	{membergroup.KeyBillingManage, http.MethodPut, "/api/cloud-subscriptions/billing-profile", "UpdateCloudWorkspaceBillingProfile"},
	{membergroup.KeyBillingManage, http.MethodPost, "/api/cloud-subscriptions/plan-changes/preview", "PreviewCloudWorkspaceSubscriptionPlanChange"},
	{membergroup.KeyBillingManage, http.MethodPost, "/api/cloud-subscriptions/plan-changes", "CreateCloudWorkspaceSubscriptionPlanChange"},
	{membergroup.KeyBillingManage, http.MethodDelete, "/api/cloud-subscriptions/plan-changes/scheduled", "CancelCloudWorkspaceSubscriptionScheduledPlanChange"},
	{membergroup.KeyBillingManage, http.MethodPost, "/api/cloud-subscriptions/cancel", "CancelCloudWorkspaceSubscription"},
	{membergroup.KeyBillingManage, http.MethodPost, "/api/cloud-subscriptions/resume", "ResumeCloudWorkspaceSubscription"},
	// integrations.manage
	{membergroup.KeyIntegrationsManage, http.MethodGet, "/github/connect", "GitHubConnect"},
	{membergroup.KeyIntegrationsManage, http.MethodGet, "/github/installations/{installationId}/repositories", "ListGitHubInstallationRepositories"},
	{membergroup.KeyIntegrationsManage, http.MethodDelete, "/github/installations/{installationId}", "DeleteGitHubInstallation"},
	{membergroup.KeyIntegrationsManage, http.MethodPost, "/vcs/connections", "ConnectVCS"},
	{membergroup.KeyIntegrationsManage, http.MethodPost, "/vcs/connections/{connectionId}/rotate-webhook", "RotateVCSConnectionWebhook"},
	{membergroup.KeyIntegrationsManage, http.MethodDelete, "/vcs/connections/{connectionId}", "DeleteVCSConnection"},
	{membergroup.KeyIntegrationsManage, http.MethodPut, "/qoder", "SaveQoderConnection"},
	{membergroup.KeyIntegrationsManage, http.MethodPost, "/qoder/check", "CheckQoderConnection"},
	{membergroup.KeyIntegrationsManage, http.MethodPost, "/qoder/environments", "ListQoderEnvironments"},
	{membergroup.KeyIntegrationsManage, http.MethodPost, "/qoder/stop", "StopQoderConnection"},
	{membergroup.KeyIntegrationsManage, http.MethodPost, "/qoder/resume", "ResumeQoderConnection"},
	{membergroup.KeyIntegrationsManage, http.MethodDelete, "/qoder", "DisconnectQoderConnection"},
	{membergroup.KeyIntegrationsManage, http.MethodDelete, "/slack/installations/{installationId}", "RevokeSlackInstallation"},
	{membergroup.KeyIntegrationsManage, http.MethodPost, "/slack/install/byo", "RegisterSlackBYO"},
	{membergroup.KeyIntegrationsManage, http.MethodDelete, "/wecom/installations/{installationId}", "RevokeWecomInstallation"},
	{membergroup.KeyIntegrationsManage, http.MethodPost, "/wecom/install/byo", "RegisterWecomBYO"},
	{membergroup.KeyIntegrationsManage, http.MethodPost, "/wecom/install/begin", "BeginWecomInstall"},
	{membergroup.KeyIntegrationsManage, http.MethodGet, "/wecom/install/{sessionId}", "GetWecomInstallStatus"},
	{membergroup.KeyIntegrationsManage, http.MethodDelete, "/telegram/installations/{installationId}", "RevokeTelegramInstallation"},
	{membergroup.KeyIntegrationsManage, http.MethodPost, "/telegram/install", "RegisterTelegramBot"},
	// runtime_profiles.manage
	{membergroup.KeyRuntimeProfilesManage, http.MethodPost, "/runtime-profiles", "CreateRuntimeProfile"},
	{membergroup.KeyRuntimeProfilesManage, http.MethodPatch, "/runtime-profiles/{profileId}", "UpdateRuntimeProfile"},
	{membergroup.KeyRuntimeProfilesManage, http.MethodPut, "/runtime-profiles/{profileId}", "UpdateRuntimeProfile"},
	{membergroup.KeyRuntimeProfilesManage, http.MethodDelete, "/runtime-profiles/{profileId}", "DeleteRuntimeProfile"},
}

// adminOnlyRoutes stay with owner and admin: no key of the catalogue opens them
// (spec 7b.2, the list of routes outside the catalogue).
var adminOnlyRoutes = []keyedRoute{
	{"", http.MethodPut, "/", "UpdateWorkspace"},
	{"", http.MethodPatch, "/members/{memberId}", "UpdateMember"},
	{"", http.MethodDelete, "/members/{memberId}", "DeleteMember"},
	{"", http.MethodGet, "/policy", "GetWorkspacePolicy"},
	{"", http.MethodPost, "/mcp-servers", "CreateWorkspaceMcpServer"},
	{"", http.MethodDelete, "/mcp-servers/{serverId}", "DeleteWorkspaceMcpServer"},
	{"", http.MethodPost, "/plugins/preview", "PreviewPlugin"},
	{"", http.MethodPost, "/plugins", "InstallPlugin"},
	{"", http.MethodDelete, "/plugins/{installationId}", "UninstallPlugin"},
	{"", http.MethodGet, "/plugins/packages", "ListPluginPackages"},
}

// fillPattern replaces the chi parameters of a pattern with fixed values.
func fillPattern(pattern string) string {
	return regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(pattern, "11111111-2222-4333-8444-555555555555")
}

// keyGateRouter mounts the catalogue and the admin-only routes behind the gate
// the router uses: one permission middleware per key, the role middleware for
// the rest. outer runs before every route (the policy and the state guards).
func keyGateRouter(q *db.Queries, outer ...func(http.Handler) http.Handler) http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	r := chi.NewRouter()
	for _, mw := range outer {
		r.Use(mw)
	}
	r.Route("/api/workspaces/{id}", func(r chi.Router) {
		for _, key := range membergroup.Catalogue() {
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireWorkspacePermissionFromURL(q, "id", key))
				for _, rt := range keyedRoutes {
					if rt.key == key && !rt.absolute() {
						r.Method(rt.method, rt.pattern, ok)
					}
				}
			})
		}
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWorkspaceRoleFromURL(q, "id", "owner", "admin"))
			for _, rt := range adminOnlyRoutes {
				r.Method(rt.method, rt.pattern, ok)
			}
		})
	})
	r.Route("/api/cloud-subscriptions", func(r chi.Router) {
		for _, key := range membergroup.Catalogue() {
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireWorkspacePermission(q, key))
				for _, rt := range keyedRoutes {
					if rt.key == key && rt.absolute() {
						r.Method(rt.method, strings.TrimPrefix(rt.pattern, "/api/cloud-subscriptions"), ok)
					}
				}
			})
		}
	})
	return r
}

// routeURL is the request path of a route in the workspace e.ws.
func (e *mgEnv) routeURL(rt keyedRoute) string {
	if rt.absolute() {
		return fillPattern(rt.pattern)
	}
	return e.path(fillPattern(rt.pattern))
}

// routeRequest builds a request for rt as user, in the workspace header form the
// router reads for the cloud-subscription routes and the URL form for the rest.
func (e *mgEnv) routeRequest(rt keyedRoute, user string) *http.Request {
	req := newRequest(rt.method, e.routeURL(rt), map[string]any{})
	req.Header.Set("X-User-ID", user)
	req.Header.Set("X-Workspace-ID", e.ws)
	return req
}

func (e *mgEnv) routeDo(rt keyedRoute, user string) *testutil.Response {
	e.t.Helper()
	return testutil.Call(e.t, e.router.ServeHTTP, e.routeRequest(rt, user))
}

// groupWith creates a group with the given keys and adds users to it, straight
// in the two tables (the group API is covered by the handler tests of Q5).
func (e *mgEnv) groupWith(keys []string, users ...string) string {
	e.t.Helper()
	raw, err := json.Marshal(keys)
	if err != nil {
		e.t.Fatal(err)
	}
	ctx := context.Background()
	var id string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO member_group (workspace_id, name, description, permissions, created_by)
		VALUES ($1, $2, '', $3::jsonb, $4)
		RETURNING id::text`, e.ws, "g-"+uuid.NewString(), string(raw), e.owner).Scan(&id); err != nil {
		e.t.Fatalf("create group: %v", err)
	}
	for _, u := range users {
		if _, err := testPool.Exec(ctx, `
			INSERT INTO member_group_member (workspace_id, group_id, user_id, added_by)
			VALUES ($1, $2, $3, $4)`, e.ws, id, u, e.owner); err != nil {
			e.t.Fatalf("add member: %v", err)
		}
	}
	return id
}

// memberRowID is the member row id of user in the workspace.
func (e *mgEnv) memberRowID(user string) string {
	e.t.Helper()
	var id string
	if err := testPool.QueryRow(context.Background(),
		`SELECT id::text FROM member WHERE workspace_id = $1 AND user_id = $2`, e.ws, user).Scan(&id); err != nil {
		e.t.Fatalf("member row: %v", err)
	}
	return id
}

func (e *mgEnv) groupRows(user string) int {
	e.t.Helper()
	return dbfx.Count(e.t, `SELECT count(*) FROM member_group_member WHERE workspace_id = $1 AND user_id = $2`, e.ws, user)
}

// (a) The key-by-route matrix: for each key, exactly the routes the table opens
// to that key open to a member holding it; every other catalogue route and every
// admin-only route refuses. Owner and admin reach every route.
func TestMemberGroupRoutesKeyByRouteMatrix(t *testing.T) {
	e := newMGEnv(t)
	e.router = keyGateRouter(testHandler.Queries)

	for _, key := range membergroup.Catalogue() {
		user := e.newMember("member")
		e.groupWith([]string{key}, user)
		for _, rt := range keyedRoutes {
			want := http.StatusForbidden
			// billing.manage implies billing.view (spec 7b.2, rule 4).
			if rt.key == key || (key == membergroup.KeyBillingManage && rt.key == membergroup.KeyBillingView) {
				want = http.StatusNoContent
			}
			if got := e.routeDo(rt, user); got.Code != want {
				t.Errorf("key %s: %s %s = %d (%s), want %d", key, rt.method, rt.pattern, got.Code, strings.TrimSpace(got.Text()), want)
			}
		}
		for _, rt := range adminOnlyRoutes {
			if got := e.routeDo(rt, user); got.Code != http.StatusForbidden {
				t.Errorf("key %s opens admin-only %s %s = %d, want 403", key, rt.method, rt.pattern, got.Code)
			}
		}
	}

	for _, rt := range append(append([]keyedRoute{}, keyedRoutes...), adminOnlyRoutes...) {
		for _, user := range []string{e.owner, e.admin} {
			if got := e.routeDo(rt, user); got.Code != http.StatusNoContent {
				t.Errorf("%s %s = %d (%s) for a human owner or admin, want 204", rt.method, rt.pattern, got.Code, strings.TrimSpace(got.Text()))
			}
		}
	}
}

// The refusal of a missing key carries permission_denied and the key it lacks.
func TestMemberGroupRouteRefusalNamesPermissionDenied(t *testing.T) {
	e := newMGEnv(t)
	e.router = keyGateRouter(testHandler.Queries)
	rt := keyedRoute{membergroup.KeyAuditRead, http.MethodGet, "/audit-events", ""}
	resp := e.routeDo(rt, e.member)
	body := wantMGError(t, resp, http.StatusForbidden, "permission_denied")
	if body.Permission != membergroup.KeyAuditRead {
		t.Fatalf("permission = %q, want %q", body.Permission, membergroup.KeyAuditRead)
	}
}

// (b) A viewer in a group with all six keys is refused on every write and reads
// by the keys. The policy's viewer rule runs in the workspace middleware.
func TestViewerGroupStillRefusedOnWrites(t *testing.T) {
	e := newMGEnv(t)
	q := testHandler.Queries
	e.router = keyGateRouter(q, middleware.WithWorkspacePolicy(workspaceguard.NewPolicyStore(q, nil)))
	e.groupWith(membergroup.Catalogue(), e.viewer)

	for _, rt := range keyedRoutes {
		got := e.routeDo(rt, e.viewer)
		if rt.isWrite() {
			if got.Code != http.StatusForbidden {
				t.Errorf("viewer %s %s = %d (%s), want 403", rt.method, rt.pattern, got.Code, strings.TrimSpace(got.Text()))
				continue
			}
			wantMGError(t, got, http.StatusForbidden, workspaceguard.CodeViewerReadOnly)
			continue
		}
		if got.Code != http.StatusNoContent {
			t.Errorf("viewer read %s %s = %d (%s), want 204 by its keys", rt.method, rt.pattern, got.Code, strings.TrimSpace(got.Text()))
		}
	}
	for _, rt := range adminOnlyRoutes {
		if got := e.routeDo(rt, e.viewer); got.Code != http.StatusForbidden {
			t.Errorf("viewer on admin-only %s %s = %d, want 403", rt.method, rt.pattern, got.Code)
		}
	}
}

// (c) A group member is refused while the workspace is suspended or pending
// deletion. Owner and admin still reach the route the state allow-list keeps
// open to them (the subscription routes).
func TestGroupMemberRefusedWhileSuspendedOrPendingDeletion(t *testing.T) {
	e := newMGEnv(t)
	q := testHandler.Queries
	e.groupWith(membergroup.Catalogue(), e.member)
	allowed := keyedRoute{membergroup.KeyBillingView, http.MethodGet, "/api/cloud-subscriptions/usage", ""}

	for _, status := range []workspacestate.Status{workspacestate.Suspended, workspacestate.PendingDeletion} {
		store := wsStateStoreFor(status)
		e.router = keyGateRouter(q, middleware.WithWorkspaceState(workspacestate.New(store, wsStateNoCache{})))

		for _, rt := range keyedRoutes {
			if rt.method == allowed.method && rt.pattern == allowed.pattern {
				continue
			}
			wantMGError(t, e.routeDo(rt, e.member), http.StatusForbidden, workspaceguard.CodeFor(string(status)))
		}
		if got := e.routeDo(allowed, e.member); got.Code != http.StatusForbidden {
			t.Errorf("%s: group member on %s = %d, want 403", status, allowed.pattern, got.Code)
		}
		for _, user := range []string{e.owner, e.admin} {
			if got := e.routeDo(allowed, user); got.Code != http.StatusNoContent {
				t.Errorf("%s: owner or admin on %s = %d (%s), want 204", status, allowed.pattern, got.Code, strings.TrimSpace(got.Text()))
			}
		}
	}
}

// (d) Someone with members.invite who invites an admin gets 403
// invite_role_not_assignable; member and viewer are assignable. That person
// cannot change a role or remove anyone.
func TestInvitationMembersInviteCannotAssignAdmin(t *testing.T) {
	e := newMGEnv(t)
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
	e.router = r
	e.groupWith([]string{membergroup.KeyMembersInvite}, e.member)

	invite := func(role string) *testutil.Response {
		return e.do(http.MethodPost, "/members", e.member, map[string]any{
			"email": testutil.UniqueEmail("mg-invitee-" + role),
			"role":  role,
		})
	}
	wantMGError(t, invite("admin"), http.StatusForbidden, "invite_role_not_assignable")
	invite("member").Want(http.StatusCreated)
	invite("viewer").Want(http.StatusCreated)

	target := e.viewer
	before := e.memberRowID(target)
	e.do(http.MethodPatch, "/members/"+before, e.member, map[string]any{"role": "admin"}).Want(http.StatusForbidden)
	e.do(http.MethodDelete, "/members/"+before, e.member, nil).Want(http.StatusForbidden)
	if role := dbfx.Count(t, `SELECT count(*) FROM member WHERE id = $1 AND role = 'viewer'`, before); role != 1 {
		t.Fatalf("the viewer's role changed or the row is gone")
	}
}

// Share links follow the same rule as invitations: members.invite cannot create
// an admin link, and can create a member or viewer link.
func TestShareLinkMembersInviteCannotCreateAdminLink(t *testing.T) {
	e := newMGEnv(t)
	q := testHandler.Queries
	r := chi.NewRouter()
	r.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWorkspacePermissionFromURL(q, "id", membergroup.KeyMembersInvite))
			r.Post("/share-links", testHandler.CreateShareLink)
		})
	})
	e.router = r
	e.groupWith([]string{membergroup.KeyMembersInvite}, e.member)

	link := func(role string) *testutil.Response {
		return e.do(http.MethodPost, "/share-links", e.member, map[string]any{"role": role})
	}
	wantMGError(t, link("admin"), http.StatusForbidden, "invite_role_not_assignable")
	link("member").Want(http.StatusCreated)
	link("viewer").Want(http.StatusCreated)
}

// (e) Taking someone out of a group makes their next request fail with
// permission_denied.
func TestMemberGroupRemovalTakesEffectOnTheNextRequest(t *testing.T) {
	e := newMGEnv(t)
	e.router = keyGateRouter(testHandler.Queries)
	gid := e.groupWith([]string{membergroup.KeyAuditRead}, e.member)
	e.routeDo(keyedRoute{membergroup.KeyAuditRead, http.MethodGet, "/audit-events", ""}, e.member).Want(http.StatusNoContent)

	remove := e.req(http.MethodDelete, "/groups/"+gid+"/members/"+e.member, e.owner, nil)
	if got := testutil.Call(t, memberGroupRouter().ServeHTTP, remove); got.Code/100 != 2 {
		t.Fatalf("remove from group = %d (%s), want 2xx", got.Code, got.Text())
	}
	wantMGError(t, e.routeDo(keyedRoute{membergroup.KeyAuditRead, http.MethodGet, "/audit-events", ""}, e.member),
		http.StatusForbidden, "permission_denied")
}

// (f) Removing a member deletes that person's group rows in the same
// transaction; when that transaction rolls back, the group rows stay.
func TestRevokeAndRemoveMemberDeletesGroupRows(t *testing.T) {
	e := newMGEnv(t)
	r := chi.NewRouter()
	r.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWorkspaceRoleFromURL(testHandler.Queries, "id", "owner", "admin"))
			r.Delete("/members/{memberId}", testHandler.DeleteMember)
		})
	})
	e.router = r
	e.groupWith([]string{membergroup.KeyBillingView}, e.member)
	if n := e.groupRows(e.member); n != 1 {
		t.Fatalf("%d group rows before removal, want 1", n)
	}

	e.do(http.MethodDelete, "/members/"+e.memberRowID(e.member), e.owner, nil).Want(http.StatusNoContent)
	if n := e.groupRows(e.member); n != 0 {
		t.Fatalf("%d group rows after removal, want 0", n)
	}
}

func TestRevokeAndRemoveMemberRollsBackGroupRows(t *testing.T) {
	e := newMGEnv(t)
	r := chi.NewRouter()
	r.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWorkspaceRoleFromURL(testHandler.Queries, "id", "owner", "admin"))
			r.Delete("/members/{memberId}", testHandler.DeleteMember)
		})
	})
	e.router = r
	e.groupWith([]string{membergroup.KeyBillingView}, e.member)
	failAuditWrites(t)

	req := r2FailingRequest(t, e.req(http.MethodDelete, "/members/"+e.memberRowID(e.member), e.owner, nil))
	if got := testutil.Call(t, r.ServeHTTP, req); got.Code < 400 {
		t.Fatalf("removal with a failing audit write = %d, want an error", got.Code)
	}
	if n := e.groupRows(e.member); n != 1 {
		t.Fatalf("%d group rows after a rolled-back removal, want 1", n)
	}
}

// Wiring: router.go mounts each catalogue route under the group of its key. The
// group is the nearest r.Group before the route line, and its first middleware
// line must name the key.
func TestPermissionRoutesAreWiredByKeyInTheRouter(t *testing.T) {
	raw, err := os.ReadFile("../../cmd/server/router.go")
	if err != nil {
		t.Fatalf("read router: %v", err)
	}
	src := string(raw)
	lines := strings.Split(src, "\n")
	constOf := map[string]string{
		membergroup.KeyMembersInvite:         "KeyMembersInvite",
		membergroup.KeyAuditRead:             "KeyAuditRead",
		membergroup.KeyBillingView:           "KeyBillingView",
		membergroup.KeyBillingManage:         "KeyBillingManage",
		membergroup.KeyIntegrationsManage:    "KeyIntegrationsManage",
		membergroup.KeyRuntimeProfilesManage: "KeyRuntimeProfilesManage",
	}
	for _, rt := range keyedRoutes {
		pattern := rt.pattern
		if rt.absolute() {
			pattern = strings.TrimPrefix(pattern, "/api/cloud-subscriptions")
		}
		line := -1
		want := fmt.Sprintf(`"%s", h.%s)`, pattern, rt.handler)
		for i, l := range lines {
			if strings.Contains(l, want) {
				line = i
				break
			}
		}
		if line < 0 {
			t.Errorf("router.go has no %s %s (%s)", rt.method, rt.pattern, rt.handler)
			continue
		}
		group := -1
		for i := line; i >= 0; i-- {
			if strings.Contains(lines[i], "r.Group(func(r chi.Router) {") {
				group = i
				break
			}
		}
		if group < 0 || group+1 >= len(lines) || !strings.Contains(lines[group+1], "r.Use(") {
			t.Errorf("%s %s is not in a group that starts with a middleware", rt.method, rt.pattern)
			continue
		}
		if !strings.Contains(lines[group+1]+lines[group+2], constOf[rt.key]) {
			t.Errorf("%s %s is not under membergroup.%s (group starts %q)", rt.method, rt.pattern, constOf[rt.key], strings.TrimSpace(lines[group+1]))
		}
	}
}
