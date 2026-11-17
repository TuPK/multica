package invoice

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Pure tests: no database. The fake provider is the only provider these
// tests touch; the real SePay adapter is not called (spec 7.3, P43.3).

func testBuyer() Buyer {
	return Buyer{
		Name:    "Cong ty SECRET-NAME",
		TaxCode: "0312345678",
		Address: "1 SECRET-STREET",
		Email:   "buyer@example.com",
	}
}

func testIssueRequest(ref string) IssueRequest {
	return IssueRequest{
		ReferenceCode: ref,
		Currency:      "VND",
		Amount:        150000,
		VATAmount:     0,
		Buyer:         testBuyer(),
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestRequiredBuyerFieldsReportedByName: a buyer missing fields is reported by
// field name, in the provider's required order, and the provider is not called.
func TestRequiredBuyerFieldsReportedByName(t *testing.T) {
	p := NewFake()

	b := testBuyer()
	b.TaxCode = ""
	b.Address = ""
	if got, want := CheckBuyer(p, b), []string{"tax_code", "address"}; !sameStrings(got, want) {
		t.Fatalf("CheckBuyer missing = %v, want %v", got, want)
	}
	if got := CheckBuyer(p, testBuyer()); len(got) != 0 {
		t.Fatalf("CheckBuyer on complete buyer = %v, want none", got)
	}

	_, err := p.Issue(context.Background(), IssueRequest{ReferenceCode: ReferenceCode("ord_missing", 1), Currency: "VND", Buyer: b})
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.Kind != KindMissingFields {
		t.Fatalf("Issue with missing buyer fields: err = %v, want KindMissingFields", err)
	}
	if !sameStrings(pe.Fields, []string{"tax_code", "address"}) {
		t.Fatalf("missing fields = %v, want [tax_code address]", pe.Fields)
	}
	if p.IssueCalls() != 0 {
		t.Fatalf("provider was called %d times for a buyer with missing fields; want 0", p.IssueCalls())
	}
}

// TestIssueSendsClientRef: the client reference code is order_id and revision,
// and the provider receives the same code it is given.
func TestIssueSendsClientRef(t *testing.T) {
	p := NewFake()
	ref := ReferenceCode("ord_1", 2)
	if ref != "ord_1-2" {
		t.Fatalf("ReferenceCode(ord_1, 2) = %q, want ord_1-2", ref)
	}
	if ReferenceCode("ord_1", 1) == ReferenceCode("ord_1", 2) {
		t.Fatal("reference codes for different revisions must differ")
	}

	tk, err := p.Issue(context.Background(), testIssueRequest(ref))
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if got := p.LastIssue().ReferenceCode; got != ref {
		t.Fatalf("provider received reference %q, want %q", got, ref)
	}
	if tk.ReferenceCode != ref || tk.TrackingCode == "" {
		t.Fatalf("ticket = %+v, want reference %q and a tracking code", tk, ref)
	}
}

// TestLookupBeforeReissueAfterTimeout: after an unknown-outcome error the
// caller looks the reference code up first. An invoice that exists is never
// issued again; one that does not exist is issued once.
func TestLookupBeforeReissueAfterTimeout(t *testing.T) {
	ctx := context.Background()
	ref := ReferenceCode("ord_timeout", 1)

	t.Run("created before timeout is found, not reissued", func(t *testing.T) {
		p := NewFake()
		p.FailNextIssue(&ProviderError{Kind: KindUnknownOutcome, Code: "TIMEOUT"}, true)

		if _, err := Submit(ctx, p, testIssueRequest(ref), false); err == nil {
			t.Fatal("first Submit: want the timeout error")
		}
		sub, err := Submit(ctx, p, testIssueRequest(ref), true)
		if err != nil {
			t.Fatalf("second Submit: %v", err)
		}
		if !sub.Existing || sub.Invoice.ProviderInvoiceID == "" {
			t.Fatalf("second Submit = %+v, want the existing invoice", sub)
		}
		if p.IssueCalls() != 1 {
			t.Fatalf("Issue calls = %d, want 1 (no reissue)", p.IssueCalls())
		}
	})

	t.Run("not created before timeout is issued once", func(t *testing.T) {
		p := NewFake()
		p.FailNextIssue(&ProviderError{Kind: KindUnknownOutcome, Code: "TIMEOUT"}, false)

		if _, err := Submit(ctx, p, testIssueRequest(ref), false); err == nil {
			t.Fatal("first Submit: want the timeout error")
		}
		sub, err := Submit(ctx, p, testIssueRequest(ref), true)
		if err != nil {
			t.Fatalf("second Submit: %v", err)
		}
		if sub.Existing || sub.Ticket.TrackingCode == "" {
			t.Fatalf("second Submit = %+v, want a new ticket", sub)
		}
		if p.IssueCalls() != 2 {
			t.Fatalf("Issue calls = %d, want 2", p.IssueCalls())
		}
	})

	t.Run("lookup failure does not reissue", func(t *testing.T) {
		p := NewFake()
		p.FailNextIssue(&ProviderError{Kind: KindUnknownOutcome, Code: "TIMEOUT"}, true)
		p.FailNextLookup(&ProviderError{Kind: KindTransient, Code: "HTTP_503"})

		if _, err := Submit(ctx, p, testIssueRequest(ref), false); err == nil {
			t.Fatal("first Submit: want the timeout error")
		}
		if _, err := Submit(ctx, p, testIssueRequest(ref), true); err == nil {
			t.Fatal("second Submit: want the lookup error")
		}
		if p.IssueCalls() != 1 {
			t.Fatalf("Issue calls = %d, want 1 (lookup failed, must not reissue)", p.IssueCalls())
		}
	})
}

// TestErrorsNeverContainBuyerValues: provider errors name fields, never values.
func TestErrorsNeverContainBuyerValues(t *testing.T) {
	ctx := context.Background()
	p := NewFake()

	b := testBuyer()
	b.Address = ""
	_, err := p.Issue(ctx, IssueRequest{ReferenceCode: ReferenceCode("ord_e", 1), Currency: "VND", Buyer: b})
	if err == nil {
		t.Fatal("Issue with missing address: want an error")
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("missing-field error leaks a buyer value: %q", err.Error())
	}

	p.FailNextIssue(&ProviderError{Kind: KindTransient, Code: "HTTP_503"}, false)
	_, err = p.Issue(ctx, testIssueRequest(ReferenceCode("ord_e", 2)))
	if err == nil {
		t.Fatal("Issue with scripted 503: want an error")
	}
	if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "buyer@example.com") {
		t.Fatalf("transient error leaks a buyer value: %q", err.Error())
	}
}

// TestForeignCurrencyForwarded: the currency and amount reach the provider as given (AC60).
func TestForeignCurrencyForwarded(t *testing.T) {
	p := NewFake()
	req := testIssueRequest(ReferenceCode("ord_usd", 1))
	req.Currency = "USD"
	req.Amount = 1234

	if _, err := p.Issue(context.Background(), req); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	got := p.LastIssue()
	if got.Currency != "USD" || got.Amount != 1234 {
		t.Fatalf("provider received currency=%q amount=%d, want USD 1234", got.Currency, got.Amount)
	}
}

// TestReplaceOncePerOriginal: the provider accepts one replacement per original invoice.
func TestReplaceOncePerOriginal(t *testing.T) {
	ctx := context.Background()
	p := NewFake()
	ref := ReferenceCode("ord_rep", 1)
	if _, err := p.Issue(ctx, testIssueRequest(ref)); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	orig, err := p.Lookup(ctx, ref)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}

	if _, err := p.Replace(ctx, orig.ProviderInvoiceID, testIssueRequest(ReferenceCode("ord_rep", 2))); err != nil {
		t.Fatalf("first Replace: %v", err)
	}
	_, err = p.Replace(ctx, orig.ProviderInvoiceID, testIssueRequest(ReferenceCode("ord_rep", 3)))
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.Code != "ALREADY_REPLACED" || pe.Kind != KindPermanent {
		t.Fatalf("second Replace of the same original: err = %v, want permanent ALREADY_REPLACED", err)
	}
}

// TestStatusPollsUntilSuccess: an async issue reports pending until the provider finishes.
func TestStatusPollsUntilSuccess(t *testing.T) {
	ctx := context.Background()
	p := NewFake()
	p.SetPendingPolls(2)

	tk, err := p.Issue(ctx, testIssueRequest(ReferenceCode("ord_poll", 1)))
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	for i := 1; i <= 2; i++ {
		poll, err := p.Status(ctx, tk.TrackingCode)
		if err != nil {
			t.Fatalf("Status %d: %v", i, err)
		}
		if poll.State != PollPending {
			t.Fatalf("Status %d state = %q, want pending", i, poll.State)
		}
	}
	poll, err := p.Status(ctx, tk.TrackingCode)
	if err != nil {
		t.Fatalf("Status 3: %v", err)
	}
	if poll.State != PollSuccess || poll.Invoice.Number == "" || !poll.Invoice.Issued {
		t.Fatalf("Status 3 = %+v, want success with an issued invoice number", poll)
	}
}
