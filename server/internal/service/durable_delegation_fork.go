package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// Delegation by prediction (durable spec 6.5, F5). A person lets the predictor decide one kind of
// question, once their predictions for that kind have proved right: at least 20 resolved and at
// least 85% matched. A delegated decision is applied as the person's own, is logged, and can be
// reverted for 72 hours. Two reverts in the last twenty delegated decisions, or an accuracy under
// 80%, turn the delegation off.

const (
	// delegationMinResolved is the fewest resolved predictions a kind needs before it can be delegated.
	delegationMinResolved = 20
	// delegationMinMatchRate is the match rate a kind needs before it can be delegated (spec 6.5).
	delegationMinMatchRate = 0.85
	// delegationDisableRate turns a delegation off when its accuracy falls under it (spec 6.5).
	delegationDisableRate = 0.80
	// delegationRevertLimit turns a delegation off at this many reverts in the last twenty (spec 6.5).
	delegationRevertLimit = 2
	// delegationDefaultMinConfidence is the confidence a prediction needs to be applied (spec 6.2).
	delegationDefaultMinConfidence = 0.90
	// delegationRevertWindow is how long after a delegated decision the person may undo it (spec 6.5).
	delegationRevertWindow = 72 * time.Hour

	GovernanceKindPrincipalDelegated        = "principal_delegated_decision"
	GovernanceKindPrincipalDelegationOn     = "principal_delegation_enabled"
	GovernanceKindPrincipalDelegationOff    = "principal_delegation_disabled"
	GovernanceKindPrincipalDelegationRevert = "principal_delegation_reverted"
)

func init() {
	for _, kind := range []string{
		GovernanceKindPrincipalDelegated, GovernanceKindPrincipalDelegationOn,
		GovernanceKindPrincipalDelegationOff, GovernanceKindPrincipalDelegationRevert,
	} {
		governanceKinds[kind] = true
	}
}

var (
	// ErrDelegationNotEligible: the kind lacks 20 resolved predictions or an 85% match rate.
	ErrDelegationNotEligible = errors.New("delegation needs 20 resolved predictions of this kind with at least 85% matched")
	// ErrDelegationForbiddenKind: the kind may never be delegated (spec 6.5).
	ErrDelegationForbiddenKind = errors.New("this kind of decision cannot be delegated")
	// ErrDelegationRevertWindow: the 72 hours to undo a delegated decision have passed.
	ErrDelegationRevertWindow = errors.New("a delegated decision can be reverted within 72 hours")
)

// forbiddenDelegationTerms are the words of a kind that never delegate: budget, autonomy, envelope,
// policy, profile, the launch and stage gates, org changes, and spend and share effects (spec 6.5).
var forbiddenDelegationTerms = []string{
	"budget", "autonomy", "envelope", "policy", "profile",
	"launch", "stage_gate", "org_change", "effect:spend", "effect:share",
}

// ForbiddenDelegationKind reports whether a decision of this kind, asked this way, must stay with the
// person. Irreversible intents and urgent questions never delegate, whatever their kind.
func ForbiddenDelegationKind(kind string, ask AskInput) bool {
	if ask.Irreversible || ask.Urgent {
		return true
	}
	lower := strings.ToLower(kind)
	for _, term := range forbiddenDelegationTerms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

// DelegationService turns delegation on and off, and applies a delegated decision.
type DelegationService struct {
	Queries     *db.Queries
	Flags       *featureflag.Service
	Predictions *PredictionService
}

func (s *DelegationService) on(ctx context.Context) bool {
	return featureflags.DurablePrincipalEnabled(ctx, s.Flags)
}

// Enable delegates a kind for the person. Only the person does it, and only when their predictions
// of the kind have proved right (the rule is checked here, not in the caller). minConf of 0 means the
// default 0.90.
func (s *DelegationService) Enable(ctx context.Context, actor PrincipalActor, ws, member pgtype.UUID, kind string, minConf float64) error {
	if !s.on(ctx) {
		return ErrPrincipalDisabled
	}
	if actor.Type != GovernanceActorMember || !principalSameUUID(actor.ID, member) {
		return ErrPrincipalForbidden
	}
	if !validPrincipalKind(kind) {
		return ErrPredictionInvalid
	}
	if ForbiddenDelegationKind(kind, AskInput{}) {
		return ErrDelegationForbiddenKind
	}
	if minConf == 0 {
		minConf = delegationDefaultMinConfidence
	}
	if minConf <= 0 || minConf > 1 {
		return ErrPredictionInvalid
	}
	if _, err := s.Queries.GetPrincipalProfile(ctx, db.GetPrincipalProfileParams{WorkspaceID: ws, MemberID: member}); errors.Is(err, pgx.ErrNoRows) {
		return ErrPrincipalNoProfile
	} else if err != nil {
		return fmt.Errorf("load principal profile: %w", err)
	}
	matched, n, _, err := s.Predictions.Accuracy(ctx, ws, member, kind)
	if err != nil {
		return err
	}
	if n < delegationMinResolved || float64(matched) < delegationMinMatchRate*float64(n) {
		return ErrDelegationNotEligible
	}
	if _, err := s.Queries.InsertPrincipalDelegation(ctx, db.InsertPrincipalDelegationParams{
		ID: dbid.NewV7(), WorkspaceID: ws, MemberID: member, DecisionKind: kind,
		MinConfidence: predictionNumeric(minConf),
	}); err != nil {
		return fmt.Errorf("enable delegation: %w", err)
	}
	return s.logEvent(ctx, ws, member, GovernanceKindPrincipalDelegationOn, "principal_delegation", member, map[string]any{
		"decision_kind": kind, "min_confidence": minConf, "matched": matched, "resolved": n,
	})
}

// MaybeApply decides whether a prediction is applied as the person's own decision. It is applied when
// the kind is delegated, the question is not forbidden, and the prediction is confident enough. The
// caller records the decision; this marks the prediction and logs it, with the "Undo" window of 72
// hours the FYI shows.
func (s *DelegationService) MaybeApply(ctx context.Context, pred db.DecisionPrediction, ask AskInput) (bool, error) {
	if !s.on(ctx) || pred.Predicted == "" || ForbiddenDelegationKind(pred.DecisionKind, ask) {
		return false, nil
	}
	delegation, err := s.Queries.GetActivePrincipalDelegation(ctx, db.GetActivePrincipalDelegationParams{
		WorkspaceID: pred.WorkspaceID, MemberID: pred.MemberID, DecisionKind: pred.DecisionKind,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load delegation: %w", err)
	}
	confidence, err := pred.Confidence.Float64Value()
	if err != nil {
		return false, fmt.Errorf("read prediction confidence: %w", err)
	}
	minConf, err := delegation.MinConfidence.Float64Value()
	if err != nil {
		return false, fmt.Errorf("read delegation confidence: %w", err)
	}
	if !confidence.Valid || !minConf.Valid || confidence.Float64 < minConf.Float64 {
		return false, nil
	}
	if _, err := s.Queries.MarkPredictionAppliedByDelegation(ctx, db.MarkPredictionAppliedByDelegationParams{
		ID: pred.ID, WorkspaceID: pred.WorkspaceID,
	}); err != nil {
		return false, fmt.Errorf("mark delegated prediction: %w", err)
	}
	if err := s.logEvent(ctx, pred.WorkspaceID, pred.MemberID, GovernanceKindPrincipalDelegated,
		"decision_prediction", pred.ID, map[string]any{
			"decision_kind": pred.DecisionKind, "decision": pred.Predicted, "confidence": confidence.Float64,
		}); err != nil {
		return false, err
	}
	return true, nil
}

// OnReverted records that the person undid a delegated decision, within 72 hours of it. Two such
// reverts in the last twenty delegated decisions of a kind turn its delegation off, with the reason
// in the row and the log. Only the person whose decision it was may revert it.
func (s *DelegationService) OnReverted(ctx context.Context, ws pgtype.UUID, actor PrincipalActor, predID pgtype.UUID) error {
	pred, err := s.Queries.GetDecisionPredictionByID(ctx, db.GetDecisionPredictionByIDParams{ID: predID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !pred.AppliedByDelegation) {
		return ErrPredictionInvalid
	}
	if err != nil {
		return fmt.Errorf("load delegated prediction: %w", err)
	}
	if actor.Type != GovernanceActorMember || !principalSameUUID(actor.ID, pred.MemberID) {
		return ErrPrincipalForbidden
	}
	if !pred.ResolvedAt.Valid || time.Since(pred.ResolvedAt.Time) > delegationRevertWindow {
		return ErrDelegationRevertWindow
	}
	if err := s.logEvent(ctx, ws, pred.MemberID, GovernanceKindPrincipalDelegationRevert,
		"decision_prediction", pred.ID, map[string]any{"decision_kind": pred.DecisionKind}); err != nil {
		return err
	}
	recent, err := s.Queries.ListRecentDelegatedPredictionIDs(ctx, db.ListRecentDelegatedPredictionIDsParams{
		WorkspaceID: ws, MemberID: pred.MemberID, DecisionKind: pred.DecisionKind,
	})
	if err != nil {
		return fmt.Errorf("list delegated decisions: %w", err)
	}
	reverts, err := s.Queries.CountDelegationReverts(ctx, db.CountDelegationRevertsParams{WorkspaceID: ws, SubjectIds: recent})
	if err != nil {
		return fmt.Errorf("count reverts: %w", err)
	}
	if reverts >= delegationRevertLimit {
		return s.disable(ctx, ws, pred.MemberID, pred.DecisionKind,
			fmt.Sprintf("reverted %d of the last %d delegated decisions", reverts, len(recent)))
	}
	return nil
}

// ReviewAccuracy turns a delegation off when the kind's accuracy over the last 50 resolved predictions
// falls under 80%. Fewer than the 20 predictions a kind needs to be delegated is not enough to judge.
func (s *DelegationService) ReviewAccuracy(ctx context.Context, ws, member pgtype.UUID, kind string) error {
	if _, err := s.Queries.GetActivePrincipalDelegation(ctx, db.GetActivePrincipalDelegationParams{
		WorkspaceID: ws, MemberID: member, DecisionKind: kind,
	}); errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("load delegation: %w", err)
	}
	matched, n, _, err := s.Predictions.Accuracy(ctx, ws, member, kind)
	if err != nil {
		return err
	}
	if n < delegationMinResolved || float64(matched) >= delegationDisableRate*float64(n) {
		return nil
	}
	return s.disable(ctx, ws, member, kind, fmt.Sprintf("accuracy fell to %d of %d matched", matched, n))
}

// Disable turns a delegation off at the person's word. Only the person does it.
func (s *DelegationService) Disable(ctx context.Context, actor PrincipalActor, ws, member pgtype.UUID, kind string) error {
	if !s.on(ctx) {
		return ErrPrincipalDisabled
	}
	if actor.Type != GovernanceActorMember || !principalSameUUID(actor.ID, member) {
		return ErrPrincipalForbidden
	}
	return s.disable(ctx, ws, member, kind, "turned off by the person")
}

// disable turns the delegation of a kind off with its reason, and logs it.
func (s *DelegationService) disable(ctx context.Context, ws, member pgtype.UUID, kind, reason string) error {
	n, err := s.Queries.DisablePrincipalDelegation(ctx, db.DisablePrincipalDelegationParams{
		WorkspaceID: ws, MemberID: member, DecisionKind: kind, DisabledReason: reason,
	})
	if err != nil {
		return fmt.Errorf("disable delegation: %w", err)
	}
	if n == 0 {
		return nil
	}
	return s.logEvent(ctx, ws, member, GovernanceKindPrincipalDelegationOff, "principal_delegation", member, map[string]any{
		"decision_kind": kind, "reason": reason,
	})
}

// logEvent writes one governance entry for the person's delegation about subject.
func (s *DelegationService) logEvent(ctx context.Context, ws, member pgtype.UUID, kind, subjectType string, subject pgtype.UUID, details map[string]any) error {
	return NewGovernanceLog().Record(ctx, s.Queries, GovernanceEvent{
		WorkspaceID: ws, Kind: kind,
		ActorType: GovernanceActorMember, ActorID: member,
		SubjectType: subjectType, SubjectID: subject,
		Summary: "Principal delegation: " + kind, Details: details,
	})
}
