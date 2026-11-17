package billing

import (
	"bytes"
	"log/slog"
	"net/http"
	"testing"
)

// parseSpy counts ParseNotification calls.
type parseSpy struct {
	Manual
	parses int
}

func (s *parseSpy) ParseNotification([]byte, http.Header) (Notification, error) {
	s.parses++
	return Notification{}, nil
}

// TestOversizeBodyRefusedBeforeParse: a body over the limit is answered 413
// without reaching the provider or the store.
func TestOversizeBodyRefusedBeforeParse(t *testing.T) {
	spy := &parseSpy{}
	reg := &Registry{
		byCurrency: map[string]string{"vnd": "sepay"},
		providers:  map[string]PaymentProvider{"sepay": spy},
	}
	store := newFakeNoticeStore()
	var logs bytes.Buffer
	rec := &fakeReconciler{store: store}
	h := NewWebhookHandler(WebhookDeps{Registry: reg, Notices: store, Reconciler: rec, MaxBytes: 16, Log: slog.New(slog.NewTextHandler(&logs, nil))})
	rr := postWebhook(h, webhookTestPath, bytes.Repeat([]byte("x"), 64), nil)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rr.Code)
	}
	if spy.parses != 0 {
		t.Errorf("ParseNotification calls = %d, want 0", spy.parses)
	}
	if n := store.insertCount(); n != 0 {
		t.Errorf("inserts = %d, want 0", n)
	}
}
