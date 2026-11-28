package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/featureflag"
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

// decideEnv is a workspace where the boss has an active profile, the Secretary is the workspace's
// agent, and the boss has delegated proposal:general on 25 resolved predictions. Proposals filed in it
// wait for a decision (autonomy level 1), so a delegated approval is the only one made.
type decideEnv struct {
	*policyEnv
	member pgtype.UUID // the boss's member row, which the predictions and the delegation name
}

func newDecideEnv(t *testing.T) decideEnv {
	t.Helper()
	ctx := context.Background()
	e := newPolicyEnv(t)
	e.props.Principal = &PredictionService{Queries: e.q, Flags: flagsWith(true)}
	boss, err := e.q.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{UserID: e.owner, WorkspaceID: e.ws})
	if err != nil {
		t.Fatalf("boss member: %v", err)
	}
	d := decideEnv{policyEnv: e, member: boss.ID}
	e.fx.Exec(t, `UPDATE workspace SET secretary_agent_id = $2 WHERE id = $1`, e.wsID, e.agent)
	e.fx.Exec(t, `INSERT INTO principal_profile (workspace_id, member_id, status, consent_at, share_with_owners) VALUES ($1, $2, 'active', now(), false)`, e.wsID, boss.ID)
	e.fx.Cleanup(t, `DELETE FROM principal_delegation WHERE workspace_id = $1`, e.wsID)
	e.fx.Cleanup(t, `DELETE FROM decision_prediction WHERE workspace_id = $1`, e.wsID)
	e.fx.Cleanup(t, `DELETE FROM principal_profile WHERE workspace_id = $1`, e.wsID)
	e.fx.Cleanup(t, `DELETE FROM governance_event WHERE workspace_id = $1`, e.wsID)
	d.seedResolved(t, "proposal:general", 1, trueN(25))
	if err := d.props.Principal.delegation().Enable(ctx, PrincipalActor{Type: GovernanceActorMember, ID: boss.ID}, e.ws, boss.ID, "proposal:general", 0.9); err != nil {
		t.Fatalf("enable: %v", err)
	}
	return d
}

// seedResolved inserts resolved predictions of one kind for the boss, newest first: one minute apart,
// the first starting `ago` minutes back. A true match is a decision of "approve".
func (d decideEnv) seedResolved(t *testing.T, kind string, ago int, matches []bool) {
	t.Helper()
	for i, m := range matches {
		actual := "reject"
		if m {
			actual = "approve"
		}
		d.fx.Insert(t, "decision_prediction", testutil.Cols{
			"workspace_id": d.wsID, "member_id": util.UUIDToString(d.member),
			"subject_type": "proposal", "subject_id": util.UUIDToString(dbid.NewV7()),
			"decision_kind": kind, "predicted": "approve", "confidence": 0.9,
			"actual": actual, "matched": m,
			"resolved_at": time.Now().Add(-time.Duration(ago+i) * time.Minute),
		})
	}
}

// predicted files a proposal the boss must decide and has the Secretary predict its decision. The
// prediction is open until the proposal is decided.
func (d decideEnv) predicted(t *testing.T, decision, kind string, confidence float64) (pgtype.UUID, db.DecisionPrediction) {
	t.Helper()
	ctx := context.Background()
	squad := d.squad("Delegated "+util.UUIDToString(dbid.NewV7()), AutonomyLevelWork)
	// The proposal's own kind must be the predicted kind: a prediction is recorded only for a
	// decision that exists under that kind (spec 6.2).
	fileKind := ""
	if strings.HasPrefix(kind, "proposal:") && kind != "proposal:general" {
		fileKind = strings.TrimPrefix(kind, "proposal:")
	}
	payload := "{}"
	if fileKind == "goal" {
		payload = `{"title":"Ship the seat","period_start":"2026-12-01","period_end":"2026-12-31"}`
	}
	proposal := d.file(squad, fileKind, payload, false).Proposal.ID
	if err := d.props.Principal.Request(ctx, AskInput{WorkspaceID: d.ws, SubjectType: "proposal", SubjectID: proposal}); err != nil {
		t.Fatalf("request: %v", err)
	}
	pred, err := d.props.Principal.Record(ctx, PrincipalActor{Type: GovernanceActorAgent, ID: d.agent}, PredictionInput{
		WorkspaceID: d.ws, MemberID: d.member, SubjectType: "proposal", SubjectID: proposal,
		DecisionKind: kind, Predicted: decision, Confidence: confidence,
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	return proposal, pred
}

// delegatedApproval has the Secretary predict an approval at 0.95 and the boss's delegation decide
// it. It returns the prediction, which is the delegated decision.
func (d decideEnv) delegatedApproval(t *testing.T) db.DecisionPrediction {
	t.Helper()
	proposal, pred := d.predicted(t, ProposalDecisionApprove, "proposal:general", 0.95)
	if _, err := d.props.Decide(context.Background(), ProposalDecideParams{
		WorkspaceID: d.ws, ProposalID: proposal, DeciderID: d.owner, Decision: ProposalDecisionApprove,
		Delegated: &DelegatedDecision{PredictionID: pred.ID},
	}); err != nil {
		t.Fatalf("delegated approve: %v", err)
	}
	return pred
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
	d := newDecideEnv(t)
	ctx := context.Background()
	proposal, pred := d.predicted(t, ProposalDecisionApprove, "proposal:general", 0.95)
	res, err := d.props.Decide(ctx, ProposalDecideParams{
		WorkspaceID: d.ws, ProposalID: proposal, DeciderID: d.owner, Decision: ProposalDecisionApprove,
		Delegated: &DelegatedDecision{PredictionID: pred.ID},
	})
	if err != nil {
		t.Fatalf("delegated approve: %v", err)
	}
	if res.Proposal.State != ProposalStateApproved {
		t.Fatalf("proposal state = %q, want approved", res.Proposal.State)
	}
	if n := d.fx.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE id = $1 AND applied_by_delegation = true AND matched = true AND resolved_at IS NOT NULL`, pred.ID); n != 1 {
		t.Fatalf("prediction not marked applied by delegation and resolved")
	}
	if n := d.fx.Count(t, `SELECT COUNT(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'principal_delegated_decision' AND actor_type = 'member' AND actor_id = $2 AND subject_id = $3`,
		d.wsID, d.member, pred.ID); n != 1 {
		t.Fatalf("%d delegated-decision events for the principal, want 1", n)
	}
	if err := d.props.Principal.delegation().OnReverted(ctx, d.ws, PrincipalActor{Type: GovernanceActorMember, ID: d.member}, pred.ID); err != nil {
		t.Fatalf("revert inside 72 hours: %v", err)
	}
}

// A delegated decision goes through only when every rule holds: the same decision as predicted, a
// confident prediction, a kind the boss delegated, a kind that may be delegated, and the boss as decider.
// A refused decision is not made, and its prediction stays open.
func TestDelegatedDecisionRefusedWhenIneligible(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		decision string
		kind     string
		conf     float64
		decider  func(d decideEnv) pgtype.UUID
	}{
		{"a different decision", ProposalDecisionReject, "proposal:general", 0.95, nil},
		{"a low confidence", ProposalDecisionApprove, "proposal:general", 0.5, nil},
		{"a kind never delegated", ProposalDecisionApprove, "proposal:goal", 0.95, nil},
		{"a decider who is not the boss", ProposalDecisionApprove, "proposal:general", 0.95,
			func(d decideEnv) pgtype.UUID { return dbid.NewV7() }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := newDecideEnv(t)
			proposal, pred := d.predicted(t, ProposalDecisionApprove, c.kind, c.conf)
			decider := d.owner
			if c.decider != nil {
				decider = c.decider(d)
			}
			_, err := d.props.Decide(ctx, ProposalDecideParams{
				WorkspaceID: d.ws, ProposalID: proposal, DeciderID: decider, Decision: c.decision,
				Delegated: &DelegatedDecision{PredictionID: pred.ID},
			})
			if err == nil {
				t.Fatalf("the decision was made")
			}
			if n := d.fx.Count(t, `SELECT COUNT(*) FROM proposal WHERE id = $1 AND state = 'pending'`, proposal); n != 1 {
				t.Fatalf("the refused proposal is no longer pending")
			}
			if n := d.fx.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE id = $1 AND applied_by_delegation = false AND resolved_at IS NULL`, pred.ID); n != 1 {
				t.Fatalf("the refused prediction was resolved or applied")
			}
		})
	}
}

func TestAutoDisableOnTwoReverts(t *testing.T) {
	d := newDecideEnv(t)
	ctx := context.Background()
	del := d.props.Principal.delegation()
	first := d.delegatedApproval(t)
	second := d.delegatedApproval(t)

	if err := del.OnReverted(ctx, d.ws, PrincipalActor{Type: GovernanceActorMember, ID: d.member}, first.ID); err != nil {
		t.Fatalf("first revert: %v", err)
	}
	if n := d.fx.Count(t, `SELECT COUNT(*) FROM principal_delegation WHERE workspace_id = $1 AND disabled_at IS NULL`, d.wsID); n != 1 {
		t.Fatalf("after one revert the delegation is off")
	}
	if err := del.OnReverted(ctx, d.ws, PrincipalActor{Type: GovernanceActorMember, ID: d.member}, second.ID); err != nil {
		t.Fatalf("second revert: %v", err)
	}
	if n := d.fx.Count(t, `SELECT COUNT(*) FROM principal_delegation WHERE workspace_id = $1 AND disabled_at IS NULL`, d.wsID); n != 0 {
		t.Fatalf("after two reverts the delegation is still on")
	}
	if n := d.fx.Count(t, `SELECT COUNT(*) FROM principal_delegation WHERE workspace_id = $1 AND disabled_reason <> ''`, d.wsID); n != 1 {
		t.Fatalf("the disable has no reason")
	}
	if n := d.fx.Count(t, `SELECT COUNT(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'principal_delegation_disabled'`, d.wsID); n != 1 {
		t.Fatalf("%d disabled events, want 1", n)
	}
}

// A delegated decision can be undone within 72 hours, and only by the person it was made for.
func TestRevertWindowAndActor(t *testing.T) {
	d := newDecideEnv(t)
	ctx := context.Background()
	del := d.props.Principal.delegation()
	pred := d.delegatedApproval(t)
	if err := del.OnReverted(ctx, d.ws, PrincipalActor{Type: GovernanceActorMember, ID: dbid.NewV7()}, pred.ID); !errors.Is(err, ErrPrincipalForbidden) {
		t.Fatalf("another member reverting: err = %v, want ErrPrincipalForbidden", err)
	}
	d.fx.Exec(t, `UPDATE decision_prediction SET resolved_at = now() - interval '73 hours' WHERE id = $1`, pred.ID)
	if err := del.OnReverted(ctx, d.ws, PrincipalActor{Type: GovernanceActorMember, ID: d.member}, pred.ID); !errors.Is(err, ErrDelegationRevertWindow) {
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

// A manual check the boss delegated is recorded as the boss's verdict, through the check path.
func TestDelegatedManualCheckLogged(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	boss, err := vf.q.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{UserID: vf.member.ID, WorkspaceID: vf.ws})
	if err != nil {
		t.Fatalf("boss member: %v", err)
	}
	vf.Exec(t, `UPDATE workspace SET secretary_agent_id = $2 WHERE id = $1`, vf.WorkspaceID, util.MustParseUUID(vf.verifier))
	vf.Exec(t, `INSERT INTO principal_profile (workspace_id, member_id, status, consent_at, share_with_owners) VALUES ($1, $2, 'active', now(), false)`, vf.WorkspaceID, boss.ID)
	for i := 0; i < 25; i++ {
		vf.Insert(t, "decision_prediction", testutil.Cols{
			"workspace_id": vf.WorkspaceID, "member_id": util.UUIDToString(boss.ID),
			"subject_type": "outcome_check", "subject_id": util.UUIDToString(dbid.NewV7()),
			"decision_kind": "outcome", "predicted": "pass", "confidence": 0.9,
			"actual": "pass", "matched": true, "resolved_at": time.Now().Add(-time.Duration(i+1) * time.Minute),
		})
	}
	// Criteria are written only while durable_outcomes is on (VerificationService).
	p := featureflag.NewStaticProvider()
	p.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: true})
	p.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: true})
	flags := featureflag.NewService(p)
	vf.checks.Flags = flags
	predictions := &PredictionService{Queries: vf.q, Flags: flags}
	if err := predictions.delegation().Enable(ctx, PrincipalActor{Type: GovernanceActorMember, ID: boss.ID}, vf.ws, boss.ID, "outcome", 0.9); err != nil {
		t.Fatalf("enable: %v", err)
	}

	issue := vf.doneIssue(t, vf.doer)
	c := vf.addCriterion(t, issue, "The page loads.", "manual", map[string]any{})
	outcome := vf.currentOutcome(t, issue)
	pred, err := predictions.Record(ctx, PrincipalActor{Type: GovernanceActorAgent, ID: util.MustParseUUID(vf.verifier)}, PredictionInput{
		WorkspaceID: vf.ws, MemberID: boss.ID, SubjectType: "outcome_check", SubjectID: outcome.ID,
		DecisionKind: "outcome", Predicted: "pass", Confidence: 0.95,
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := vf.checks.RecordAsPrincipal(ctx, vf.ws, vf.member.ID, outcome.ID,
		VerificationInput{CriterionID: c.ID, Result: "pass"}, DelegatedDecision{PredictionID: pred.ID}); err != nil {
		t.Fatalf("delegated check: %v", err)
	}
	if n := vf.Count(t, `SELECT COUNT(*) FROM outcome_verification WHERE outcome_id = $1 AND criterion_id = $2 AND verifier_type = 'member' AND verifier_id = $3 AND result = 'pass'`,
		outcome.ID, c.ID, vf.member.ID); n != 1 {
		t.Fatalf("%d member verifications, want 1", n)
	}
	if n := vf.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE id = $1 AND applied_by_delegation = true AND resolved_at IS NOT NULL`, pred.ID); n != 1 {
		t.Fatalf("prediction not applied by delegation")
	}
	if n := vf.Count(t, `SELECT COUNT(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'principal_delegated_decision' AND subject_id = $2`,
		vf.WorkspaceID, pred.ID); n != 1 {
		t.Fatalf("%d delegated-decision events, want 1", n)
	}
}
