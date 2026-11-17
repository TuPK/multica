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
	// Insert stores a pending record with revision max+1 for the order.
	Insert(ctx context.Context, n NewRecord) (Record, error)
	Get(ctx context.Context, orderID string, revision int) (Record, error)
	// MarkIssued moves a pending record to issued; any other state is ErrStateConflict.
	MarkIssued(ctx context.Context, orderID string, revision int, i Issued) error
	// ListDue returns pending records whose next_attempt_at is not after now, oldest first.
	ListDue(ctx context.Context, now time.Time, limit int) ([]Record, error)
	// ReplaceIssued cancels an issued record locally (no provider call) and
	// inserts the next revision as pending with the given buyer, in one
	// transaction. A record that is not issued gives ErrStateConflict.
	ReplaceIssued(ctx context.Context, orderID string, revision int, buyer []byte, by, reason string) (Record, error)
	// ListPaidWithoutRecord returns paid, invoiceable orders paid at or after
	// startAt that have no record, oldest first.
	ListPaidWithoutRecord(ctx context.Context, startAt time.Time, limit int) ([]PaidOrder, error)
	// Claim leases due pending records for the lease duration and counts an
	// attempt on each. A leased record is skipped by other claims.
	Claim(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]Record, error)
	// RecordAttempt stores the outcome of an attempt and releases the lease.
	// A record that is not pending gives ErrStateConflict.
	RecordAttempt(ctx context.Context, orderID string, revision int, a Attempt) error
}
