package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/featureflag"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// Decision predictions (durable spec 6.4, F5). When a question is admitted, each owner or admin with
// an active profile gets a pending prediction row. The Secretary fills it in with what the person
// would decide; the person's decision resolves it, and the match rate of the last 50 says how far
// the predictions can be trusted (delegation, F5 part 5, reads it).

const (
	// principalAccuracyWindow is how many resolved predictions the accuracy reads (spec 6.4).
	principalAccuracyWindow = 50
	// predictionReasonMax bounds the why of a prediction (spec 6.2).
	predictionReasonMax = 1000
	// predictionValueMax bounds the predicted outcome text.
	predictionValueMax = 64
	// wilsonZ is the z of a 95% interval.
	wilsonZ = 1.959963984540054
)

var (
	// ErrPredictionInvalid: a prediction breaks a rule of spec 6.2 (predicted value, confidence, why, kind).
	ErrPredictionInvalid = errors.New("invalid decision prediction")
	// ErrPredictionLate: the decision was already resolved before the prediction arrived.
	ErrPredictionLate = errors.New("decision already resolved")
)

// predictionSubjectTypes are the questions a prediction can be made for (spec 6.2).
var predictionSubjectTypes = map[string]bool{
	"proposal": true, "effect_intent": true, "outcome_check": true, "inquiry": true,
}

// PredictionInput is the Secretary's prediction of one decision.
type PredictionInput struct {
	WorkspaceID  pgtype.UUID
	MemberID     pgtype.UUID
	SubjectType  string
	SubjectID    pgtype.UUID
	DecisionKind string
	Predicted    string
	Confidence   float64
	Why          string
	TraitIDs     []pgtype.UUID
	TaskID       pgtype.UUID
}

// PredictionService makes, records, resolves and measures the predictions. Flags is read on each
// call, so the flag can change while the service runs.
type PredictionService struct {
	Queries   *db.Queries
	TxStarter TxStarter
	Tasks     *TaskService
	Flags     *featureflag.Service
}

func (s *PredictionService) on(ctx context.Context) bool {
	return featureflags.DurablePrincipalEnabled(ctx, s.Flags)
}

// predictionKind is the decision kind a question is predicted under: the kind of the proposal,
// effect or inquiry, as the profile's traits name them.
func predictionKind(ask AskInput) string {
	switch ask.SubjectType {
	case "proposal":
		if ask.EstimateKind != "" && ask.EstimateKind != "proposal" {
			return "proposal:" + ask.EstimateKind
		}
		return "proposal:general"
	case "effect_intent":
		return "effect:" + firstNonEmpty(ask.EstimateKind, "write")
	case "outcome_check":
		return "outcome"
	case "inquiry":
		return "inquiry"
	}
	return "general"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// Request makes a pending prediction for each profiled decider of the workspace, for a question just
// admitted. Nothing is made about the person the question is about (spec 6.4), and nothing while
// durable_principal is off or when no decider has a profile.
func (s *PredictionService) Request(ctx context.Context, ask AskInput) error {
	if !s.on(ctx) || !predictionSubjectTypes[ask.SubjectType] || !ask.SubjectID.Valid {
		return nil
	}
	deciders, err := s.Queries.ListProfiledDeciders(ctx, ask.WorkspaceID)
	if err != nil {
		return fmt.Errorf("list profiled deciders: %w", err)
	}
	kind := predictionKind(ask)
	for _, member := range deciders {
		if ask.AboutMember.Valid && principalSameUUID(ask.AboutMember, member) {
			continue
		}
		if _, err := s.Queries.InsertPendingPrediction(ctx, db.InsertPendingPredictionParams{
			ID: dbid.NewV7(), WorkspaceID: ask.WorkspaceID, MemberID: member,
			SubjectType: ask.SubjectType, SubjectID: ask.SubjectID, DecisionKind: kind,
		}); err != nil {
			return fmt.Errorf("request prediction: %w", err)
		}
	}
	return nil
}

// Record stores the Secretary's prediction for a pending question. Only the Secretary records one,
// and only for a person whose profile is active. A resolved question keeps its outcome.
func (s *PredictionService) Record(ctx context.Context, actor PrincipalActor, in PredictionInput) (db.DecisionPrediction, error) {
	if !s.on(ctx) {
		return db.DecisionPrediction{}, ErrPrincipalDisabled
	}
	if actor.Type != GovernanceActorAgent || !s.isSecretaryAgent(ctx, in.WorkspaceID, actor.ID) {
		return db.DecisionPrediction{}, ErrPrincipalForbidden
	}
	predicted := strings.TrimSpace(in.Predicted)
	why := strings.TrimSpace(in.Why)
	if predicted == "" || utf8.RuneCountInString(predicted) > predictionValueMax ||
		in.Confidence < 0 || in.Confidence > 1 || utf8.RuneCountInString(why) > predictionReasonMax ||
		!predictionSubjectTypes[in.SubjectType] || !validPrincipalKind(in.DecisionKind) {
		return db.DecisionPrediction{}, ErrPredictionInvalid
	}
	profile, err := s.Queries.GetPrincipalProfile(ctx, db.GetPrincipalProfileParams{WorkspaceID: in.WorkspaceID, MemberID: in.MemberID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && profile.Status != "active") {
		return db.DecisionPrediction{}, ErrPrincipalNoProfile
	}
	if err != nil {
		return db.DecisionPrediction{}, fmt.Errorf("load principal profile: %w", err)
	}
	row, err := s.Queries.RecordDecisionPrediction(ctx, db.RecordDecisionPredictionParams{
		ID: dbid.NewV7(), WorkspaceID: in.WorkspaceID, MemberID: in.MemberID,
		SubjectType: in.SubjectType, SubjectID: in.SubjectID, DecisionKind: in.DecisionKind,
		Predicted: predicted, Confidence: predictionNumeric(in.Confidence), Why: why,
		TraitIds: traitIDsOrEmpty(in.TraitIDs), ProfileVersion: profile.Version, PredictorTaskID: in.TaskID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// The question is already decided: its prediction keeps the outcome it had.
		return db.DecisionPrediction{}, ErrPredictionLate
	}
	if err != nil {
		return db.DecisionPrediction{}, fmt.Errorf("record prediction: %w", err)
	}
	if in.SubjectType == "inquiry" {
		row = s.delegateInquiry(ctx, row)
	}
	return row, nil
}

// Resolve records the person's decision on a question against the predictions made for it, and
// persists the resolution state for active profiles even if no prediction existed yet so a late
// Record is refused with ErrPredictionLate.
func (s *PredictionService) Resolve(ctx context.Context, ws pgtype.UUID, subjectType string, subjectID pgtype.UUID, actual string) error {
	if !s.on(ctx) {
		return nil
	}
	actual = strings.TrimSpace(actual)
	if actual == "" || !predictionSubjectTypes[subjectType] || !subjectID.Valid {
		return ErrPredictionInvalid
	}
	actualText := pgtype.Text{String: actual, Valid: true}
	if _, err := s.Queries.ResolveDecisionPredictions(ctx, db.ResolveDecisionPredictionsParams{
		Actual: actualText, WorkspaceID: ws, SubjectType: subjectType, SubjectID: subjectID,
	}); err != nil {
		return fmt.Errorf("resolve predictions: %w", err)
	}
	if _, err := s.Queries.InsertResolvedPredictionTombstones(ctx, db.InsertResolvedPredictionTombstonesParams{
		WorkspaceID: ws, SubjectType: subjectType, SubjectID: subjectID, Actual: actualText,
	}); err != nil {
		return fmt.Errorf("record resolved decision outcome: %w", err)
	}
	return nil
}

// ErrPredictionNone: the person has no prediction of that question (none was made, or it said nothing).
var ErrPredictionNone = errors.New("no prediction for this question")

// Prediction reads the person's own prediction of one question (GET /api/principal/predictions). A
// question with no predicted decision has no prediction to show.
func (s *PredictionService) Prediction(ctx context.Context, ws, member pgtype.UUID, subjectType string, subjectID pgtype.UUID) (db.DecisionPrediction, error) {
	if !predictionSubjectTypes[subjectType] {
		return db.DecisionPrediction{}, ErrPredictionInvalid
	}
	pred, err := s.Queries.GetDecisionPrediction(ctx, db.GetDecisionPredictionParams{
		WorkspaceID: ws, MemberID: member, SubjectType: subjectType, SubjectID: subjectID,
	})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && pred.Predicted == "") {
		return db.DecisionPrediction{}, ErrPredictionNone
	}
	if err != nil {
		return db.DecisionPrediction{}, fmt.Errorf("read prediction: %w", err)
	}
	return pred, nil
}

// Accuracy reads a person's prediction accuracy for one decision kind over the last 50 resolved
// predictions: how many matched, how many there are, and the Wilson lower bound of the match rate.
func (s *PredictionService) Accuracy(ctx context.Context, ws, member pgtype.UUID, kind string) (matched, n int, lower float64, err error) {
	rows, err := s.Queries.ListRecentResolvedPredictions(ctx, db.ListRecentResolvedPredictionsParams{
		WorkspaceID: ws, MemberID: member, DecisionKind: kind,
	})
	if err != nil {
		return 0, 0, 0, fmt.Errorf("read prediction accuracy: %w", err)
	}
	for _, r := range rows {
		if r.Bool {
			matched++
		}
	}
	n = len(rows)
	return matched, n, wilsonLower(matched, n), nil
}

// wilsonLower is the lower bound of the 95% Wilson interval of matched out of n (spec 6.4). With no
// predictions there is no bound to show, so it is 0.
func wilsonLower(matched, n int) float64 {
	if n == 0 {
		return 0
	}
	nf := float64(n)
	p := float64(matched) / nf
	z2 := wilsonZ * wilsonZ
	centre := p + z2/(2*nf)
	margin := wilsonZ * math.Sqrt(p*(1-p)/nf+z2/(4*nf*nf))
	return (centre - margin) / (1 + z2/nf)
}

// isSecretaryAgent reports whether the agent is the workspace's Secretary.
func (s *PredictionService) isSecretaryAgent(ctx context.Context, ws, agent pgtype.UUID) bool {
	secretary, err := s.Queries.GetWorkspaceSecretaryAgentID(ctx, ws)
	return err == nil && principalSameUUID(secretary, agent)
}

// predictionNumeric stores a confidence with two decimals, the precision a prediction is made to.
func predictionNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(fmt.Sprintf("%.2f", f)); err != nil {
		slog.Warn("prediction confidence not stored", "error", err)
	}
	return n
}

// traitIDsOrEmpty stores no traits as an empty array: the column is NOT NULL, and a nil slice is NULL.
func traitIDsOrEmpty(ids []pgtype.UUID) []pgtype.UUID {
	if ids == nil {
		return []pgtype.UUID{}
	}
	return ids
}

// delegation is the delegation service over this prediction service's queries.
func (s *PredictionService) delegation() *DelegationService {
	return &DelegationService{Queries: s.Queries, Flags: s.Flags, Predictions: s}
}

// principalLearningTitle titles the Secretary routine task that updates a person's profile.
const (
	principalLearningTitle = "Cập nhật hồ sơ"
	principalLearningKind  = "principal_learning"
)

// PrincipalLearningDecisionContext is one resolved decision carried in a learning routine task.
type PrincipalLearningDecisionContext struct {
	ID           string `json:"id"`
	SubjectType  string `json:"subject_type"`
	SubjectID    string `json:"subject_id"`
	DecisionKind string `json:"decision_kind"`
	Predicted    string `json:"predicted"`
	Actual       string `json:"actual"`
	Matched      *bool  `json:"matched,omitempty"`
	ResolvedAt   string `json:"resolved_at"`
}

// PrincipalLearningTraitContext is one active trait carried in a learning routine task. Traits
// edited by the person carry ReadOnly=true and must never be overwritten or retired by Secretary.
type PrincipalLearningTraitContext struct {
	ID           string `json:"id"`
	DecisionKind string `json:"decision_kind"`
	Statement    string `json:"statement"`
	Origin       string `json:"origin"`
	ReadOnly     bool   `json:"read_only"`
}

// PrincipalLearningTaskContext is the JSON context carried by a Secretary routine task for
// updating a principal profile (durable spec 6.3, V1.9).
type PrincipalLearningTaskContext struct {
	Kind           string                             `json:"kind"`
	RunClass       string                             `json:"run_class"`
	Title          string                             `json:"title"`
	WorkspaceID    string                             `json:"workspace_id"`
	MemberID       string                             `json:"member_id"`
	ProfileVersion int32                              `json:"profile_version"`
	Prompt         string                             `json:"prompt"`
	Decisions      []PrincipalLearningDecisionContext `json:"decisions"`
	Traits         []PrincipalLearningTraitContext    `json:"traits"`
}

// LearnBatches runs the profile update job over every active profile: each person with unlearned
// decisions resolved in the six-hour window gets one routine task enqueued for the workspace
// Secretary (durable spec 6.3, V1.9). It returns the number of tasks enqueued.
func (s *PredictionService) LearnBatches(ctx context.Context, now time.Time) (int, error) {
	if !s.on(ctx) {
		return 0, nil
	}
	profiles, err := s.Queries.ListActivePrincipalProfiles(ctx)
	if err != nil {
		return 0, fmt.Errorf("list active profiles: %w", err)
	}
	enqueued := 0
	for _, p := range profiles {
		ok, err := s.learnBatch(ctx, p.WorkspaceID, p.MemberID, now)
		if err != nil {
			return enqueued, err
		}
		if ok {
			enqueued++
		}
	}
	return enqueued, nil
}

// learnBatch enqueues a routine task for the workspace Secretary covering unlearned decisions
// resolved in the last six hours. Decisions are stamped with learning_task_id so re-running the
// job never enqueues a duplicate task for the same decisions.
func (s *PredictionService) learnBatch(ctx context.Context, ws, member pgtype.UUID, now time.Time) (bool, error) {
	secretaryID, err := s.Queries.GetWorkspaceSecretaryAgentID(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !secretaryID.Valid) {
		slog.Warn("principal learning: workspace has no secretary agent; skipping", "workspace_id", util.UUIDToString(ws))
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load workspace secretary: %w", err)
	}
	secretary, err := s.Queries.GetAgent(ctx, secretaryID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (secretary.WorkspaceID != ws || secretary.ArchivedAt.Valid || !secretary.RuntimeID.Valid)) {
		slog.Warn("principal learning: workspace secretary unavailable; skipping",
			"workspace_id", util.UUIDToString(ws), "secretary_id", util.UUIDToString(secretaryID))
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load secretary agent: %w", err)
	}

	since := now.Add(-predictionBatchWindow)
	rows, err := s.Queries.ListUnlearnedResolvedPredictionsSince(ctx, db.ListUnlearnedResolvedPredictionsSinceParams{
		WorkspaceID: ws, MemberID: member, Since: pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("list resolved predictions: %w", err)
	}
	if len(rows) == 0 {
		return false, nil
	}

	profile, err := s.Queries.GetPrincipalProfile(ctx, db.GetPrincipalProfileParams{WorkspaceID: ws, MemberID: member})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && profile.Status != "active") {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load principal profile: %w", err)
	}
	traits, err := s.Queries.ListActivePrincipalTraits(ctx, db.ListActivePrincipalTraitsParams{WorkspaceID: ws, MemberID: member})
	if err != nil {
		return false, fmt.Errorf("list active principal traits: %w", err)
	}

	contextJSON, err := buildPrincipalLearningContext(ws, member, profile.Version, rows, traits)
	if err != nil {
		return false, fmt.Errorf("encode learning task context: %w", err)
	}
	decisionIDs := make([]pgtype.UUID, len(rows))
	for i, r := range rows {
		decisionIDs[i] = r.ID
	}

	taskID := dbid.NewV7()
	summary := fmt.Sprintf("%s: %d quyết định", principalLearningTitle, len(rows))
	var task db.AgentTaskQueue
	enqueued := false

	runEnqueue := func(q *db.Queries) error {
		marked, err := q.MarkPredictionsLearned(ctx, db.MarkPredictionsLearnedParams{
			LearningTaskID: taskID, WorkspaceID: ws, MemberID: member, Ids: decisionIDs,
		})
		if err != nil {
			return fmt.Errorf("mark decisions learned: %w", err)
		}
		if marked == 0 {
			return nil
		}
		created, err := q.EnqueuePrincipalLearningTask(ctx, db.EnqueuePrincipalLearningTaskParams{
			ID:             taskID,
			AgentID:        secretary.ID,
			WorkspaceID:    ws,
			TriggerSummary: pgtype.Text{String: summary, Valid: true},
			Context:        contextJSON,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("principal learning: secretary became unavailable during enqueue; skipping",
				"workspace_id", util.UUIDToString(ws), "secretary_id", util.UUIDToString(secretary.ID))
			if s.TxStarter == nil {
				_, _ = q.ClearPredictionsLearned(ctx, db.ClearPredictionsLearnedParams{
					WorkspaceID: ws, MemberID: member, LearningTaskID: taskID,
				})
			}
			return errLearningSecretaryUnavailable
		}
		if err != nil {
			if s.TxStarter == nil {
				_, _ = q.ClearPredictionsLearned(ctx, db.ClearPredictionsLearnedParams{
					WorkspaceID: ws, MemberID: member, LearningTaskID: taskID,
				})
			}
			return fmt.Errorf("enqueue principal learning task: %w", err)
		}
		task = created
		enqueued = true
		return nil
	}

	if s.TxStarter != nil {
		tx, err := s.TxStarter.Begin(ctx)
		if err != nil {
			return false, fmt.Errorf("begin learning batch tx: %w", err)
		}
		defer tx.Rollback(ctx)
		if err := runEnqueue(s.Queries.WithTx(tx)); err != nil {
			if errors.Is(err, errLearningSecretaryUnavailable) {
				return false, nil
			}
			return false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit learning batch tx: %w", err)
		}
	} else {
		if err := runEnqueue(s.Queries); err != nil {
			if errors.Is(err, errLearningSecretaryUnavailable) {
				return false, nil
			}
			return false, err
		}
	}

	if enqueued && s.Tasks != nil {
		s.Tasks.broadcastTaskEvent(ctx, protocol.EventTaskQueued, task)
		s.Tasks.NotifyTaskEnqueued(ctx, task)
	}
	return enqueued, nil
}

var errLearningSecretaryUnavailable = errors.New("secretary unavailable")

func buildPrincipalLearningContext(
	ws, member pgtype.UUID,
	profileVersion int32,
	rows []db.ListUnlearnedResolvedPredictionsSinceRow,
	traits []db.PrincipalTrait,
) ([]byte, error) {
	decisions := make([]PrincipalLearningDecisionContext, len(rows))
	for i, r := range rows {
		var matched *bool
		if r.Matched.Valid {
			v := r.Matched.Bool
			matched = &v
		}
		decisions[i] = PrincipalLearningDecisionContext{
			ID:           util.UUIDToString(r.ID),
			SubjectType:  r.SubjectType,
			SubjectID:    util.UUIDToString(r.SubjectID),
			DecisionKind: r.DecisionKind,
			Predicted:    r.Predicted,
			Actual:       r.Actual.String,
			Matched:      matched,
			ResolvedAt:   r.ResolvedAt.Time.UTC().Format(time.RFC3339),
		}
	}
	traitCtx := make([]PrincipalLearningTraitContext, len(traits))
	for i, t := range traits {
		traitCtx[i] = PrincipalLearningTraitContext{
			ID:           util.UUIDToString(t.ID),
			DecisionKind: t.DecisionKind,
			Statement:    t.Statement,
			Origin:       t.Origin,
			ReadOnly:     t.Origin == principalOriginEdited,
		}
	}
	payload := PrincipalLearningTaskContext{
		Kind:           principalLearningKind,
		RunClass:       string(RunClassRoutine),
		Title:          principalLearningTitle,
		WorkspaceID:    util.UUIDToString(ws),
		MemberID:       util.UUIDToString(member),
		ProfileVersion: profileVersion,
		Prompt:         learningBatchPrompt(rows),
		Decisions:      decisions,
		Traits:         traitCtx,
	}
	return json.Marshal(payload)
}

func learningBatchPrompt(rows []db.ListUnlearnedResolvedPredictionsSinceRow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Cập nhật hồ sơ: %d quyết định mới trong 6 giờ qua.\n", len(rows))
	b.WriteString("Đọc các quyết định, ghi chú và các trait hiện có trong context; đề xuất thêm, sửa (retire + tạo mới kèm bằng chứng) hoặc retire trait đã học.\n")
	b.WriteString("Không bao giờ sửa hoặc retire các trait có read_only=true (origin=edited): chỉ chính người đó mới được retire.\n")
	for _, r := range rows {
		verdict := "chưa có dự đoán"
		if r.Matched.Valid {
			if r.Matched.Bool {
				verdict = "khớp dự đoán"
			} else {
				verdict = "lệch dự đoán"
			}
		}
		fmt.Fprintf(&b, "- %s (%s): dự đoán %s, quyết định %s, %s\n", r.DecisionKind, r.SubjectType, r.Predicted, r.Actual.String, verdict)
	}
	return b.String()
}

// predictionBatchWindow is how often a person gets a profile update batch (spec 6.3).
const predictionBatchWindow = 6 * time.Hour
