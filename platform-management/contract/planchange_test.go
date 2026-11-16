package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The sample responses of the self-service subscription routes (spec 7b.1). Q4
// validates them with the zod schemas; here each must exist and carry the keys
// its handler writes.
func TestPlanChangeSamplesExistWithTheirKeys(t *testing.T) {
	samples := map[string][]string{
		"plan_change_preview.json": {"mode", "plan_key", "interval", "current_seats", "resulting_seats", "currency",
			"amount", "vat_amount", "effective_at", "expected_version", "quoted_at"},
		"plan_change_order.json": {"request_id", "kind", "status", "plan_key", "interval", "seats", "currency",
			"amount", "vat_amount", "expires_at"},
		"plan_change_scheduled.json":         {"mode", "plan_key", "interval", "seats", "effective_at", "version", "changed"},
		"plan_change_scheduled_cleared.json": {"pending_change", "version", "changed"},
		"cancel.json":                        {"cancel_at_period_end", "status", "current_period_end", "version", "changed"},
		"resume.json":                        {"cancel_at_period_end", "status", "current_period_end", "version", "changed"},
	}
	for name, want := range samples {
		raw, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("%s is not a JSON object: %v", name, err)
			continue
		}
		var got []string
		for k := range body {
			got = append(got, k)
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s keys = %v, want %v", name, got, want)
		}
	}
}
