package subscription

import (
	"strings"
	"testing"
)

// AC32: GET /prices never returns the price of a usage plan (spec 7b.3, rule 15).
func TestPricesNeverReturnTheMeterPricesOfAUsagePlan(t *testing.T) {
	e, _ := usageEnv(t)

	// A workspace on the usage plan gets the cheapest public seat plan's prices.
	e.subs.rows[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "payg", Status: StatusActive, Interval: IntervalMonth, Version: 2}
	if month, year := unitAmounts(t, e.pricesFor("")); month != 149000 || year != 1490000 {
		t.Errorf("prices of a usage workspace = %v, %v; want pro's seat prices", month, year)
	}

	// Naming the usage plan, as a visitor or as its own workspace, has no price to give.
	for name, setup := range map[string]func(){
		"a free workspace": func() { delete(e.subs.rows, wsID) },
		"its own workspace": func() {
			e.subs.rows[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "payg", Status: StatusActive, Interval: IntervalMonth, Version: 2}
		},
	} {
		setup()
		rec := e.pricesFor("?plan=payg")
		if rec.Code != 503 || decodeMap(t, rec)["code"] != "prices_unavailable" {
			t.Errorf("%s: ?plan=payg = %d %s, want 503 prices_unavailable", name, rec.Code, rec.Body)
		}
	}

	// The public plan list carries no price for it either.
	rec := e.pricesFor("")
	list, _ := decodeMap(t, rec)["plans"].([]any)
	for _, p := range list {
		entry := obj(t, p, "plan")
		if entry["key"] != "payg" {
			continue
		}
		if entry["seat_price_month"] != nil || entry["seat_price_year"] != nil {
			t.Errorf("the usage plan is listed with a seat price: %v", entry)
		}
	}
	if strings.Contains(rec.Body.String(), "unit_price") || strings.Contains(rec.Body.String(), "usage_rates") {
		t.Errorf("the price body names a usage price: %s", rec.Body)
	}
}
