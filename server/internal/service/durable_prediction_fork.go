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
	proposalkinds "github.com/multica-ai/multica/server/internal/service/proposal_kinds"
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
	// Proposals, Intents, and Checks are optional targets used when an eligible prediction
	// auto-applies a delegated decision (durable spec 6.5).
	Proposals *ProposalService
	Intents   *IntentService
	Checks    *VerificationService
}

func (s *PredictionService) on(ctx context.Context) bool {
	return featureflags.DurablePrincipalEnabled(ctx, s.Flags)
}

func proposalDecisionKind(kind string) string {
	switch kind {
	case "", "proposal", proposalkinds.KeyDecision:
		return "proposal:general"
	default:
		return "proposal:" + kind
	}
}

func effectDecisionKind(class string) string {
	return "effect:" + firstNonEmpty(class, "write")
}

// validSubjectDecisionKind checks that decision_kind has the exact prefix or value required by
// subject_type (durable spec 6.2, 6.4).
func validSubjectDecisionKind(subjectType, kind string) bool {
	if !validPrincipalKind(kind) {
		return false
	}
	switch subjectType {
	case "proposal":
		return strings.HasPrefix(kind, "proposal:")
	case "effect_intent":
		return strings.HasPrefix(kind, "effect:")
	case "outcome_check":
		return kind == "outcome"
	case "inquiry":
		return kind == "inquiry"
	default:
		return false
	}
}

// lookupSubjectDecisionKind resolves the authoritative decision_kind and delegation constraints of
// a subject row in workspace ws. found is false when the subject does not exist in ws.
func lookupSubjectDecisionKind(ctx context.Context, q *db.Queries, ws pgtype.UUID, subjectType string, subjectID pgtype.UUID) (kind string, ask AskInput, found bool, err error) {
	if !subjectID.Valid {
		return "", AskInput{}, false, nil
	}
	switch subjectType {
	case "proposal":
		prop, err := q.GetProposalInWorkspace(ctx, db.GetProposalInWorkspaceParams{ID: subjectID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			return "", AskInput{}, false, nil
		}
		if err != nil {
			return "", AskInput{}, false, fmt.Errorf("load proposal subject: %w", err)
		}
		return proposalDecisionKind(prop.Kind), AskInput{SubjectType: "proposal", Urgent: prop.Urgent}, true, nil
	case "effect_intent":
		it, err := q.GetEffectIntent(ctx, db.GetEffectIntentParams{ID: subjectID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			return "", AskInput{}, false, nil
		}
		if err != nil {
			return "", AskInput{}, false, fmt.Errorf("load effect intent subject: %w", err)
		}
		return effectDecisionKind(it.EffectClass), AskInput{SubjectType: "effect_intent", Irreversible: it.Reversibility == "irreversible"}, true, nil
	case "outcome_check":
		_, err := q.GetWorkOutcomeInWorkspace(ctx, db.GetWorkOutcomeInWorkspaceParams{ID: subjectID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			return "", AskInput{}, false, nil
		}
		if err != nil {
			return "", AskInput{}, false, fmt.Errorf("load outcome subject: %w", err)
		}
		return "outcome", AskInput{SubjectType: "outcome_check"}, true, nil
	case "inquiry":
		exists, err := q.ExistsBriefInquiryInWorkspace(ctx, db.ExistsBriefInquiryInWorkspaceParams{ID: subjectID, WorkspaceID: ws})
		if err != nil {
			return "", AskInput{}, false, fmt.Errorf("load inquiry subject: %w", err)
		}
		if !exists {
			return "", AskInput{}, false, nil
		}
		return "inquiry", AskInput{SubjectType: "inquiry"}, true, nil
	default:
		return "", AskInput{}, false, nil
	}
}

// predictionKind is the decision kind a question is predicted under: the kind of the proposal,
// effect or inquiry, as the profile's traits name them.
func predictionKind(ask AskInput) string {
	switch ask.SubjectType {
	case "proposal":
		return proposalDecisionKind(ask.EstimateKind)
	case "effect_intent":
		return effectDecisionKind(ask.EstimateKind)
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
// admitted, and enqueues a lightweight routine prediction task for the workspace Secretary
// (spec 6.4). Nothing is made about the person the question is about, and nothing while
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
	if subjectKind, _, found, err := lookupSubjectDecisionKind(ctx, s.Queries, ask.WorkspaceID, ask.SubjectType, ask.SubjectID); err != nil {
		return err
	} else if found {
		kind = subjectKind
	}
	var pendingMembers []pgtype.UUID
	for _, member := range deciders {
		if ask.AboutMember.Valid && principalSameUUID(ask.AboutMember, member) {
			continue
		}
		n, err := s.Queries.InsertPendingPrediction(ctx, db.InsertPendingPredictionParams{
			ID: dbid.NewV7(), WorkspaceID: ask.WorkspaceID, MemberID: member,
			SubjectType: ask.SubjectType, SubjectID: ask.SubjectID, DecisionKind: kind,
		})
		if err != nil {
			return fmt.Errorf("request prediction: %w", err)
		}
		if n > 0 {
			pendingMembers = append(pendingMembers, member)
		}
	}
	if len(pendingMembers) == 0 {
		return nil
	}
	return s.enqueuePredictionTask(ctx, ask, kind, pendingMembers)
}

// principalPredictionKind and principalPredictionTitle identify Secretary routine prediction tasks.
const (
	principalPredictionTitle = "Dự đoán quyết định"
	principalPredictionKind  = "principal_prediction"
)

// PrincipalPredictionTargetContext carries the traits and recent decisions for one profiled decider.
type PrincipalPredictionTargetContext struct {
	MemberID        string                             `json:"member_id"`
	Traits          []PrincipalLearningTraitContext    `json:"traits"`
	RecentDecisions []PrincipalLearningDecisionContext `json:"recent_decisions"`
}

// PrincipalPredictionTaskContext is the JSON context carried by a Secretary routine task for
// predicting decisions on a newly admitted question (durable spec 6.4).
type PrincipalPredictionTaskContext struct {
	Kind         string                             `json:"kind"`
	RunClass     string                             `json:"run_class"`
	Title        string                             `json:"title"`
	WorkspaceID  string                             `json:"workspace_id"`
	SubjectType  string                             `json:"subject_type"`
	SubjectID    string                             `json:"subject_id"`
	DecisionKind string                             `json:"decision_kind"`
	Prompt       string                             `json:"prompt"`
	Targets      []PrincipalPredictionTargetContext `json:"targets"`
}

// enqueuePredictionTask queues a routine task for the workspace Secretary following the standard
// row-lock order (agent first, then lock_task_owner_rows inside EnqueuePrincipalLearningTask).
func (s *PredictionService) enqueuePredictionTask(ctx context.Context, ask AskInput, kind string, members []pgtype.UUID) error {
	secretaryID, err := s.Queries.GetWorkspaceSecretaryAgentID(ctx, ask.WorkspaceID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !secretaryID.Valid) {
		slog.Warn("principal prediction: workspace has no secretary agent; skipping task",
			"workspace_id", util.UUIDToString(ask.WorkspaceID))
		return nil
	}
	if err != nil {
		return fmt.Errorf("load workspace secretary: %w", err)
	}
	secretary, err := s.Queries.GetAgent(ctx, secretaryID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (secretary.WorkspaceID != ask.WorkspaceID || secretary.ArchivedAt.Valid || !secretary.RuntimeID.Valid)) {
		slog.Warn("principal prediction: workspace secretary unavailable; skipping task",
			"workspace_id", util.UUIDToString(ask.WorkspaceID), "secretary_id", util.UUIDToString(secretaryID))
		return nil
	}
	if err != nil {
		return fmt.Errorf("load secretary agent: %w", err)
	}

	targets := make([]PrincipalPredictionTargetContext, 0, len(members))
	for _, member := range members {
		allTraits, err := s.Queries.ListActivePrincipalTraits(ctx, db.ListActivePrincipalTraitsParams{
			WorkspaceID: ask.WorkspaceID, MemberID: member,
		})
		if err != nil {
			return fmt.Errorf("list active principal traits: %w", err)
		}
		traits := make([]PrincipalLearningTraitContext, 0, len(allTraits))
		for _, t := range allTraits {
			if t.DecisionKind != kind && t.DecisionKind != "general" {
				continue
			}
			traits = append(traits, PrincipalLearningTraitContext{
				ID:           util.UUIDToString(t.ID),
				DecisionKind: t.DecisionKind,
				Statement:    t.Statement,
				Origin:       t.Origin,
				ReadOnly:     t.Origin == principalOriginEdited,
			})
		}
		recentRows, err := s.Queries.ListRecentResolvedDecisionsForKind(ctx, db.ListRecentResolvedDecisionsForKindParams{
			WorkspaceID: ask.WorkspaceID, MemberID: member, DecisionKind: kind,
		})
		if err != nil {
			return fmt.Errorf("list recent resolved decisions: %w", err)
		}
		recent := make([]PrincipalLearningDecisionContext, len(recentRows))
		for i, r := range recentRows {
			var matched *bool
			if r.Matched.Valid {
				v := r.Matched.Bool
				matched = &v
			}
			recent[i] = PrincipalLearningDecisionContext{
				ID:           util.UUIDToString(r.ID),
				SubjectType:  r.SubjectType,
				SubjectID:    util.UUIDToString(r.SubjectID),
				DecisionKind: kind,
				Predicted:    r.Predicted,
				Actual:       r.Actual.String,
				Matched:      matched,
				ResolvedAt:   r.ResolvedAt.Time.UTC().Format(time.RFC3339),
			}
		}
		targets = append(targets, PrincipalPredictionTargetContext{
			MemberID:        util.UUIDToString(member),
			Traits:          traits,
			RecentDecisions: recent,
		})
	}

	subjectIDStr := util.UUIDToString(ask.SubjectID)
	payload := PrincipalPredictionTaskContext{
		Kind:         principalPredictionKind,
		RunClass:     string(RunClassRoutine),
		Title:        principalPredictionTitle,
		WorkspaceID:  util.UUIDToString(ask.WorkspaceID),
		SubjectType:  ask.SubjectType,
		SubjectID:    subjectIDStr,
		DecisionKind: kind,
		Prompt: fmt.Sprintf(
			"Dự đoán quyết định cho %s (%s, kind=%s). Đọc traits và 10 quyết định gần nhất trong context rồi ghi kết quả qua `multica principal predict --subject %s:%s --predicted <value> --confidence <0..1> --why <reason> --traits <ids>`.",
			ask.SubjectType, subjectIDStr, kind, ask.SubjectType, subjectIDStr,
		),
		Targets: targets,
	}
	contextJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode prediction task context: %w", err)
	}

	taskID := dbid.NewV7()
	summary := fmt.Sprintf("%s: %s (%s)", principalPredictionTitle, kind, ask.SubjectType)
	var task db.AgentTaskQueue
	enqueued := false

	runEnqueue := func(q *db.Queries) error {
		created, err := q.EnqueuePrincipalLearningTask(ctx, db.EnqueuePrincipalLearningTaskParams{
			ID:             taskID,
			AgentID:        secretary.ID,
			WorkspaceID:    ask.WorkspaceID,
			TriggerSummary: pgtype.Text{String: summary, Valid: true},
			Context:        contextJSON,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errLearningSecretaryUnavailable
		}
		if err != nil {
			return fmt.Errorf("enqueue principal prediction task: %w", err)
		}
		if _, err := q.SetPendingPredictionTaskID(ctx, db.SetPendingPredictionTaskIDParams{
			PredictorTaskID: created.ID,
			WorkspaceID:     ask.WorkspaceID,
			SubjectType:     ask.SubjectType,
			SubjectID:       ask.SubjectID,
			MemberIds:       members,
		}); err != nil {
			return fmt.Errorf("link pending prediction task: %w", err)
		}
		task = created
		enqueued = true
		return nil
	}

	if s.TxStarter != nil {
		tx, err := s.TxStarter.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin prediction enqueue tx: %w", err)
		}
		defer tx.Rollback(ctx)
		if err := runEnqueue(s.Queries.WithTx(tx)); err != nil {
			if errors.Is(err, errLearningSecretaryUnavailable) {
				return nil
			}
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit prediction enqueue tx: %w", err)
		}
	} else {
		if err := runEnqueue(s.Queries); err != nil {
			if errors.Is(err, errLearningSecretaryUnavailable) {
				return nil
			}
			return err
		}
	}

	if enqueued && s.Tasks != nil {
		s.Tasks.broadcastTaskEvent(ctx, protocol.EventTaskQueued, task)
		s.Tasks.NotifyTaskEnqueued(ctx, task)
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
		!predictionSubjectTypes[in.SubjectType] || !validSubjectDecisionKind(in.SubjectType, in.DecisionKind) {
		return db.DecisionPrediction{}, ErrPredictionInvalid
	}
	profile, err := s.Queries.GetPrincipalProfile(ctx, db.GetPrincipalProfileParams{WorkspaceID: in.WorkspaceID, MemberID: in.MemberID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && profile.Status != "active") {
		return db.DecisionPrediction{}, ErrPrincipalNoProfile
	}
	if err != nil {
		return db.DecisionPrediction{}, fmt.Errorf("load principal profile: %w", err)
	}
	if existing, err := s.Queries.GetDecisionPrediction(ctx, db.GetDecisionPredictionParams{
		WorkspaceID: in.WorkspaceID, MemberID: in.MemberID, SubjectType: in.SubjectType, SubjectID: in.SubjectID,
	}); err == nil {
		if existing.ResolvedAt.Valid {
			return db.DecisionPrediction{}, ErrPredictionLate
		}
		if existing.DecisionKind != in.DecisionKind {
			return db.DecisionPrediction{}, ErrPredictionInvalid
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return db.DecisionPrediction{}, fmt.Errorf("load existing prediction: %w", err)
	}
	subjectKind, _, found, err := lookupSubjectDecisionKind(ctx, s.Queries, in.WorkspaceID, in.SubjectType, in.SubjectID)
	if err != nil {
		return db.DecisionPrediction{}, err
	}
	if !found || subjectKind != in.DecisionKind {
		return db.DecisionPrediction{}, ErrPredictionInvalid
	}
	row, err := s.Queries.RecordDecisionPrediction(ctx, db.RecordDecisionPredictionParams{
		ID: dbid.NewV7(), WorkspaceID: in.WorkspaceID, MemberID: in.MemberID,
		SubjectType: in.SubjectType, SubjectID: in.SubjectID, DecisionKind: in.DecisionKind,
		Predicted: predicted, Confidence: predictionNumeric(in.Confidence), Why: why,
		TraitIds: traitIDsOrEmpty(in.TraitIDs), ProfileVersion: profile.Version, PredictorTaskID: in.TaskID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, gErr := s.Queries.GetDecisionPrediction(ctx, db.GetDecisionPredictionParams{
			WorkspaceID: in.WorkspaceID, MemberID: in.MemberID, SubjectType: in.SubjectType, SubjectID: in.SubjectID,
		})
		if gErr == nil && existing.ResolvedAt.Valid {
			// The question is already decided: its prediction keeps the outcome it had.
			return db.DecisionPrediction{}, ErrPredictionLate
		}
		return db.DecisionPrediction{}, ErrPredictionInvalid
	}
	if err != nil {
		return db.DecisionPrediction{}, fmt.Errorf("record prediction: %w", err)
	}
	row = s.maybeAutoDelegate(ctx, row)
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
	owners, err := s.Queries.ListResolvedPredictionOwnersForSubject(ctx, db.ListResolvedPredictionOwnersForSubjectParams{
		WorkspaceID: ws, SubjectType: subjectType, SubjectID: subjectID,
	})
	if err != nil {
		return fmt.Errorf("list resolved prediction owners: %w", err)
	}
	for _, owner := range owners {
		if err := s.delegation().ReviewAccuracy(ctx, ws, owner.MemberID, owner.DecisionKind); err != nil {
			return fmt.Errorf("review delegation accuracy: %w", err)
		}
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
	return &DelegationService{
		Queries:     s.Queries,
		TxStarter:   s.TxStarter,
		Flags:       s.Flags,
		Predictions: s,
		Checks:      s.Checks,
	}
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
// decisions gets one routine task enqueued for the workspace
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
// at any time. Decisions are stamped with learning_task_id so re-running the
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

	rows, err := s.Queries.ListUnlearnedResolvedPredictions(ctx, db.ListUnlearnedResolvedPredictionsParams{
		WorkspaceID: ws, MemberID: member,
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
	rows []db.ListUnlearnedResolvedPredictionsRow,
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

func learningBatchPrompt(rows []db.ListUnlearnedResolvedPredictionsRow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Cập nhật hồ sơ: %d quyết định chưa được học.\n", len(rows))
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
