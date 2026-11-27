package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// predictionAsk is a question put to the workspace's deciders, about a proposal.
func predictionAsk(pt principalTest, subject pgtype.UUID) AskInput {
	return AskInput{
		WorkspaceID: pt.ws, SubjectType: "proposal", SubjectID: subject, EstimateKind: "budget",
		Reason: "judgment",
	}
}

func TestNoProfileNoPrediction(t *testing.T) {
	pt := newPrincipalTest(t, true)
	ctx := context.Background()
	subject := util.MustParseUUID(pt.Issue(t, "a question"))
	if err := pt.pred().Request(ctx, predictionAsk(pt, subject)); err != nil {
		t.Fatalf("request: %v", err)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE workspace_id = $1`, pt.WorkspaceID); n != 0 {
		t.Fatalf("%d prediction rows for a workspace with no profile", n)
	}
}

func TestNoPredictionAboutSelf(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	subject := util.MustParseUUID(pt.Issue(t, "an org change about the owner's own role"))
	ask := predictionAsk(pt, subject)
	ask.AboutMember = pt.owner.ID
	if err := pt.pred().Request(ctx, ask); err != nil {
		t.Fatalf("request: %v", err)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE subject_id = $1`, util.UUIDToString(subject)); n != 0 {
		t.Fatalf("%d predictions about the person's own question", n)
	}
}

func TestResolveSetsMatch(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	subject := util.MustParseUUID(pt.Issue(t, "a budget question"))
	if err := pt.pred().Request(ctx, predictionAsk(pt, subject)); err != nil {
		t.Fatalf("request: %v", err)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE subject_id = $1 AND member_id = $2 AND resolved_at IS NULL`,
		util.UUIDToString(subject), pt.owner.ID); n != 1 {
		t.Fatalf("pending rows = %d, want 1 for the owner", n)
	}
	if _, err := pt.pred().Record(ctx, pt.secretary, PredictionInput{
		WorkspaceID: pt.ws, MemberID: pt.owner.ID, SubjectType: "proposal", SubjectID: subject,
		DecisionKind: "proposal:budget", Predicted: "approve", Confidence: 0.9, Why: "small spend",
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := pt.pred().Resolve(ctx, pt.ws, "proposal", subject, "approve"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE subject_id = $1 AND matched = true AND resolved_at IS NOT NULL`,
		util.UUIDToString(subject)); n != 1 {
		t.Fatalf("resolved matched rows = %d, want 1", n)
	}
}

func TestPredictionRecordedOnlyByTheSecretary(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	subject := util.MustParseUUID(pt.Issue(t, "a question for the secretary"))
	in := PredictionInput{
		WorkspaceID: pt.ws, MemberID: pt.owner.ID, SubjectType: "proposal", SubjectID: subject,
		DecisionKind: "proposal:budget", Predicted: "approve", Confidence: 0.8,
	}
	if _, err := pt.pred().Record(ctx, pt.owner, in); !errors.Is(err, ErrPrincipalForbidden) {
		t.Fatalf("the person recording a prediction: err = %v, want ErrPrincipalForbidden", err)
	}
	in.Confidence = 1.5
	if _, err := pt.pred().Record(ctx, pt.secretary, in); !errors.Is(err, ErrPredictionInvalid) {
		t.Fatalf("confidence 1.5: err = %v, want ErrPredictionInvalid", err)
	}
}

func TestAccuracyLast50(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	// Sixty resolved predictions of one kind: the newest fifty all matched, the ten older missed.
	pt.Exec(t, `INSERT INTO decision_prediction
		(workspace_id, member_id, subject_type, subject_id, decision_kind, predicted, confidence, actual, matched, resolved_at)
		SELECT $1, $2, 'proposal', gen_random_uuid(), 'proposal:budget', 'approve', 0.9, 'approve', i <= 50,
		       now() - (i * interval '1 minute')
		FROM generate_series(1, 60) AS i`, pt.WorkspaceID, pt.owner.ID)

	matched, n, lower, err := pt.pred().Accuracy(ctx, pt.ws, pt.owner.ID, "proposal:budget")
	if err != nil {
		t.Fatalf("accuracy: %v", err)
	}
	if n != 50 || matched != 50 {
		t.Fatalf("accuracy over %d predictions with %d matched, want 50 of 50", n, matched)
	}
	if lower < 0.92 || lower > 1 {
		t.Fatalf("Wilson lower bound of 50 of 50 = %.3f, want about 0.93", lower)
	}
	if _, n, _, _ := pt.pred().Accuracy(ctx, pt.ws, pt.owner.ID, "outcome"); n != 0 {
		t.Fatalf("another kind counted %d predictions", n)
	}
}

func TestWilsonLowerBound(t *testing.T) {
	if got := wilsonLower(0, 0); got != 0 {
		t.Fatalf("no predictions: %v, want 0", got)
	}
	// 17 of 20 matched: the lower bound is well under the rate, but above 0.6.
	if got := wilsonLower(17, 20); got < 0.6 || got > 0.85 {
		t.Fatalf("wilsonLower(17, 20) = %.3f, want between 0.6 and 0.85", got)
	}
}

func TestPredictionRequestFlagOff(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	pt.svc.Flags = flagsWith(false)
	pred := &PredictionService{Queries: pt.q, Flags: pt.svc.Flags}
	subject := util.MustParseUUID(pt.Issue(t, "a question with the flag off"))
	if err := pred.Request(ctx, predictionAsk(pt, subject)); err != nil {
		t.Fatalf("request with the flag off: %v", err)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE subject_id = $1`, util.UUIDToString(subject)); n != 0 {
		t.Fatalf("%d rows written with the flag off", n)
	}
}

// pred is the prediction service over the same fixture as the profile service.
func (pt principalTest) pred() *PredictionService {
	return &PredictionService{Queries: pt.q, TxStarter: pt.Pool, Flags: pt.svc.Flags}
}

// flagsWith is the flag service with durable_principal set on or off.
func flagsWith(on bool) *featureflag.Service {
	p := featureflag.NewStaticProvider()
	p.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: on})
	return featureflag.NewService(p)
}

// Deleting a profile keeps the match of a resolved prediction and drops its reasons (spec 6.6).
func TestDeleteKeepsMatchCountsOnly(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	subject := util.MustParseUUID(pt.Issue(t, "a question to be forgotten"))
	if err := pt.pred().Request(ctx, predictionAsk(pt, subject)); err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := pt.pred().Record(ctx, pt.secretary, PredictionInput{
		WorkspaceID: pt.ws, MemberID: pt.owner.ID, SubjectType: "proposal", SubjectID: subject,
		DecisionKind: "proposal:budget", Predicted: "approve", Confidence: 0.9, Why: "a reason",
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := pt.pred().Resolve(ctx, pt.ws, "proposal", subject, "approve"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := pt.svc.Delete(ctx, pt.owner, pt.ws, pt.owner.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	subj := util.UUIDToString(subject)
	if n := pt.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE subject_id = $1 AND matched = true`, subj); n != 1 {
		t.Fatalf("the match was lost: %d rows", n)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE subject_id = $1 AND why = '' AND trait_ids = '{}'`, subj); n != 1 {
		t.Fatalf("the reasons were kept")
	}
}

// Several decisions in six hours enqueue one routine task for the Secretary with no chat messages;
// a second run for the same decisions enqueues nothing new (durable spec 6.3, V1.9).
func TestProfileUpdateJobBatches(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	pt.Cleanup(t, `DELETE FROM agent_task_queue WHERE agent_id = $1`, util.UUIDToString(pt.secretary.ID))
	for i := 0; i < 3; i++ {
		pt.Exec(t, `INSERT INTO decision_prediction
			(workspace_id, member_id, subject_type, subject_id, decision_kind, predicted, confidence, actual, matched, resolved_at)
			VALUES ($1, $2, 'proposal', gen_random_uuid(), 'proposal:general', 'approve', 0.9, 'approve', true, now() - interval '1 hour')`,
			pt.WorkspaceID, pt.owner.ID)
	}
	now := time.Now()
	posted, err := pt.pred().LearnBatches(ctx, now)
	if err != nil || posted < 1 {
		t.Fatalf("first run: enqueued=%d err=%v, want >= 1", posted, err)
	}
	secID := util.UUIDToString(pt.secretary.ID)
	if n := pt.Count(t, `SELECT COUNT(*) FROM agent_task_queue WHERE agent_id = $1 AND status = 'queued'`, secID); n != 1 {
		t.Fatalf("after first run: %d queued tasks for secretary, want 1", n)
	}
	if _, err := pt.pred().LearnBatches(ctx, now.Add(time.Hour)); err != nil {
		t.Fatalf("second run for same decisions: err=%v, want nil", err)
	}
	// No learning chat session or assistant message is written.
	if n := pt.Count(t, `SELECT COUNT(*) FROM chat_session WHERE workspace_id = $1 AND title = $2`,
		pt.WorkspaceID, principalLearningTitle); n != 0 {
		t.Fatalf("%d learning chat sessions written, want 0", n)
	}
	// Still exactly one routine task is queued for the workspace Secretary after the second run.
	if n := pt.Count(t, `SELECT COUNT(*) FROM agent_task_queue WHERE agent_id = $1 AND status = 'queued'`, secID); n != 1 {
		t.Fatalf("after second run: %d queued tasks for secretary, want 1", n)
	}
	var taskID string
	pt.QueryRow(t, `SELECT id::text FROM agent_task_queue WHERE agent_id = $1`, secID).Scan(&taskID)
	task, err := pt.q.GetAgentTask(ctx, util.MustParseUUID(taskID))
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	runClass, err := RunClassOfTask(ctx, pt.q, task)
	if err != nil || runClass != RunClassRoutine {
		t.Fatalf("RunClassOfTask = %q (err=%v), want %q", runClass, err, RunClassRoutine)
	}
	var payload struct {
		Kind      string            `json:"kind"`
		RunClass  string            `json:"run_class"`
		Decisions []json.RawMessage `json:"decisions"`
	}
	if err := json.Unmarshal(task.Context, &payload); err != nil {
		t.Fatalf("unmarshal task context: %v (raw=%s)", err, string(task.Context))
	}
	if payload.Kind != "principal_learning" || payload.RunClass != string(RunClassRoutine) {
		t.Fatalf("task context kind=%q run_class=%q, want principal_learning/routine", payload.Kind, payload.RunClass)
	}
	if len(payload.Decisions) != 3 {
		t.Fatalf("task context has %d decisions, want 3", len(payload.Decisions))
	}
}

// TestPrincipalLearningFlagOffWritesNothing: with durable_principal off, LearnBatches enqueues no
// task and writes no chat message (V1.9).
func TestPrincipalLearningFlagOffWritesNothing(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	pt.Cleanup(t, `DELETE FROM agent_task_queue WHERE agent_id = $1`, util.UUIDToString(pt.secretary.ID))
	pt.Exec(t, `INSERT INTO decision_prediction
		(workspace_id, member_id, subject_type, subject_id, decision_kind, predicted, confidence, actual, matched, resolved_at)
		VALUES ($1, $2, 'proposal', gen_random_uuid(), 'proposal:general', 'approve', 0.9, 'approve', true, now() - interval '1 hour')`,
		pt.WorkspaceID, pt.owner.ID)

	pred := &PredictionService{Queries: pt.q, Flags: flagsWith(false)}
	n, err := pred.LearnBatches(ctx, time.Now())
	if err != nil || n != 0 {
		t.Fatalf("LearnBatches with flag off: n=%d err=%v, want 0/nil", n, err)
	}
	if tasks := pt.Count(t, `SELECT COUNT(*) FROM agent_task_queue WHERE agent_id = $1`, util.UUIDToString(pt.secretary.ID)); tasks != 0 {
		t.Fatalf("%d tasks enqueued with flag off, want 0", tasks)
	}
	if chats := pt.Count(t, `SELECT COUNT(*) FROM chat_session WHERE workspace_id = $1 AND title = $2`, pt.WorkspaceID, principalLearningTitle); chats != 0 {
		t.Fatalf("%d learning chats created with flag off, want 0", chats)
	}
}

// TestPrincipalLearningCarriesEditedTraitReadOnly: a person's edited trait is never mutated by the
// learning batch and is carried in the Secretary task context as read-only data (spec 6.3, V1.9).
func TestPrincipalLearningCarriesEditedTraitReadOnly(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	pt.Cleanup(t, `DELETE FROM agent_task_queue WHERE agent_id = $1`, util.UUIDToString(pt.secretary.ID))

	commentID := pt.comment(t, pt.owner)
	learned, err := pt.svc.AddTrait(ctx, pt.secretary, TraitInput{
		WorkspaceID: pt.ws, MemberID: pt.owner.ID, DecisionKind: "proposal:general",
		Statement: "prefers short proposals", Origin: principalOriginLearned,
		Evidence: evidenceOf("comment", commentID),
	})
	if err != nil {
		t.Fatalf("add learned trait: %v", err)
	}
	edited, err := pt.svc.AddTrait(ctx, pt.owner, TraitInput{
		WorkspaceID: pt.ws, MemberID: pt.owner.ID, DecisionKind: "proposal:budget",
		Statement: "never auto-approve over $500", Origin: principalOriginEdited,
		Evidence: evidenceOf("comment", commentID),
	})
	if err != nil {
		t.Fatalf("add edited trait: %v", err)
	}
	pt.Exec(t, `INSERT INTO decision_prediction
		(workspace_id, member_id, subject_type, subject_id, decision_kind, predicted, confidence, actual, matched, resolved_at)
		VALUES ($1, $2, 'proposal', gen_random_uuid(), 'proposal:budget', 'reject', 0.85, 'reject', true, now() - interval '30 minutes')`,
		pt.WorkspaceID, pt.owner.ID)

	n, err := pt.pred().LearnBatches(ctx, time.Now())
	if err != nil || n < 1 {
		t.Fatalf("LearnBatches: n=%d err=%v, want >= 1/nil", n, err)
	}

	// Verify the edited trait row in the DB was not altered or retired.
	gotTrait, err := pt.q.GetPrincipalTrait(ctx, db.GetPrincipalTraitParams{
		ID: edited.ID, WorkspaceID: pt.ws, MemberID: pt.owner.ID,
	})
	if err != nil {
		t.Fatalf("reload edited trait: %v", err)
	}
	if gotTrait.Status != "active" || gotTrait.Origin != principalOriginEdited || gotTrait.Statement != "never auto-approve over $500" {
		t.Fatalf("edited trait mutated: %+v", gotTrait)
	}

	// Verify the Secretary task context carries both traits, marking the edited one read_only=true.
	var ctxJSON []byte
	pt.QueryRow(t, `SELECT context FROM agent_task_queue WHERE agent_id = $1`, util.UUIDToString(pt.secretary.ID)).Scan(&ctxJSON)
	var payload struct {
		Traits []struct {
			ID        string `json:"id"`
			Origin    string `json:"origin"`
			Statement string `json:"statement"`
			ReadOnly  bool   `json:"read_only"`
		} `json:"traits"`
	}
	if err := json.Unmarshal(ctxJSON, &payload); err != nil {
		t.Fatalf("unmarshal task context: %v", err)
	}
	foundEdited, foundLearned := false, false
	for _, tr := range payload.Traits {
		switch tr.ID {
		case util.UUIDToString(edited.ID):
			foundEdited = true
			if tr.Origin != principalOriginEdited || !tr.ReadOnly || tr.Statement != "never auto-approve over $500" {
				t.Fatalf("edited trait in context = %+v, want origin=edited read_only=true", tr)
			}
		case util.UUIDToString(learned.ID):
			foundLearned = true
			if tr.Origin != principalOriginLearned || tr.ReadOnly {
				t.Fatalf("learned trait in context = %+v, want origin=learned read_only=false", tr)
			}
		}
	}
	if !foundEdited || !foundLearned {
		t.Fatalf("missing traits in context: foundEdited=%v foundLearned=%v traits=%+v", foundEdited, foundLearned, payload.Traits)
	}
}

// TestPrincipalLearningSkipsMissingSecretary: a workspace without a Secretary is skipped without
// inventing an agent or enqueuing a task (V1.9).
func TestPrincipalLearningSkipsMissingSecretary(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	pt.Exec(t, `UPDATE workspace SET secretary_agent_id = NULL WHERE id = $1`, pt.WorkspaceID)
	agentsBefore := pt.Count(t, `SELECT COUNT(*) FROM agent WHERE workspace_id = $1`, pt.WorkspaceID)
	pt.Exec(t, `INSERT INTO decision_prediction
		(workspace_id, member_id, subject_type, subject_id, decision_kind, predicted, confidence, actual, matched, resolved_at)
		VALUES ($1, $2, 'proposal', gen_random_uuid(), 'proposal:general', 'approve', 0.9, 'approve', true, now() - interval '1 hour')`,
		pt.WorkspaceID, pt.owner.ID)

	if _, err := pt.pred().LearnBatches(ctx, time.Now()); err != nil {
		t.Fatalf("LearnBatches without secretary: err=%v, want nil", err)
	}
	if agentsAfter := pt.Count(t, `SELECT COUNT(*) FROM agent WHERE workspace_id = $1`, pt.WorkspaceID); agentsAfter != agentsBefore {
		t.Fatalf("agents changed from %d to %d; must not invent an agent", agentsBefore, agentsAfter)
	}
	if tasks := pt.Count(t, `SELECT COUNT(*) FROM agent_task_queue WHERE agent_id IN (SELECT id FROM agent WHERE workspace_id = $1)`, pt.WorkspaceID); tasks != 0 {
		t.Fatalf("%d tasks enqueued without secretary, want 0", tasks)
	}
}

// TestPredictionResolveBeforeRecord: resolving a decision before a prediction is recorded
// persists the resolution state and makes a late Record fail with ErrPredictionLate without
// writing a prediction or creating a second row (V1.8).
func TestPredictionResolveBeforeRecord(t *testing.T) {
	t.Run("pending row from Request resolved before Record", func(t *testing.T) {
		pt := newPrincipalTest(t, true)
		pt.enable(t)
		ctx := context.Background()
		subject := util.MustParseUUID(pt.Issue(t, "decided while pending"))
		if err := pt.pred().Request(ctx, predictionAsk(pt, subject)); err != nil {
			t.Fatalf("request: %v", err)
		}
		if err := pt.pred().Resolve(ctx, pt.ws, "proposal", subject, "approve"); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		_, err := pt.pred().Record(ctx, pt.secretary, PredictionInput{
			WorkspaceID: pt.ws, MemberID: pt.owner.ID, SubjectType: "proposal", SubjectID: subject,
			DecisionKind: "proposal:budget", Predicted: "approve", Confidence: 0.9, Why: "too late",
		})
		if !errors.Is(err, ErrPredictionLate) {
			t.Fatalf("late Record after Resolve: err = %v, want ErrPredictionLate", err)
		}
		subj := util.UUIDToString(subject)
		if n := pt.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE subject_id = $1`, subj); n != 1 {
			t.Fatalf("rows for subject = %d, want 1", n)
		}
		if n := pt.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE subject_id = $1 AND predicted = '' AND actual = 'approve' AND matched IS NULL AND resolved_at IS NOT NULL`, subj); n != 1 {
			t.Fatalf("resolved row was overwritten or missing outcome")
		}
		// Unpredicted resolved decisions must not count toward prediction accuracy.
		if _, n, _, err := pt.pred().Accuracy(ctx, pt.ws, pt.owner.ID, "proposal:budget"); err != nil || n != 0 {
			t.Fatalf("accuracy counted unpredicted decision: n=%d err=%v, want 0", n, err)
		}
	})

	t.Run("Resolve with no prior row blocks late Record", func(t *testing.T) {
		pt := newPrincipalTest(t, true)
		pt.enable(t)
		ctx := context.Background()
		subject := util.MustParseUUID(pt.Issue(t, "decided without pending row"))
		if err := pt.pred().Resolve(ctx, pt.ws, "proposal", subject, "reject"); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		_, err := pt.pred().Record(ctx, pt.secretary, PredictionInput{
			WorkspaceID: pt.ws, MemberID: pt.owner.ID, SubjectType: "proposal", SubjectID: subject,
			DecisionKind: "proposal:budget", Predicted: "reject", Confidence: 0.85, Why: "too late",
		})
		if !errors.Is(err, ErrPredictionLate) {
			t.Fatalf("late Record after Resolve (no prior row): err = %v, want ErrPredictionLate", err)
		}
		subj := util.UUIDToString(subject)
		if n := pt.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE subject_id = $1`, subj); n != 1 {
			t.Fatalf("rows for subject = %d, want 1", n)
		}
		if n := pt.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE subject_id = $1 AND predicted = '' AND actual = 'reject' AND matched IS NULL AND resolved_at IS NOT NULL`, subj); n != 1 {
			t.Fatalf("resolved tombstone missing or mutated")
		}
	})
}

// TestPredictionRecordBeforeResolveAndSecondRecordAfterResolve: Record before Resolve completes
// normally, and a second Record after Resolve returns ErrPredictionLate without creating a second
// row or changing the resolved prediction (V1.8).
func TestPredictionRecordBeforeResolveAndSecondRecordAfterResolve(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	subject := util.MustParseUUID(pt.Issue(t, "normal record then resolve"))
	if err := pt.pred().Request(ctx, predictionAsk(pt, subject)); err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := pt.pred().Record(ctx, pt.secretary, PredictionInput{
		WorkspaceID: pt.ws, MemberID: pt.owner.ID, SubjectType: "proposal", SubjectID: subject,
		DecisionKind: "proposal:budget", Predicted: "approve", Confidence: 0.9, Why: "fits budget",
	}); err != nil {
		t.Fatalf("first record: %v", err)
	}
	if err := pt.pred().Resolve(ctx, pt.ws, "proposal", subject, "approve"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	_, err := pt.pred().Record(ctx, pt.secretary, PredictionInput{
		WorkspaceID: pt.ws, MemberID: pt.owner.ID, SubjectType: "proposal", SubjectID: subject,
		DecisionKind: "proposal:budget", Predicted: "reject", Confidence: 0.7, Why: "second attempt",
	})
	if !errors.Is(err, ErrPredictionLate) {
		t.Fatalf("second Record after Resolve: err = %v, want ErrPredictionLate", err)
	}
	subj := util.UUIDToString(subject)
	if n := pt.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE subject_id = $1`, subj); n != 1 {
		t.Fatalf("rows for subject = %d, want 1", n)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE subject_id = $1 AND predicted = 'approve' AND actual = 'approve' AND matched = true AND resolved_at IS NOT NULL`, subj); n != 1 {
		t.Fatalf("resolved prediction was modified by second Record")
	}
}

// TestPredictionResolveAndRecordFlagOff: with durable_principal off, neither Resolve nor Record
// writes anything (V1.8).
func TestPredictionResolveAndRecordFlagOff(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	pred := &PredictionService{Queries: pt.q, Flags: flagsWith(false)}
	subject := util.MustParseUUID(pt.Issue(t, "flag off decision"))
	if err := pred.Resolve(ctx, pt.ws, "proposal", subject, "approve"); err != nil {
		t.Fatalf("resolve with flag off: %v", err)
	}
	if _, err := pred.Record(ctx, pt.secretary, PredictionInput{
		WorkspaceID: pt.ws, MemberID: pt.owner.ID, SubjectType: "proposal", SubjectID: subject,
		DecisionKind: "proposal:budget", Predicted: "approve", Confidence: 0.9,
	}); !errors.Is(err, ErrPrincipalDisabled) {
		t.Fatalf("record with flag off: err = %v, want ErrPrincipalDisabled", err)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE subject_id = $1`, util.UUIDToString(subject)); n != 0 {
		t.Fatalf("%d rows written with flag off, want 0", n)
	}
}
