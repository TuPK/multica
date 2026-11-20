package subscription

import (
	"strings"
	"testing"
)

// Lowering a limit of a plan that has subscribers needs confirm_affects_subscribers
// (spec 3b.1 rule 3, D88, AC111). Raising a limit, or lowering one on a plan with
// no subscription, does not.
func TestLimitLoweringNeedsConfirmWhenSubscribed(t *testing.T) {
	cases := []struct {
		name, limits string
		lowered      bool
	}{
		{"lower a limit", `{"issue_count":500,"autopilot_runs":null}`, true},
		{"lower from unlimited", `{"issue_count":1000,"autopilot_runs":5}`, true},
		{"raise a limit", `{"issue_count":2000,"autopilot_runs":null}`, false},
		{"lift a limit to unlimited", `{"issue_count":null,"autopilot_runs":null}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newConsoleEnv(t)
			seedTeamPlan(t, e)
			e.plans.subs["team"] = 3
			body := strings.Replace(teamBody, `"limits":{"issue_count":1000,"autopilot_runs":null}`, `"limits":`+c.limits, 1)
			before := e.plans.rows["team"]
			auditsBefore := len(e.plans.audits)

			rec := e.do(adminUser, "PUT", "/api/platform/plans/team", body)
			if !c.lowered {
				wantStatus(t, rec, 200)
				return
			}
			wantCode(t, rec, 409, "plan_limit_change_confirm_required")
			if got := decodeMap(t, rec)["affected_subscriptions"]; got != float64(3) {
				t.Fatalf("affected_subscriptions = %v, want 3", got)
			}
			if e.plans.rows["team"].Limits["issue_count"] == nil || *e.plans.rows["team"].Limits["issue_count"] != *before.Limits["issue_count"] {
				t.Fatalf("a refused change stored limits %v", e.plans.rows["team"].Limits)
			}
			if len(e.plans.audits) != auditsBefore {
				t.Error("a refused change wrote an audit row")
			}

			confirmed := strings.Replace(body, `"archived":false`, `"archived":false,"confirm_affects_subscribers":true`, 1)
			wantStatus(t, e.do(adminUser, "PUT", "/api/platform/plans/team", confirmed), 200)
		})
	}
}

func TestLimitLoweringWithoutSubscribersNeedsNoConfirm(t *testing.T) {
	e := newConsoleEnv(t)
	seedTeamPlan(t, e)
	body := strings.Replace(teamBody, `"issue_count":1000`, `"issue_count":500`, 1)
	wantStatus(t, e.do(adminUser, "PUT", "/api/platform/plans/team", body), 200)
}
