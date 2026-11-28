package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

type delegationFakeIntentExecutor struct {
	calls []connectedapps.CanonicalCall
}

func (f *delegationFakeIntentExecutor) ExecuteIntent(_ context.Context, _, _ pgtype.UUID, _ string, call connectedapps.CanonicalCall) (map[string]any, error) {
	f.calls = append(f.calls, call)
	return map[string]any{"ok": true}, nil
}

type fakeUndoExecutor struct {
	calls []connectedapps.UndoSpec
}

func (f *fakeUndoExecutor) ExecuteUndo(_ context.Context, _, _ pgtype.UUID, _ string, spec connectedapps.UndoSpec) (map[string]any, error) {
	f.calls = append(f.calls, spec)
	return map[string]any{"undone": true}, nil
}

func TestPredictionRequestEnqueuesSecretaryTask(t *testing.T) {
	t.Run("no profile enqueues no task", func(t *testing.T) {
		pt := newPrincipalTest(t, true)
		ctx := context.Background()
		secID := util.UUIDToString(pt.secretary.ID)
		pt.Cleanup(t, `DELETE FROM agent_task_queue WHERE agent_id = $1`, secID)

		subject := util.MustParseUUID(pt.Issue(t, "ask without profile"))
		if err := pt.pred().Request(ctx, predictionAsk(pt, subject)); err != nil {
			t.Fatalf("request: %v", err)
		}
		if n := pt.Count(t, `SELECT COUNT(*) FROM agent_task_queue WHERE agent_id = $1`, secID); n != 0 {
			t.Fatalf("queued tasks without profile = %d, want 0", n)
		}
	})

	t.Run("active profile enqueues routine prediction task once with context", func(t *testing.T) {
		pt := newPrincipalTest(t, true)
		pt.enable(t)
		ctx := context.Background()
		secID := util.UUIDToString(pt.secretary.ID)
		pt.Cleanup(t, `DELETE FROM agent_task_queue WHERE agent_id = $1`, secID)

		commentID := pt.comment(t, pt.owner)
		if _, err := pt.svc.AddTrait(ctx, pt.owner, TraitInput{
			WorkspaceID: pt.ws, MemberID: pt.owner.ID, DecisionKind: "proposal:general",
			Statement: "approves small routine proposals", Origin: principalOriginEdited,
			Evidence: evidenceOf("comment", commentID),
		}); err != nil {
			t.Fatalf("add kind trait: %v", err)
		}
		if _, err := pt.svc.AddTrait(ctx, pt.owner, TraitInput{
			WorkspaceID: pt.ws, MemberID: pt.owner.ID, DecisionKind: "general",
			Statement: "prefers concise updates", Origin: principalOriginEdited,
			Evidence: evidenceOf("comment", commentID),
		}); err != nil {
			t.Fatalf("add general trait: %v", err)
		}
		if _, err := pt.svc.AddTrait(ctx, pt.owner, TraitInput{
			WorkspaceID: pt.ws, MemberID: pt.owner.ID, DecisionKind: "inquiry",
			Statement: "unrelated trait", Origin: principalOriginEdited,
			Evidence: evidenceOf("comment", commentID),
		}); err != nil {
			t.Fatalf("add unrelated trait: %v", err)
		}
		for i := 0; i < 12; i++ {
			pt.Insert(t, "decision_prediction", testutil.Cols{
				"workspace_id": pt.WorkspaceID, "member_id": util.UUIDToString(pt.owner.ID),
				"subject_type": "proposal", "subject_id": util.UUIDToString(dbid.NewV7()),
				"decision_kind": "proposal:general", "predicted": "approve", "confidence": 0.9,
				"actual": "approve", "matched": true, "resolved_at": time.Now().Add(-time.Duration(i+1) * time.Minute),
			})
		}

		subject := util.MustParseUUID(pt.Issue(t, "ask with active profile"))
		ask := AskInput{WorkspaceID: pt.ws, SubjectType: "proposal", SubjectID: subject, Reason: "judgment"}
		if err := pt.pred().Request(ctx, ask); err != nil {
			t.Fatalf("first request: %v", err)
		}
		// Calling Request again for the same subject must not enqueue a duplicate task.
		if err := pt.pred().Request(ctx, ask); err != nil {
			t.Fatalf("second request: %v", err)
		}

		if n := pt.Count(t, `SELECT COUNT(*) FROM agent_task_queue WHERE agent_id = $1 AND status = 'queued'`, secID); n != 1 {
			t.Fatalf("queued prediction tasks = %d, want 1", n)
		}
		var taskID string
		pt.QueryRow(t, `SELECT id::text FROM agent_task_queue WHERE agent_id = $1`, secID).Scan(&taskID)
		task, err := pt.q.GetAgentTask(ctx, util.MustParseUUID(taskID))
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if rc, err := RunClassOfTask(ctx, pt.q, task); err != nil || rc != RunClassRoutine {
			t.Fatalf("RunClassOfTask = %q (err=%v), want %q", rc, err, RunClassRoutine)
		}
		var payload PrincipalPredictionTaskContext
		if err := json.Unmarshal(task.Context, &payload); err != nil {
			t.Fatalf("unmarshal prediction context: %v", err)
		}
		if payload.Kind != principalPredictionKind || payload.DecisionKind != "proposal:general" {
			t.Fatalf("payload kind=%q decision_kind=%q", payload.Kind, payload.DecisionKind)
		}
		if len(payload.Targets) != 1 || len(payload.Targets[0].Traits) != 2 || len(payload.Targets[0].RecentDecisions) != 10 {
			t.Fatalf("unexpected targets: %+v", payload.Targets)
		}
		if n := pt.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE subject_id = $1 AND predictor_task_id = $2`,
			util.UUIDToString(subject), taskID); n != 1 {
			t.Fatalf("pending prediction not linked to predictor_task_id")
		}
	})
}

func TestPredictionRecordAutoDelegatesEndToEnd(t *testing.T) {
	t.Run("proposal auto-applied when eligible and skipped when ineligible", func(t *testing.T) {
		d := newDecideEnv(t)
		ctx := context.Background()
		secID := util.UUIDToString(d.agent)
		rt := d.fx.Runtime(t, "auto-prop runtime", testutil.Cols{"provider": "anthropic"})
		d.fx.Exec(t, `UPDATE agent SET runtime_id = $2 WHERE id = $1`, secID, rt)
		d.fx.Cleanup(t, `DELETE FROM agent_task_queue WHERE agent_id = $1`, secID)
		// Use the user_id profile (spec 6.2) so Request creates pending rows & Secretary tasks.
		d.fx.Exec(t, `DELETE FROM principal_delegation WHERE workspace_id = $1`, d.wsID)
		d.fx.Exec(t, `DELETE FROM decision_prediction WHERE workspace_id = $1`, d.wsID)
		d.fx.Exec(t, `DELETE FROM principal_profile WHERE workspace_id = $1`, d.wsID)
		d.fx.Exec(t, `INSERT INTO principal_profile (workspace_id, member_id, status, consent_at, share_with_owners) VALUES ($1, $2, 'active', now(), false)`, d.wsID, d.owner)
		for i := 0; i < 25; i++ {
			d.fx.Insert(t, "decision_prediction", testutil.Cols{
				"workspace_id": d.wsID, "member_id": util.UUIDToString(d.owner),
				"subject_type": "proposal", "subject_id": util.UUIDToString(dbid.NewV7()),
				"decision_kind": "proposal:general", "predicted": "approve", "confidence": 0.95,
				"actual": "approve", "matched": true, "resolved_at": time.Now().Add(-time.Duration(i+1) * time.Minute),
			})
		}
		d.props.Principal = &PredictionService{
			Queries: d.q, TxStarter: d.fx.Pool, Flags: flagsWith(true), Proposals: d.props,
		}
		if err := d.props.Principal.delegation().Enable(ctx, PrincipalActor{Type: GovernanceActorMember, ID: d.owner}, d.ws, d.owner, "proposal:general", 0.9); err != nil {
			t.Fatalf("enable delegation: %v", err)
		}

		// Eligible proposal: Request -> task -> Record -> auto-approved.
		squad := d.squad("AutoProp "+util.UUIDToString(dbid.NewV7()), AutonomyLevelWork)
		prop1 := d.file(squad, "", "{}", false).Proposal.ID
		if err := d.props.Principal.Request(ctx, AskInput{WorkspaceID: d.ws, SubjectType: "proposal", SubjectID: prop1}); err != nil {
			t.Fatalf("request prop1: %v", err)
		}
		if n := d.fx.Count(t, `SELECT COUNT(*) FROM agent_task_queue WHERE agent_id = $1 AND context->>'kind' = 'principal_prediction'`, secID); n != 1 {
			t.Fatalf("prediction tasks = %d, want 1", n)
		}
		pred1, err := d.props.Principal.Record(ctx, PrincipalActor{Type: GovernanceActorAgent, ID: d.agent}, PredictionInput{
			WorkspaceID: d.ws, MemberID: d.owner, SubjectType: "proposal", SubjectID: prop1,
			DecisionKind: "proposal:general", Predicted: ProposalDecisionApprove, Confidence: 0.95, Why: "routine",
		})
		if err != nil {
			t.Fatalf("record prop1: %v", err)
		}
		if !pred1.AppliedByDelegation {
			t.Fatalf("expected prop1 prediction to be applied_by_delegation")
		}
		if n := d.fx.Count(t, `SELECT COUNT(*) FROM proposal WHERE id = $1 AND state = 'approved'`, prop1); n != 1 {
			t.Fatalf("prop1 was not auto-approved")
		}

		// Ineligible (low confidence): stays pending.
		prop2 := d.file(squad, "", "{}", false).Proposal.ID
		pred2, err := d.props.Principal.Record(ctx, PrincipalActor{Type: GovernanceActorAgent, ID: d.agent}, PredictionInput{
			WorkspaceID: d.ws, MemberID: d.owner, SubjectType: "proposal", SubjectID: prop2,
			DecisionKind: "proposal:general", Predicted: ProposalDecisionApprove, Confidence: 0.5, Why: "uncertain",
		})
		if err != nil {
			t.Fatalf("record prop2: %v", err)
		}
		if pred2.AppliedByDelegation {
			t.Fatalf("low confidence prediction must not auto-apply")
		}
		if n := d.fx.Count(t, `SELECT COUNT(*) FROM proposal WHERE id = $1 AND state = 'pending'`, prop2); n != 1 {
			t.Fatalf("prop2 should stay pending")
		}
	})

	t.Run("effect_intent auto-applied when reversible and skipped when irreversible", func(t *testing.T) {
		pt := newPrincipalTest(t, true)
		pt.enable(t)
		ctx := context.Background()
		secID := util.UUIDToString(pt.secretary.ID)
		pt.Cleanup(t, `DELETE FROM agent_task_queue WHERE agent_id = $1`, secID)
		pt.Cleanup(t, `DELETE FROM effect_intent WHERE workspace_id = $1`, pt.WorkspaceID)
		pt.Cleanup(t, `DELETE FROM effect_event WHERE workspace_id = $1`, pt.WorkspaceID)

		p := featureflag.NewStaticProvider()
		p.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: true})
		p.Set(featureflags.DurableEffects, featureflag.Rule{Default: true})
		flags := featureflag.NewService(p)

		exec := &delegationFakeIntentExecutor{}
		intents := &IntentService{Queries: pt.q, TxStarter: pt.Pool, Flags: flags, Apps: exec}
		predSvc := &PredictionService{Queries: pt.q, TxStarter: pt.Pool, Flags: flags, Intents: intents}

		for i := 0; i < 25; i++ {
			pt.Insert(t, "decision_prediction", testutil.Cols{
				"workspace_id": pt.WorkspaceID, "member_id": util.UUIDToString(pt.owner.ID),
				"subject_type": "effect_intent", "subject_id": util.UUIDToString(dbid.NewV7()),
				"decision_kind": "effect:write", "predicted": "approve", "confidence": 0.95,
				"actual": "approve", "matched": true, "resolved_at": time.Now().Add(-time.Duration(i+1) * time.Minute),
			})
		}
		if err := predSvc.delegation().Enable(ctx, pt.owner, pt.ws, pt.owner.ID, "effect:write", 0.9); err != nil {
			t.Fatalf("enable effect:write delegation: %v", err)
		}

		// Reversible intent -> auto-approved & executed once.
		call := connectedapps.CanonicalCall{Tool: "GMAIL_CREATE_DRAFT", AccountID: "acc1", Args: map[string]any{"body": "hi"}}
		evID, err := pt.q.InsertEffectEvent(ctx, db.InsertEffectEventParams{
			WorkspaceID: pt.ws, MemberID: pt.owner.ID, Source: connectedapps.SourceConnectedApp,
			Target: "gmail/GMAIL_CREATE_DRAFT", EffectClass: "write", Reversibility: "reversible",
			Decision: "rehearse", Status: "awaiting", Summary: "draft email",
		})
		if err != nil {
			t.Fatalf("insert effect_event: %v", err)
		}
		intentID, err := intents.Rehearse(ctx, evID, connectedapps.EffectRequest{
			WS: pt.ws, AgentID: pt.secretary.ID, MemberID: pt.owner.ID, Source: connectedapps.SourceConnectedApp,
			Target: "gmail/GMAIL_CREATE_DRAFT", Class: "write", Reversibility: "reversible", Summary: "draft email",
		}, call, "draft")
		if err != nil {
			t.Fatalf("rehearse reversible: %v", err)
		}
		if err := predSvc.Request(ctx, AskInput{
			WorkspaceID: pt.ws, SubjectType: "effect_intent", SubjectID: intentID, EstimateKind: "write",
		}); err != nil {
			t.Fatalf("request intent prediction: %v", err)
		}
		pred, err := predSvc.Record(ctx, pt.secretary, PredictionInput{
			WorkspaceID: pt.ws, MemberID: pt.owner.ID, SubjectType: "effect_intent", SubjectID: intentID,
			DecisionKind: "effect:write", Predicted: "approve", Confidence: 0.95, Why: "reversible draft",
		})
		if err != nil {
			t.Fatalf("record reversible intent prediction: %v", err)
		}
		if !pred.AppliedByDelegation {
			t.Fatalf("reversible intent prediction was not applied_by_delegation")
		}
		if len(exec.calls) != 1 {
			t.Fatalf("executor calls = %d, want 1", len(exec.calls))
		}
		gotIntent, err := intents.Get(ctx, pt.ws, intentID)
		if err != nil || gotIntent.State != "executed" {
			t.Fatalf("intent state = %q (err=%v), want executed", gotIntent.State, err)
		}

		// Irreversible intent -> never auto-applied even with high confidence.
		irrevID, err := intents.Rehearse(ctx, pgtype.UUID{}, connectedapps.EffectRequest{
			WS: pt.ws, AgentID: pt.secretary.ID, MemberID: pt.owner.ID, Source: connectedapps.SourceConnectedApp,
			Target: "gmail/GMAIL_SEND_EMAIL", Class: "write", Reversibility: "irreversible", Summary: "send email",
		}, call, "send")
		if err != nil {
			t.Fatalf("rehearse irreversible: %v", err)
		}
		predIrrev, err := predSvc.Record(ctx, pt.secretary, PredictionInput{
			WorkspaceID: pt.ws, MemberID: pt.owner.ID, SubjectType: "effect_intent", SubjectID: irrevID,
			DecisionKind: "effect:write", Predicted: "approve", Confidence: 0.99, Why: "test",
		})
		if err != nil {
			t.Fatalf("record irreversible prediction: %v", err)
		}
		if predIrrev.AppliedByDelegation {
			t.Fatalf("irreversible intent must never auto-apply")
		}
		if len(exec.calls) != 1 {
			t.Fatalf("executor calls after irreversible = %d, want still 1", len(exec.calls))
		}
	})

	t.Run("outcome_check auto-applied when eligible", func(t *testing.T) {
		vf := newVerificationFixture(t)
		ctx := context.Background()
		vf.Exec(t, `UPDATE workspace SET secretary_agent_id = $2 WHERE id = $1`, vf.WorkspaceID, util.MustParseUUID(vf.verifier))
		vf.Exec(t, `INSERT INTO principal_profile (workspace_id, member_id, status, consent_at, share_with_owners) VALUES ($1, $2, 'active', now(), false)`, vf.WorkspaceID, vf.member.ID)
		vf.Cleanup(t, `DELETE FROM principal_delegation WHERE workspace_id = $1`, vf.WorkspaceID)
		vf.Cleanup(t, `DELETE FROM decision_prediction WHERE workspace_id = $1`, vf.WorkspaceID)
		vf.Cleanup(t, `DELETE FROM principal_profile WHERE workspace_id = $1`, vf.WorkspaceID)
		vf.Cleanup(t, `DELETE FROM agent_task_queue WHERE agent_id = $1`, vf.verifier)

		for i := 0; i < 25; i++ {
			vf.Insert(t, "decision_prediction", testutil.Cols{
				"workspace_id": vf.WorkspaceID, "member_id": util.UUIDToString(vf.member.ID),
				"subject_type": "outcome_check", "subject_id": util.UUIDToString(dbid.NewV7()),
				"decision_kind": "outcome", "predicted": "pass", "confidence": 0.95,
				"actual": "pass", "matched": true, "resolved_at": time.Now().Add(-time.Duration(i+1) * time.Minute),
			})
		}
		p := featureflag.NewStaticProvider()
		p.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: true})
		p.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: true})
		flags := featureflag.NewService(p)
		vf.checks.Flags = flags
		predSvc := &PredictionService{Queries: vf.q, TxStarter: vf.Pool, Flags: flags, Checks: vf.checks}
		if err := predSvc.delegation().Enable(ctx, PrincipalActor{Type: GovernanceActorMember, ID: vf.member.ID}, vf.ws, vf.member.ID, "outcome", 0.9); err != nil {
			t.Fatalf("enable outcome delegation: %v", err)
		}

		issue := vf.doneIssue(t, vf.doer)
		c := vf.addCriterion(t, issue, "Manual check passes", CheckKindManual, map[string]any{})
		outcome := vf.currentOutcome(t, issue)
		if err := predSvc.Request(ctx, AskInput{
			WorkspaceID: vf.ws, SubjectType: "outcome_check", SubjectID: outcome.ID,
		}); err != nil {
			t.Fatalf("request outcome prediction: %v", err)
		}
		pred, err := predSvc.Record(ctx, PrincipalActor{Type: GovernanceActorAgent, ID: util.MustParseUUID(vf.verifier)}, PredictionInput{
			WorkspaceID: vf.ws, MemberID: vf.member.ID, SubjectType: "outcome_check", SubjectID: outcome.ID,
			DecisionKind: "outcome", Predicted: "pass", Confidence: 0.95, Why: "matches past checks",
		})
		if err != nil {
			t.Fatalf("record outcome prediction: %v", err)
		}
		if !pred.AppliedByDelegation {
			t.Fatalf("outcome_check prediction was not applied_by_delegation")
		}
		if n := vf.Count(t, `SELECT COUNT(*) FROM outcome_verification WHERE outcome_id = $1 AND criterion_id = $2 AND verifier_type = 'member' AND result = 'pass'`,
			outcome.ID, c.ID); n != 1 {
			t.Fatalf("member verifications = %d, want 1", n)
		}
	})
}

func TestOnRevertedIdempotentAndRevertsSubjects(t *testing.T) {
	t.Run("proposal revert is idempotent and restores proposal and issue", func(t *testing.T) {
		d := newDecideEnv(t)
		ctx := context.Background()
		del := d.props.Principal.delegation()
		del.TxStarter = d.fx.Pool
		pred := d.delegatedApproval(t)
		actor := PrincipalActor{Type: GovernanceActorMember, ID: d.member}

		if err := del.OnReverted(ctx, d.ws, actor, pred.ID); err != nil {
			t.Fatalf("first revert: %v", err)
		}
		// Second revert of the same predID must be idempotent: no second event, count remains 1, delegation stays active.
		if err := del.OnReverted(ctx, d.ws, actor, pred.ID); err != nil {
			t.Fatalf("second revert: %v", err)
		}
		if n := d.fx.Count(t, `SELECT COUNT(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'principal_delegation_reverted' AND subject_id = $2`, d.wsID, pred.ID); n != 1 {
			t.Fatalf("revert events = %d, want 1", n)
		}
		count, err := d.q.CountDelegationReverts(ctx, db.CountDelegationRevertsParams{
			WorkspaceID: d.ws, SubjectIds: []pgtype.UUID{pred.ID},
		})
		if err != nil || count != 1 {
			t.Fatalf("CountDelegationReverts = %d (err=%v), want 1", count, err)
		}
		if n := d.fx.Count(t, `SELECT COUNT(*) FROM principal_delegation WHERE workspace_id = $1 AND disabled_at IS NULL`, d.wsID); n != 1 {
			t.Fatalf("delegation disabled after duplicate revert of a single decision")
		}
		if n := d.fx.Count(t, `SELECT COUNT(*) FROM proposal WHERE id = $1 AND state = 'pending' AND decided_at IS NULL`, pred.SubjectID); n != 1 {
			t.Fatalf("proposal was not reverted to pending")
		}
		if n := d.fx.Count(t, `SELECT COUNT(*) FROM issue i JOIN proposal p ON p.issue_id = i.id WHERE p.id = $1 AND i.status = 'in_review'`, pred.SubjectID); n != 1 {
			t.Fatalf("proposal backing issue was not reverted to in_review")
		}
	})

	t.Run("effect_intent revert executes F3 undo for reversible executed intent", func(t *testing.T) {
		pt := newPrincipalTest(t, true)
		pt.enable(t)
		ctx := context.Background()
		pt.Cleanup(t, `DELETE FROM effect_intent WHERE workspace_id = $1`, pt.WorkspaceID)
		pt.Cleanup(t, `DELETE FROM effect_event WHERE workspace_id = $1`, pt.WorkspaceID)

		p := featureflag.NewStaticProvider()
		p.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: true})
		p.Set(featureflags.DurableEffects, featureflag.Rule{Default: true})
		flags := featureflag.NewService(p)

		exec := &delegationFakeIntentExecutor{}
		undoExec := &fakeUndoExecutor{}
		intents := &IntentService{Queries: pt.q, TxStarter: pt.Pool, Flags: flags, Apps: exec}
		predSvc := &PredictionService{Queries: pt.q, TxStarter: pt.Pool, Flags: flags, Intents: intents}

		for i := 0; i < 25; i++ {
			pt.Insert(t, "decision_prediction", testutil.Cols{
				"workspace_id": pt.WorkspaceID, "member_id": util.UUIDToString(pt.owner.ID),
				"subject_type": "effect_intent", "subject_id": util.UUIDToString(dbid.NewV7()),
				"decision_kind": "effect:write", "predicted": "approve", "confidence": 0.95,
				"actual": "approve", "matched": true, "resolved_at": time.Now().Add(-time.Duration(i+1) * time.Minute),
			})
		}
		if err := predSvc.delegation().Enable(ctx, pt.owner, pt.ws, pt.owner.ID, "effect:write", 0.9); err != nil {
			t.Fatalf("enable: %v", err)
		}
		undoSpec, _ := json.Marshal(connectedapps.UndoSpec{Tool: "GMAIL_DELETE_DRAFT", Args: map[string]any{"draft_id": "d1"}})
		evID, err := pt.q.InsertEffectEvent(ctx, db.InsertEffectEventParams{
			WorkspaceID: pt.ws, MemberID: pt.owner.ID, Source: connectedapps.SourceConnectedApp,
			Target: "gmail/GMAIL_CREATE_DRAFT", EffectClass: "write", Reversibility: "reversible",
			Decision: "rehearse", Status: "awaiting", Summary: "draft email",
		})
		if err != nil {
			t.Fatalf("insert effect_event: %v", err)
		}
		pt.Exec(t, `UPDATE effect_event SET undo = $2::jsonb, undo_until = now() + interval '24 hours' WHERE id = $1`, util.UUIDToString(evID), string(undoSpec))
		call := connectedapps.CanonicalCall{Tool: "GMAIL_CREATE_DRAFT", AccountID: "acc1", Args: map[string]any{"body": "hi"}}
		intentID, err := intents.Rehearse(ctx, evID, connectedapps.EffectRequest{
			WS: pt.ws, AgentID: pt.secretary.ID, MemberID: pt.owner.ID, Source: connectedapps.SourceConnectedApp,
			Target: "gmail/GMAIL_CREATE_DRAFT", Class: "write", Reversibility: "reversible", Summary: "draft email",
		}, call, "draft")
		if err != nil {
			t.Fatalf("rehearse: %v", err)
		}
		pred, err := predSvc.Record(ctx, pt.secretary, PredictionInput{
			WorkspaceID: pt.ws, MemberID: pt.owner.ID, SubjectType: "effect_intent", SubjectID: intentID,
			DecisionKind: "effect:write", Predicted: "approve", Confidence: 0.95, Why: "reversible",
		})
		if err != nil || !pred.AppliedByDelegation {
			t.Fatalf("delegated intent approve failed: applied=%v err=%v", pred.AppliedByDelegation, err)
		}

		del := &DelegationService{Queries: pt.q, TxStarter: pt.Pool, Flags: flags, Predictions: predSvc, AppsUndo: undoExec}
		if err := del.OnReverted(ctx, pt.ws, pt.owner, pred.ID); err != nil {
			t.Fatalf("revert intent: %v", err)
		}
		// Calling a second time must not undo twice.
		if err := del.OnReverted(ctx, pt.ws, pt.owner, pred.ID); err != nil {
			t.Fatalf("second revert intent: %v", err)
		}
		if len(undoExec.calls) != 1 || undoExec.calls[0].Tool != "GMAIL_DELETE_DRAFT" {
			t.Fatalf("undo calls = %+v, want 1 GMAIL_DELETE_DRAFT", undoExec.calls)
		}
		if n := pt.Count(t, `SELECT COUNT(*) FROM effect_event WHERE id = $1 AND status = 'undone'`, util.UUIDToString(evID)); n != 1 {
			t.Fatalf("effect_event was not marked undone")
		}
	})

	t.Run("outcome_check revert neutralizes unsettled verification", func(t *testing.T) {
		vf := newVerificationFixture(t)
		ctx := context.Background()
		vf.Exec(t, `UPDATE workspace SET secretary_agent_id = $2 WHERE id = $1`, vf.WorkspaceID, util.MustParseUUID(vf.verifier))
		vf.Exec(t, `INSERT INTO principal_profile (workspace_id, member_id, status, consent_at, share_with_owners) VALUES ($1, $2, 'active', now(), false)`, vf.WorkspaceID, vf.member.ID)
		vf.Cleanup(t, `DELETE FROM principal_delegation WHERE workspace_id = $1`, vf.WorkspaceID)
		vf.Cleanup(t, `DELETE FROM decision_prediction WHERE workspace_id = $1`, vf.WorkspaceID)
		vf.Cleanup(t, `DELETE FROM principal_profile WHERE workspace_id = $1`, vf.WorkspaceID)

		for i := 0; i < 25; i++ {
			vf.Insert(t, "decision_prediction", testutil.Cols{
				"workspace_id": vf.WorkspaceID, "member_id": util.UUIDToString(vf.member.ID),
				"subject_type": "outcome_check", "subject_id": util.UUIDToString(dbid.NewV7()),
				"decision_kind": "outcome", "predicted": "pass", "confidence": 0.95,
				"actual": "pass", "matched": true, "resolved_at": time.Now().Add(-time.Duration(i+1) * time.Minute),
			})
		}
		p := featureflag.NewStaticProvider()
		p.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: true})
		p.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: true})
		flags := featureflag.NewService(p)
		vf.checks.Flags = flags
		predSvc := &PredictionService{Queries: vf.q, TxStarter: vf.Pool, Flags: flags, Checks: vf.checks}
		if err := predSvc.delegation().Enable(ctx, PrincipalActor{Type: GovernanceActorMember, ID: vf.member.ID}, vf.ws, vf.member.ID, "outcome", 0.9); err != nil {
			t.Fatalf("enable: %v", err)
		}

		issue := vf.doneIssue(t, vf.doer)
		c := vf.addCriterion(t, issue, "Manual check", CheckKindManual, map[string]any{})
		outcome := vf.currentOutcome(t, issue)
		pred, err := predSvc.Record(ctx, PrincipalActor{Type: GovernanceActorAgent, ID: util.MustParseUUID(vf.verifier)}, PredictionInput{
			WorkspaceID: vf.ws, MemberID: vf.member.ID, SubjectType: "outcome_check", SubjectID: outcome.ID,
			DecisionKind: "outcome", Predicted: "pass", Confidence: 0.95, Why: "ok",
		})
		if err != nil || !pred.AppliedByDelegation {
			t.Fatalf("delegated outcome check failed: applied=%v err=%v", pred.AppliedByDelegation, err)
		}

		del := predSvc.delegation()
		if err := del.OnReverted(ctx, vf.ws, PrincipalActor{Type: GovernanceActorMember, ID: vf.member.ID}, pred.ID); err != nil {
			t.Fatalf("revert outcome check: %v", err)
		}
		latest, err := vf.q.ListLatestCriterionVerifications(ctx, outcome.ID)
		if err != nil {
			t.Fatalf("list latest verifications: %v", err)
		}
		found := false
		for _, v := range latest {
			if v.CriterionID == c.ID {
				found = true
				if v.Result != "unable" {
					t.Fatalf("latest verification after revert = %q, want unable", v.Result)
				}
			}
		}
		if !found {
			t.Fatalf("expected verification row for criterion %s", util.UUIDToString(c.ID))
		}
	})
}
