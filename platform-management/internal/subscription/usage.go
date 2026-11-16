package subscription

import (
	"context"
	"time"
)

// UsageProfileReady is the profile check of a pay as you go activation (spec
// 7b.3, rule 6). A business account needs the four fields of spec 7b.1 rule 3;
// an individual account needs the payer's name and an invoice email. It returns
// the 409 billing_profile_incomplete refusal that names the missing fields.
func UsageProfileReady(p BillingProfile) *Error {
	if p.AccountKind == AccountBusiness {
		if missing := missingFields(fieldsOf(p)); len(missing) > 0 {
			return incomplete(missing)
		}
		return nil
	}
	var missing []string
	if p.LegalName == "" {
		missing = append(missing, fieldLegalName)
	}
	if p.InvoiceEmail == "" {
		missing = append(missing, fieldInvoiceEmail)
	}
	if len(missing) > 0 {
		return incomplete(missing)
	}
	return nil
}

// UsagePeriod returns the current period of a usage subscription: the UTC
// calendar month that holds now, as the half-open interval [start, end) (spec
// 7b.3, rule 6; D28).
func UsagePeriod(now time.Time) (start, end time.Time) {
	u := now.UTC()
	start = time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0)
}

// UsageMeter is one meter of the customer's usage block (spec 7b.3, rule 16).
type UsageMeter struct {
	Key         string `json:"key"`
	Quantity    int64  `json:"quantity"`
	UnitPrice   int64  `json:"unit_price"`
	Per         int64  `json:"per"`
	DiscountPct int64  `json:"discount_pct"`
	Amount      int64  `json:"amount"`
}

// Usage is the usage block of a usage plan's summary and of GET /usage.
type Usage struct {
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	Currency    string `json:"currency"`
	// AccruedAmount is the net usage not yet on an invoice, all time.
	AccruedAmount int64 `json:"accrued_amount"`
	// CreditLimit is null when the plan sets none for the account kind.
	CreditLimit *int64 `json:"credit_limit"`
	// UnpaidOrderID is the oldest unpaid usage invoice, or null.
	UnpaidOrderID *string      `json:"unpaid_order_id"`
	Meters        []UsageMeter `json:"meters"`
}

// UsageReader reads the usage block of a workspace on a usage plan. *usage.View
// is one.
type UsageReader interface {
	Usage(ctx context.Context, workspaceID string, now time.Time) (Usage, error)
}
