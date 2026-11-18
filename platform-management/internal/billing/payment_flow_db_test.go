package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/paymentnotice"
)

// endToEndProvider is a fake automatic provider. Its notification body is
// {"id","code","amount"}; the header X-Fake-Signature must equal "ok".
type endToEndProvider struct{ fakeProvider }

func (endToEndProvider) ParseNotification(body []byte, h http.Header) (Notification, error) {
	if h.Get("X-Fake-Signature") != "ok" {
		return Notification{}, ErrSignatureInvalid
	}
	var in struct {
		ID     int64  `json:"id"`
		Code   string `json:"code"`
		Amount int64  `json:"amount"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return Notification{}, err
	}
	return Notification{TransferCode: in.Code, Amount: in.Amount, EventID: strconv.FormatInt(in.ID, 10), Currency: "VND"}, nil
}

// TestManualConfirmStillWorks: with the manual provider, a staff confirmation
// pays the order and a repeat applies nothing twice.
func TestManualConfirmStillWorks(t *testing.T) {
	f := newDBFixture(t)
	f.seedSubscription(1)
	s := f.service(nil, nil)
	ctx := context.Background()
	o, _, err := s.CreateOrder(ctx, f.draft("manual"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ApplyPayment(ctx, Confirmation{OrderID: o.ID, ConfirmedBy: f.staff, ActorLabel: "billing@example.com"})
	if err != nil || got.AlreadyPaid || got.Order.Status != StatusPaid {
		t.Fatalf("ApplyPayment = %+v, %v; want a paid order", got, err)
	}
	again, err := s.ApplyPayment(ctx, Confirmation{OrderID: o.ID, ConfirmedBy: f.staff})
	if err != nil || !again.AlreadyPaid {
		t.Fatalf("repeat ApplyPayment = %+v, %v; want AlreadyPaid", again, err)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = $2`, f.ws, audit.ActionBillingOrderPaid); n != 1 {
		t.Fatalf("billing.order_paid rows = %d, want 1", n)
	}
}

// TestPaymentFlowThroughFakeProviderEndToEnd: a signed notice posted to the
// webhook handler is stored, matched by transfer code, and pays the order once;
// a bad signature stores nothing; a retry of the same event changes nothing.
func TestPaymentFlowThroughFakeProviderEndToEnd(t *testing.T) {
	f := newReconFixture(t)
	f.seedSubscription(1)
	o := f.order()

	reg := &Registry{
		byCurrency: map[string]string{"vnd": f.provider},
		providers:  map[string]PaymentProvider{f.provider: endToEndProvider{}},
	}
	var logs bytes.Buffer
	h := NewWebhookHandler(WebhookDeps{
		Registry: reg, Notices: f.notices, Reconciler: f.rec, Log: slog.New(slog.NewTextHandler(&logs, nil)),
	})
	path := "/api/payments/webhook/" + f.provider
	body := []byte(fmt.Sprintf(`{"id":7001,"code":%q,"amount":%d}`, o.TransferCode, o.Total()))
	post := func(sig string) *httptest.ResponseRecorder {
		return postWebhook(h, path, body, http.Header{"X-Fake-Signature": {sig}})
	}

	if rec := post("bad"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature = %d, want 401", rec.Code)
	}
	if f.orderStatus(o.ID) == StatusPaid {
		t.Fatal("order paid by an unsigned notice")
	}

	if rec := post("ok"); rec.Code != http.StatusOK || rec.Body.String() != `{"success":true}` {
		t.Fatalf("signed notice = %d %s, want 200 success", rec.Code, rec.Body.String())
	}
	if f.orderStatus(o.ID) != StatusPaid {
		t.Fatal("order is not paid after the notice")
	}
	if rec := post("ok"); rec.Code != http.StatusOK {
		t.Fatalf("retry = %d, want 200", rec.Code)
	}

	if n := f.count(`SELECT count(*) FROM payment_notice WHERE provider = $1`, f.provider); n != 1 {
		t.Fatalf("payment_notice rows = %d, want 1", n)
	}
	n, _, err := f.notices.Insert(context.Background(), paymentnotice.NewNotice{Provider: f.provider, ProviderEventID: "7001"})
	if err != nil || n.State != paymentnotice.StateApplied || n.OrderID != o.ID {
		t.Fatalf("notice = %s/order %q, %v; want applied to %s", n.State, n.OrderID, err, o.ID)
	}
	if got := f.auditCount(audit.ActionBillingOrderPaid, o.ID); got != 1 {
		t.Fatalf("billing.order_paid rows = %d, want 1", got)
	}
}
