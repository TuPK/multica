package invoice

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	enqueueBatch = 500
	issueBatch   = 50
	claimLease   = 5 * time.Minute
)

// ErrProviderNotConfigured is returned by Retry when the service has
// no provider, before any store or provider call.
var ErrProviderNotConfigured = errors.New("invoice: e-invoicing provider is not configured")

type Config struct {
	// StartAt is the earliest paid_at that gets an invoice.
	StartAt     time.Time
	MaxAttempts int
}

type Service struct {
	store    Store
	provider Provider
	cfg      Config
	now      func() time.Time
}

// NewService returns a service. A nil provider turns e-invoicing off: every
// job does nothing.
func NewService(store Store, provider Provider, cfg Config) *Service {
	return &Service{store: store, provider: provider, cfg: cfg, now: time.Now}
}

// Enqueue creates a pending record for each paid order without one and returns
// how many it created.
func (s *Service) Enqueue(ctx context.Context) (int, error) {
	if s.provider == nil {
		return 0, nil
	}
	orders, err := s.store.ListPaidWithoutRecord(ctx, s.cfg.StartAt, enqueueBatch)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, o := range orders {
		_, err := s.store.Insert(ctx, NewRecord{
			OrderID:     o.ID,
			WorkspaceID: o.WorkspaceID,
			Provider:    s.provider.Name(),
			Currency:    o.Currency,
			Amount:      o.Amount,
			VATAmount:   o.VATAmount,
			Buyer:       o.Buyer,
		})
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// A concurrent run created the record.
			continue
		}
		if err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

// Issue claims the due records and sends each to the provider. Provider errors
// are recorded on the row; only store errors are returned.
func (s *Service) Issue(ctx context.Context) (int, error) {
	if s.provider == nil {
		return 0, nil
	}
	now := s.now()
	recs, err := s.store.Claim(ctx, now, claimLease, issueBatch)
	if err != nil {
		return 0, err
	}
	for i, rec := range recs {
		if err := s.process(ctx, now, rec); err != nil {
			return i, err
		}
	}
	return len(recs), nil
}

func (s *Service) process(ctx context.Context, now time.Time, rec Record) error {
	var buyer Buyer
	if err := json.Unmarshal(rec.Buyer, &buyer); err != nil {
		buyer = Buyer{}
	}
	if missing := CheckBuyer(s.provider, buyer); len(missing) > 0 {
		return s.store.RecordAttempt(ctx, rec.OrderID, rec.Revision, Attempt{
			State:         StateNeedsInfo,
			NextAttemptAt: now,
			LastErrorCode: "missing_fields",
			MissingFields: missing,
		})
	}
	req := IssueRequest{
		ReferenceCode: ReferenceCode(rec.OrderID, rec.Revision),
		Currency:      rec.Currency,
		Amount:        rec.Amount,
		VATAmount:     rec.VATAmount,
		Buyer:         buyer,
	}
	// The claim already counted this attempt, so any earlier one may have
	// reached the provider.
	afterUnknown := rec.Attempts > 1

	sub, err := s.submit(ctx, req, afterUnknown)
	if err == nil {
		var inv Invoice
		inv, err = s.resolve(ctx, sub)
		if err == nil {
			return s.store.MarkIssued(ctx, rec.OrderID, rec.Revision, Issued{
				ProviderInvoiceID: inv.ProviderInvoiceID,
				InvoiceNumber:     inv.Number,
				LookupURL:         inv.LookupURL,
				IssuedAt:          inv.IssuedAt,
			})
		}
	}
	return s.fail(ctx, now, rec, err)
}

// submit issues the invoice, first looking it up by reference when an earlier
// attempt ended unknown.
func (s *Service) submit(ctx context.Context, req IssueRequest, afterUnknown bool) (Submission, error) {
	return Submit(ctx, s.provider, req, afterUnknown)
}

// resolve turns a submission into an issued invoice. An invoice that is not
// issued yet is a transient error, so the next run looks it up by reference.
func (s *Service) resolve(ctx context.Context, sub Submission) (Invoice, error) {
	if sub.Existing {
		if !sub.Invoice.Issued {
			return Invoice{}, errors.New("invoice provider: invoice not issued yet")
		}
		return sub.Invoice, nil
	}
	poll, err := s.provider.Status(ctx, sub.Ticket.TrackingCode)
	if err != nil {
		return Invoice{}, err
	}
	switch poll.State {
	case PollSuccess:
		return poll.Invoice, nil
	case PollFailed:
		return Invoice{}, &ProviderError{Kind: KindPermanent, Code: poll.Code}
	}
	return Invoice{}, errors.New("invoice provider: invoice still pending")
}

// fail records a failed attempt. Only the error's code and field names are
// stored, never its message.
func (s *Service) fail(ctx context.Context, now time.Time, rec Record, err error) error {
	d := Decide(err, rec.Attempts, s.cfg.MaxAttempts)
	a := Attempt{State: d.State, NextAttemptAt: now.Add(d.Delay), LastErrorCode: "unknown_error"}
	var pe *ProviderError
	if errors.As(err, &pe) {
		a.LastErrorCode = pe.Code
		if pe.Kind == KindMissingFields {
			a.MissingFields = pe.Fields
		}
	}
	return s.store.RecordAttempt(ctx, rec.OrderID, rec.Revision, a)
}

// Latest returns the highest revision of the order.
func (s *Service) Latest(ctx context.Context, orderID string) (Record, error) {
	return s.store.GetLatest(ctx, orderID)
}

// Retry queues a failed or needs_info record for the next Issue run. A buyer
// that still lacks required fields is refused with a missing-fields
// ProviderError and the record is left as it is.
func (s *Service) Retry(ctx context.Context, orderID string, revision int, by string) (Record, error) {
	if s.provider == nil {
		return Record{}, ErrProviderNotConfigured
	}
	rec, err := s.store.Get(ctx, orderID, revision)
	if err != nil {
		return Record{}, err
	}
	if rec.State != StateFailed && rec.State != StateNeedsInfo {
		return Record{}, ErrStateConflict
	}
	var buyer Buyer
	if err := json.Unmarshal(rec.Buyer, &buyer); err != nil {
		buyer = Buyer{}
	}
	if missing := CheckBuyer(s.provider, buyer); len(missing) > 0 {
		return Record{}, &ProviderError{Kind: KindMissingFields, Code: "VALIDATION_ERROR", Fields: missing}
	}
	return s.store.RetryRecord(ctx, orderID, revision, by, s.now())
}
