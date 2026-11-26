package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

// delegationTest is a principalTest with the delegation service over the same fixture.
type delegationTest struct {
	principalTest
}

func newDelegationTest(t *testing.T) delegationTest {
	t.Helper()
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	return delegationTest{pt}
}

func (dt delegationTest) del() *DelegationService {
	return &DelegationService{Queries: dt.q, Flags: dt.svc.Flags, Predictions: dt.pred()}
}

// seedResolved inserts resolved predictions of one kind for the owner, newest first: one minute
// apart, the first starting `ago` minutes back. A true match is a decision of "approve".
func (dt delegationTest) seedResolved(t *testing.T, kind string, ago int, matches []bool) {
	t.Helper()
	for i, m := range matches {
		actual := "reject"
		if m {
			actual = "approve"
		}
		dt.Insert(t, "decision_prediction", testutil.Cols{
			"workspace_id": dt.WorkspaceID, "member_id": util.UUIDToString(dt.owner.ID),
			"subject_type": "proposal", "subject_id": util.UUIDToString(dbid.NewV7()),
			"decision_kind": kind, "predicted": "approve", "confidence": 0.9,
			"actual": actual, "matched": m,
			"resolved_at": time.Now().Add(-time.Duration(ago+i) * time.Minute),
		})
	}
}

// applyDelegated makes a prediction at confidence 0.95 and lets the delegation apply it. It returns
// the prediction row's id, and whether the delegation applied it.
func (dt delegationTest) applyDelegated(t *testing.T, kind string) (string, bool) {
	t.Helper()
	ctx := context.Background()
	subject := util.MustParseUUID(dt.Issue(t, "a delegated question"))
	if err := dt.pred().Request(ctx, AskInput{
		WorkspaceID: dt.ws, SubjectType: "proposal", SubjectID: subject, EstimateKind: kind[len("proposal:"):],
	}); err != nil {
		t.Fatalf("request: %v", err)
	}
	row, err := dt.pred().Record(ctx, dt.secretary, PredictionInput{
		WorkspaceID: dt.ws, MemberID: dt.owner.ID, SubjectType: "proposal", SubjectID: subject,
		DecisionKind: kind, Predicted: "approve", Confidence: 0.95,
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	applied, err := dt.del().MaybeApply(ctx, row, AskInput{WorkspaceID: dt.ws, SubjectType: "proposal", SubjectID: subject})
	if err != nil {
		t.Fatalf("maybe apply: %v", err)
	}
	return util.UUIDToString(row.ID), applied
}

func TestEnableRequires20And85(t *testing.T) {
	dt := newDelegationTest(t)
	ctx := context.Background()
	const kind = "proposal:hire"

	dt.seedResolved(t, kind, 1, trueN(19))
	if err := dt.del().Enable(ctx, dt.owner, dt.ws, dt.owner.ID, kind, 0.9); !errors.Is(err, ErrDelegationNotEligible) {
		t.Fatalf("19 resolved: err = %v, want ErrDelegationNotEligible", err)
	}
	dt.seedResolved(t, kind, 100, []bool{true})
	if err := dt.del().Enable(ctx, dt.owner, dt.ws, dt.owner.ID, kind, 0.9); err != nil {
		t.Fatalf("20 resolved, all matched: %v", err)
	}
	if n := dt.Count(t, `SELECT COUNT(*) FROM principal_delegation WHERE workspace_id = $1 AND disabled_at IS NULL`, dt.WorkspaceID); n != 1 {
		t.Fatalf("active delegations = %d, want 1", n)
	}

	const other = "proposal:promote"
	dt.seedResolved(t, other, 1, append(trueN(16), falseN(4)...)) // 80%
	if err := dt.del().Enable(ctx, dt.owner, dt.ws, dt.owner.ID, other, 0.9); !errors.Is(err, ErrDelegationNotEligible) {
		t.Fatalf("80%% matched: err = %v, want ErrDelegationNotEligible", err)
	}
}

func TestForbiddenKindsNeverDelegated(t *testing.T) {
	plain := AskInput{SubjectType: "proposal"}
	cases := []struct {
		name string
		kind string
		ask  AskInput
		want bool
	}{
		{"org change", "proposal:org_change", plain, true},
		{"launch gate", "proposal:launch", plain, true},
		{"stage gate", "proposal:stage_gate", plain, true},
		{"spend effect", "effect:spend", AskInput{SubjectType: "effect_intent"}, true},
		{"share effect", "effect:share", AskInput{SubjectType: "effect_intent"}, true},
		{"budget", "proposal:budget", plain, true},
		{"autonomy", "proposal:autonomy", plain, true},
		{"envelope", "proposal:envelope", plain, true},
		{"policy", "proposal:policy", plain, true},
		{"profile", "proposal:profile", plain, true},
		{"irreversible intent", "effect:write", AskInput{SubjectType: "effect_intent", Irreversible: true}, true},
		{"urgent question", "proposal:hire", AskInput{SubjectType: "proposal", Urgent: true}, true},
		{"ordinary hiring", "proposal:hire", plain, false},
		{"reversible write", "effect:write", AskInput{SubjectType: "effect_intent"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ForbiddenDelegationKind(c.kind, c.ask); got != c.want {
				t.Fatalf("ForbiddenDelegationKind(%q) = %v, want %v", c.kind, got, c.want)
			}
		})
	}
}

func TestDelegationKindMustBeAllowed(t *testing.T) {
	dt := newDelegationTest(t)
	dt.seedResolved(t, "proposal:budget", 1, trueN(25))
	err := dt.del().Enable(context.Background(), dt.owner, dt.ws, dt.owner.ID, "proposal:budget", 0.9)
	if !errors.Is(err, ErrDelegationForbiddenKind) {
		t.Fatalf("enabling budget: err = %v, want ErrDelegationForbiddenKind", err)
	}
}

func TestOnlyProfileOwnerEnables(t *testing.T) {
	dt := newDelegationTest(t)
	const kind = "proposal:hire"
	dt.seedResolved(t, kind, 1, trueN(25))
	ctx := context.Background()
	if err := dt.del().Enable(ctx, dt.other, dt.ws, dt.owner.ID, kind, 0.9); !errors.Is(err, ErrPrincipalForbidden) {
		t.Fatalf("another member enabling: err = %v, want ErrPrincipalForbidden", err)
	}
	if err := dt.del().Enable(ctx, dt.secretary, dt.ws, dt.owner.ID, kind, 0.9); !errors.Is(err, ErrPrincipalForbidden) {
		t.Fatalf("the Secretary enabling: err = %v, want ErrPrincipalForbidden", err)
	}
}

func TestDelegatedDecisionLogged(t *testing.T) {
	dt := newDelegationTest(t)
	const kind = "proposal:hire"
	dt.seedResolved(t, kind, 1, trueN(25))
	ctx := context.Background()
	if err := dt.del().Enable(ctx, dt.owner, dt.ws, dt.owner.ID, kind, 0.9); err != nil {
		t.Fatalf("enable: %v", err)
	}
	predID, applied := dt.applyDelegated(t, kind)
	if !applied {
		t.Fatalf("a 0.95 prediction of an enabled kind was not applied")
	}
	if n := dt.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE id = $1 AND applied_by_delegation = true`, predID); n != 1 {
		t.Fatalf("prediction not marked as applied by delegation")
	}
	if n := dt.Count(t, `SELECT COUNT(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'principal_delegated_decision' AND actor_type = 'member' AND actor_id = $2`,
		dt.WorkspaceID, dt.owner.ID); n != 1 {
		t.Fatalf("%d delegated-decision events, want 1", n)
	}

	// A prediction below the minimum confidence is left to the person.
	subject := util.MustParseUUID(dt.Issue(t, "a low-confidence question"))
	low, err := dt.pred().Record(ctx, dt.secretary, PredictionInput{
		WorkspaceID: dt.ws, MemberID: dt.owner.ID, SubjectType: "proposal", SubjectID: subject,
		DecisionKind: kind, Predicted: "approve", Confidence: 0.5,
	})
	if err != nil {
		t.Fatalf("record low: %v", err)
	}
	if ok, err := dt.del().MaybeApply(ctx, low, AskInput{SubjectType: "proposal"}); err != nil || ok {
		t.Fatalf("0.5 prediction applied = %v (err %v), want not applied", ok, err)
	}
}

func TestAutoDisableOnTwoReverts(t *testing.T) {
	dt := newDelegationTest(t)
	const kind = "proposal:hire"
	dt.seedResolved(t, kind, 1, trueN(25))
	ctx := context.Background()
	if err := dt.del().Enable(ctx, dt.owner, dt.ws, dt.owner.ID, kind, 0.9); err != nil {
		t.Fatalf("enable: %v", err)
	}
	first, _ := dt.applyDelegated(t, kind)
	second, _ := dt.applyDelegated(t, kind)

	if err := dt.del().OnReverted(ctx, dt.ws, dt.owner, util.MustParseUUID(first)); err != nil {
		t.Fatalf("first revert: %v", err)
	}
	if n := dt.Count(t, `SELECT COUNT(*) FROM principal_delegation WHERE workspace_id = $1 AND disabled_at IS NULL`, dt.WorkspaceID); n != 1 {
		t.Fatalf("after one revert the delegation is off")
	}
	if err := dt.del().OnReverted(ctx, dt.ws, dt.owner, util.MustParseUUID(second)); err != nil {
		t.Fatalf("second revert: %v", err)
	}
	if n := dt.Count(t, `SELECT COUNT(*) FROM principal_delegation WHERE workspace_id = $1 AND disabled_at IS NULL`, dt.WorkspaceID); n != 0 {
		t.Fatalf("after two reverts the delegation is still on")
	}
	if n := dt.Count(t, `SELECT COUNT(*) FROM principal_delegation WHERE workspace_id = $1 AND disabled_reason <> ''`, dt.WorkspaceID); n != 1 {
		t.Fatalf("the disable has no reason")
	}
	if n := dt.Count(t, `SELECT COUNT(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'principal_delegation_disabled'`, dt.WorkspaceID); n != 1 {
		t.Fatalf("%d disabled events, want 1", n)
	}
}

// A delegated decision can be undone within 72 hours, and only by the person it was made for.
func TestRevertWindowAndActor(t *testing.T) {
	dt := newDelegationTest(t)
	const kind = "proposal:hire"
	dt.seedResolved(t, kind, 1, trueN(25))
	ctx := context.Background()
	if err := dt.del().Enable(ctx, dt.owner, dt.ws, dt.owner.ID, kind, 0.9); err != nil {
		t.Fatalf("enable: %v", err)
	}
	id, _ := dt.applyDelegated(t, kind)
	if err := dt.del().OnReverted(ctx, dt.ws, dt.other, util.MustParseUUID(id)); !errors.Is(err, ErrPrincipalForbidden) {
		t.Fatalf("another member reverting: err = %v, want ErrPrincipalForbidden", err)
	}
	dt.Exec(t, `UPDATE decision_prediction SET resolved_at = now() - interval '73 hours' WHERE id = $1`, id)
	if err := dt.del().OnReverted(ctx, dt.ws, dt.owner, util.MustParseUUID(id)); !errors.Is(err, ErrDelegationRevertWindow) {
		t.Fatalf("revert after 73 hours: err = %v, want ErrDelegationRevertWindow", err)
	}
}

func TestAutoDisableBelow80(t *testing.T) {
	dt := newDelegationTest(t)
	const kind = "proposal:hire"
	dt.seedResolved(t, kind, 100, trueN(20))
	ctx := context.Background()
	if err := dt.del().Enable(ctx, dt.owner, dt.ws, dt.owner.ID, kind, 0.9); err != nil {
		t.Fatalf("enable: %v", err)
	}
	// Forty newer predictions, half missed, then the window's tail of matched ones: 30 of 50.
	newer := make([]bool, 0, 40)
	for i := 0; i < 40; i++ {
		newer = append(newer, i%2 == 0)
	}
	dt.seedResolved(t, kind, 1, newer)
	if err := dt.del().ReviewAccuracy(ctx, dt.ws, dt.owner.ID, kind); err != nil {
		t.Fatalf("review: %v", err)
	}
	if n := dt.Count(t, `SELECT COUNT(*) FROM principal_delegation WHERE workspace_id = $1 AND disabled_at IS NULL`, dt.WorkspaceID); n != 0 {
		t.Fatalf("with 60%% matched over the window the delegation is still on")
	}
}

func trueN(n int) []bool {
	out := make([]bool, n)
	for i := range out {
		out[i] = true
	}
	return out
}

func falseN(n int) []bool {
	return make([]bool, n)
}

// Deleting a profile removes its delegations (spec 6.6).
func TestDeleteRemovesDelegations(t *testing.T) {
	dt := newDelegationTest(t)
	const kind = "proposal:hire"
	dt.seedResolved(t, kind, 1, trueN(25))
	if err := dt.del().Enable(context.Background(), dt.owner, dt.ws, dt.owner.ID, kind, 0.9); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := dt.svc.Delete(context.Background(), dt.owner, dt.ws, dt.owner.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n := dt.Count(t, `SELECT COUNT(*) FROM principal_delegation WHERE workspace_id = $1`, dt.WorkspaceID); n != 0 {
		t.Fatalf("%d delegation rows left after delete", n)
	}
}
