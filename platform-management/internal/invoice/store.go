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
}
