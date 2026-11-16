package subscription

import "time"

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
