package handler

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/platform-management/workspacestate"
	"github.com/multica-ai/multica/server/internal/cloudruntime"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/membergroup"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/workspaceguard"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The billing.view and billing.manage keys on the real handlers (spec 7b.2,
// AC25 and AC27). member_group_permission_routes_test.go judges the gate with
// sentinel handlers; these tests mount the real handlers behind the same
// middleware, so a handler that still wants owner or admin shows up here.

// billingKeyRoute is one billing route and what a caller who passes the gate
// gets. upstream is the path the cloud proxy must see; it is empty for the
// routes the server answers from its own database.
type billingKeyRoute struct {
	key      string
	method   string
	path     string // "{order}" is replaced with the fixture order
	body     any
	header   bool // send an Idempotency-Key
	want     int
	upstream string
}

func (rt billingKeyRoute) isWrite() bool { return rt.method != http.MethodGet }

// billingKeyRoutes is spec 7b.2 for billing.view and billing.manage, as the
// router mounts them. The orders usage route answers 404 for the fixture
// order, which is not a usage invoice: that is the handler's own answer, so it
// proves the caller got past the gate.
func billingKeyRoutes(ws string) []billingKeyRoute {
	sub := "/api/v1/subscriptions/" + ws
	return []billingKeyRoute{
		{key: membergroup.KeyBillingView, method: http.MethodGet, path: "/api/cloud-subscriptions/usage", want: 200, upstream: sub + "/usage"},
		{key: membergroup.KeyBillingView, method: http.MethodGet, path: "/api/cloud-subscriptions/billing-profile", want: 200, upstream: sub + "/billing-profile"},
		{key: membergroup.KeyBillingView, method: http.MethodGet, path: "/api/workspaces/" + ws + "/orders", want: 200},
		{key: membergroup.KeyBillingView, method: http.MethodGet, path: "/api/workspaces/" + ws + "/orders/{order}", want: 200},
		{key: membergroup.KeyBillingView, method: http.MethodGet, path: "/api/workspaces/" + ws + "/orders/{order}/usage", want: 404},

		{key: membergroup.KeyBillingManage, method: http.MethodPut, path: "/api/workspaces/" + ws + "/orders/{order}/buyer", body: buyerPayload(), want: 200},
		{key: membergroup.KeyBillingManage, method: http.MethodPost, path: "/api/cloud-subscriptions/checkout-sessions",
			body: map[string]any{"interval": "month", "idempotency_key": "k-checkout"}, want: 200, upstream: "/api/v1/subscriptions/checkout-sessions"},
		{key: membergroup.KeyBillingManage, method: http.MethodPost, path: "/api/cloud-subscriptions/seats/purchase-preview",
			body: map[string]any{"additional_seats": 2}, want: 200, upstream: sub + "/seats/purchase-preview"},
		{key: membergroup.KeyBillingManage, method: http.MethodPost, path: "/api/cloud-subscriptions/seats/purchases",
			body: map[string]any{"additional_seats": 2, "expected_current_seats": 5, "expected_purchase_version": 9,
				"accepted_proration_amount": 425, "currency": "usd", "idempotency_key": "k-seats"},
			want: 200, upstream: sub + "/seats/purchases"},
		{key: membergroup.KeyBillingManage, method: http.MethodPost, path: "/api/cloud-subscriptions/seats/reconcile", want: 200, upstream: sub + "/seats/reconcile"},
		{key: membergroup.KeyBillingManage, method: http.MethodPost, path: "/api/cloud-subscriptions/portal-sessions", header: true, want: 200, upstream: sub + "/portal-sessions"},
		{key: membergroup.KeyBillingManage, method: http.MethodPut, path: "/api/cloud-subscriptions/billing-profile",
			body: map[string]any{"account_kind": "individual"}, header: true, want: 200, upstream: sub + "/billing-profile"},
		{key: membergroup.KeyBillingManage, method: http.MethodPost, path: "/api/cloud-subscriptions/plan-changes/preview",
			body: map[string]any{"plan_key": "pro", "interval": "month"}, want: 200, upstream: sub + "/plan-changes/preview"},
		{key: membergroup.KeyBillingManage, method: http.MethodPost, path: "/api/cloud-subscriptions/plan-changes",
			body: map[string]any{"plan_key": "pro", "interval": "month"}, header: true, want: 200, upstream: sub + "/plan-changes"},
		{key: membergroup.KeyBillingManage, method: http.MethodDelete, path: "/api/cloud-subscriptions/plan-changes/scheduled", header: true, want: 200, upstream: sub + "/plan-changes/scheduled"},
		{key: membergroup.KeyBillingManage, method: http.MethodPost, path: "/api/cloud-subscriptions/cancel", header: true, want: 200, upstream: sub + "/cancel"},
		{key: membergroup.KeyBillingManage, method: http.MethodPost, path: "/api/cloud-subscriptions/resume", header: true, want: 200, upstream: sub + "/resume"},
	}
}

// billingKeyRouter mounts the real billing handlers as router.go does: the
// permission middleware of each key, then the human-actor guard. The wiring
// itself is pinned by TestPermissionRoutesAreWiredByKeyInTheRouter.
func billingKeyRouter(q *db.Queries, outer ...func(http.Handler) http.Handler) http.Handler {
	h := testHandler
	r := chi.NewRouter()
	for _, mw := range outer {
		r.Use(mw)
	}
	r.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWorkspacePermissionFromURL(q, "id", membergroup.KeyBillingView))
			r.Use(RequireHumanActor)
			r.Get("/orders", h.ListWorkspaceOrders)
			r.Get("/orders/{orderId}", h.GetWorkspaceOrder)
			r.Get("/orders/{orderId}/usage", h.GetWorkspaceOrderUsage)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWorkspacePermissionFromURL(q, "id", membergroup.KeyBillingManage))
			r.Use(RequireHumanActor)
			r.Put("/orders/{orderId}/buyer", h.UpdateWorkspaceOrderBuyer)
		})
	})
	r.Route("/api/cloud-subscriptions", func(r chi.Router) {
		r.Use(RequireHumanActor)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWorkspaceMember(q))
			r.Get("/summary", h.GetCloudWorkspaceSubscriptionSummary)
			r.Get("/prices", h.GetCloudWorkspaceSubscriptionPrices)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWorkspacePermission(q, membergroup.KeyBillingView))
			r.Get("/usage", h.GetCloudWorkspaceSubscriptionUsage)
			r.Get("/billing-profile", h.GetCloudWorkspaceBillingProfile)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWorkspacePermission(q, membergroup.KeyBillingManage))
			r.Post("/checkout-sessions", h.CreateCloudWorkspaceSubscriptionCheckout)
			r.Post("/seats/purchase-preview", h.PreviewCloudWorkspaceSubscriptionSeatPurchase)
			r.Post("/seats/purchases", h.PurchaseCloudWorkspaceSubscriptionSeats)
			r.Post("/seats/reconcile", h.ReconcileCloudWorkspaceSubscriptionSeats)
			r.Post("/portal-sessions", h.CreateCloudWorkspaceSubscriptionPortal)
			r.Put("/billing-profile", h.UpdateCloudWorkspaceBillingProfile)
			r.Post("/plan-changes/preview", h.PreviewCloudWorkspaceSubscriptionPlanChange)
			r.Post("/plan-changes", h.CreateCloudWorkspaceSubscriptionPlanChange)
			r.Delete("/plan-changes/scheduled", h.CancelCloudWorkspaceSubscriptionScheduledPlanChange)
			r.Post("/cancel", h.CancelCloudWorkspaceSubscription)
			r.Post("/resume", h.ResumeCloudWorkspaceSubscription)
		})
	})
	return r
}

// billingKeyEnv is an account-group environment with the subscription flag on,
// a fake cloud proxy and a pending order of the workspace.
type billingKeyEnv struct {
	*mgEnv
	proxy *fakeCloudRuntimeProxy
	order string
}

func newBillingKeyEnv(t *testing.T, outer ...func(http.Handler) http.Handler) *billingKeyEnv {
	t.Helper()
	e := newMGEnv(t)
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	proxy := &fakeCloudRuntimeProxy{enabled: true, resp: &cloudruntime.Response{StatusCode: http.StatusOK, Body: []byte(`{"ok":true}`)}}
	useCloudRuntimeProxy(t, proxy)
	e.router = billingKeyRouter(testHandler.Queries, outer...)
	// The order rows go before the workspace does.
	order := billingOrder(t, e.ws, "pending", time.Now().UTC())
	return &billingKeyEnv{mgEnv: e, proxy: proxy, order: order}
}

func (e *billingKeyEnv) call(rt billingKeyRoute, user string, extra ...[2]string) *testutil.Response {
	e.t.Helper()
	req := newRequest(rt.method, strings.ReplaceAll(rt.path, "{order}", e.order), rt.body)
	req.Header.Set("X-User-ID", user)
	req.Header.Set("X-Workspace-ID", e.ws)
	if rt.header {
		req.Header.Set(idempotencyKeyHeader, "k-"+strings.ReplaceAll(rt.path, "/", "-"))
	}
	for _, h := range extra {
		req.Header.Set(h[0], h[1])
	}
	e.proxy.called = false
	return testutil.Call(e.t, e.router.ServeHTTP, req)
}

// wantReaches asserts the route ran: the status the handler gives, and for a
// cloud route the upstream path and the caller's user id.
func (e *billingKeyEnv) wantReaches(rt billingKeyRoute, user, who string, got *testutil.Response) {
	e.t.Helper()
	if got.Code != rt.want {
		e.t.Errorf("%s: %s %s = %d (%s), want %d", who, rt.method, rt.path, got.Code, strings.TrimSpace(got.Text()), rt.want)
		return
	}
	if rt.upstream == "" {
		return
	}
	if !e.proxy.called || e.proxy.req.Path != rt.upstream || e.proxy.req.UserID != user {
		e.t.Errorf("%s: %s %s upstream = called %v, %q for %q; want %q for %q",
			who, rt.method, rt.path, e.proxy.called, e.proxy.req.Path, e.proxy.req.UserID, rt.upstream, user)
	}
}

// wantRefused asserts permission_denied naming the key of the route, and that
// nothing reached the cloud.
func (e *billingKeyEnv) wantRefused(rt billingKeyRoute, who string, got *testutil.Response) {
	e.t.Helper()
	if got.Code != http.StatusForbidden {
		e.t.Errorf("%s: %s %s = %d (%s), want 403", who, rt.method, rt.path, got.Code, strings.TrimSpace(got.Text()))
		return
	}
	body := wantMGError(e.t, got, http.StatusForbidden, "permission_denied")
	if body.Permission != rt.key {
		e.t.Errorf("%s: %s %s names permission %q, want %q", who, rt.method, rt.path, body.Permission, rt.key)
	}
	if e.proxy.called {
		e.t.Errorf("%s: %s %s reached the cloud", who, rt.method, rt.path)
	}
}

// A member whose group holds billing.manage reaches every write and, because
// manage implies view, every read; the cloud sees the member's own user id.
func TestBillingManageGroupMemberReachesEveryBillingRoute(t *testing.T) {
	e := newBillingKeyEnv(t)
	e.groupWith([]string{membergroup.KeyBillingManage}, e.member)
	for _, rt := range billingKeyRoutes(e.ws) {
		e.wantReaches(rt, e.member, "manage member", e.call(rt, e.member))
	}
}

// A member whose group holds billing.view reads and cannot write.
func TestBillingViewGroupMemberReadsAndCannotWrite(t *testing.T) {
	e := newBillingKeyEnv(t)
	e.groupWith([]string{membergroup.KeyBillingView}, e.member)
	for _, rt := range billingKeyRoutes(e.ws) {
		got := e.call(rt, e.member)
		if rt.isWrite() {
			e.wantRefused(rt, "view member", got)
			continue
		}
		e.wantReaches(rt, e.member, "view member", got)
	}
}

// A member with no key is refused on every route the keys open, and the summary
// and prices stay readable by any member.
func TestBillingRoutesRefuseAMemberWithoutTheKey(t *testing.T) {
	e := newBillingKeyEnv(t)
	for _, rt := range billingKeyRoutes(e.ws) {
		e.wantRefused(rt, "member without a key", e.call(rt, e.member))
	}
	// Another key does not open billing.
	other := e.newMember("member")
	e.groupWith([]string{membergroup.KeyAuditRead, membergroup.KeyMembersInvite}, other)
	for _, rt := range billingKeyRoutes(e.ws) {
		e.wantRefused(rt, "member with other keys", e.call(rt, other))
	}
	for _, path := range []string{"/api/cloud-subscriptions/summary", "/api/cloud-subscriptions/prices"} {
		rt := billingKeyRoute{method: http.MethodGet, path: path, want: 200, upstream: "/api/v1/subscriptions/" + e.ws + strings.TrimPrefix(path, "/api/cloud-subscriptions")}
		e.wantReaches(rt, e.member, "member", e.call(rt, e.member))
	}
}

// Owner and admin keep every billing route, with no group.
func TestBillingRoutesStayOpenToOwnerAndAdmin(t *testing.T) {
	e := newBillingKeyEnv(t)
	for _, user := range []string{e.owner, e.admin} {
		for _, rt := range billingKeyRoutes(e.ws) {
			e.wantReaches(rt, user, "owner or admin", e.call(rt, user))
		}
	}
}

// A machine credential is refused on every billing route, even one that acts
// for a person who holds the key.
func TestBillingRoutesRefuseMachineCredentialsWithTheKey(t *testing.T) {
	e := newBillingKeyEnv(t)
	e.groupWith([]string{membergroup.KeyBillingManage}, e.member)
	for _, source := range []string{"task_token", "cloud_pat"} {
		for _, user := range []string{e.member, e.owner} {
			for _, rt := range billingKeyRoutes(e.ws) {
				got := e.call(rt, user, [2]string{"X-Actor-Source", source})
				if got.Code != http.StatusForbidden {
					t.Errorf("%s as %s: %s %s = %d, want 403", source, user, rt.method, rt.path, got.Code)
				}
				if e.proxy.called {
					t.Errorf("%s as %s: %s %s reached the cloud", source, user, rt.method, rt.path)
				}
			}
		}
	}
}

// A viewer in a group with both keys reads by the key and is refused on every
// write: by the viewer guard in the router, and by the handler when that guard
// is not in front of it.
func TestBillingGroupViewerIsRefusedOnWrites(t *testing.T) {
	policy := middleware.WithWorkspacePolicy(workspaceguard.NewPolicyStore(testHandler.Queries, nil))
	for name, outer := range map[string][]func(http.Handler) http.Handler{
		"with the viewer guard":  {policy},
		"handler backstop alone": nil,
	} {
		t.Run(name, func(t *testing.T) {
			e := newBillingKeyEnv(t, outer...)
			e.groupWith([]string{membergroup.KeyBillingManage, membergroup.KeyBillingView}, e.viewer)
			for _, rt := range billingKeyRoutes(e.ws) {
				got := e.call(rt, e.viewer)
				if !rt.isWrite() {
					e.wantReaches(rt, e.viewer, "viewer", got)
					continue
				}
				if got.Code != http.StatusForbidden || e.proxy.called {
					t.Errorf("viewer: %s %s = %d (%s), cloud called %v, want 403 and not called", rt.method, rt.path, got.Code, strings.TrimSpace(got.Text()), e.proxy.called)
				}
			}
		})
	}
}

// A group member is refused while the workspace is suspended or pending
// deletion; owner and admin still use the usage read the state allow-list keeps
// open to them.
func TestBillingGroupMemberRefusedWhileSuspendedOrPendingDeletion(t *testing.T) {
	for _, status := range []workspacestate.Status{workspacestate.Suspended, workspacestate.PendingDeletion} {
		t.Run(string(status), func(t *testing.T) {
			store := wsStateStoreFor(status)
			e := newBillingKeyEnv(t, middleware.WithWorkspaceState(workspacestate.New(store, wsStateNoCache{})))
			e.groupWith([]string{membergroup.KeyBillingManage, membergroup.KeyBillingView}, e.member)

			usage := billingKeyRoutes(e.ws)[0]
			for _, rt := range billingKeyRoutes(e.ws) {
				got := e.call(rt, e.member)
				if got.Code != http.StatusForbidden || e.proxy.called {
					t.Errorf("group member: %s %s = %d (%s), cloud called %v, want 403 and not called", rt.method, rt.path, got.Code, strings.TrimSpace(got.Text()), e.proxy.called)
				}
			}
			for _, user := range []string{e.owner, e.admin} {
				e.wantReaches(usage, user, "owner or admin", e.call(usage, user))
			}
		})
	}
}

// The billing profile read hands the caller the platform's body as it is, so
// verified_by stays where the platform leaves it out (spec 7b.1, API table).
func TestBillingProfileReadReturnsTheProxiedBodyWithoutVerifiedBy(t *testing.T) {
	e := newBillingKeyEnv(t)
	body := `{"account_kind":"business","legal_name":"Acme Co","tax_code":"0123456789","address":"1 Main St","invoice_email":"ap@acme.test","verified":true}`
	e.proxy.resp = &cloudruntime.Response{StatusCode: http.StatusOK, Body: []byte(body)}
	e.groupWith([]string{membergroup.KeyBillingView}, e.member)
	rt := billingKeyRoute{key: membergroup.KeyBillingView, method: http.MethodGet, path: "/api/cloud-subscriptions/billing-profile"}
	got := e.call(rt, e.member)
	if got.Code != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", got.Code, strings.TrimSpace(got.Text()))
	}
	if strings.TrimSpace(got.Text()) != body {
		t.Errorf("body = %s, want the proxied %s", got.Text(), body)
	}
	if strings.Contains(got.Text(), "verified_by") {
		t.Errorf("the body carries verified_by: %s", got.Text())
	}
	if e.proxy.req.Method != http.MethodGet || e.proxy.req.Path != "/api/v1/subscriptions/"+e.ws+"/billing-profile" || e.proxy.req.UserID != e.member {
		t.Errorf("upstream = %+v", e.proxy.req)
	}
}
