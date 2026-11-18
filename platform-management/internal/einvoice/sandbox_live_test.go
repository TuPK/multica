package einvoice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/invoice"
)

// TestSepaySandboxLive issues one test invoice against the SePay sandbox. It
// needs SEPAY_EINVOICE_API_KEY (client_id:client_secret) and
// SEPAY_EINVOICE_PROVIDER_ACCOUNT_ID, and logs only the reference and number.
func TestSepaySandboxLive(t *testing.T) {
	key := os.Getenv("SEPAY_EINVOICE_API_KEY")
	account := os.Getenv("SEPAY_EINVOICE_PROVIDER_ACCOUNT_ID")
	if key == "" || account == "" {
		t.Skip("SEPAY_EINVOICE_API_KEY and SEPAY_EINVOICE_PROVIDER_ACCOUNT_ID are not set")
	}
	p, err := New(Config{
		BaseURL: "https://einvoice-api-sandbox.sepay.vn", APIKey: key, ProviderAccountID: account,
		TaxRate: 10, PollInterval: 2 * time.Second,
	})
	if err != nil {
		t.Fatal("bad live config")
	}
	ctx := context.Background()
	ref := fmt.Sprintf("P43TEST-%d-1", time.Now().Unix())
	tk, err := p.Issue(ctx, invoice.IssueRequest{
		ReferenceCode: ref, Currency: "VND", Amount: 10000, VATAmount: 1000,
		Buyer: invoice.Buyer{Name: "CONG TY TEST P43", TaxCode: "0000000000", Address: "Test address", Email: "test@example.com"},
	})
	if err != nil {
		t.Fatalf("issue %s: %v", ref, err)
	}
	var poll invoice.Poll
	for range 60 {
		if poll, err = p.Status(ctx, tk.TrackingCode); err != nil {
			t.Fatalf("status %s: %v", ref, err)
		}
		if poll.State != invoice.PollPending {
			break
		}
		time.Sleep(3 * time.Second)
	}
	if poll.State != invoice.PollSuccess {
		t.Fatalf("invoice %s ended %s %s", ref, poll.State, poll.Code)
	}
	got, err := p.Lookup(ctx, ref)
	if err != nil && !errors.Is(err, invoice.ErrNoInvoice) {
		t.Fatalf("lookup %s: %v", ref, err)
	}
	t.Logf("reference %s invoice number %s lookup issued=%v", ref, poll.Invoice.Number, got.Issued)
}
