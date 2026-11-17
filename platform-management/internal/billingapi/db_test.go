package billingapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it.

type noCache struct{}

func (noCache) Invalidate(string) {}
func (noCache) InvalidateAll()    {}

type noWorkspaceCache struct{}

func (noWorkspaceCache) Invalidate(context.Context, string) error { return nil }

func suffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func TestDBCheckoutAndPurchaseWriteScopedPendingOrders(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	s := suffix(t)

	var user, ws string
	if err := pool.QueryRow(ctx, `INSERT INTO "user" (name, email) VALUES ('Order Test', $1) RETURNING id::text`, "order-"+s+"@example.test").Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ($1, $2) RETURNING id::text`, "Order Test "+s, "order-test-"+s).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, ws, user); err != nil {
		t.Fatal(err)
	}
	key := "t" + s
	m, y := int64(100000), int64(1000000)
	seats := 1
	if err := plan.NewPgxRunner(pool).InTx(ctx, func(tx plan.Tx) error {
		return tx.Upsert(ctx, plan.Plan{
			Key: key, Name: "Order test", Currency: "vnd", IsPublic: true, SeatPriceMonth: &m, SeatPriceYear: &y, MinSeats: &seats,
			Limits: plan.Limits{}, Features: map[string]bool{},
		})
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		for _, q := range []string{
			`DELETE FROM audit_event WHERE workspace_id = $1`, `DELETE FROM billing_order WHERE workspace_id = $1`,
			`DELETE FROM workspace_subscription WHERE workspace_id = $1`, `DELETE FROM member WHERE workspace_id = $1`,
			`DELETE FROM workspace WHERE id = $1`,
		} {
			pool.Exec(bg, q, ws)
		}
		pool.Exec(bg, `DELETE FROM "user" WHERE id = $1`, user)
		pool.Exec(bg, `DELETE FROM plan WHERE key = $1`, key)
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
		Orders: orders, Workspaces: NewPgxWorkspaces(pool), IndividualMaxSeats: 5, VATRate: billing.DefaultVATRate, AppURL: "https://app.example.com",
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	api := h.Handler(trust.New(trust.DefaultCIDRs()).RequireUser)
	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:5000"
		req.Header.Set(trust.UserHeader, user)
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		return rec
	}

	// Checkout writes one pending subscribe order for this workspace, and the
	// same key writes no second one.
	body := `{"workspace_id":"` + ws + `","interval":"month","idempotency_key":"k-` + s + `","plan_key":"` + key + `","seats":2}`
	first, second := post(checkoutPath, body), post(checkoutPath, body)
	if first.Code != 200 || second.Code != 200 || first.Body.String() != second.Body.String() {
		t.Fatalf("checkout: %d %s / %d %s", first.Code, first.Body, second.Code, second.Body)
	}
	var kind, status string
	var amount, vat int64
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*), min(kind), min(status), min(amount), min(vat_amount) FROM billing_order WHERE workspace_id = $1`, ws).
		Scan(&n, &kind, &status, &amount, &vat); err != nil {
		t.Fatal(err)
	}
	if n != 1 || kind != "subscribe" || status != "pending" || amount != 200000 || vat != 20000 {
		t.Fatalf("orders = %d %s %s %d %d", n, kind, status, amount, vat)
	}

	// A seat purchase needs a subscription inside its period.
	now := time.Now().UTC()
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspace_subscription (workspace_id, plan_key, status, seats, interval, current_period_start, current_period_end, version)
		VALUES ($1, $2, 'active', 2, 'month', $3, $4, 3)`, ws, key, now.Add(-24*time.Hour), now.Add(29*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	preview := post("/api/v1/subscriptions/"+ws+"/seats/purchase-preview", `{"additional_seats":1}`)
	if preview.Code != 200 {
		t.Fatalf("preview: %d %s", preview.Code, preview.Body)
	}
	stale := post("/api/v1/subscriptions/"+ws+"/seats/purchases",
		`{"additional_seats":1,"expected_current_seats":2,"expected_purchase_version":2,"accepted_proration_amount":1,"currency":"vnd","idempotency_key":"p-`+s+`"}`)
	if stale.Code != 409 {
		t.Fatalf("stale purchase: %d %s", stale.Code, stale.Body)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_order WHERE workspace_id = $1 AND kind = 'add_seats'`, ws).Scan(&n); err != nil || n != 0 {
		t.Fatalf("add_seats orders after a refused purchase = %d, %v", n, err)
	}
}
