package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/connectedapps"
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
	// ErrDelegationUndoInProgress: another request is undoing this decision's effect now.
	ErrDelegationUndoInProgress = errors.New("the undo of this delegated decision is already in progress")
	// ErrDelegationNotUndoable: the effect has no recorded undo, so it cannot be taken back.
	// ErrDelegationOutcomeSettled: a settled outcome's verdict is set again by hand, not reverted.
	ErrDelegationOutcomeSettled = errors.New("a settled outcome's verdict must be set again by hand")
	ErrDelegationNotUndoable    = errors.New("this delegated decision's effect cannot be undone")
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

// IntentUndoExecutor runs the inverse tool call of an executed reversible effect (durable spec 4.6, 6.5).
type IntentUndoExecutor interface {
	ExecuteUndo(ctx context.Context, workspaceID, memberID pgtype.UUID, target string, spec connectedapps.UndoSpec) (map[string]any, error)
}

// DelegationService turns delegation on and off, and applies a delegated decision.
type DelegationService struct {
	Queries     *db.Queries
	TxStarter   TxStarter
	Flags       *featureflag.Service
	Predictions *PredictionService
	AppsUndo    IntentUndoExecutor
	Checks      *VerificationService
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
	if !delegationEligible(matched, n) {
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

// delegationEligible is the rule for delegating a kind (spec 6.5): 20 resolved predictions of it,
// at least 85% of them matched.
func delegationEligible(matched, n int) bool {
	return n >= delegationMinResolved && float64(matched) >= delegationMinMatchRate*float64(n)
}

// DelegationKindAccuracy is one decision kind the person has predictions of: the match record over
// the accuracy window, its Wilson lower bound, and whether the kind may be delegated.
type DelegationKindAccuracy struct {
	DecisionKind string
	Matched      int
	N            int
	Lower        float64
	Eligible     bool
}

// Status reads the person's delegations, one row per kind (the active one, or the latest disabled),
// and the accuracy of every kind they have predictions of, for the delegation panel.
func (s *DelegationService) Status(ctx context.Context, ws, member pgtype.UUID) ([]db.PrincipalDelegation, []DelegationKindAccuracy, error) {
	delegations, err := s.Queries.ListPrincipalDelegationsLatest(ctx, db.ListPrincipalDelegationsLatestParams{WorkspaceID: ws, MemberID: member})
	if err != nil {
		return nil, nil, fmt.Errorf("list delegations: %w", err)
	}
	kinds, err := s.Queries.ListPredictionKinds(ctx, db.ListPredictionKindsParams{WorkspaceID: ws, MemberID: member})
	if err != nil {
		return nil, nil, fmt.Errorf("list prediction kinds: %w", err)
	}
	accuracy := make([]DelegationKindAccuracy, 0, len(kinds))
	for _, kind := range kinds {
		matched, n, lower, err := s.Predictions.Accuracy(ctx, ws, member, kind)
		if err != nil {
			return nil, nil, err
		}
		accuracy = append(accuracy, DelegationKindAccuracy{
			DecisionKind: kind, Matched: matched, N: n, Lower: lower, Eligible: delegationEligible(matched, n),
		})
	}
	return delegations, accuracy, nil
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
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !principalSameUUID(member.ID, pred.MemberID) && !principalSameUUID(member.UserID, pred.MemberID)) {
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
// in the row and the log. Only the person whose decision it was may revert it. Calling OnReverted a
// second time for the same prediction is a no-op (idempotent by predID).
func (s *DelegationService) OnReverted(ctx context.Context, ws pgtype.UUID, actor PrincipalActor, predID pgtype.UUID) error {
	var (
		pred              db.DecisionPrediction
		needsExternalUndo bool
	)
	checkAndRevertDB := func(q *db.Queries, inTx bool) (alreadyReverted bool, err error) {
		if inTx {
			pred, err = q.LockDecisionPredictionByID(ctx, db.LockDecisionPredictionByIDParams{ID: predID, WorkspaceID: ws})
		} else {
			pred, err = q.GetDecisionPredictionByID(ctx, db.GetDecisionPredictionByIDParams{ID: predID, WorkspaceID: ws})
		}
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !pred.AppliedByDelegation) {
			return false, ErrPredictionInvalid
		}
		if err != nil {
			return false, fmt.Errorf("load delegated prediction: %w", err)
		}
		if actor.Type != GovernanceActorMember || !principalSameUUID(actor.ID, pred.MemberID) {
			return false, ErrPrincipalForbidden
		}
		if !pred.ResolvedAt.Valid || time.Since(pred.ResolvedAt.Time) > delegationRevertWindow {
			return false, ErrDelegationRevertWindow
		}
		reverted, err := q.HasDelegationRevertEvent(ctx, db.HasDelegationRevertEventParams{
			WorkspaceID: ws, SubjectID: pred.ID,
		})
		if err != nil {
			return false, fmt.Errorf("check delegation revert: %w", err)
		}
		if reverted {
			return true, nil
		}
		switch pred.SubjectType {
		case "inquiry":
			if _, err := q.ReopenBriefInquiryAnswer(ctx, db.ReopenBriefInquiryAnswerParams{
				ID: pred.SubjectID, WorkspaceID: ws, AnswerExcerpt: briefInquiryExcerptText(pred.Predicted),
			}); err != nil {
				return false, fmt.Errorf("reopen inquiry: %w", err)
			}
		case "proposal":
			if err := s.revertDelegatedProposal(ctx, q, ws, pred, inTx); err != nil {
				return false, err
			}
		case "effect_intent":
			needsExternalUndo = true
		case "outcome_check":
			if err := s.revertDelegatedOutcomeCheck(ctx, q, ws, pred); err != nil {
				return false, err
			}
		}
		if !needsExternalUndo {
			if err := s.logEventWith(ctx, q, ws, pred.MemberID, GovernanceKindPrincipalDelegationRevert,
				"decision_prediction", pred.ID, map[string]any{"decision_kind": pred.DecisionKind, "subject_type": pred.SubjectType}); err != nil {
				return false, err
			}
		}
		return false, nil
	}

	var alreadyReverted bool
	if s.TxStarter != nil {
		tx, err := s.TxStarter.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin delegation revert tx: %w", err)
		}
		defer tx.Rollback(ctx)
		qtx := s.Queries.WithTx(tx)
		alreadyReverted, err = checkAndRevertDB(qtx, true)
		if err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit delegation revert tx: %w", err)
		}
	} else {
		var err error
		alreadyReverted, err = checkAndRevertDB(s.Queries, false)
		if err != nil {
			return err
		}
	}
	if alreadyReverted {
		return nil
	}
	if needsExternalUndo {
		// External effect undo runs outside the DB lock transaction so the provider call never
		// holds a row lock, then we re-check idempotency and log the revert event.
		if err := s.revertDelegatedEffectIntent(ctx, ws, pred); err != nil {
			return err
		}
		if s.TxStarter != nil {
			tx, err := s.TxStarter.Begin(ctx)
			if err != nil {
				return fmt.Errorf("begin delegation revert log tx: %w", err)
			}
			defer tx.Rollback(ctx)
			qtx := s.Queries.WithTx(tx)
			if _, err := qtx.LockDecisionPredictionByID(ctx, db.LockDecisionPredictionByIDParams{ID: predID, WorkspaceID: ws}); err != nil {
				return fmt.Errorf("lock delegated prediction: %w", err)
			}
			reverted, err := qtx.HasDelegationRevertEvent(ctx, db.HasDelegationRevertEventParams{
				WorkspaceID: ws, SubjectID: pred.ID,
			})
			if err != nil {
				return fmt.Errorf("check delegation revert: %w", err)
			}
			if reverted {
				return nil
			}
			if err := s.logEventWith(ctx, qtx, ws, pred.MemberID, GovernanceKindPrincipalDelegationRevert,
				"decision_prediction", pred.ID, map[string]any{"decision_kind": pred.DecisionKind, "subject_type": pred.SubjectType}); err != nil {
				return err
			}
			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("commit delegation revert log tx: %w", err)
			}
		} else {
			if err := s.logEvent(ctx, ws, pred.MemberID, GovernanceKindPrincipalDelegationRevert,
				"decision_prediction", pred.ID, map[string]any{"decision_kind": pred.DecisionKind, "subject_type": pred.SubjectType}); err != nil {
				return err
			}
		}
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

// revertDelegatedProposal returns a delegated proposal to pending and moves its backing issue back
// to in_review (durable spec 6.5).
func (s *DelegationService) revertDelegatedProposal(ctx context.Context, q *db.Queries, ws pgtype.UUID, pred db.DecisionPrediction, inTx bool) error {
	prop, err := q.GetProposalInWorkspace(ctx, db.GetProposalInWorkspaceParams{ID: pred.SubjectID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load delegated proposal: %w", err)
	}
	if prop.State == ProposalStatePending {
		return nil
	}
	if _, err := q.RevertProposalDecision(ctx, db.RevertProposalDecisionParams{ID: prop.ID, WorkspaceID: ws}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("revert delegated proposal: %w", err)
	}
	if prop.IssueID.Valid {
		if inTx {
			if userID, err := resolvePrincipalUserID(ctx, q, ws, pred.MemberID); err == nil {
				if err := WithActorSession(ctx, q, GovernanceActor{Type: GovernanceActorMember, ID: userID}, pgtype.UUID{}); err != nil {
					return fmt.Errorf("set revert proposal actor session: %w", err)
				}
			}
		}
		if _, err := q.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{
			ID: prop.IssueID, WorkspaceID: ws, Status: "in_review",
		}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("reopen delegated proposal issue: %w", err)
		}
	}
	return nil
}

// revertDelegatedEffectIntent undoes an executed reversible effect intent through F3's undo path
// (durable spec 6.5, 4.6).
func (s *DelegationService) revertDelegatedEffectIntent(ctx context.Context, ws pgtype.UUID, pred db.DecisionPrediction) error {
	it, err := s.Queries.GetEffectIntent(ctx, db.GetEffectIntentParams{ID: pred.SubjectID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load delegated intent: %w", err)
	}
	if it.State != "executed" {
		// Nothing ran on the provider, so there is nothing to take back.
		return nil
	}
	if it.Reversibility != "reversible" {
		return ErrDelegationNotUndoable
	}
	ev, err := s.Queries.GetEffectEventByIntent(ctx, db.GetEffectEventByIntentParams{IntentID: it.ID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrDelegationNotUndoable
	}
	if err != nil {
		return fmt.Errorf("load delegated intent effect event: %w", err)
	}
	if ev.Status == "undoing" {
		return ErrDelegationUndoInProgress
	}
	if ev.Status != "executed" || len(ev.Undo) == 0 || s.AppsUndo == nil {
		return ErrDelegationNotUndoable
	}
	var spec connectedapps.UndoSpec
	if err := json.Unmarshal(ev.Undo, &spec); err != nil || spec.Tool == "" {
		return ErrDelegationNotUndoable
	}
	claimed, err := s.Queries.ClaimEffectEventUndo(ctx, db.ClaimEffectEventUndoParams{ID: ev.ID, WorkspaceID: ws})
	if err != nil {
		return fmt.Errorf("claim effect undo: %w", err)
	}
	if claimed == 0 {
		// A concurrent request claimed the undo between our read and the claim.
		return ErrDelegationUndoInProgress
	}
	if _, err := s.AppsUndo.ExecuteUndo(ctx, ws, ev.MemberID, ev.Target, spec); err != nil {
		_, _ = s.Queries.RevertEffectEventUndo(ctx, db.RevertEffectEventUndoParams{ID: ev.ID, WorkspaceID: ws})
		return fmt.Errorf("execute delegated intent undo: %w", err)
	}
	if _, err := s.Queries.MarkEffectEventUndone(ctx, db.MarkEffectEventUndoneParams{ID: ev.ID, WorkspaceID: ws}); err != nil {
		return fmt.Errorf("mark delegated effect undone: %w", err)
	}
	userID, _ := resolvePrincipalUserID(ctx, s.Queries, ws, pred.MemberID)
	if _, err := s.Queries.InsertEffectEvent(ctx, db.InsertEffectEventParams{
		WorkspaceID:   ws,
		MemberID:      userID,
		Source:        connectedapps.SourceConnectedApp,
		Target:        ev.Target,
		EffectClass:   ev.EffectClass,
		Reversibility: ev.Reversibility,
		Decision:      connectedapps.PolicyAllow,
		Status:        "executed",
		Summary:       effectTruncate("Hoàn tác "+util.UUIDToString(ev.ID), effectSummaryMax),
	}); err != nil {
		slog.Warn("delegated intent undo: record undo event", "error", err, "effect_id", util.UUIDToString(ev.ID))
	}
	return nil
}

// revertDelegatedOutcomeCheck undoes a delegated manual outcome verification (durable spec 6.5).
// Because outcome_verification is append-only (spec 2.3), an unsettled outcome receives a newer
// member verification with result "unable" (which does not count at settle time), while a settled
// outcome is overridden through the standard member override path (spec 2.6).
func (s *DelegationService) revertDelegatedOutcomeCheck(ctx context.Context, q *db.Queries, ws pgtype.UUID, pred db.DecisionPrediction) error {
	o, err := q.GetWorkOutcomeInWorkspace(ctx, db.GetWorkOutcomeInWorkspaceParams{ID: pred.SubjectID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load delegated outcome: %w", err)
	}
	userID, err := resolvePrincipalUserID(ctx, q, ws, pred.MemberID)
	if err != nil {
		return fmt.Errorf("resolve principal user: %w", err)
	}
	if !o.SettledAt.Valid {
		latest, err := q.ListLatestCriterionVerifications(ctx, o.ID)
		if err != nil {
			return fmt.Errorf("list outcome verifications: %w", err)
		}
		var criterionID pgtype.UUID
		for _, v := range latest {
			if v.VerifierType == verifierTypeMember && principalSameUUID(v.VerifierID, userID) {
				criterionID = v.CriterionID
				break
			}
		}
		if _, err := q.InsertOutcomeVerification(ctx, verificationParams(ws, o.ID, VerificationInput{
			CriterionID: criterionID,
			Result:      "unable",
			Note:        "Reverted delegated verification",
		}, verifierTypeMember, userID, pgtype.UUID{})); err != nil {
			return fmt.Errorf("append reverted outcome verification: %w", err)
		}
		return nil
	}
	// A settled outcome cannot be reverted automatically: the platform does not keep the
	// verdict it had before the delegated decision, and picking one could flip a failed
	// criterion to pass. The person sets the verdict again through the outcome override.
	return ErrDelegationOutcomeSettled
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
	return s.logEventWith(ctx, s.Queries, ws, member, kind, subjectType, subject, details)
}

func (s *DelegationService) logEventWith(ctx context.Context, q *db.Queries, ws, member pgtype.UUID, kind, subjectType string, subject pgtype.UUID, details map[string]any) error {
	return NewGovernanceLog().Record(ctx, q, GovernanceEvent{
		WorkspaceID: ws, Kind: kind,
		ActorType: GovernanceActorMember, ActorID: member,
		SubjectType: subjectType, SubjectID: subject,
		Summary: "Principal delegation: " + kind, Details: details,
	})
}

// resolvePrincipalUserID resolves the user UUID for a principal_profile.member_id, accepting both
// user IDs (spec 6.2) and member row IDs (used by legacy fixtures).
func resolvePrincipalUserID(ctx context.Context, q *db.Queries, ws, memberOrUserID pgtype.UUID) (pgtype.UUID, error) {
	if m, err := q.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{UserID: memberOrUserID, WorkspaceID: ws}); err == nil {
		return m.UserID, nil
	}
	m, err := q.GetMember(ctx, memberOrUserID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if !principalSameUUID(m.WorkspaceID, ws) {
		return pgtype.UUID{}, pgx.ErrNoRows
	}
	return m.UserID, nil
}

// decideSettle settles a proposal's approval or rejection through the delegation gate (spec 6.5).
func (s *ProposalService) decideSettle(ctx context.Context, p ProposalDecideParams) (ProposalDecideResult, error) {
	ask := AskInput{SubjectType: "proposal"}
	if p.Delegated != nil {
		prop, err := s.Issues.Queries.GetProposalInWorkspace(ctx, db.GetProposalInWorkspaceParams{
			ID: p.ProposalID, WorkspaceID: p.WorkspaceID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ProposalDecideResult{}, ErrProposalNotFound
			}
			return ProposalDecideResult{}, fmt.Errorf("load proposal for delegation: %w", err)
		}
		ask.Urgent = prop.Urgent
		if ForbiddenDelegationKind("proposal:"+prop.Kind, ask) {
			return ProposalDecideResult{}, ErrDelegationForbiddenKind
		}
	}
	var res ProposalDecideResult
	err := s.delegation().Settle(ctx, p.WorkspaceID, p.Delegated, "proposal", p.ProposalID, p.DeciderID, p.Decision, ask, func() error {
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
// its kind (spec 6.5). Irreversible intents and forbidden classes are never delegated.
func (s *IntentService) ApproveAsPrincipal(ctx context.Context, ws, decider, id pgtype.UUID, d DelegatedDecision) (db.EffectIntent, error) {
	it, err := s.Get(ctx, ws, id)
	if err != nil {
		return db.EffectIntent{}, err
	}
	ask := AskInput{Irreversible: it.Reversibility == "irreversible"}
	if ForbiddenDelegationKind("effect:"+it.EffectClass, ask) {
		return db.EffectIntent{}, ErrDelegationForbiddenKind
	}
	var out db.EffectIntent
	err = s.delegation().Settle(ctx, ws, &d, "effect_intent", id, decider, "approve",
		ask, func() error {
			var err error
			out, err = s.Approve(ctx, ws, decider, id, nil, "")
			return err
		})
	return out, err
}

// RejectAsPrincipal rejects a pending intent as the person's own decision, when the person
// delegated its kind (spec 6.5). Irreversible intents and forbidden classes are never delegated.
func (s *IntentService) RejectAsPrincipal(ctx context.Context, ws, decider, id pgtype.UUID, note string, d DelegatedDecision) (db.EffectIntent, error) {
	it, err := s.Get(ctx, ws, id)
	if err != nil {
		return db.EffectIntent{}, err
	}
	ask := AskInput{Irreversible: it.Reversibility == "irreversible"}
	if ForbiddenDelegationKind("effect:"+it.EffectClass, ask) {
		return db.EffectIntent{}, ErrDelegationForbiddenKind
	}
	if strings.TrimSpace(note) == "" {
		note = "Delegated by prediction"
	}
	var out db.EffectIntent
	err = s.delegation().Settle(ctx, ws, &d, "effect_intent", id, decider, "reject",
		ask, func() error {
			var err error
			out, err = s.Reject(ctx, ws, decider, id, note)
			return err
		})
	return out, err
}

// delegation is the delegation gate for intents.
func (s *IntentService) delegation() *DelegationService {
	return &DelegationService{Queries: s.Queries, TxStarter: s.TxStarter, Flags: s.Flags, Predictions: &PredictionService{Queries: s.Queries, TxStarter: s.TxStarter, Flags: s.Flags}}
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
	return &DelegationService{Queries: s.Queries, TxStarter: s.TxStarter, Flags: s.Flags, Predictions: &PredictionService{Queries: s.Queries, TxStarter: s.TxStarter, Flags: s.Flags}}
}

// maybeAutoDelegate applies a newly recorded prediction automatically when the person has enabled
// delegation for that kind and all eligibility rules hold (durable spec 6.5).
func (s *PredictionService) maybeAutoDelegate(ctx context.Context, pred db.DecisionPrediction) db.DecisionPrediction {
	switch pred.SubjectType {
	case "inquiry":
		return s.delegateInquiry(ctx, pred)
	case "proposal":
		return s.delegateProposal(ctx, pred)
	case "effect_intent":
		return s.delegateIntent(ctx, pred)
	case "outcome_check":
		return s.delegateOutcomeCheck(ctx, pred)
	default:
		return pred
	}
}

// delegateProposal applies an eligible prediction for a pending proposal via ProposalService.Decide.
func (s *PredictionService) delegateProposal(ctx context.Context, pred db.DecisionPrediction) db.DecisionPrediction {
	if s.Proposals == nil || (pred.Predicted != ProposalDecisionApprove && pred.Predicted != ProposalDecisionReject) {
		return pred
	}
	userID, err := resolvePrincipalUserID(ctx, s.Queries, pred.WorkspaceID, pred.MemberID)
	if err != nil {
		slog.Warn("proposal delegation skipped", "prediction_id", util.UUIDToString(pred.ID), "error", err)
		return pred
	}
	if s.Proposals.Principal == nil {
		s.Proposals.Principal = s
	}
	_, err = s.Proposals.Decide(ctx, ProposalDecideParams{
		WorkspaceID: pred.WorkspaceID,
		ProposalID:  pred.SubjectID,
		DeciderID:   userID,
		Decision:    pred.Predicted,
		Note:        "Delegated by prediction",
		Delegated:   &DelegatedDecision{PredictionID: pred.ID},
	})
	if err != nil && !errors.Is(err, ErrDelegationNotEligible) && !errors.Is(err, ErrDelegationForbiddenKind) && !errors.Is(err, ErrProposalNotFound) {
		slog.Warn("proposal delegation failed", "prediction_id", util.UUIDToString(pred.ID), "error", err)
	}
	if fresh, err := s.Queries.GetDecisionPredictionByID(ctx, db.GetDecisionPredictionByIDParams{ID: pred.ID, WorkspaceID: pred.WorkspaceID}); err == nil {
		return fresh
	}
	return pred
}

// delegateIntent applies an eligible prediction for a pending effect intent via
// IntentService.ApproveAsPrincipal or RejectAsPrincipal (durable spec 6.5).
func (s *PredictionService) delegateIntent(ctx context.Context, pred db.DecisionPrediction) db.DecisionPrediction {
	if s.Intents == nil || (pred.Predicted != "approve" && pred.Predicted != "reject") {
		return pred
	}
	userID, err := resolvePrincipalUserID(ctx, s.Queries, pred.WorkspaceID, pred.MemberID)
	if err != nil {
		slog.Warn("intent delegation skipped", "prediction_id", util.UUIDToString(pred.ID), "error", err)
		return pred
	}
	if pred.Predicted == "approve" {
		_, err = s.Intents.ApproveAsPrincipal(ctx, pred.WorkspaceID, userID, pred.SubjectID, DelegatedDecision{PredictionID: pred.ID})
	} else {
		_, err = s.Intents.RejectAsPrincipal(ctx, pred.WorkspaceID, userID, pred.SubjectID, "Delegated by prediction", DelegatedDecision{PredictionID: pred.ID})
	}
	if err != nil && !errors.Is(err, ErrDelegationNotEligible) && !errors.Is(err, ErrDelegationForbiddenKind) && !errors.Is(err, ErrIntentNotFound) {
		slog.Warn("intent delegation failed", "prediction_id", util.UUIDToString(pred.ID), "error", err)
	}
	if fresh, err := s.Queries.GetDecisionPredictionByID(ctx, db.GetDecisionPredictionByIDParams{ID: pred.ID, WorkspaceID: pred.WorkspaceID}); err == nil {
		return fresh
	}
	return pred
}

// delegateOutcomeCheck applies an eligible prediction for an open outcome manual check via
// VerificationService.RecordAsPrincipal (durable spec 6.5).
func (s *PredictionService) delegateOutcomeCheck(ctx context.Context, pred db.DecisionPrediction) db.DecisionPrediction {
	if s.Checks == nil || (pred.Predicted != "pass" && pred.Predicted != "partial" && pred.Predicted != "fail") {
		return pred
	}
	userID, err := resolvePrincipalUserID(ctx, s.Queries, pred.WorkspaceID, pred.MemberID)
	if err != nil {
		slog.Warn("outcome check delegation skipped", "prediction_id", util.UUIDToString(pred.ID), "error", err)
		return pred
	}
	o, err := s.Queries.GetWorkOutcomeInWorkspace(ctx, db.GetWorkOutcomeInWorkspaceParams{
		ID: pred.SubjectID, WorkspaceID: pred.WorkspaceID,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("outcome check delegation load failed", "prediction_id", util.UUIDToString(pred.ID), "error", err)
		}
		return pred
	}
	var criterionID pgtype.UUID
	if criteria, err := s.Queries.ListIssueAcceptanceCriteria(ctx, db.ListIssueAcceptanceCriteriaParams{
		WorkspaceID: pred.WorkspaceID, IssueID: o.IssueID,
	}); err == nil {
		for _, c := range criteria {
			if !c.ArchivedAt.Valid && c.CheckKind == CheckKindManual {
				criterionID = c.ID
				break
			}
		}
	}
	_, err = s.Checks.RecordAsPrincipal(ctx, pred.WorkspaceID, userID, pred.SubjectID, VerificationInput{
		CriterionID: criterionID,
		Result:      pred.Predicted,
		Note:        "Delegated by prediction",
	}, DelegatedDecision{PredictionID: pred.ID})
	if err != nil && !errors.Is(err, ErrDelegationNotEligible) && !errors.Is(err, ErrDelegationForbiddenKind) && !errors.Is(err, ErrOutcomeNotFound) {
		slog.Warn("outcome check delegation failed", "prediction_id", util.UUIDToString(pred.ID), "error", err)
	}
	if fresh, err := s.Queries.GetDecisionPredictionByID(ctx, db.GetDecisionPredictionByIDParams{ID: pred.ID, WorkspaceID: pred.WorkspaceID}); err == nil {
		return fresh
	}
	return pred
}

// delegateInquiry answers an open inquiry as the person's own decision, when the person delegated
// inquiries and the prediction the Secretary recorded for them is eligible (durable spec 6.5). The
// predicted answer is the answer. Without an eligible prediction the inquiry stays with its squad
// leader, and no principal actor is recorded. It returns the prediction as it stands afterwards.
func (s *PredictionService) delegateInquiry(ctx context.Context, pred db.DecisionPrediction) db.DecisionPrediction {
	userID, err := resolvePrincipalUserID(ctx, s.Queries, pred.WorkspaceID, pred.MemberID)
	if err != nil {
		slog.Warn("inquiry delegation skipped", "prediction_id", util.UUIDToString(pred.ID), "error", err)
		return pred
	}
	err = s.delegation().Settle(ctx, pred.WorkspaceID, &DelegatedDecision{PredictionID: pred.ID},
		"inquiry", pred.SubjectID, userID, pred.Predicted, AskInput{}, func() error {
			n, err := s.Queries.AnswerBriefInquiryAsPrincipal(ctx, db.AnswerBriefInquiryAsPrincipalParams{
				ID: pred.SubjectID, WorkspaceID: pred.WorkspaceID, AnswerExcerpt: briefInquiryExcerptText(pred.Predicted),
			})
			if err != nil {
				return fmt.Errorf("answer inquiry: %w", err)
			}
			if n == 0 {
				return ErrDelegationNotEligible
			}
			return nil
		})
	if err != nil && !errors.Is(err, ErrDelegationNotEligible) {
		slog.Warn("inquiry delegation failed", "prediction_id", util.UUIDToString(pred.ID), "error", err)
	}
	if fresh, err := s.Queries.GetDecisionPredictionByID(ctx, db.GetDecisionPredictionByIDParams{ID: pred.ID, WorkspaceID: pred.WorkspaceID}); err == nil {
		return fresh
	}
	return pred
}

// briefInquiryExcerptText is the answer excerpt a delegated inquiry answer stores.
func briefInquiryExcerptText(predicted string) pgtype.Text {
	return pgtype.Text{String: briefInquiryExcerpt(predicted), Valid: true}
}
