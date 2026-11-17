package pricechange

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

// The billing summary of a workspace shows the plan's scheduled change once its
// notice is sent, with the prices the change sets and the plan's own for the rest.
func TestDBSummaryScheduledPriceChangeReadsTheStore(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	store := NewPgxStore(f.pool)
	var owner string
	f.query(&owner, `SELECT user_id::text FROM member WHERE workspace_id = $1 AND role = 'owner'`, f.paidWS)

	api := subscription.NewAPI(subscription.APIDeps{
		Subscriptions: subscription.NewPgxReader(f.pool), Plans: plan.NewPgxReader(f.pool),
		Members: subscription.NewPgxMembers(f.pool), Pending: subscription.NewPgxPending(f.pool),
		PriceChanges: store,
	}).Handler(trust.New(trust.DefaultCIDRs()).RequireUser)
	scheduled := func() any {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/"+f.paidWS+"/summary", nil)
		req.RemoteAddr = "127.0.0.1:5000"
		req.Header.Set(trust.UserHeader, owner)
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("summary status %d: %s", rec.Code, rec.Body)
		}
		var m map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		v, present := m["scheduled_price_change"]
		if !present {
			t.Fatal("scheduled_price_change is absent")
		}
		return v
	}

	effective := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	var id string
	err := store.InTx(ctx, func(tx Tx) error {
		c, err := tx.Insert(ctx, Change{
			PlanKey: f.key, EffectiveFrom: effective, Reason: "r",
			Old: Prices{FieldSeatPriceMonth: raw("100000")}, New: Prices{FieldSeatPriceMonth: raw("120000")},
		})
		id = c.ID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if v := scheduled(); v != nil {
		t.Errorf("before the notice: scheduled_price_change = %v, want null", v)
	}

	if err := store.MarkNoticeSent(ctx, id, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, _ := scheduled().(map[string]any)
	if got["effective_from"] != effective.Format(time.RFC3339) || got["seat_price_month"] != float64(120000) || got["seat_price_year"] != float64(1000000) {
		t.Errorf("after the notice: scheduled_price_change = %v, want %s, 120000 a month, the current 1000000 a year", got, effective.Format(time.RFC3339))
	}
}
