// Package paymentnotice stores bank transfer notices and their review state.
// A notice keeps the body's SHA-256 only, never the body itself.
package paymentnotice

import (
	"errors"
	"time"
)

type State string

const (
	StateReceived    State = "received"
	StateApplied     State = "applied"
	StateNeedsReview State = "needs_review"
	StateResolved    State = "resolved"
	StateDismissed   State = "dismissed"
)

var (
	ErrNotFound          = errors.New("paymentnotice: not found")
	ErrInvalidTransition = errors.New("paymentnotice: invalid state transition")
	ErrStateConflict     = errors.New("paymentnotice: notice is not in the expected state")
)

var transitions = map[State][]State{
	StateReceived:    {StateApplied, StateNeedsReview},
	StateNeedsReview: {StateResolved, StateDismissed},
}

// CanMoveTo reports whether the state matrix allows s -> to.
func (s State) CanMoveTo(to State) bool {
	for _, next := range transitions[s] {
		if next == to {
			return true
		}
	}
	return false
}

type Notice struct {
	ID              string
	Provider        string
	ProviderEventID string
	State           State
	ReviewReason    string
	TransferCode    string
	Amount          int64
	Currency        string
	OrderID         string
	BodySHA256      string
	ReceivedAt      time.Time
	DecidedAt       time.Time // zero when unset
	DecidedBy       string
	DecisionReason  string
}

type NewNotice struct {
	Provider        string
	ProviderEventID string
	TransferCode    string
	Amount          int64
	Currency        string
	BodySHA256      string
	ReceivedAt      time.Time // zero means database now()
}

type Change struct {
	ID             string
	From, To       State
	ReviewReason   string
	DecidedBy      string
	DecisionReason string
}
