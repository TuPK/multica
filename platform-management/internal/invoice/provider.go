package invoice

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

type Buyer struct {
	Name    string `json:"name"`
	TaxCode string `json:"tax_code"`
	Address string `json:"address"`
	Email   string `json:"email"`
}

type IssueRequest struct {
	ReferenceCode string
	Currency      string
	Amount        int64
	VATAmount     int64
	Buyer         Buyer
}

func ReferenceCode(orderID string, revision int) string {
	return orderID + "-" + strconv.Itoa(revision)
}

type Ticket struct {
	ReferenceCode string
	TrackingCode  string
}

type Invoice struct {
	ProviderInvoiceID string
	Number            string
	LookupURL         string
	Issued            bool
	IssuedAt          time.Time
}

type PollState string

const (
	PollPending PollState = "pending"
	PollSuccess PollState = "success"
	PollFailed  PollState = "failed"
)

type Poll struct {
	State   PollState
	Invoice Invoice
	Code    string
}

type Kind int

const (
	KindTransient Kind = iota + 1
	KindUnknownOutcome
	KindPermanent
	KindMissingFields
)

func (k Kind) String() string {
	switch k {
	case KindTransient:
		return "transient"
	case KindUnknownOutcome:
		return "unknown_outcome"
	case KindPermanent:
		return "permanent"
	case KindMissingFields:
		return "missing_fields"
	}
	return "unknown"
}

// ProviderError carries field names only, never buyer values.
type ProviderError struct {
	Kind   Kind
	Code   string
	Fields []string
}

func (e *ProviderError) Error() string {
	msg := "invoice provider: " + e.Code + " (" + e.Kind.String() + ")"
	if len(e.Fields) > 0 {
		msg += ": fields " + strings.Join(e.Fields, ",")
	}
	return msg
}

var ErrNoInvoice = errors.New("invoice provider: no invoice for reference")

type Provider interface {
	Name() string
	RequiredBuyerFields() []string
	Issue(ctx context.Context, req IssueRequest) (Ticket, error)
	Status(ctx context.Context, trackingCode string) (Poll, error)
	Lookup(ctx context.Context, referenceCode string) (Invoice, error)
}

func buyerField(b Buyer, name string) string {
	switch name {
	case "name":
		return b.Name
	case "tax_code":
		return b.TaxCode
	case "address":
		return b.Address
	case "email":
		return b.Email
	}
	return ""
}

func CheckBuyer(p Provider, b Buyer) []string {
	var missing []string
	for _, f := range p.RequiredBuyerFields() {
		if buyerField(b, f) == "" {
			missing = append(missing, f)
		}
	}
	return missing
}

type Submission struct {
	Ticket   Ticket
	Existing bool
	Invoice  Invoice
}

// After an unknown outcome the invoice may already exist at the provider, so
// the reference code is looked up before issuing again.
func Submit(ctx context.Context, p Provider, req IssueRequest, afterUnknown bool) (Submission, error) {
	if afterUnknown {
		found, err := p.Lookup(ctx, req.ReferenceCode)
		if err == nil {
			return Submission{Ticket: Ticket{ReferenceCode: req.ReferenceCode}, Existing: true, Invoice: found}, nil
		}
		if !errors.Is(err, ErrNoInvoice) {
			return Submission{}, err
		}
	}
	tk, err := p.Issue(ctx, req)
	if err != nil {
		return Submission{}, err
	}
	return Submission{Ticket: tk}, nil
}
