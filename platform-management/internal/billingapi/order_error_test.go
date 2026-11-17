package billingapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/billing"
)

func TestNoProviderIs503WithRetryAfter(t *testing.T) {
	h := &Handler{Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	w := httptest.NewRecorder()
	h.orderError(w, httptest.NewRequest(http.MethodPost, "/x", nil), billing.ErrNoProvider)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After header")
	}
	if !strings.Contains(w.Body.String(), codePaymentProviderUnavailable) {
		t.Fatalf("body %q does not name %s", w.Body.String(), codePaymentProviderUnavailable)
	}
}

func TestOtherOrderErrorIsInternal(t *testing.T) {
	h := &Handler{Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	w := httptest.NewRecorder()
	h.orderError(w, httptest.NewRequest(http.MethodPost, "/x", nil), errors.New("db down"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}
