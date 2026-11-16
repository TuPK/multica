package billingapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

// TestDBCheckoutActivatesAUsagePlanOnACompleteProfile covers the checkout of a
// pay as you go plan (spec 7b.3, rule 6): a complete profile activates the plan
// at once; an incomplete one is refused with billing_profile_incomplete; a
// workspace on a paid seat plan is refused with plan_change_unavailable.
func TestDBCheckoutActivatesAUsagePlanOnACompleteProfile(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	s := suffix(t)
	usageKey, seatKey := "payg-"+s, "seat-"+s
	var user, complete, incomplete, seated string
	for _, dst := range []*string{&user, &complete, &incomplete, &seated} {
		if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO "user" (id, name, email) VALUES ($1, 'Usage Test', $2)`, user, "usage-"+s+"@example.test"); err != nil {
		t.Fatal(err)
	}
	workspaces := []string{complete, incomplete, seated}
	for i, ws := range workspaces {
		if _, err := pool.Exec(ctx, `INSERT INTO workspace (id, name, slug) VALUES ($1, $2, $3)`, ws, "Usage "+s, "usage-"+s+"-"+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, ws, user); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO billing_profile (workspace_id, account_kind, legal_name, invoice_email)
		VALUES ($1, 'individual', 'Nguyen Van A', 'a@example.test')`, complete); err != nil {
		t.Fatal(err)
	}
	m, y, one := int64(149000), int64(1490000), 1
	if err := plan.NewPgxRunner(pool).InTx(ctx, func(tx plan.Tx) error {
		if err := tx.Upsert(ctx, plan.Plan{
			Key: usageKey, Name: "Usage test", Currency: "vnd", IsPublic: true,
			Audience: plan.AudienceAny, BillingModel: plan.BillingUsage,
			Limits: plan.Limits{}, Features: map[string]bool{},
		}); err != nil {
			return err
		}
		return tx.Upsert(ctx, plan.Plan{
			Key: seatKey, Name: "Seat test", Currency: "vnd", IsPublic: true,
			SeatPriceMonth: &m, SeatPriceYear: &y, MinSeats: &one,
			Limits: plan.Limits{}, Features: map[string]bool{},
		})
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workspace_subscription (workspace_id, plan_key, status, seats, interval, version)
		VALUES ($1, $2, 'active', 1, 'month', 1)`, seated, seatKey); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		for _, ws := range workspaces {
			for _, q := range []string{
				`DELETE FROM audit_event WHERE workspace_id = $1`, `DELETE FROM billing_order WHERE workspace_id = $1`,
				`DELETE FROM workspace_subscription WHERE workspace_id = $1`, `DELETE FROM billing_profile WHERE workspace_id = $1`,
				`DELETE FROM member WHERE workspace_id = $1`, `DELETE FROM workspace WHERE id = $1`,
			} {
				pool.Exec(bg, q, ws)
			}
		}
		pool.Exec(bg, `DELETE FROM "user" WHERE id = $1`, user)
		pool.Exec(bg, `DELETE FROM plan WHERE key IN ($1, $2)`, usageKey, seatKey)
	})

	orders, err := billing.New(billing.Deps{
		Runner: billing.NewPgxRunner(pool), Provider: billing.ProviderManual,
		Workspaces: noWorkspaceCache{}, Entitlements: noCache{},
	})
	if err != nil {
		t.Fatal(err)
	}
	h, err := New(Deps{
		Subscriptions: subscription.NewPgxReader(pool), Plans: plan.NewPgxReader(pool), Members: subscription.NewPgxMembers(pool),
		Orders: orders, Activator: orders, Workspaces: NewPgxWorkspaces(pool), VATRate: billing.DefaultVATRate,
		AppURL: "https://app.example.com", Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	api := h.Handler(trust.New(trust.DefaultCIDRs()).RequireUser)
	checkout := func(ws, key string) *httptest.ResponseRecorder {
		body := `{"workspace_id":"` + ws + `","interval":"month","idempotency_key":"` + key + `","plan_key":"` + usageKey + `"}`
		req := httptest.NewRequest(http.MethodPost, checkoutPath, strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:5000"
		req.Header.Set(trust.UserHeader, user)
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		return rec
	}

	// A complete profile activates the plan; the same key returns the same answer.
	first, second := checkout(complete, "u1-"+s), checkout(complete, "u1-"+s)
	if first.Code != http.StatusOK || second.Code != http.StatusOK || first.Body.String() != second.Body.String() {
		t.Fatalf("checkout: %d %s / %d %s", first.Code, first.Body, second.Code, second.Body)
	}
	var resp checkoutResponse
	if err := json.Unmarshal(first.Body.Bytes(), &resp); err != nil || !strings.Contains(resp.URL, "/billing/orders/"+resp.RequestID) {
		t.Fatalf("response = %s (%v), want an order url", first.Body, err)
	}
	var status, planKey string
	var seats, n int
	var amount int64
	if err := pool.QueryRow(ctx, `SELECT status, plan_key, seats FROM workspace_subscription WHERE workspace_id = $1`, complete).Scan(&status, &planKey, &seats); err != nil {
		t.Fatal(err)
	}
	if status != "active" || planKey != usageKey || seats != 0 {
		t.Fatalf("subscription = %s %s seats %d, want active %s with 0 seats", status, planKey, seats, usageKey)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*), min(amount) FROM billing_order WHERE workspace_id = $1 AND status = 'paid' AND kind = 'subscribe'`, complete).Scan(&n, &amount); err != nil {
		t.Fatal(err)
	}
	if n != 1 || amount != 0 {
		t.Fatalf("paid subscribe orders = %d at %d, want one at 0", n, amount)
	}

	// An incomplete profile is refused and changes nothing.
	refused := checkout(incomplete, "u2-"+s)
	var body struct {
		Code   string   `json:"code"`
		Fields []string `json:"fields"`
	}
	if err := json.Unmarshal(refused.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if refused.Code != http.StatusConflict || body.Code != "billing_profile_incomplete" ||
		strings.Join(body.Fields, ",") != "legal_name,invoice_email" {
		t.Fatalf("incomplete profile = %d %s, want 409 billing_profile_incomplete with the payer name and invoice email", refused.Code, refused.Body)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM workspace_subscription WHERE workspace_id = $1`, incomplete).Scan(&n); err != nil || n != 0 {
		t.Fatalf("a refused activation wrote %d subscription rows (%v)", n, err)
	}

	// A workspace on a paid seat plan does not switch here yet.
	switched := checkout(seated, "u3-"+s)
	if switched.Code != http.StatusConflict || !strings.Contains(switched.Body.String(), `"reason":"from_paid_plan"`) {
		t.Fatalf("switch from a paid plan = %d %s, want 409 plan_change_unavailable from_paid_plan", switched.Code, switched.Body)
	}
}
