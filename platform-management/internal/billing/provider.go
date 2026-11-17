package billing

import (
	"errors"
	"net/http"
	"time"
)

// ErrUnsupported is returned by a provider method the provider does not have.
var ErrUnsupported = errors.New("billing: not supported by this payment provider")

// BankDetails is where a customer sends a manual transfer.
type BankDetails struct {
	AccountName   string
	AccountNumber string
	BankName      string
}

// Instructions tell the customer how to pay one order.
type Instructions struct {
	Provider          string
	BankAccountName   string
	BankAccountNumber string
	BankName          string
	// TransferCode goes in the transfer note.
	TransferCode string
	// Amount is the total to transfer, VAT included.
	Amount   int64
	Currency string
}

// Notification is a provider's report that an order was paid. The manual
// provider sends none; the R4 adapter fills it.
type Notification struct {
	TransferCode string
	ProviderRef  string
	Amount       int64
	// EventID is the provider's event id, as a decimal string.
	EventID string
	// Currency is the ISO code as the provider sent it.
	Currency string
	// OccurredAt is the transaction time the provider reported.
	OccurredAt time.Time
}

// PaymentProvider is the part of billing that differs between payment
// channels. A provider replaces only how a payment is confirmed; the order and
// ApplyPayment stay the same (spec 5.8).
type PaymentProvider interface {
	// CreatePaymentRequest returns what the customer needs to pay the order.
	CreatePaymentRequest(o Order) (Instructions, error)
	// ParseNotification verifies and reads a notification from the provider.
	ParseNotification(body []byte, headers http.Header) (Notification, error)
	// QueryStatus asks the provider for the order's status.
	QueryStatus(o Order) (Status, error)
}

// Manual is the bank-transfer provider: staff match the transfer and confirm.
type Manual struct{ bank BankDetails }

// NewManualProvider returns the manual provider for the given bank account.
func NewManualProvider(bank BankDetails) *Manual { return &Manual{bank: bank} }

// CreatePaymentRequest returns the bank details and the order's transfer code.
func (m *Manual) CreatePaymentRequest(o Order) (Instructions, error) {
	if !ValidTransferCode(o.TransferCode) {
		return Instructions{}, errors.New("billing: the order has no valid transfer code")
	}
	return Instructions{
		Provider:          ProviderManual,
		BankAccountName:   m.bank.AccountName,
		BankAccountNumber: m.bank.AccountNumber,
		BankName:          m.bank.BankName,
		TransferCode:      o.TransferCode,
		Amount:            o.Total(),
		Currency:          o.Currency,
	}, nil
}

// ParseNotification is ErrUnsupported: a bank transfer sends no notification.
func (*Manual) ParseNotification([]byte, http.Header) (Notification, error) {
	return Notification{}, ErrUnsupported
}

// QueryStatus is ErrUnsupported: there is no provider to ask.
func (*Manual) QueryStatus(Order) (Status, error) { return "", ErrUnsupported }
