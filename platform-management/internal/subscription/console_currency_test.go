package subscription

import (
	"strings"
	"testing"
)

func TestPutPlanInDisabledCurrencyIs400PriceInvalid(t *testing.T) {
	e := newConsoleEnv(t)
	body := strings.Replace(teamBody, `"vnd"`, `"usd"`, 1)
	rec := e.do(adminUser, "PUT", "/api/platform/plans/team", body)
	wantCode(t, rec, 400, "price_invalid")
	if !strings.Contains(rec.Body.String(), `"field":"currency"`) {
		t.Fatalf("body %s does not name field currency", rec.Body.String())
	}
	if _, ok := e.plans.rows["team"]; ok {
		t.Fatal("a plan in a disabled currency was stored")
	}
}
