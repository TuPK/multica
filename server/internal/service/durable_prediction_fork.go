package service

import (
	"context"
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
	Queries *db.Queries
	Flags   *featureflag.Service
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
		return db.DecisionPrediction{}, ErrPredictionInvalid
	}
	if err != nil {
		return db.DecisionPrediction{}, fmt.Errorf("record prediction: %w", err)
	}
	return row, nil
}

// Resolve records the person's decision on a question against the predictions made for it.
func (s *PredictionService) Resolve(ctx context.Context, ws pgtype.UUID, subjectType string, subjectID pgtype.UUID, actual string) error {
	if !s.on(ctx) {
		return nil
	}
	actual = strings.TrimSpace(actual)
	if actual == "" {
		return ErrPredictionInvalid
	}
	if _, err := s.Queries.ResolveDecisionPredictions(ctx, db.ResolveDecisionPredictionsParams{
		Actual: pgtype.Text{String: actual, Valid: true}, WorkspaceID: ws, SubjectType: subjectType, SubjectID: subjectID,
	}); err != nil {
		return fmt.Errorf("resolve predictions: %w", err)
	}
	return nil
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

// resolvePredictions resolves the open predictions of a decided proposal, after the decision commits.
// A failure is logged and never fails the decision.
func (s *ProposalService) resolvePredictions(ctx context.Context, ws, proposal pgtype.UUID, decision string) {
	if s.Principal == nil {
		return
	}
	if err := s.Principal.Resolve(ctx, ws, "proposal", proposal, decision); err != nil {
		slog.Warn("decision predictions not resolved", "proposal_id", util.UUIDToString(proposal), "error", err)
	}
}

// principalLearningTitle titles the chat where the Secretary posts a person's profile update batches.
const principalLearningTitle = "Cập nhật hồ sơ"

// LearnBatches runs the profile update job over every active profile: each person gets at most one
// batch per six hours, holding the decisions resolved in that window (durable spec 6.3). It returns
// the number of batches posted.
func (s *PredictionService) LearnBatches(ctx context.Context, now time.Time) (int, error) {
	if !s.on(ctx) {
		return 0, nil
	}
	profiles, err := s.Queries.ListActivePrincipalProfiles(ctx)
	if err != nil {
		return 0, fmt.Errorf("list active profiles: %w", err)
	}
	posted := 0
	for _, p := range profiles {
		ok, err := s.learnBatch(ctx, p.WorkspaceID, p.MemberID, now)
		if err != nil {
			return posted, err
		}
		if ok {
			posted++
		}
	}
	return posted, nil
}

// learnBatch posts the person's batch if decisions were resolved in the last six hours and no batch
// was posted in that window. It reports whether it posted one.
func (s *PredictionService) learnBatch(ctx context.Context, ws, member pgtype.UUID, now time.Time) (bool, error) {
	secretary, err := s.Queries.GetWorkspaceSecretaryAgentID(ctx, ws)
	if err != nil || !secretary.Valid {
		return false, nil
	}
	since := now.Add(-predictionBatchWindow)
	rows, err := s.Queries.ListResolvedPredictionsSince(ctx, db.ListResolvedPredictionsSinceParams{
		WorkspaceID: ws, MemberID: member, Since: pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("list resolved predictions: %w", err)
	}
	if len(rows) == 0 {
		return false, nil
	}
	session, err := s.learningSession(ctx, ws, member, secretary)
	if err != nil {
		return false, err
	}
	last, err := s.Queries.LatestAssistantMessageAt(ctx, session.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("read last batch: %w", err)
	}
	if err == nil && last.Valid && last.Time.After(since) {
		return false, nil
	}
	if _, err := s.Queries.CreateChatMessage(ctx, db.CreateChatMessageParams{
		ChatSessionID: session.ID, Role: "assistant", Content: learningBatchText(rows), ID: dbid.NewV7(),
	}); err != nil {
		return false, fmt.Errorf("post profile update batch: %w", err)
	}
	return true, nil
}

// learningSession is the person's profile update chat with the Secretary, opened on first use.
func (s *PredictionService) learningSession(ctx context.Context, ws, member, secretary pgtype.UUID) (db.ChatSession, error) {
	session, err := s.Queries.GetPrincipalInterviewSession(ctx, db.GetPrincipalInterviewSessionParams{
		WorkspaceID: ws, MemberID: member, AgentID: secretary, Title: principalLearningTitle,
	})
	if err == nil {
		return session, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.ChatSession{}, fmt.Errorf("load learning chat: %w", err)
	}
	created, err := s.Queries.CreateChatSession(ctx, db.CreateChatSessionParams{
		WorkspaceID: ws, AgentID: secretary, CreatorID: member, Title: principalLearningTitle, ID: dbid.NewV7(),
	})
	if err != nil {
		return db.ChatSession{}, fmt.Errorf("open learning chat: %w", err)
	}
	return created, nil
}

// learningBatchText is one batch: the decisions it holds, one line each, and whether the prediction
// matched.
func learningBatchText(rows []db.ListResolvedPredictionsSinceRow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Cập nhật hồ sơ: %d quyết định trong 6 giờ qua.\n", len(rows))
	for _, r := range rows {
		verdict := "lệch dự đoán"
		if r.Matched.Valid && r.Matched.Bool {
			verdict = "khớp dự đoán"
		}
		fmt.Fprintf(&b, "- %s (%s): dự đoán %s, quyết định %s, %s\n", r.DecisionKind, r.SubjectType, r.Predicted, r.Actual.String, verdict)
	}
	return b.String()
}

// predictionBatchWindow is how often a person gets a profile update batch (spec 6.3).
const predictionBatchWindow = 6 * time.Hour
