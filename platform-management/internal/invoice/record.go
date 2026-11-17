// Package invoice stores e-invoice issuance records, one row per
// (order_id, revision). The application never deletes a record.
package invoice

import "time"

type State string

const (
	StatePending   State = "pending"
	StateIssued    State = "issued"
	StateNeedsInfo State = "needs_info"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

// Record is one row of einvoice_issuance. Buyer holds buyer personal data and
// must never be logged or included in an error.
type Record struct {
	OrderID           string
	Revision          int
	WorkspaceID       string
	State             State
	Provider          string
	ProviderInvoiceID string
	InvoiceNumber     string
	LookupURL         string
	Currency          string
	Amount            int64
	VATAmount         int64
	Buyer             []byte
	MissingFields     []string
	Attempts          int
	NextAttemptAt     time.Time
	LockedUntil       *time.Time
	LastErrorCode     string
	IssuedAt          *time.Time
	CancelledAt       *time.Time
	CancelledBy       string
	CancelReason      string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// NewRecord is the input of Insert. The revision is assigned by the store.
type NewRecord struct {
	OrderID     string
	WorkspaceID string
	Provider    string
	Currency    string
	Amount      int64
	VATAmount   int64
	Buyer       []byte
}

// Issued is what the provider returned for a successful issue.
type Issued struct {
	ProviderInvoiceID string
	InvoiceNumber     string
	LookupURL         string
	IssuedAt          time.Time
}
