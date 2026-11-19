package quota

import (
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// Spec R6 section 3c.5 rules 2 and 3: the input rules, without a database.

func asQuotaError(t *testing.T, err *Error, code string) {
	t.Helper()
	if err == nil || err.Code != code {
		t.Fatalf("error = %+v, want %s", err, code)
	}
}

func TestRequestedMustBeAboveCurrentAndGateMustHaveALimit(t *testing.T) {
	if e := checkRequested(plan.GateMaxWorkspaces, 3, false, 5); e != nil {
		t.Fatalf("raising 3 to 5 must be allowed: %+v", e)
	}
	asQuotaError(t, checkRequested(plan.GateMaxWorkspaces, 3, false, 3), CodeInvalid)
	asQuotaError(t, checkRequested(plan.GateMaxWorkspaces, 0, true, 9), CodeInvalid)
}

func TestApprovalMayBeLowerThanRequestedButNotHigher(t *testing.T) {
	if e := checkApproved(3, 10, 6); e != nil {
		t.Fatalf("approving 6 for 10 must be allowed: %+v", e)
	}
	asQuotaError(t, checkApproved(3, 10, 11), CodeInvalid)
	asQuotaError(t, checkApproved(3, 10, 3), CodeInvalid)
}

func TestGateAndReasonChecks(t *testing.T) {
	asQuotaError(t, checkGate("not_a_gate"), CodeInvalid)
	if e := checkGate(plan.GateMaxWorkspaces); e != nil {
		t.Fatalf("a plan gate must pass: %+v", e)
	}
	if _, e := cleanReason("   "); e == nil {
		t.Fatal("a blank reason must be refused")
	}
	if got, e := cleanReason("  needs more agents  "); e != nil || got != "needs more agents" {
		t.Fatalf("reason = %q, %+v", got, e)
	}
}
