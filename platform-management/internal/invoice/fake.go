package invoice

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"
)

type Fake struct {
	mu           sync.Mutex
	seq          int
	byRef        map[string]Invoice
	byID         map[string]bool // provider invoice ID -> replaced
	tracking     map[string]string
	polls        map[string]int
	pendingPolls int
	issueCalls   int
	lastIssue    IssueRequest
	failIssue    *ProviderError
	failCreated  bool
	failLookup   error
}

var _ Provider = (*Fake)(nil)

func NewFake() *Fake {
	return &Fake{
		byRef:    map[string]Invoice{},
		byID:     map[string]bool{},
		tracking: map[string]string{},
		polls:    map[string]int{},
	}
}

func (f *Fake) Name() string { return "fake" }

func (f *Fake) RequiredBuyerFields() []string {
	return []string{"name", "tax_code", "address", "email"}
}

func (f *Fake) FailNextIssue(err *ProviderError, created bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failIssue, f.failCreated = err, created
}

func (f *Fake) FailNextLookup(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failLookup = err
}

func (f *Fake) SetPendingPolls(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pendingPolls = n
}

func (f *Fake) IssueCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.issueCalls
}

func (f *Fake) LastIssue() IssueRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastIssue
}

// create must be called with f.mu held.
func (f *Fake) create(ref string) Ticket {
	f.seq++
	n := strconv.Itoa(f.seq)
	inv := Invoice{
		ProviderInvoiceID: "fake-inv-" + n,
		Number:            "FK" + n,
		LookupURL:         "https://fake.invalid/lookup/" + n,
		Issued:            true,
		IssuedAt:          time.Now().UTC(),
	}
	f.byRef[ref] = inv
	f.byID[inv.ProviderInvoiceID] = false
	track := "fake-trk-" + n
	f.tracking[track] = ref
	return Ticket{ReferenceCode: ref, TrackingCode: track}
}

func (f *Fake) Issue(_ context.Context, req IssueRequest) (Ticket, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Buyer validation rejects before the request counts as reaching the provider.
	if f.failIssue == nil {
		if missing := CheckBuyer(f, req.Buyer); len(missing) > 0 {
			return Ticket{}, &ProviderError{Kind: KindMissingFields, Code: "VALIDATION_ERROR", Fields: missing}
		}
	}
	f.issueCalls++
	f.lastIssue = req

	if f.failIssue != nil {
		err, created := f.failIssue, f.failCreated
		f.failIssue, f.failCreated = nil, false
		if created {
			f.create(req.ReferenceCode)
		}
		return Ticket{}, err
	}
	if _, ok := f.byRef[req.ReferenceCode]; ok {
		return Ticket{}, &ProviderError{Kind: KindPermanent, Code: "EINVOICE_DOCUMENT_EXISTED"}
	}
	return f.create(req.ReferenceCode), nil
}

func (f *Fake) Status(_ context.Context, trackingCode string) (Poll, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ref, ok := f.tracking[trackingCode]
	if !ok {
		return Poll{}, errors.New("invoice provider: unknown tracking code")
	}
	if f.polls[trackingCode] < f.pendingPolls {
		f.polls[trackingCode]++
		return Poll{State: PollPending}, nil
	}
	return Poll{State: PollSuccess, Invoice: f.byRef[ref]}, nil
}

func (f *Fake) Lookup(_ context.Context, referenceCode string) (Invoice, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failLookup != nil {
		err := f.failLookup
		f.failLookup = nil
		return Invoice{}, err
	}
	inv, ok := f.byRef[referenceCode]
	if !ok {
		return Invoice{}, ErrNoInvoice
	}
	return inv, nil
}

func (f *Fake) Replace(_ context.Context, originalInvoiceID string, req IssueRequest) (Ticket, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	replaced, ok := f.byID[originalInvoiceID]
	if !ok {
		return Ticket{}, ErrNoInvoice
	}
	if replaced {
		return Ticket{}, &ProviderError{Kind: KindPermanent, Code: "ALREADY_REPLACED"}
	}
	f.byID[originalInvoiceID] = true
	return f.create(req.ReferenceCode), nil
}
