package config

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/billing"
)

// TestWebhookDisabledByDefault is the P42 part of AC61: with every R4 payment
// variable empty there is no webhook port, the only provider is the manual
// one, and a handler built from that registry has no provider to answer for.
func TestWebhookDisabledByDefault(t *testing.T) {
	env := resendEnv()
	for _, name := range []string{"PLATFORM_WEBHOOK_PORT", "PAYMENT_PROVIDERS", "PAYMENT_WEBHOOK_MAX_BYTES", "PAYMENT_WEBHOOK_CIDRS"} {
		delete(env, name)
	}
	cfg, err := load(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Payments.WebhookPort != "" {
		t.Fatalf("WebhookPort = %q, want empty (no listener)", cfg.Payments.WebhookPort)
	}
	if _, name, err := cfg.Payments.Registry.ForCurrency("vnd"); err != nil || name != billing.ProviderManual {
		t.Fatalf("vnd provider = %q, %v; want %q", name, err, billing.ProviderManual)
	}
	if _, ok := cfg.Payments.Registry.ByName(billing.ProviderSePay); ok {
		t.Fatal("sepay is registered without PAYMENT_PROVIDERS")
	}

	var logs bytes.Buffer
	h := billing.NewWebhookHandler(billing.WebhookDeps{
		Registry: cfg.Payments.Registry, Log: slog.New(slog.NewTextHandler(&logs, nil)),
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/payments/webhook/"+billing.ProviderSePay, bytes.NewReader([]byte(`{}`))))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST for sepay = %d, want 404", rec.Code)
	}
}
