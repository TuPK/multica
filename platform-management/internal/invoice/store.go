package invoice

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrStateConflict means the record is not in the state the operation requires.
	ErrStateConflict = errors.New("invoice: record not in required state")
	ErrNotFound      = errors.New("invoice: record not found")
	// ErrInvalidActor means the staff actor of a retry is not a UUID.
	ErrInvalidActor = errors.New("invoice: actor is not a staff id")
)

// PaidOrder is a paid billing order that has no issuance record yet. Buyer
// holds personal data and must never be logged.
type PaidOrder struct {
	ID          string
	WorkspaceID string
	Currency    string
	Amount      int64
	VATAmount   int64
	Buyer       []byte
}

// Attempt is the outcome of one issue attempt on a pending record.
type Attempt struct {
	State         State
	NextAttemptAt time.Time
	LastErrorCode string
	MissingFields []string
}

type Store interface {
	// Insert stores a pending record with revision max+1 for the order and an
	// einvoice.requested audit row, in one transaction.
	Insert(ctx context.Context, n NewRecord) (Record, error)
	Get(ctx context.Context, orderID string, revision int) (Record, error)
	// MarkIssued moves a pending record to issued and writes einvoice.issued in
	// the same transaction; any other state is ErrStateConflict.
	MarkIssued(ctx context.Context, orderID string, revision int, i Issued) error
	// ListDue returns pending records whose next_attempt_at is not after now, oldest first.
	ListDue(ctx context.Context, now time.Time, limit int) ([]Record, error)
	// GetLatest returns the highest revision of the order; ErrNotFound when it has none.
	GetLatest(ctx context.Context, orderID string) (Record, error)
	// RetryRecord moves a failed or needs_info record back to pending with its
	// attempts kept and next_attempt_at at now, and writes einvoice.retried in
	// the same transaction. Any other state is ErrStateConflict. The audit
	// detail holds no buyer value.
	RetryRecord(ctx context.Context, orderID string, revision int, by string, now time.Time) (Record, error)
	// ListPaidWithoutRecord returns paid, invoiceable orders paid at or after
	// startAt that have no record, oldest first.
	ListPaidWithoutRecord(ctx context.Context, startAt time.Time, limit int) ([]PaidOrder, error)
	// Claim leases due pending records for the lease duration and counts an
	// attempt on each. A leased record is skipped by other claims.
	Claim(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]Record, error)
	// RecordAttempt stores the outcome of an attempt and releases the lease.
	// A failed or needs_info outcome also writes its audit row in the same
	// transaction. A record that is not pending gives ErrStateConflict.
	RecordAttempt(ctx context.Context, orderID string, revision int, a Attempt) error
}
