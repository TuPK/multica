package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const paygRates = `{"tasks_run":{"per":1000,"price":30000},"storage_bytes":{"per":1073741824,"price":3000},"members":{"per":1,"price":49000}}`

func usagePlan(t *testing.T, model, rates, terms string) string {
	t.Helper()
	key := "t" + strings.ReplaceAll(uuid.NewString()[:12], "-", "")
	cols := testutil.Cols{
		"key": key, "name": "Test " + key, "currency": "vnd", "is_public": true, "sort": 9000,
		"billing_model": model, "usage_rates": rates, "usage_terms": terms,
	}
	dbfx.InsertNoID(t, "plan", cols, "key = $1", key)
	return key
}

func workspaceWithProfile(t *testing.T, kind string, verified bool) string {
	t.Helper()
	ws := dbfx.Workspace(t, "Plans "+kind, "plans-"+uuid.NewString()[:8])
	cols := testutil.Cols{"workspace_id": ws, "account_kind": kind}
	if verified {
		cols["verified_at"] = testutil.Raw("now()")
	}
	dbfx.InsertNoID(t, "billing_profile", cols, "workspace_id = $1", ws)
	return ws
}

func listPlansFor(t *testing.T, workspaceID string) map[string]workspacePlanJSON {
	t.Helper()
	req := newRequest(http.MethodGet, "/api/workspaces/x/plans", nil)
	req = req.WithContext(middleware.SetMemberContext(req.Context(), workspaceID, db.Member{Role: "owner"}))
	w := httptest.NewRecorder()
	testHandler.ListWorkspacePlans(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var out workspacePlansResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	byKey := map[string]workspacePlanJSON{}
	for _, p := range out.Plans {
		byKey[p.Key] = p
	}
	return byKey
}

func TestWorkspacePlansReturnsUsageRatesAndDiscount(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	t.Setenv("PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION", "")
	payg := usagePlan(t, "usage", paygRates, `{"discount_pct":{"business":20}}`)
	seat := billingPlan(t, true, false, int64Ptr(149000), int64Ptr(1490000))

	verified := listPlansFor(t, workspaceWithProfile(t, "business", true))
	p := verified[payg]
	if p.BillingModel != "usage" || p.EffectiveDiscountPct != 20 {
		t.Errorf("verified business: model %q discount %d, want usage and 20", p.BillingModel, p.EffectiveDiscountPct)
	}
	if got := p.UsageRates["tasks_run"]; len(p.UsageRates) != 3 || got.Per != 1000 || got.Price != 30000 {
		t.Errorf("usage_rates = %+v", p.UsageRates)
	}
	if got := p.UsageRates["storage_bytes"]; got.Per != 1073741824 || got.Price != 3000 {
		t.Errorf("storage_bytes = %+v", got)
	}
	s := verified[seat]
	if s.BillingModel != "seat" || s.EffectiveDiscountPct != 0 || s.UsageRates == nil || len(s.UsageRates) != 0 {
		t.Errorf("seat plan = %+v, want seat, 0 and empty non-nil usage_rates", s)
	}
	if s.SeatPriceMonth == nil || *s.SeatPriceMonth != 149000 {
		t.Errorf("seat plan lost its seat price: %+v", s)
	}

	if got := listPlansFor(t, workspaceWithProfile(t, "individual", true))[payg].EffectiveDiscountPct; got != 0 {
		t.Errorf("individual: discount = %d, want 0", got)
	}
	// No billing profile at all reads as an unverified individual.
	noProfile := dbfx.Workspace(t, "Plans none", "plans-"+uuid.NewString()[:8])
	if got := listPlansFor(t, noProfile)[payg].EffectiveDiscountPct; got != 0 {
		t.Errorf("no profile: discount = %d, want 0", got)
	}
}

func TestWorkspacePlansDiscountZeroWhenUnverified(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	payg := usagePlan(t, "usage", paygRates, `{"discount_pct":{"business":20}}`)
	ws := workspaceWithProfile(t, "business", false)

	t.Setenv("PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION", "")
	if got := listPlansFor(t, ws)[payg].EffectiveDiscountPct; got != 0 {
		t.Errorf("unverified, default env: discount = %d, want 0", got)
	}
	t.Setenv("PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION", "true")
	if got := listPlansFor(t, ws)[payg].EffectiveDiscountPct; got != 0 {
		t.Errorf("unverified, env true: discount = %d, want 0", got)
	}
	t.Setenv("PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION", "false")
	if got := listPlansFor(t, ws)[payg].EffectiveDiscountPct; got != 20 {
		t.Errorf("unverified, env false: discount = %d, want 20", got)
	}
}

func usageOrder(t *testing.T, workspaceID, kind string) string {
	t.Helper()
	now := time.Now().UTC()
	return dbfx.Insert(t, "billing_order", testutil.Cols{
		"id": testutil.Raw("gen_random_uuid()"), "workspace_id": workspaceID, "kind": kind, "plan_key": "payg",
		"interval": "month", "seats": 0, "amount": 100000, "vat_amount": 10000, "currency": "vnd", "status": "pending",
		"provider": "manual", "transfer_code": "MUL" + strings.ToUpper(uuid.NewString()[:8]), "created_at": now,
	})
}

func usageCharge(t *testing.T, workspaceID, orderID, day, meter string, net int64, note string) {
	t.Helper()
	dbfx.InsertNoID(t, "usage_charge", testutil.Cols{
		"workspace_id": workspaceID, "day": day, "meter": meter, "quantity": 5, "plan_key": "payg",
		"unit_price": 30000, "per": 1000, "discount_pct": 20, "net_amount": net, "note": note, "order_id": orderID,
	}, "workspace_id = $1 AND day = $2 AND meter = $3", workspaceID, day, meter)
}

func orderUsageRequest(role, orderID string) *http.Request {
	return billingRequest(http.MethodGet, "/api/workspaces/x/orders/y/usage", nil, role, orderID)
}

func TestOrderUsageLinesAndNotFound(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	other := dbfx.Workspace(t, "Usage Other", "usage-other-"+uuid.NewString()[:8])
	order := usageOrder(t, testWorkspaceID, "usage")
	// Inserted out of order on purpose.
	usageCharge(t, testWorkspaceID, order, "2031-03-02", "members", 300, "")
	usageCharge(t, testWorkspaceID, order, "2031-03-01", "members", 100, "")
	usageCharge(t, testWorkspaceID, order, "2031-03-01", "storage_bytes", 200, "")
	usageCharge(t, testWorkspaceID, order, "2031-03-01", "tasks_run", 400, "suspended")
	// A charge of the same workspace on no order, and one of another order, stay out.
	usageCharge(t, testWorkspaceID, usageOrder(t, testWorkspaceID, "usage"), "2031-03-05", "tasks_run", 9, "")

	for _, role := range []string{"owner", "admin"} {
		w := httptest.NewRecorder()
		testHandler.GetWorkspaceOrderUsage(w, orderUsageRequest(role, order))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body = %s", role, w.Code, w.Body.String())
		}
		var out orderUsageResponse
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.OrderID != order {
			t.Errorf("order_id = %s, want %s", out.OrderID, order)
		}
		var got []string
		for _, l := range out.Lines {
			got = append(got, l.Day+"/"+l.Meter)
		}
		want := []string{"2031-03-01/tasks_run", "2031-03-01/storage_bytes", "2031-03-01/members", "2031-03-02/members"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s: lines = %v, want %v", role, got, want)
		}
		first := out.Lines[0]
		if first.Quantity != 5 || first.UnitPrice != 30000 || first.Per != 1000 || first.DiscountPct != 20 || first.NetAmount != 400 || first.Note != "suspended" {
			t.Errorf("%s: first line = %+v", role, first)
		}
	}

	var raw map[string]any
	w := httptest.NewRecorder()
	testHandler.GetWorkspaceOrderUsage(w, orderUsageRequest("owner", usageOrder(t, testWorkspaceID, "usage")))
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &raw) != nil || !isEmptyList(raw["lines"]) {
		t.Errorf("order without lines: status %d body %s, want 200 and lines []", w.Code, w.Body.String())
	}

	notFound := map[string]string{
		"another workspace's usage order": usageOrder(t, other, "usage"),
		"a subscribe order":               usageOrder(t, testWorkspaceID, "subscribe"),
		"a missing order":                 uuid.NewString(),
	}
	for name, id := range notFound {
		w := httptest.NewRecorder()
		testHandler.GetWorkspaceOrderUsage(w, orderUsageRequest("owner", id))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", name, w.Code)
		}
	}
	for _, role := range []string{"member", "viewer"} {
		w := httptest.NewRecorder()
		testHandler.GetWorkspaceOrderUsage(w, orderUsageRequest(role, order))
		if w.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", role, w.Code)
		}
	}
}

func isEmptyList(v any) bool {
	l, ok := v.([]any)
	return ok && len(l) == 0
}
