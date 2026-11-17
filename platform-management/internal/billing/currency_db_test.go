package billing

import (
	"context"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/paymentnotice"
)

// AC60: a plan in another currency is recorded and confirmed in that currency.
// The money table is not touched, so this relies only on the order and the
// reconciler carrying the draft's currency through unchanged.
func TestPlanInOtherCurrencyPricedAndInvoicedInThatCurrency(t *testing.T) {
	f := newReconFixture(t)
	ctx := context.Background()

	d := f.draft("usd-1")
	d.Amount, d.VATAmount, d.Currency = 2900, 290, "usd"
	o, created, err := f.svc.CreateOrder(ctx, d)
	if err != nil || !created {
		t.Fatalf("CreateOrder = %v, %v", created, err)
	}

	var currency string
	var amount, vat int64
	if err := f.pool.QueryRow(ctx,
		`SELECT currency, amount, vat_amount FROM billing_order WHERE id = $1`, o.ID).Scan(&currency, &amount, &vat); err != nil {
		t.Fatal(err)
	}
	if currency != "usd" || amount != 2900 || vat != 290 {
		t.Fatalf("stored order = %s %d + %d, want usd 2900 + 290", currency, amount, vat)
	}
	if o.Total() != 3190 {
		t.Fatalf("Total = %d, want 3190", o.Total())
	}

	// The same amount in another currency does not pay the order.
	f.wantReview(f.notice("evt-vnd", o.TransferCode, o.Total(), "vnd", time.Time{}), ReasonCurrencyMismatch)
	if f.orderStatus(o.ID) != StatusPending {
		t.Fatal("a vnd notice changed the usd order")
	}

	got := f.reconcile(f.notice("evt-usd", o.TransferCode, o.Total(), "usd", time.Time{}))
	if got.State != paymentnotice.StateApplied || got.OrderID != o.ID {
		t.Fatalf("usd notice = %s/order %q, want applied to %s", got.State, got.OrderID, o.ID)
	}
	if f.orderStatus(o.ID) != StatusPaid {
		t.Fatal("order is not paid")
	}
}
