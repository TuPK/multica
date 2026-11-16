package billing

import (
	"encoding/json"
	"time"
)

// Kind is what an order buys. The database does not check it.
type Kind string

const (
	KindSubscribe  Kind = "subscribe"
	KindRenew      Kind = "renew"
	KindAddSeats   Kind = "add_seats"
	KindChangePlan Kind = "change_plan"
	// KindUsage is the invoice of a pay as you go plan (spec 7b.3, rule 11). It
	// has no seats and no expiry.
	KindUsage Kind = "usage"
)

// NoticeKindUsage80 is the billing_notice kind of the email sent when a
// workspace's uninvoiced usage reaches 80 percent of its credit limit (spec
// 7b.3, rule 12).
const NoticeKindUsage80 = "usage_80"

// Valid reports whether k is a kind this release sells.
func (k Kind) Valid() bool {
	switch k {
	case KindSubscribe, KindRenew, KindAddSeats, KindChangePlan, KindUsage:
		return true
	}
	return false
}

// ProviderManual is the bank-transfer provider confirmed by staff.
const ProviderManual = "manual"

// ReasonBilling is the platform_workspace reason of a workspace made read-only
// because its subscription is overdue (spec 5.9).
const ReasonBilling = "billing"

// Order is one row of billing_order. IDs are canonical UUID text. For
// add_seats, Seats is the number of seats added, not the new total. Amount is
// the price before VAT; the customer pays Amount + VATAmount.
type Order struct {
	ID             string
	WorkspaceID    string
	Kind           Kind
	PlanKey        string
	Interval       string
	Seats          int
	Amount         int64
	VATAmount      int64
	Currency       string
	Status         Status
	Provider       string
	ProviderRef    string
	TransferCode   string
	IdempotencyKey string
	Buyer          json.RawMessage
	PeriodStart    *time.Time
	PeriodEnd      *time.Time
	EInvoiceRef    string
	CreatedBy      string
	ConfirmedBy    string
	PaidAt         *time.Time
	// ExpiresAt is the zero time for an order that does not expire: a usage
	// invoice.
	ExpiresAt time.Time
	CreatedAt time.Time
}

// Total is what the customer pays: the amount plus VAT.
func (o Order) Total() int64 { return o.Amount + o.VATAmount }
