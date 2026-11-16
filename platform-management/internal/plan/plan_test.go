package plan

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/db"
)

func i64(n int64) *int64 { return &n }

func TestEffectiveLimit(t *testing.T) {
	planLimits := Limits{"issue_count": i64(200), "autopilot_runs": nil}
	cases := []struct {
		name      string
		overrides Limits
		gate      string
		want      *int64
	}{
		{"plan limit", nil, "issue_count", i64(200)},
		{"plan unlimited", nil, "autopilot_runs", nil},
		{"absent from the plan is unlimited", nil, "agents", nil},
		{"override replaces the limit", Limits{"issue_count": i64(500)}, "issue_count", i64(500)},
		{"override gives a limit to an unlimited gate", Limits{"autopilot_runs": i64(9)}, "autopilot_runs", i64(9)},
		{"an empty override lifts the limit", Limits{"issue_count": nil}, "issue_count", nil},
		{"an override of another gate changes nothing", Limits{"agents": i64(1)}, "issue_count", i64(200)},
	}
	for _, tc := range cases {
		got := EffectiveLimit(planLimits, tc.overrides, tc.gate)
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A JSON null is an override that lifts the limit, so it must survive decoding
// as a present key and not be dropped.
func TestLimitsKeepAnExplicitNull(t *testing.T) {
	var l Limits
	if err := json.Unmarshal([]byte(`{"issue_count":null,"agents":4}`), &l); err != nil {
		t.Fatal(err)
	}
	if v, ok := l["issue_count"]; !ok || v != nil {
		t.Fatalf("issue_count = %v, present %v; want present and nil", v, ok)
	}
	if got := EffectiveLimit(Limits{"issue_count": i64(200)}, l, "issue_count"); got != nil {
		t.Fatalf("effective = %d, want unlimited", *got)
	}
	out, _ := json.Marshal(l)
	if !strings.Contains(string(out), `"issue_count":null`) {
		t.Fatalf("encoded %s loses the null", out)
	}
}

func valid() Plan {
	return Plan{
		Key: "team", Name: "Team", Currency: "vnd", Limits: Limits{"issue_count": i64(10)},
		Features: map[string]bool{"sso": true}, SeatPriceMonth: i64(1), TrialDays: 7,
	}
}

func TestValidate(t *testing.T) {
	if err := Validate(valid()); err != nil {
		t.Fatalf("valid plan: %v", err)
	}
	neg := int64(-1)
	zero, three := 0, 3
	bad := map[string]func(*Plan){
		"empty key":                 func(p *Plan) { p.Key = "" },
		"upper-case key":            func(p *Plan) { p.Key = "Team" },
		"key with a dash":           func(p *Plan) { p.Key = "a-b" },
		"key starting with a digit": func(p *Plan) { p.Key = "1team" },
		"key too long":              func(p *Plan) { p.Key = "a" + strings.Repeat("b", 32) },
		"blank name":                func(p *Plan) { p.Name = "  " },
		"NUL in name":               func(p *Plan) { p.Name = "a\x00b" },
		"long name":                 func(p *Plan) { p.Name = strings.Repeat("x", 101) },
		"currency in capitals":      func(p *Plan) { p.Currency = "VND" },
		"currency of two letters":   func(p *Plan) { p.Currency = "vn" },
		"negative price":            func(p *Plan) { p.SeatPriceMonth = &neg },
		"negative yearly price":     func(p *Plan) { p.SeatPriceYear = &neg },
		"min seats of zero":         func(p *Plan) { p.MinSeats = &zero },
		"fixed and minimum seats":   func(p *Plan) { p.SeatsFixed, p.MinSeats = &three, &three },
		"negative trial":            func(p *Plan) { p.TrialDays = -1 },
		"trial over a year":         func(p *Plan) { p.TrialDays = 366 },
		"unknown limit":             func(p *Plan) { p.Limits = Limits{"typo": i64(1)} },
		"negative limit":            func(p *Plan) { p.Limits = Limits{"agents": &neg} },
		"zero issue limit":          func(p *Plan) { p.Limits = Limits{"issue_count": i64(0)} },
		"zero autopilot limit":      func(p *Plan) { p.Limits = Limits{"autopilot_runs": i64(0)} },
		"unknown feature":           func(p *Plan) { p.Features = map[string]bool{"teleport": true} },
		"negative fixed seats":      func(p *Plan) { n := -1; p.SeatsFixed = &n },
		"absurd seat count":         func(p *Plan) { n := 1 << 30; p.SeatsFixed = &n },
		"unknown billing model":     func(p *Plan) { p.BillingModel = "metered" },
		"billing model in capitals": func(p *Plan) { p.BillingModel = "Seat" },
		"usage plan with a monthly seat price": func(p *Plan) {
			p.BillingModel, p.SeatPriceYear = BillingUsage, nil
		},
		"usage plan with a yearly seat price": func(p *Plan) {
			p.BillingModel, p.SeatPriceMonth, p.SeatPriceYear = BillingUsage, nil, i64(10)
		},
		"usage plan with a zero seat price": func(p *Plan) {
			p.BillingModel, p.SeatPriceMonth, p.SeatPriceYear = BillingUsage, i64(0), nil
		},
		"unknown audience":     func(p *Plan) { p.Audience = "enterprise" },
		"audience in capitals": func(p *Plan) { p.Audience = "Business" },
		"negative version":     func(p *Plan) { p.Version = -1 },
	}
	for name, mutate := range bad {
		p := valid()
		mutate(&p)
		if err := Validate(p); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	// The R5 fields: the three audiences, both billing models, and "unset",
	// which the store reads as the default.
	for _, audience := range []string{"", AudienceAny, AudienceIndividual, AudienceBusiness} {
		p := valid()
		p.Audience = audience
		if err := Validate(p); err != nil {
			t.Errorf("audience %q: %v", audience, err)
		}
	}
	for _, model := range []string{"", BillingSeat} {
		p := valid()
		p.BillingModel = model
		if err := Validate(p); err != nil {
			t.Errorf("billing model %q: %v", model, err)
		}
	}
	usage := valid()
	usage.BillingModel, usage.SeatPriceMonth, usage.SeatPriceYear = BillingUsage, nil, nil
	if err := Validate(usage); err != nil {
		t.Errorf("usage plan without seat prices: %v", err)
	}

	// Zero is fine for a gate the billing page does not show.
	p := valid()
	p.Limits = Limits{"agents": i64(0), "storage_bytes": i64(0), GateMemberGroups: i64(0)}
	p.SeatsFixed = &zero
	if err := Validate(p); err != nil {
		t.Errorf("zero agents and storage: %v", err)
	}
}

func TestValidateLimitsForOverrides(t *testing.T) {
	if err := ValidateLimits(Limits{"issue_count": nil, "agents": i64(7)}); err != nil {
		t.Errorf("lifting a limit with null: %v", err)
	}
	if err := ValidateLimits(Limits{"issue_count": i64(0)}); err == nil {
		t.Error("an override of 0 issues is not representable on the billing page")
	}
}

// The row conversion is lossless in both directions.
func TestRowConversionRoundTrips(t *testing.T) {
	archived := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	seats := 3
	in := Plan{
		Key: "team", Name: "Team", Currency: "vnd",
		Limits:         Limits{"issue_count": nil, "autopilot_runs": i64(50), "storage_bytes": i64(1 << 40)},
		Features:       map[string]bool{"sso": true, "audit_export": false},
		SeatPriceMonth: i64(149000), SeatPriceYear: nil, SeatsFixed: &seats, MinSeats: nil,
		TrialDays: 14, IsPublic: true, Sort: 20, ArchivedAt: &archived,
		Audience: AudienceBusiness, BillingModel: BillingSeat, Version: 4,
	}
	params, err := toInsertParams(in)
	if err != nil {
		t.Fatal(err)
	}
	row := db.Plan{
		Key: params.Key, Name: params.Name, Limits: params.Limits, Features: params.Features, Currency: params.Currency,
		SeatPriceMonth: params.SeatPriceMonth, SeatPriceYear: params.SeatPriceYear, SeatsFixed: params.SeatsFixed,
		MinSeats: params.MinSeats, TrialDays: params.TrialDays, IsPublic: params.IsPublic, Sort: params.Sort,
		ArchivedAt: params.ArchivedAt, Audience: params.Audience, BillingModel: params.BillingModel,
		Version: in.Version, // the store owns the version: it is never a write parameter
	}
	out, err := FromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip\n in: %+v\nout: %+v", in, out)
	}
}

func TestRowConversionDefaultsEmptyDocuments(t *testing.T) {
	p, err := FromRow(db.Plan{Key: "x", Limits: []byte(`{}`), Features: []byte(`{}`), SeatPriceMonth: pgtype.Int8{}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Limits == nil || p.Features == nil || p.SeatPriceMonth != nil || p.ArchivedAt != nil {
		t.Fatalf("plan = %+v", p)
	}
	if _, err := FromRow(db.Plan{Key: "x", Limits: []byte(`[1]`), Features: []byte(`{}`)}); err == nil {
		t.Error("a limits array was accepted")
	}
}
