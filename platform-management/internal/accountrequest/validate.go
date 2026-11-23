package accountrequest

import (
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

const (
	maxEmailBytes  = 254
	maxNameChars   = 100
	maxSlugChars   = 63
	maxReasonChars = 500
	maxSeats       = 1_000_000
)

var (
	// A slug is lower-case words joined by single hyphens. Delegora decides at
	// step 2 whether it is taken or reserved.
	slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	// A tax code is 10 digits, or 10 digits and a 3 digit branch with an optional
	// hyphen between the two groups (spec 7b.1, rule 3). Only the format is checked.
	taxCodePattern = regexp.MustCompile(`^[0-9]{10}(-?[0-9]{3})?$`)
)

// validEmail reports whether s is one plain address: no display name, no
// comment, and a domain with a dot.
func validEmail(s string) bool {
	if s == "" || len(s) > maxEmailBytes {
		return false
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s {
		return false
	}
	at := strings.LastIndexByte(s, '@')
	return at > 0 && strings.Contains(s[at+1:], ".")
}

func badLength(s string, max int) bool {
	n := utf8.RuneCountInString(s)
	return n == 0 || n > max || strings.ContainsRune(s, 0)
}

// ValidateInput checks the input fields of spec 7b.5 rule 3 and returns the
// input cleaned: the email lower-cased, text trimmed, and the plan and interval
// defaulted. A refusal is a 400 account_request_invalid naming every bad field.
func ValidateInput(in Input) (Input, *Error) {
	out := in
	out.Kind = strings.TrimSpace(in.Kind)
	out.Email = strings.ToLower(strings.TrimSpace(in.Email))
	out.DisplayName = strings.TrimSpace(in.DisplayName)
	out.WorkspaceName = strings.TrimSpace(in.WorkspaceName)
	out.WorkspaceSlug = strings.TrimSpace(in.WorkspaceSlug)
	out.PlanKey = strings.TrimSpace(in.PlanKey)
	out.Interval = strings.TrimSpace(in.Interval)
	out.CompReason = strings.TrimSpace(in.CompReason)

	var bad []string
	flag := func(cond bool, field string) {
		if cond {
			bad = append(bad, field)
		}
	}
	kindOK := out.Kind == KindIndividual || out.Kind == KindBusiness
	flag(!kindOK, "kind")
	flag(!validEmail(out.Email), "email")
	flag(badLength(out.DisplayName, maxNameChars), "display_name")
	flag(badLength(out.WorkspaceName, maxNameChars), "workspace_name")
	flag(out.WorkspaceSlug != "" && (len(out.WorkspaceSlug) > maxSlugChars || !slugPattern.MatchString(out.WorkspaceSlug)), "workspace_slug")
	flag(out.PlanKey != "" && plan.ValidateKey(out.PlanKey) != nil, "plan_key")
	flag(out.Interval != "" && out.Interval != IntervalMonth && out.Interval != IntervalYear, "interval")
	flag(out.Seats < 0 || out.Seats > maxSeats, "seats")

	switch {
	case kindOK && out.Kind == KindBusiness:
		bad = append(bad, checkProfile(in.BillingProfile)...)
	case kindOK && in.BillingProfile != nil:
		bad = append(bad, "billing_profile")
	}
	if len(bad) > 0 {
		return Input{}, invalid("the request has invalid fields", bad...)
	}

	if in.BillingProfile != nil {
		p := trimProfile(*in.BillingProfile)
		out.BillingProfile = &p
	}
	if out.PlanKey == "" {
		out.PlanKey = plan.FreeKey
		if out.Kind == KindBusiness {
			out.PlanKey = "business"
		}
	}
	if out.Interval == "" {
		out.Interval = IntervalMonth
	}
	return out, nil
}

func trimProfile(p BillingProfile) BillingProfile {
	return BillingProfile{
		LegalName: strings.TrimSpace(p.LegalName), TaxCode: strings.TrimSpace(p.TaxCode),
		Address: strings.TrimSpace(p.Address), InvoiceEmail: strings.TrimSpace(p.InvoiceEmail),
	}
}

// checkProfile returns the bad fields of a business account's billing profile,
// the four fields of spec 7b.1 rule 3.
func checkProfile(in *BillingProfile) []string {
	if in == nil {
		return []string{"billing_profile"}
	}
	p := trimProfile(*in)
	var bad []string
	if p.LegalName == "" {
		bad = append(bad, "billing_profile.legal_name")
	}
	if !taxCodePattern.MatchString(p.TaxCode) {
		bad = append(bad, "billing_profile.tax_code")
	}
	if p.Address == "" {
		bad = append(bad, "billing_profile.address")
	}
	if !validEmail(strings.ToLower(p.InvoiceEmail)) {
		bad = append(bad, "billing_profile.invoice_email")
	}
	return bad
}

// CheckPlan applies spec 7b.5 rule 6 to the plan row p that in names: the plan
// is not archived, its audience fits the kind, the seats fit, and the
// activation matches the billing model. It returns in with the seats defaulted
// to the plan's min_seats, or 1. individualMaxSeats is PLATFORM_INDIVIDUAL_MAX_SEATS.
func CheckPlan(in Input, p plan.Plan, individualMaxSeats int) (Input, *Error) {
	if p.ArchivedAt != nil {
		return Input{}, invalid("the plan is archived", "plan_key")
	}
	if p.Audience != "" && p.Audience != plan.AudienceAny && p.Audience != in.Kind {
		return Input{}, &Error{Status: 409, Code: CodeAudienceMismatch, Message: "the plan is not sold to this kind of account"}
	}

	minSeats := 1
	if p.MinSeats != nil && *p.MinSeats > minSeats {
		minSeats = *p.MinSeats
	}
	if in.Seats == 0 {
		in.Seats = minSeats
	}
	if in.Seats < minSeats {
		return Input{}, invalid("seats are below the plan's minimum", "seats")
	}
	if in.Kind == KindIndividual && in.Seats > individualMaxSeats {
		return Input{}, invalid("seats are above the limit for an individual account", "seats")
	}

	if utf8.RuneCountInString(in.CompReason) > maxReasonChars {
		return Input{}, invalid("comp_reason is too long", "comp_reason")
	}
	if in.Trial && in.CompReason != "" {
		return Input{}, invalid("a request is either a trial or a free grant, not both", "trial", "comp_reason")
	}
	paidSeatPlan := p.BillingModel != plan.BillingUsage && p.Key != plan.FreeKey
	if !paidSeatPlan {
		var bad []string
		if in.Trial {
			bad = append(bad, "trial")
		}
		if in.CompReason != "" {
			bad = append(bad, "comp_reason")
		}
		if len(bad) > 0 {
			return Input{}, invalid("this plan is activated without a trial or a free grant", bad...)
		}
		return in, nil
	}
	if in.Trial && p.TrialDays <= 0 {
		return Input{}, invalid("the plan has no trial", "trial")
	}
	if !in.Trial && in.CompReason == "" {
		return Input{}, &Error{Status: 400, Code: CodeActivationNeeded, Message: "a paid plan needs a trial or a free grant with a reason"}
	}
	return in, nil
}
