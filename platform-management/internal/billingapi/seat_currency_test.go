package billingapi

import (
	"context"
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

// A seat purchase whose currency differs from the subscription's is refused with
// 409 currency_mismatch, and no billing_order row is written (spec 7.4, rule 1).
func TestSeatPurchaseCurrencyMismatchRefused(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	s := suffix(t)

	var user, ws string
	if err := pool.QueryRow(ctx, `INSERT INTO "user" (name, email) VALUES ('Seat Currency', $1) RETURNING id::text`, "seatcur-"+s+"@example.test").Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ($1, $2) RETURNING id::text`, "Seat Currency "+s, "seat-currency-"+s).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, ws, user); err != nil {
		t.Fatal(err)
	}
	key := "c" + s
	m, y := int64(100000), int64(1000000)
	seats := 1
	if err := plan.NewPgxRunner(pool).InTx(ctx, func(tx plan.Tx) error {
		return tx.Upsert(ctx, plan.Plan{
			Key: key, Name: "Seat currency", Currency: "usd", IsPublic: true, SeatPriceMonth: &m, SeatPriceYear: &y, MinSeats: &seats,
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
	now := time.Now().UTC()
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspace_subscription (workspace_id, plan_key, status, seats, interval, current_period_start, current_period_end, version)
		VALUES ($1, $2, 'active', 2, 'month', $3, $4, 3)`, ws, key, now.Add(-24*time.Hour), now.Add(29*24*time.Hour)); err != nil {
		t.Fatal(err)
	}

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

	// The request matches the subscription in every field but the currency; it
	// is lower-cased before the comparison, so "VND" and "vnd" are the same.
	for _, currency := range []string{"vnd", "VND"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/"+ws+"/seats/purchases", strings.NewReader(
			`{"additional_seats":1,"expected_current_seats":2,"expected_purchase_version":3,"accepted_proration_amount":1,"currency":"`+currency+`","idempotency_key":"cur-`+currency+s+`"}`))
		req.RemoteAddr = "127.0.0.1:5000"
		req.Header.Set(trust.UserHeader, user)
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		wantCode(t, rec, http.StatusConflict, "currency_mismatch")
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_order WHERE workspace_id = $1`, ws).Scan(&n); err != nil || n != 0 {
		t.Fatalf("billing_order rows after a refused seat purchase = %d, %v", n, err)
	}
}
