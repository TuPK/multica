package plan

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// specPath is the spec the catalogue must match (section 5.5). The test reads
// the table from the file, so the two cannot drift apart unnoticed.
const specPath = "../../../docs/engineering/platform-management-spec.md"

// gigabyte is the size of the spec's "1 GB". Assumption, labelled: storage
// limits are binary gigabytes.
const gigabyte = int64(1) << 30

const contractCell = "theo hợp đồng"

// specTable returns the cells of each row of the plan table in spec 5.5, keyed
// by the row's first cell. Column 0 of each slice is the first plan.
func specTable(t *testing.T) (header []string, rows map[string][]string) {
	t.Helper()
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read the spec: %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "| | Free | Pro | Business | Enterprise |") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatal("the plan table of spec 5.5 was not found")
	}
	cells := func(line string) []string {
		parts := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}
	header = cells(lines[start])[1:]
	rows = map[string][]string{}
	for _, l := range lines[start+2:] { // skip the separator row
		if !strings.HasPrefix(l, "|") {
			break
		}
		c := cells(l)
		rows[c[0]] = c[1:]
	}
	return header, rows
}

func yes(t *testing.T, cell string) bool {
	t.Helper()
	switch cell {
	case "có":
		return true
	case "không":
		return false
	}
	t.Fatalf("cell %q is neither có nor không", cell)
	return false
}

func money(t *testing.T, cell string) *int64 {
	t.Helper()
	if cell == contractCell {
		return nil
	}
	n, err := strconv.ParseInt(strings.ReplaceAll(cell, ".", ""), 10, 64)
	if err != nil {
		t.Fatalf("price cell %q: %v", cell, err)
	}
	return &n
}

var (
	fixedSeats = regexp.MustCompile(`^cố định (\d+)$`)
	minSeats   = regexp.MustCompile(`^mua, tối thiểu (\d+)$`)
	trialDays  = regexp.MustCompile(`^(\d+) ngày$`)
	sizeGB     = regexp.MustCompile(`^(\d+) GB$`)
)

// limitCell reads a limit cell. The second result is true for "theo hợp đồng":
// Enterprise is Business plus per-workspace overrides, so it takes Business's.
func limitCell(t *testing.T, cell string) (limit *int64, fromBusiness bool) {
	t.Helper()
	switch {
	case cell == "không giới hạn":
		return nil, false
	case cell == contractCell:
		return nil, true
	}
	if m := sizeGB.FindStringSubmatch(cell); m != nil {
		n, _ := strconv.ParseInt(m[1], 10, 64)
		n *= gigabyte
		return &n, false
	}
	n, err := strconv.ParseInt(strings.ReplaceAll(cell, ".", ""), 10, 64)
	if err != nil {
		t.Fatalf("limit cell %q: %v", cell, err)
	}
	return &n, false
}

func intPtr(n int) *int { return &n }

// planFromSpec builds the plan in column col of the spec table.
func planFromSpec(t *testing.T, header []string, rows map[string][]string, col int) Plan {
	t.Helper()
	cell := func(label string) string {
		r, ok := rows[label]
		if !ok {
			t.Fatalf("spec table has no row %q", label)
		}
		return r[col]
	}
	p := Plan{
		Key:            strings.ToLower(header[col]),
		Name:           header[col],
		Currency:       "vnd",
		IsPublic:       yes(t, cell("Công khai")),
		SeatPriceMonth: money(t, cell("Đơn giá ghế mỗi tháng (VND)")),
		SeatPriceYear:  money(t, cell("Đơn giá ghế mỗi năm (VND)")),
		Limits:         Limits{},
		Features:       map[string]bool{},
	}
	switch seats := cell("Ghế"); {
	case fixedSeats.MatchString(seats):
		n, _ := strconv.Atoi(fixedSeats.FindStringSubmatch(seats)[1])
		p.SeatsFixed = intPtr(n)
	case minSeats.MatchString(seats):
		n, _ := strconv.Atoi(minSeats.FindStringSubmatch(seats)[1])
		p.MinSeats = intPtr(n)
	case seats == contractCell:
	default:
		t.Fatalf("seats cell %q", seats)
	}
	if trial := cell("Dùng thử"); trial != "không" {
		m := trialDays.FindStringSubmatch(trial)
		if m == nil {
			t.Fatalf("trial cell %q", trial)
		}
		p.TrialDays, _ = strconv.Atoi(m[1])
	}
	for gate, label := range map[string]string{
		"issue_count":          "`issue_count`",
		"autopilot_runs":       "`autopilot_runs` mỗi tháng",
		"agents":               "`agents`",
		"runtimes":             "`runtimes`",
		"storage_bytes":        "`storage_bytes`",
		"api_rate_per_minute":  "`api_rate_per_minute`",
		"audit_retention_days": "`audit_retention_days`",
	} {
		limit, _ := limitCell(t, cell(label))
		p.Limits[gate] = limit
	}
	p.Features["audit_export"] = yes(t, cell("`audit_export`"))
	for _, f := range []string{"sso", "workspace_policies", "viewer_role"} {
		p.Features[f] = yes(t, cell("`sso`, `workspace_policies`, `viewer_role`"))
	}
	return p
}

// The embedded file carries exactly the table of spec 5.5 (D24).
func TestEmbeddedCatalogueMatchesTheSpecTable(t *testing.T) {
	header, rows := specTable(t)
	got, err := Catalogue()
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	// The table of 5.5 holds four plans; the catalogue adds the plans of 7b.3.
	if len(got) < len(header) {
		t.Fatalf("catalogue has %d plans, the spec table has %d", len(got), len(header))
	}

	business := -1
	for i, h := range header {
		if h == "Business" {
			business = i
		}
	}
	for col := range header {
		want := planFromSpec(t, header, rows, col)
		// A contract limit is the Business limit; the staff override sits on
		// the workspace.
		for gate, label := range map[string]string{
			"issue_count": "`issue_count`", "autopilot_runs": "`autopilot_runs` mỗi tháng",
			"agents": "`agents`", "runtimes": "`runtimes`", "storage_bytes": "`storage_bytes`",
			"api_rate_per_minute": "`api_rate_per_minute`", "audit_retention_days": "`audit_retention_days`",
		} {
			if _, fromBusiness := limitCell(t, rows[label][col]); fromBusiness {
				want.Limits[gate] = planFromSpec(t, header, rows, business).Limits[gate]
			}
		}

		var have *Plan
		for i := range got {
			if got[i].Key == want.Key {
				have = &got[i]
			}
		}
		if have == nil {
			t.Errorf("catalogue has no plan %q", want.Key)
			continue
		}
		// Sort and archived_at are not in the table, nor are the R5 fields
		// (TestCatalogueCarriesTheR5Fields owns those).
		cmp := *have
		cmp.Sort, cmp.ArchivedAt = 0, nil
		cmp.Audience, cmp.BillingModel = "", ""
		cmp.Limits = Limits{}
		for gate, v := range have.Limits {
			if gate != GateMemberGroups && gate != GateMaxWorkspaces {
				cmp.Limits[gate] = v
			}
		}
		if !reflect.DeepEqual(cmp, want) {
			gotJSON, _ := json.Marshal(cmp)
			wantJSON, _ := json.Marshal(want)
			t.Errorf("plan %s differs from the spec table\n got: %s\nwant: %s", want.Key, gotJSON, wantJSON)
		}
	}
}

func TestCatalogueOrderFollowsTheSpecColumns(t *testing.T) {
	got, err := Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for i, p := range got {
		keys = append(keys, p.Key)
		if i > 0 && got[i-1].Sort >= p.Sort {
			t.Errorf("%s has sort %d, not after %s (%d)", p.Key, p.Sort, got[i-1].Key, got[i-1].Sort)
		}
	}
	// payg is part of the spec's initial set (7b.3, "Gói khởi tạo bổ sung").
	if want := []string{"free", "pro", "business", "enterprise", "unlimited", "payg"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
}

func byKey(t *testing.T, key string) Plan {
	t.Helper()
	all, err := Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		if p.Key == key {
			return p
		}
	}
	t.Fatalf("no plan %q", key)
	return Plan{}
}

func TestFreePlanHasThreeFixedSeats(t *testing.T) {
	free := byKey(t, "free")
	if free.SeatsFixed == nil {
		t.Fatal("free seats_fixed is unset, want 3")
	}
	if *free.SeatsFixed != 3 {
		t.Fatalf("free seats_fixed = %d, want 3", *free.SeatsFixed)
	}
	if free.MinSeats != nil {
		t.Errorf("free min_seats = %v, want none", *free.MinSeats)
	}
}

func TestPaidPricesAreWholeDong(t *testing.T) {
	for key, want := range map[string][2]int64{
		"pro":      {149_000, 1_490_000},
		"business": {349_000, 3_490_000},
	} {
		p := byKey(t, key)
		if p.Currency != "vnd" {
			t.Errorf("%s currency = %q, want vnd", key, p.Currency)
		}
		if p.SeatPriceMonth == nil || *p.SeatPriceMonth != want[0] || p.SeatPriceYear == nil || *p.SeatPriceYear != want[1] {
			prices, _ := json.Marshal([]*int64{p.SeatPriceMonth, p.SeatPriceYear})
			t.Errorf("%s prices = %s, want %d/%d", key, prices, want[0], want[1])
		}
	}

	// "No decimals" is a property of the file, not only of the Go type: a price
	// written 149000.0 would decode into an int64 in some decoders and not in
	// others.
	dec := json.NewDecoder(bytes.NewReader(catalogueJSON))
	dec.UseNumber()
	var raw struct {
		Plans []map[string]any `json:"plans"`
	}
	if err := dec.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	for _, p := range raw.Plans {
		for _, f := range []string{"seat_price_month", "seat_price_year"} {
			if n, ok := p[f].(json.Number); ok && strings.ContainsAny(n.String(), ".eE") {
				t.Errorf("%v %s = %s, want whole dong", p["key"], f, n)
			}
		}
	}
}

func TestEnterpriseIsPrivateAndTakesBusinessLimits(t *testing.T) {
	ent, biz := byKey(t, "enterprise"), byKey(t, "business")
	if ent.IsPublic {
		t.Error("enterprise must not be public")
	}
	// member_groups is the one limit that differs: Business caps it, Enterprise does not.
	entLimits, bizLimits := Limits{}, Limits{}
	// max_workspaces differs too: Business allows 10, Enterprise has no limit.
	for g, v := range ent.Limits {
		if g != GateMemberGroups && g != GateMaxWorkspaces {
			entLimits[g] = v
		}
	}
	for g, v := range biz.Limits {
		if g != GateMemberGroups && g != GateMaxWorkspaces {
			bizLimits[g] = v
		}
	}
	if !reflect.DeepEqual(entLimits, bizLimits) || !reflect.DeepEqual(ent.Features, biz.Features) {
		t.Errorf("enterprise limits/features = %v / %v, want business's %v / %v", entLimits, ent.Features, bizLimits, biz.Features)
	}
}

func TestCatalogueEntriesPassValidation(t *testing.T) {
	all, err := Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		if err := Validate(p); err != nil {
			t.Errorf("%s: %v", p.Key, err)
		}
	}
}

// The R5 fields of spec 7b.1 and plan step Q1: audience, billing_model and the
// member_groups limit (7b.7). Enterprise's member_groups is an explicit null:
// no cap.
func TestCatalogueCarriesTheR5Fields(t *testing.T) {
	groups := func(n int64) *int64 { return &n }
	for _, tc := range []struct {
		key      string
		audience string
		groups   *int64
	}{
		{"free", "any", groups(0)},
		{"pro", "any", groups(0)},
		{"business", "business", groups(50)},
		{"enterprise", "business", nil},
	} {
		p := byKey(t, tc.key)
		if p.Audience != tc.audience {
			t.Errorf("%s audience = %q, want %q", tc.key, p.Audience, tc.audience)
		}
		if p.BillingModel != "seat" {
			t.Errorf("%s billing_model = %q, want seat", tc.key, p.BillingModel)
		}
		got, present := p.Limits[GateMemberGroups]
		if !present {
			t.Errorf("%s has no member_groups limit; want an explicit entry", tc.key)
			continue
		}
		if (got == nil) != (tc.groups == nil) || (got != nil && *got != *tc.groups) {
			t.Errorf("%s member_groups = %v, want %v", tc.key, got, tc.groups)
		}
		if p.Version != 0 {
			t.Errorf("%s version = %d, want 0 (the store owns it)", tc.key, p.Version)
		}
	}
}

// The unlimited plan of spec 7b.3, cell by cell.
func TestCatalogueUnlimitedPlanMatchesSpec7b3(t *testing.T) {
	p := byKey(t, UnlimitedKey)
	n := func(v int64) *int64 { return &v }
	want := Plan{
		Key: "unlimited", Name: "Unlimited", Currency: "vnd",
		Limits: Limits{
			GateIssueCount: nil, GateAutopilotRuns: nil, GateAgents: nil, GateRuntimes: nil,
			GateStorageBytes: nil, GateMemberGroups: nil, GateMaxWorkspaces: nil,
			GateAPIRatePerMinute: nil, GateAuditRetentionDay: n(180), // the rate cap is off (contract 2)
		},
		Features:       map[string]bool{"audit_export": true, "sso": false, "workspace_policies": false, "viewer_role": false},
		SeatPriceMonth: n(249000), SeatPriceYear: n(2490000), MinSeats: intPtr(1),
		TrialDays: 14, IsPublic: true, Sort: p.Sort, Audience: AudienceAny, BillingModel: BillingSeat,
	}
	if !reflect.DeepEqual(p, want) {
		gotJSON, _ := json.Marshal(p)
		wantJSON, _ := json.Marshal(want)
		t.Errorf("unlimited differs from spec 7b.3\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
	if !IsUnlimited(p) {
		t.Error("the unlimited plan has a counted limit set")
	}
}

// resourceUnlimited reports whether p leaves every counted limit empty except
// max_workspaces, which spec R6 section 5.6 gives payg (5) as a count of
// workspaces, not a cap on what the meter bills.
func resourceUnlimited(p Plan) bool {
	for _, gate := range CountedGates {
		if gate != GateMaxWorkspaces && p.Limits[gate] != nil {
			return false
		}
	}
	return true
}

// The resource limits are all empty on exactly two plans: unlimited, and payg,
// whose usage is paid for by the meter instead of capped (spec 7b.3, D42).
// IsUnlimited, which also looks at max_workspaces, holds for unlimited alone.
func TestCatalogueOnlyUnlimitedAndPaygLeaveEveryCountedLimitEmpty(t *testing.T) {
	all, err := Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		if got, want := resourceUnlimited(p), p.Key == UnlimitedKey || p.Key == "payg"; got != want {
			t.Errorf("%s resourceUnlimited = %v, want %v", p.Key, got, want)
		}
		if got, want := IsUnlimited(p), p.Key == UnlimitedKey; got != want {
			t.Errorf("%s IsUnlimited = %v, want %v (payg carries max_workspaces 5)", p.Key, got, want)
		}
	}
}

// The pay-as-you-go plan of spec 7b.3 (AC32), cell by cell. Its usage prices are
// checked on the raw file in TestCataloguePaygUsagePricesMatchSpec7b3.
func TestCataloguePaygPlanMatchesSpec7b3(t *testing.T) {
	p := byKey(t, "payg")
	if p.Name != "Pay as you go" || p.Currency != "vnd" || !p.IsPublic {
		t.Errorf("payg name/currency/public = %q/%q/%v, want Pay as you go/vnd/true", p.Name, p.Currency, p.IsPublic)
	}
	if p.Audience != AudienceAny || p.BillingModel != BillingUsage {
		t.Errorf("payg audience/billing_model = %q/%q, want any/usage", p.Audience, p.BillingModel)
	}
	n := func(v int64) *int64 { return &v }
	wantLimits := Limits{
		GateIssueCount: nil, GateAutopilotRuns: nil, GateAgents: nil, GateRuntimes: nil,
		GateStorageBytes: nil, GateMemberGroups: nil, GateMaxWorkspaces: n(5),
		GateAPIRatePerMinute: n(1200), GateAuditRetentionDay: n(180),
	}
	if !reflect.DeepEqual(p.Limits, wantLimits) {
		gotJSON, _ := json.Marshal(p.Limits)
		wantJSON, _ := json.Marshal(wantLimits)
		t.Errorf("payg limits\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
	wantFeatures := map[string]bool{"audit_export": true, "sso": false, "workspace_policies": false, "viewer_role": false}
	if !reflect.DeepEqual(p.Features, wantFeatures) {
		t.Errorf("payg features = %v, want %v", p.Features, wantFeatures)
	}
	if !resourceUnlimited(p) {
		t.Error("payg has a counted resource limit set")
	}
}

// A usage plan is not bought by seats: no seat price, no fixed or minimum seats,
// no trial (spec 7b.3 rule 5).
func TestCataloguePaygIsNotASeatPlan(t *testing.T) {
	p := byKey(t, "payg")
	if p.BillingModel == BillingSeat {
		t.Error("payg billing_model is seat")
	}
	if p.SeatPriceMonth != nil || p.SeatPriceYear != nil {
		t.Errorf("payg has a seat price: %v / %v", p.SeatPriceMonth, p.SeatPriceYear)
	}
	if p.SeatsFixed != nil || p.MinSeats != nil {
		t.Errorf("payg has seats_fixed %v or min_seats %v, want neither", p.SeatsFixed, p.MinSeats)
	}
	if p.TrialDays != 0 {
		t.Errorf("payg trial_days = %d, want 0", p.TrialDays)
	}
}

// rawCatalogueEntry returns the catalogue file's object for key, untouched by
// the Plan type.
func rawCatalogueEntry(t *testing.T, key string) map[string]json.RawMessage {
	t.Helper()
	var file struct {
		Plans []map[string]json.RawMessage `json:"plans"`
	}
	if err := json.Unmarshal(catalogueJSON, &file); err != nil {
		t.Fatal(err)
	}
	for _, e := range file.Plans {
		if string(e["key"]) == strconv.Quote(key) {
			return e
		}
	}
	t.Fatalf("catalogue file has no plan %q", key)
	return nil
}

// usage_rates and usage_terms of payg, spec 7b.3: 30.000 per 1.000 tasks, 3.000
// per GiB-month, 49.000 per member-month; Business discount 20; credit limits
// 2.000.000 (individual) and 20.000.000 (business).
func TestCataloguePaygUsagePricesMatchSpec7b3(t *testing.T) {
	e := rawCatalogueEntry(t, "payg")
	type rate struct{ Per, Price int64 }
	var rates map[string]rate
	if err := json.Unmarshal(e["usage_rates"], &rates); err != nil {
		t.Fatalf("usage_rates: %v (%s)", err, e["usage_rates"])
	}
	wantRates := map[string]rate{
		"tasks_run":     {Per: 1000, Price: 30000},
		"storage_bytes": {Per: gigabyte, Price: 3000},
		"members":       {Per: 1, Price: 49000},
	}
	if !reflect.DeepEqual(rates, wantRates) {
		t.Errorf("payg usage_rates = %v, want %v", rates, wantRates)
	}
	var terms struct {
		DiscountPct map[string]int64 `json:"discount_pct"`
		CreditLimit map[string]int64 `json:"credit_limit"`
	}
	if err := json.Unmarshal(e["usage_terms"], &terms); err != nil {
		t.Fatalf("usage_terms: %v (%s)", err, e["usage_terms"])
	}
	if want := map[string]int64{"business": 20}; !reflect.DeepEqual(terms.DiscountPct, want) {
		t.Errorf("payg discount_pct = %v, want %v", terms.DiscountPct, want)
	}
	if want := map[string]int64{"individual": 2_000_000, "business": 20_000_000}; !reflect.DeepEqual(terms.CreditLimit, want) {
		t.Errorf("payg credit_limit = %v, want %v", terms.CreditLimit, want)
	}
	// Seat plans carry no usage prices.
	for _, key := range []string{"free", "pro", "business", "enterprise", "unlimited"} {
		if _, ok := rawCatalogueEntry(t, key)["usage_rates"]; ok {
			t.Errorf("%s has usage_rates, want none", key)
		}
	}
}
