package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
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
	if s.Flags == nil {
		return false
	}
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

// DelegatedDecision names the prediction a decision path settles as the person's own (spec 6.5). The
// decision must be the one predicted, on a question still open, for a kind the person delegated.
type DelegatedDecision struct {
	PredictionID pgtype.UUID
}

// Settle runs one decision on a question and then resolves the predictions made for it. With a
// delegation, the prediction must be eligible before decide runs, and it is marked applied by
// delegation once decide succeeds. The person is the actor of the decision; the governance log names
// the principal as the one who made it (durable spec 6.5). An ineligible decision is refused, and
// decide does not run.
func (s *DelegationService) Settle(ctx context.Context, ws pgtype.UUID, d *DelegatedDecision, subjectType string, subjectID, decider pgtype.UUID, decision string, ask AskInput, decide func() error) error {
	var pred db.DecisionPrediction
	if d != nil {
		var err error
		if pred, err = s.eligible(ctx, ws, d.PredictionID, subjectID, subjectType, decider, decision, ask); err != nil {
			return err
		}
	}
	if err := decide(); err != nil {
		return err
	}
	s.resolve(ctx, ws, subjectType, subjectID, decision)
	if d == nil {
		return nil
	}
	return s.applied(ctx, pred)
}

// eligible loads the delegated prediction and checks every rule of delegation: the kind is delegated,
// not forbidden, the prediction is open and predicts this decision for this question, the decider is
// the person it was made for, and it is confident enough.
func (s *DelegationService) eligible(ctx context.Context, ws, predID, subjectID pgtype.UUID, subjectType string, decider pgtype.UUID, decision string, ask AskInput) (db.DecisionPrediction, error) {
	if !s.on(ctx) {
		return db.DecisionPrediction{}, ErrPrincipalDisabled
	}
	pred, err := s.Queries.GetDecisionPredictionByID(ctx, db.GetDecisionPredictionByIDParams{ID: predID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.DecisionPrediction{}, ErrPredictionInvalid
	}
	if err != nil {
		return db.DecisionPrediction{}, fmt.Errorf("load delegated prediction: %w", err)
	}
	if pred.SubjectType != subjectType || !principalSameUUID(pred.SubjectID, subjectID) ||
		pred.ResolvedAt.Valid || pred.AppliedByDelegation || pred.Predicted != decision ||
		ForbiddenDelegationKind(pred.DecisionKind, ask) {
		return db.DecisionPrediction{}, ErrDelegationNotEligible
	}
	member, err := s.Queries.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{UserID: decider, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !principalSameUUID(member.ID, pred.MemberID)) {
		return db.DecisionPrediction{}, ErrPrincipalForbidden
	}
	if err != nil {
		return db.DecisionPrediction{}, fmt.Errorf("load decider: %w", err)
	}
	delegation, err := s.Queries.GetActivePrincipalDelegation(ctx, db.GetActivePrincipalDelegationParams{
		WorkspaceID: ws, MemberID: pred.MemberID, DecisionKind: pred.DecisionKind,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.DecisionPrediction{}, ErrDelegationNotEligible
	}
	if err != nil {
		return db.DecisionPrediction{}, fmt.Errorf("load delegation: %w", err)
	}
	confidence, err := pred.Confidence.Float64Value()
	if err != nil {
		return db.DecisionPrediction{}, fmt.Errorf("read prediction confidence: %w", err)
	}
	minConf, err := delegation.MinConfidence.Float64Value()
	if err != nil {
		return db.DecisionPrediction{}, fmt.Errorf("read delegation confidence: %w", err)
	}
	if !confidence.Valid || !minConf.Valid || confidence.Float64 < minConf.Float64 {
		return db.DecisionPrediction{}, ErrDelegationNotEligible
	}
	return pred, nil
}

// resolve records the decision against the predictions made for the question. The decision stands
// if this fails, so the failure is logged and not returned.
func (s *DelegationService) resolve(ctx context.Context, ws pgtype.UUID, subjectType string, subjectID pgtype.UUID, decision string) {
	if s.Predictions == nil {
		return
	}
	if err := s.Predictions.Resolve(ctx, ws, subjectType, subjectID, decision); err != nil {
		slog.Warn("decision predictions not resolved", "subject_type", subjectType, "subject_id", util.UUIDToString(subjectID), "error", err)
	}
}

// applied marks a delegated prediction as applied and logs the decision with the principal as the
// one who made it. The person can revert it for 72 hours from the moment it resolved.
func (s *DelegationService) applied(ctx context.Context, pred db.DecisionPrediction) error {
	n, err := s.Queries.MarkPredictionAppliedByDelegation(ctx, db.MarkPredictionAppliedByDelegationParams{
		ID: pred.ID, WorkspaceID: pred.WorkspaceID,
	})
	if err != nil {
		return fmt.Errorf("mark delegated prediction: %w", err)
	}
	if n == 0 {
		return ErrDelegationNotEligible
	}
	return s.logEvent(ctx, pred.WorkspaceID, pred.MemberID, GovernanceKindPrincipalDelegated,
		"decision_prediction", pred.ID, map[string]any{
			"decision_kind": pred.DecisionKind, "decision": pred.Predicted, "subject_type": pred.SubjectType,
			"acting_as": "principal", "revert_window_hours": int(delegationRevertWindow / time.Hour),
		})
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

// decideSettle settles a proposal's approval or rejection through the delegation gate (spec 6.5).
func (s *ProposalService) decideSettle(ctx context.Context, p ProposalDecideParams) (ProposalDecideResult, error) {
	var res ProposalDecideResult
	err := s.delegation().Settle(ctx, p.WorkspaceID, p.Delegated, "proposal", p.ProposalID, p.DeciderID, p.Decision, AskInput{}, func() error {
		var err error
		res, err = s.settle(ctx, p)
		return err
	})
	return res, err
}

// delegation is the delegation gate for proposals. Without a prediction service, nothing is
// predicted, so no decision can be delegated.
func (s *ProposalService) delegation() *DelegationService {
	if s.Principal == nil {
		return &DelegationService{Queries: s.Issues.Queries}
	}
	return s.Principal.delegation()
}

// ApproveAsPrincipal approves a pending intent as the person's own decision, when the person delegated
// its kind (spec 6.5). Irreversible intents are never delegated.
func (s *IntentService) ApproveAsPrincipal(ctx context.Context, ws, decider, id pgtype.UUID, d DelegatedDecision) (db.EffectIntent, error) {
	it, err := s.Get(ctx, ws, id)
	if err != nil {
		return db.EffectIntent{}, err
	}
	var out db.EffectIntent
	err = s.delegation().Settle(ctx, ws, &d, "effect_intent", id, decider, "approve",
		AskInput{Irreversible: it.Reversibility == "irreversible"}, func() error {
			var err error
			out, err = s.Approve(ctx, ws, decider, id, nil, "")
			return err
		})
	return out, err
}

// delegation is the delegation gate for intents.
func (s *IntentService) delegation() *DelegationService {
	return &DelegationService{Queries: s.Queries, Flags: s.Flags, Predictions: &PredictionService{Queries: s.Queries, Flags: s.Flags}}
}

// RecordAsPrincipal records a manual check on an outcome as the person's own verdict, when the person
// delegated checks of that kind (spec 6.5).
func (s *VerificationService) RecordAsPrincipal(ctx context.Context, ws, decider, outcomeID pgtype.UUID, in VerificationInput, d DelegatedDecision) (db.OutcomeVerification, error) {
	var out db.OutcomeVerification
	err := s.delegation().Settle(ctx, ws, &d, "outcome_check", outcomeID, decider, in.Result, AskInput{}, func() error {
		var err error
		out, err = s.Record(ctx, ws, GovernanceActor{Type: GovernanceActorMember, ID: decider}, outcomeID, in)
		return err
	})
	return out, err
}

// delegation is the delegation gate for manual checks.
func (s *VerificationService) delegation() *DelegationService {
	return &DelegationService{Queries: s.Queries, Flags: s.Flags, Predictions: &PredictionService{Queries: s.Queries, Flags: s.Flags}}
}
