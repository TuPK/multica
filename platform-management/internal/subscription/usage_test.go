package subscription

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

func TestUsageActivationNeedsTheProfileFieldsOfItsKind(t *testing.T) {
	full := BillingProfile{
		AccountKind: AccountBusiness, LegalName: "Acme", TaxCode: "0123456789",
		Address: "Hanoi", InvoiceEmail: "billing@example.test",
	}
	if e := UsageProfileReady(full); e != nil {
		t.Fatalf("a complete business profile is refused: %v", e)
	}

	// A business account needs a tax code and an address; an individual needs
	// only the payer's name and an invoice email (spec 7b.3, rule 6).
	individual := BillingProfile{AccountKind: AccountIndividual, LegalName: "Nguyen Van A", InvoiceEmail: "a@example.test"}
	if e := UsageProfileReady(individual); e != nil {
		t.Fatalf("a complete individual profile is refused: %v", e)
	}

	cases := []struct {
		name   string
		p      BillingProfile
		fields []string
	}{
		{"business without tax code or address", BillingProfile{AccountKind: AccountBusiness, LegalName: "Acme", InvoiceEmail: "b@example.test"}, []string{"tax_code", "address"}},
		{"individual without a payer name", BillingProfile{AccountKind: AccountIndividual, InvoiceEmail: "a@example.test"}, []string{"legal_name"}},
		{"individual without an invoice email", BillingProfile{AccountKind: AccountIndividual, LegalName: "A"}, []string{"invoice_email"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := UsageProfileReady(c.p)
			if e == nil {
				t.Fatal("an incomplete profile is accepted")
			}
			if e.Status != http.StatusConflict || e.Code != codeProfileIncomplete {
				t.Fatalf("refusal = %d %s, want 409 billing_profile_incomplete", e.Status, e.Code)
			}
			if got := e.Detail["fields"]; !reflect.DeepEqual(got, c.fields) {
				t.Fatalf("fields = %v, want %v", got, c.fields)
			}
		})
	}
}

func TestUsagePeriodIsTheUTCCalendarMonth(t *testing.T) {
	cases := []struct {
		name      string
		now       time.Time
		wantStart time.Time
		wantEnd   time.Time
	}{
		{"mid month", time.Date(2026, 11, 16, 9, 30, 0, 0, time.UTC),
			time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)},
		{"last second of the month", time.Date(2026, 11, 30, 23, 59, 59, 0, time.UTC),
			time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)},
		{"first instant of the next month", time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"a local time that is still the previous UTC day", time.Date(2026, 12, 1, 3, 0, 0, 0, time.FixedZone("ICT", 7*3600)),
			time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end := UsagePeriod(c.now)
			if !start.Equal(c.wantStart) || !end.Equal(c.wantEnd) {
				t.Fatalf("period = [%s, %s), want [%s, %s)", start, end, c.wantStart, c.wantEnd)
			}
		})
	}
}

// A usage plan has no seats: summary carries no seat_capacity, so invitations
// are not checked against a seat count (spec 7b.3, rule 5).
func TestUsagePlanSummaryHasNoSeatCapacity(t *testing.T) {
	payg := plan.Plan{Key: "payg", Name: "Pay as you go", BillingModel: plan.BillingUsage, Audience: plan.AudienceAny}
	start, end := UsagePeriod(time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC))
	sub := Subscription{
		WorkspaceID: "11111111-1111-4111-8111-111111111111", PlanKey: "payg", Status: StatusActive,
		Seats: 0, Interval: IntervalMonth, CurrentPeriodStart: &start, CurrentPeriodEnd: &end,
		LimitOverrides: plan.Limits{},
	}
	got := buildSummary(sub.WorkspaceID, &sub, payg, 7, 2, nil, time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC))
	if got.SeatCapacity != nil {
		t.Fatalf("seat_capacity = %+v, want null on a usage plan", got.SeatCapacity)
	}
}
