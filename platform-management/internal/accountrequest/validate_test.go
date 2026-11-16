package accountrequest

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

func goodProfile() *BillingProfile {
	return &BillingProfile{LegalName: "Acme Co", TaxCode: "0123456789", Address: "1 Main St", InvoiceEmail: "ap@acme.example"}
}

func goodIndividual() Input {
	return Input{Kind: KindIndividual, Email: "Ana@Example.COM", DisplayName: "Ana", WorkspaceName: "Ana's space"}
}

func goodBusiness() Input {
	return Input{Kind: KindBusiness, Email: "boss@acme.example", DisplayName: "Boss", WorkspaceName: "Acme", BillingProfile: goodProfile()}
}

func fieldsOf(t *testing.T, err *Error) []string {
	t.Helper()
	if err == nil {
		t.Fatal("no error, want account_request_invalid")
	}
	if err.Status != 400 || err.Code != "account_request_invalid" {
		t.Fatalf("error = %d %s, want 400 account_request_invalid", err.Status, err.Code)
	}
	return err.Fields
}

func TestValidateInputNormalizesAndDefaults(t *testing.T) {
	got, err := ValidateInput(goodIndividual())
	if err != nil {
		t.Fatalf("error: %+v", err)
	}
	if got.Email != "ana@example.com" {
		t.Errorf("email = %q, want it lower-cased", got.Email)
	}
	if got.PlanKey != "free" || got.Interval != "month" {
		t.Errorf("plan %q interval %q, want the defaults free and month", got.PlanKey, got.Interval)
	}
	b, err := ValidateInput(goodBusiness())
	if err != nil {
		t.Fatalf("business: %+v", err)
	}
	if b.PlanKey != "business" {
		t.Errorf("business plan = %q, want business", b.PlanKey)
	}
}

func TestValidateInputRefusesWithFields(t *testing.T) {
	long := strings.Repeat("x", 101)
	cases := []struct {
		name   string
		mutate func(*Input)
		base   func() Input
		want   []string
	}{
		{"missing kind", func(in *Input) { in.Kind = "" }, goodIndividual, []string{"kind"}},
		{"unknown kind", func(in *Input) { in.Kind = "team" }, goodIndividual, []string{"kind"}},
		{"email empty", func(in *Input) { in.Email = "" }, goodIndividual, []string{"email"}},
		{"email without at", func(in *Input) { in.Email = "ana.example.com" }, goodIndividual, []string{"email"}},
		{"email with display name", func(in *Input) { in.Email = "Ana <ana@example.com>" }, goodIndividual, []string{"email"}},
		{"email without dot in domain", func(in *Input) { in.Email = "ana@localhost" }, goodIndividual, []string{"email"}},
		{"email over 254", func(in *Input) { in.Email = strings.Repeat("a", 250) + "@example.com" }, goodIndividual, []string{"email"}},
		{"display name empty", func(in *Input) { in.DisplayName = "  " }, goodIndividual, []string{"display_name"}},
		{"display name over 100", func(in *Input) { in.DisplayName = long }, goodIndividual, []string{"display_name"}},
		{"workspace name empty", func(in *Input) { in.WorkspaceName = "" }, goodIndividual, []string{"workspace_name"}},
		{"workspace name over 100", func(in *Input) { in.WorkspaceName = long }, goodIndividual, []string{"workspace_name"}},
		{"slug with a space", func(in *Input) { in.WorkspaceSlug = "bad slug" }, goodIndividual, []string{"workspace_slug"}},
		{"slug in capitals", func(in *Input) { in.WorkspaceSlug = "Acme" }, goodIndividual, []string{"workspace_slug"}},
		{"interval unknown", func(in *Input) { in.Interval = "week" }, goodIndividual, []string{"interval"}},
		{"seats negative", func(in *Input) { in.Seats = -1 }, goodIndividual, []string{"seats"}},
		{"business without profile", func(in *Input) { in.BillingProfile = nil }, goodBusiness, []string{"billing_profile"}},
		{"business profile missing fields", func(in *Input) { in.BillingProfile = &BillingProfile{} }, goodBusiness,
			[]string{"billing_profile.legal_name", "billing_profile.tax_code", "billing_profile.address", "billing_profile.invoice_email"}},
		{"business tax code format", func(in *Input) { in.BillingProfile.TaxCode = "12345" }, goodBusiness, []string{"billing_profile.tax_code"}},
		{"business invoice email", func(in *Input) { in.BillingProfile.InvoiceEmail = "nope" }, goodBusiness, []string{"billing_profile.invoice_email"}},
		{"individual with a profile", func(in *Input) { in.BillingProfile = goodProfile() }, goodIndividual, []string{"billing_profile"}},
		{"two problems", func(in *Input) { in.Kind = ""; in.Email = "x" }, goodIndividual, []string{"kind", "email"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := c.base()
			c.mutate(&in)
			_, err := ValidateInput(in)
			if got := fieldsOf(t, err); !reflect.DeepEqual(got, c.want) {
				t.Errorf("fields = %v, want %v", got, c.want)
			}
		})
	}
}

func TestValidateInputAcceptsTheLimits(t *testing.T) {
	in := goodIndividual()
	in.Email = strings.Repeat("a", 64) + "@" + strings.Repeat("b", 185) + ".com"
	if len(in.Email) != 254 {
		t.Fatalf("test email is %d bytes", len(in.Email))
	}
	in.DisplayName = strings.Repeat("é", 100)
	in.WorkspaceName = strings.Repeat("w", 100)
	in.WorkspaceSlug = "acme-2"
	if _, err := ValidateInput(in); err != nil {
		t.Errorf("a value at the limit is refused: %+v", err)
	}
	b := goodBusiness()
	b.BillingProfile.TaxCode = "0123456789-001"
	if _, err := ValidateInput(b); err != nil {
		t.Errorf("a 13 digit tax code with a hyphen is refused: %+v", err)
	}
}

// --- rule 6: the plan

func i(n int) *int { return &n }

func plans() map[string]plan.Plan {
	archived := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return map[string]plan.Plan{
		"free":     {Key: "free", Audience: plan.AudienceAny, BillingModel: plan.BillingSeat},
		"pro":      {Key: "pro", Audience: plan.AudienceAny, BillingModel: plan.BillingSeat, MinSeats: i(1), TrialDays: 14},
		"business": {Key: "business", Audience: plan.AudienceBusiness, BillingModel: plan.BillingSeat, MinSeats: i(5), TrialDays: 14},
		"solo":     {Key: "solo", Audience: plan.AudienceIndividual, BillingModel: plan.BillingSeat, MinSeats: i(1), TrialDays: 0},
		"metered":  {Key: "metered", Audience: plan.AudienceAny, BillingModel: plan.BillingUsage},
		"old":      {Key: "old", Audience: plan.AudienceAny, BillingModel: plan.BillingSeat, ArchivedAt: &archived},
	}
}

func TestCheckPlanTable(t *testing.T) {
	ind := func(mod func(*Input)) Input {
		in := goodIndividual()
		in.PlanKey = "free"
		in.Interval = "month"
		mod(&in)
		return in
	}
	biz := func(mod func(*Input)) Input {
		in := goodBusiness()
		in.PlanKey = "business"
		in.Interval = "month"
		mod(&in)
		return in
	}
	cases := []struct {
		name       string
		in         Input
		plan       string
		wantStatus int
		wantCode   string
		wantFields []string
	}{
		{"free plan, no activation", ind(func(*Input) {}), "free", 0, "", nil},
		{"audience mismatch: individual on a business plan", ind(func(in *Input) { in.PlanKey = "business" }), "business", 409, "plan_audience_mismatch", nil},
		{"audience mismatch: business on an individual plan", biz(func(in *Input) { in.PlanKey = "solo" }), "solo", 409, "plan_audience_mismatch", nil},
		{"archived plan", ind(func(in *Input) { in.PlanKey = "old" }), "old", 400, "account_request_invalid", []string{"plan_key"}},
		{"seats below min_seats", biz(func(in *Input) { in.Seats = 4; in.Trial = true }), "business", 400, "account_request_invalid", []string{"seats"}},
		{"seats at min_seats with trial", biz(func(in *Input) { in.Seats = 5; in.Trial = true }), "business", 0, "", nil},
		{"individual over the seat cap", ind(func(in *Input) { in.PlanKey = "pro"; in.Seats = 6; in.Trial = true }), "pro", 400, "account_request_invalid", []string{"seats"}},
		{"individual at the seat cap", ind(func(in *Input) { in.PlanKey = "pro"; in.Seats = 5; in.Trial = true }), "pro", 0, "", nil},
		{"paid plan without trial or comp_reason", ind(func(in *Input) { in.PlanKey = "pro" }), "pro", 400, "account_activation_required", nil},
		{"paid plan with comp_reason", ind(func(in *Input) { in.PlanKey = "pro"; in.CompReason = "signed contract 42" }), "pro", 0, "", nil},
		{"paid plan with both", ind(func(in *Input) { in.PlanKey = "pro"; in.Trial = true; in.CompReason = "x" }), "pro", 400, "account_request_invalid", []string{"trial", "comp_reason"}},
		{"trial on a plan with no trial_days", ind(func(in *Input) { in.PlanKey = "solo"; in.Trial = true }), "solo", 400, "account_request_invalid", []string{"trial"}},
		{"trial on the free plan", ind(func(in *Input) { in.Trial = true }), "free", 400, "account_request_invalid", []string{"trial"}},
		{"comp_reason on the free plan", ind(func(in *Input) { in.CompReason = "x" }), "free", 400, "account_request_invalid", []string{"comp_reason"}},
		{"usage plan needs neither", ind(func(in *Input) { in.PlanKey = "metered" }), "metered", 0, "", nil},
		{"usage plan with trial", ind(func(in *Input) { in.PlanKey = "metered"; in.Trial = true }), "metered", 400, "account_request_invalid", []string{"trial"}},
		{"usage plan with comp_reason", ind(func(in *Input) { in.PlanKey = "metered"; in.CompReason = "x" }), "metered", 400, "account_request_invalid", []string{"comp_reason"}},
		{"comp_reason over 500 characters", ind(func(in *Input) { in.PlanKey = "pro"; in.CompReason = strings.Repeat("x", 501) }), "pro", 400, "account_request_invalid", []string{"comp_reason"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := CheckPlan(c.in, plans()[c.plan], 5)
			if c.wantStatus == 0 {
				if err != nil {
					t.Fatalf("error: %+v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("no error, want %d %s", c.wantStatus, c.wantCode)
			}
			if err.Status != c.wantStatus || err.Code != c.wantCode {
				t.Fatalf("error = %d %s, want %d %s", err.Status, err.Code, c.wantStatus, c.wantCode)
			}
			if c.wantFields != nil && !reflect.DeepEqual(err.Fields, c.wantFields) {
				t.Errorf("fields = %v, want %v", err.Fields, c.wantFields)
			}
		})
	}
}

func TestCheckPlanDefaultsSeatsToTheMinimum(t *testing.T) {
	in := goodBusiness()
	in.PlanKey, in.Interval, in.Trial = "business", "month", true
	got, err := CheckPlan(in, plans()["business"], 5)
	if err != nil {
		t.Fatal(err)
	}
	if got.Seats != 5 {
		t.Errorf("seats = %d, want the plan's min_seats 5", got.Seats)
	}
	ind := goodIndividual()
	ind.PlanKey, ind.Interval = "free", "month"
	got, err = CheckPlan(ind, plans()["free"], 5)
	if err != nil {
		t.Fatal(err)
	}
	if got.Seats != 1 {
		t.Errorf("seats = %d, want 1 when the plan has no min_seats", got.Seats)
	}
}
