package billing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/multica-ai/multica/platform-management/internal/paymentnotice"
)

const webhookPath = "/api/payments/webhook/{provider}"

// DefaultWebhookMaxBytes caps a webhook body when WebhookDeps.MaxBytes is 0.
const DefaultWebhookMaxBytes = 64 << 10

// NoticeStore is the part of paymentnotice.Store the listener writes to.
type NoticeStore interface {
	Insert(ctx context.Context, n paymentnotice.NewNotice) (paymentnotice.Notice, bool, error)
	Transition(ctx context.Context, c paymentnotice.Change) error
}

type WebhookDeps struct {
	Registry *Registry
	Notices  NoticeStore
	MaxBytes int64 // 0 means DefaultWebhookMaxBytes
	Log      *slog.Logger
}

// NewWebhookHandler serves POST /api/payments/webhook/{provider} and answers
// 404 to everything else. It records the notice and applies nothing.
func NewWebhookHandler(d WebhookDeps) http.Handler {
	if d.MaxBytes <= 0 {
		d.MaxBytes = DefaultWebhookMaxBytes
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	h := &webhook{WebhookDeps: d}
	mux := http.NewServeMux()
	mux.HandleFunc(webhookPath, h.serve)
	mux.HandleFunc("/", h.serve)
	return mux
}

type webhook struct{ WebhookDeps }

func (h *webhook) serve(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("provider")
	if r.Method != http.MethodPost || name == "" {
		writeWebhookJSON(w, http.StatusNotFound, `{"error":"not found","code":"not_found"}`)
		return
	}
	provider, ok := h.Registry.ByName(name)
	if !ok {
		writeWebhookJSON(w, http.StatusNotFound, `{"error":"not found","code":"not_found"}`)
		return
	}

	body, err := readBody(w, r, h.MaxBytes)
	if err != nil {
		h.done(w, name, http.StatusRequestEntityTooLarge, "body_too_large",
			`{"error":"body too large","code":"payment_body_too_large"}`)
		return
	}
	sum := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(sum[:])

	n, err := provider.ParseNotification(body, r.Header)
	switch {
	case errors.Is(err, ErrSignatureInvalid):
		h.done(w, name, http.StatusUnauthorized, "signature_invalid",
			`{"error":"invalid signature","code":"payment_signature_invalid"}`)
		return
	case errors.Is(err, ErrNotIncoming):
		h.done(w, name, http.StatusOK, "not_incoming", `{"success":true}`)
		return
	case errors.Is(err, ErrTransferCodeAmbiguous):
		h.review(w, r, name, bodyHash, "ambiguous_code")
		return
	case err != nil:
		h.review(w, r, name, bodyHash, "unparsable")
		return
	}

	_, inserted, err := h.Notices.Insert(r.Context(), paymentnotice.NewNotice{
		Provider: name, ProviderEventID: n.EventID, TransferCode: n.TransferCode,
		Amount: n.Amount, Currency: n.Currency, BodySHA256: bodyHash,
	})
	if err != nil {
		h.unavailable(w, name)
		return
	}
	outcome := "stored"
	if !inserted {
		outcome = "duplicate"
	}
	h.done(w, name, http.StatusOK, outcome, `{"success":true}`)
}

// review records a signed notice the listener cannot match to a payment, keyed
// by the body hash, and moves it to needs_review once.
func (h *webhook) review(w http.ResponseWriter, r *http.Request, name, bodyHash, reason string) {
	notice, inserted, err := h.Notices.Insert(r.Context(), paymentnotice.NewNotice{
		Provider: name, ProviderEventID: bodyHash, BodySHA256: bodyHash,
	})
	if err == nil && inserted {
		err = h.Notices.Transition(r.Context(), paymentnotice.Change{
			ID: notice.ID, From: paymentnotice.StateReceived, To: paymentnotice.StateNeedsReview,
			ReviewReason: reason,
		})
	}
	if err != nil {
		h.unavailable(w, name)
		return
	}
	h.done(w, name, http.StatusOK, "needs_review_"+reason, `{"success":true}`)
}

func (h *webhook) unavailable(w http.ResponseWriter, name string) {
	h.done(w, name, http.StatusServiceUnavailable, "store_unavailable",
		`{"error":"payment store unavailable","code":"payment_store_unavailable"}`)
}

// done writes the response and logs provider, status and outcome only.
func (h *webhook) done(w http.ResponseWriter, provider string, status int, outcome, body string) {
	h.Log.Info("payment webhook", "provider", provider, "status", status, "outcome", outcome)
	writeWebhookJSON(w, status, body)
}

func readBody(w http.ResponseWriter, r *http.Request, max int64) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(w, r.Body, max))
}

func writeWebhookJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
