package billing

import (
	"errors"
	"fmt"
	"time"
)

// Status is the state of an order.
type Status string

const (
	StatusPending  Status = "pending"
	StatusPaid     Status = "paid"
	StatusExpired  Status = "expired"
	StatusCanceled Status = "canceled"
	StatusRefunded Status = "refunded"
)

// OrderLifetime is how long a pending order can be confirmed.
const OrderLifetime = 7 * 24 * time.Hour

var (
	// ErrInvalidTransition is returned for a change the state machine does not allow.
	ErrInvalidTransition = errors.New("billing: invalid order status transition")
	// ErrOrderExpired is returned when a pending order is confirmed after expires_at.
	ErrOrderExpired = errors.New("billing: the order has expired")
	// ErrNotExpired is returned when a pending order is expired before expires_at.
	ErrNotExpired = errors.New("billing: the order has not expired")
)

// IsExpired reports whether expires_at has passed. At the instant itself the
// order can still be confirmed. A zero expiresAt is an order that never expires.
func IsExpired(expiresAt, now time.Time) bool { return !expiresAt.IsZero() && now.After(expiresAt) }

// Transition checks one status change: pending to paid, expired or canceled,
// and paid to refunded. Paying a pending order needs expiresAt not passed at
// now; expiring it needs expiresAt passed.
func Transition(from, to Status, expiresAt, now time.Time) error {
	switch {
	case from == StatusPending && to == StatusPaid:
		if IsExpired(expiresAt, now) {
			return ErrOrderExpired
		}
		return nil
	case from == StatusPending && to == StatusExpired:
		if !IsExpired(expiresAt, now) {
			return ErrNotExpired
		}
		return nil
	case from == StatusPending && to == StatusCanceled, from == StatusPaid && to == StatusRefunded:
		return nil
	}
	return fmt.Errorf("%w: %q to %q", ErrInvalidTransition, from, to)
}
