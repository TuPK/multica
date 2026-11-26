package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
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
	return &PredictionService{Queries: pt.q, Flags: pt.svc.Flags}
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

// Several decisions in six hours make one profile update batch; the next one waits for the window
// (durable spec 6.3).
func TestProfileUpdateJobBatches(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		pt.Exec(t, `INSERT INTO decision_prediction
			(workspace_id, member_id, subject_type, subject_id, decision_kind, predicted, confidence, actual, matched, resolved_at)
			VALUES ($1, $2, 'proposal', gen_random_uuid(), 'proposal:general', 'approve', 0.9, 'approve', true, now() - interval '1 hour')`,
			pt.WorkspaceID, pt.owner.ID)
	}
	now := time.Now()
	posted, err := pt.pred().learnBatch(ctx, pt.ws, pt.owner.ID, now)
	if err != nil || !posted {
		t.Fatalf("first batch: posted=%v err=%v, want one batch", posted, err)
	}
	if again, err := pt.pred().learnBatch(ctx, pt.ws, pt.owner.ID, now.Add(time.Hour)); err != nil || again {
		t.Fatalf("second batch within six hours: posted=%v err=%v, want none", again, err)
	}
	var session string
	pt.QueryRow(t, `SELECT id FROM chat_session WHERE workspace_id = $1 AND creator_id = $2 AND title = $3`,
		pt.WorkspaceID, util.UUIDToString(pt.owner.ID), principalLearningTitle).Scan(&session)
	if n := pt.Count(t, `SELECT COUNT(*) FROM chat_message WHERE chat_session_id = $1 AND role = 'assistant'`, session); n != 1 {
		t.Fatalf("%d batches in the learning chat, want 1", n)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM chat_message WHERE chat_session_id = $1 AND content LIKE 'Cập nhật hồ sơ: 3 quyết định%'`, session); n != 1 {
		t.Fatalf("the batch does not hold the three decisions")
	}
}
