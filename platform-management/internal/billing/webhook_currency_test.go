package billing

import (
	"io"
	"log/slog"
	"net/http"
	"testing"
)

// noticeProvider returns a fixed notification, so a test can send a currency
// the real providers never produce.
type noticeProvider struct {
	fakeProvider
	n Notification
}

func (p noticeProvider) ParseNotification([]byte, http.Header) (Notification, error) {
	return p.n, nil
}

func newNoticeCurrencyHandler(t *testing.T, store *fakeNoticeStore, currency string) (http.Handler, *fakeReconciler) {
	t.Helper()
	rec := &fakeReconciler{store: store}
	reg := &Registry{
		byCurrency: map[string]string{"usd": "fake"},
		providers: map[string]PaymentProvider{"fake": noticeProvider{n: Notification{
			EventID: "evt-1", TransferCode: "MULABCDEFGH", Amount: 1000, Currency: currency,
		}}},
	}
	return NewWebhookHandler(WebhookDeps{Registry: reg, Notices: store, Reconciler: rec, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}), rec
}

// TestNoticeCurrencyOutsideEnabledSetRejected: a notice whose currency is not
// in the enabled set is refused at receipt with 400: no row, no reconcile.
func TestNoticeCurrencyOutsideEnabledSetRejected(t *testing.T) {
	store := newFakeNoticeStore()
	h, rec := newNoticeCurrencyHandler(t, store, "USD")
	resp := postWebhook(h, "/api/payments/webhook/fake", []byte(`{}`), nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s, want 400", resp.Code, resp.Body.String())
	}
	if n := store.insertCount(); n != 0 {
		t.Fatalf("inserts = %d, want 0", n)
	}
	if len(rec.ids) != 0 {
		t.Fatalf("reconciled %v, want none", rec.ids)
	}
}

// TestNoticeInEnabledCurrencyIsStoredAndMatched: a vnd notice is stored and
// handed to the reconciler as before.
func TestNoticeInEnabledCurrencyIsStoredAndMatched(t *testing.T) {
	store := newFakeNoticeStore()
	h, rec := newNoticeCurrencyHandler(t, store, "VND")
	resp := postWebhook(h, "/api/payments/webhook/fake", []byte(`{}`), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d %s, want 200", resp.Code, resp.Body.String())
	}
	if n := store.insertCount(); n != 1 || store.inserts[0].Currency != "VND" {
		t.Fatalf("inserts = %+v, want one row with currency VND as received", store.inserts)
	}
	if len(rec.ids) != 1 {
		t.Fatalf("reconciled %v, want one", rec.ids)
	}
}
