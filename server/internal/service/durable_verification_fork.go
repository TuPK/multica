package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
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

// Acceptance criteria, the checks that decide an outcome, and the verifier that makes
// an agent check (durable spec 2.3, 2.5 to 2.7, A3).

const (
	// MaxOpenCriteriaPerIssue bounds the criteria an issue carries at once (spec 2.7).
	MaxOpenCriteriaPerIssue = 20
	// OutcomeOverrideWindow is how long a settled conclusion stays changeable by a
	// person (spec 2.6, D4).
	OutcomeOverrideWindow = 30 * 24 * time.Hour

	criterionTextMax    = 500
	criterionRubricMax  = 2000
	verificationNoteMax = 4000

	CheckKindManual         = "manual"
	CheckKindAgentJudgment  = "agent_judgment"
	CheckKindCommand        = "command"
	CheckKindArtifact       = "artifact"
	CheckKindMetric         = "metric"
	criterionSourceMember   = "member"
	criterionSourceAgent    = "agent"
	criterionSourceTemplate = "template"

	verifierTypeAgent  = "agent"
	verifierTypeMember = "member"
	verifierTypeSystem = "system"
	verifierTypeMetric = "metric"

	// implicitRubric is the criterion a sampled outcome gets when its issue has none
	// (spec 2.5).
	implicitRubric = "Does the work resolve what the description and the comments of the person ask for?"

	// outcomeVerifierAgentSetting and outcomeVerifierPoolSetting are the workspace
	// settings that name the verifier (spec 2.5). Both live in workspace.settings.
	outcomeVerifierAgentSetting = "outcome_verifier_agent_id"
	outcomeVerifierPoolSetting  = "outcome_verifier_pool"

	metricCheckBatch = 1000
)

// Errors a caller maps to a response. The message is the machine code.
var (
	ErrCriterionLimit       = errors.New("criterion_limit")
	ErrCriterionInvalid     = errors.New("criterion_invalid")
	ErrCriterionNotFound    = errors.New("criterion_not_found")
	ErrCriterionChecked     = errors.New("criterion_checked")
	ErrOutcomeNotFound      = errors.New("outcome_not_found")
	ErrOutcomeSettled       = errors.New("outcome_settled")
	ErrOutcomeNotSettled    = errors.New("outcome_not_settled")
	ErrOverrideWindowClosed = errors.New("override_window_closed")
	ErrVerifierIsDoer       = errors.New("verifier_is_doer")
	ErrNotVerificationTask  = errors.New("not_verification_task")
	ErrVerificationInvalid  = errors.New("verification_invalid")
)

// CriterionInput is a criterion as its writer gives it. Source is set by the caller
// from who writes it; empty means member or agent by the actor's type.
type CriterionInput struct {
	Text      string
	CheckKind string
	CheckSpec map[string]any
	Source    string
}

// CriterionPatch changes a criterion: a new text, or archive it. A nil Text keeps it.
type CriterionPatch struct {
	Text    *string
	Archive bool
}

// VerificationInput is one check as its writer gives it. CriterionID is zero for a
// conclusion on the whole outcome. TaskID is the caller's task when an agent writes.
type VerificationInput struct {
	CriterionID pgtype.UUID
	Result      string
	Severity    string
	Note        string
	Evidence    json.RawMessage
	TaskID      pgtype.UUID
}

// TemplateCriterion is one criterion a squad template declares for its role (spec 2.5).
type TemplateCriterion struct {
	Role      string
	Text      string
	CheckKind string
	CheckSpec map[string]any
}

// VerificationService owns the criteria of an issue, the checks made on an outcome,
// the choice of verifier, and the overrides. Tasks is needed to give an agent its
// check; without it an agent check is recorded as unable.
type VerificationService struct {
	Queries   *db.Queries
	TxStarter TxStarter
	Tasks     *TaskService
	// Jury takes the agent_judgment criteria a jury checks (durable spec 8.2). Nil means
	// every criterion has one verifier.
	Jury *JuryService
	// Now is the clock for windows. Nil means time.Now.
	Now func() time.Time
	// Flags decides durable_principal: with it on, a manual check waits in the attention ledger.
	Flags *featureflag.Service
}

func (s *VerificationService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *VerificationService) outcomesEnabled(ctx context.Context) bool {
	return featureflags.DurableOutcomesEnabled(ctx, s.Flags)
}

// AddCriterion writes a criterion on an issue. The open-criteria limit is checked under
// the issue's row lock, so two writers cannot both take the last slot.
func (s *VerificationService) AddCriterion(ctx context.Context, ws, issue pgtype.UUID, actor GovernanceActor, in CriterionInput) (db.AcceptanceCriterion, error) {
	if !s.outcomesEnabled(ctx) {
		return db.AcceptanceCriterion{}, ErrOutcomesOff
	}
	source := in.Source
	if source == "" {
		source = criterionSourceMember
		if actor.Type == GovernanceActorAgent {
			source = criterionSourceAgent
		}
	}
	var out db.AcceptanceCriterion
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		out, err = s.insertCriterion(ctx, q, ws, issue, actor, source, in)
		return err
	})
	return out, err
}

// insertCriterion validates and writes one criterion inside q's transaction.
func (s *VerificationService) insertCriterion(ctx context.Context, q *db.Queries, ws, issue pgtype.UUID, actor GovernanceActor, source string, in CriterionInput) (db.AcceptanceCriterion, error) {
	text := strings.TrimSpace(in.Text)
	if text == "" || len([]rune(text)) > criterionTextMax {
		return db.AcceptanceCriterion{}, fmt.Errorf("%w: text must be 1 to %d characters", ErrCriterionInvalid, criterionTextMax)
	}
	spec, err := validateCriterionSpec(in.CheckKind, in.CheckSpec)
	if err != nil {
		return db.AcceptanceCriterion{}, err
	}
	switch source {
	case criterionSourceMember, criterionSourceAgent, "flow", criterionSourceTemplate:
	default:
		return db.AcceptanceCriterion{}, fmt.Errorf("%w: unknown source %q", ErrCriterionInvalid, source)
	}
	if _, err := q.LockIssueForCriterionWrite(ctx, db.LockIssueForCriterionWriteParams{ID: issue, WorkspaceID: ws}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.AcceptanceCriterion{}, ErrCriterionNotFound
		}
		return db.AcceptanceCriterion{}, fmt.Errorf("lock issue: %w", err)
	}
	open, err := q.CountOpenAcceptanceCriteria(ctx, db.CountOpenAcceptanceCriteriaParams{WorkspaceID: ws, IssueID: issue})
	if err != nil {
		return db.AcceptanceCriterion{}, fmt.Errorf("count criteria: %w", err)
	}
	if open >= MaxOpenCriteriaPerIssue {
		return db.AcceptanceCriterion{}, ErrCriterionLimit
	}
	all, err := q.ListIssueAcceptanceCriteria(ctx, db.ListIssueAcceptanceCriteriaParams{WorkspaceID: ws, IssueID: issue})
	if err != nil {
		return db.AcceptanceCriterion{}, fmt.Errorf("list criteria: %w", err)
	}
	var createdBy pgtype.UUID
	if actor.ID.Valid {
		createdBy = actor.ID
	}
	return q.InsertAcceptanceCriterion(ctx, db.InsertAcceptanceCriterionParams{
		ID:            dbid.NewV7(),
		WorkspaceID:   ws,
		IssueID:       issue,
		Position:      int32(len(all)),
		Text:          text,
		CheckKind:     in.CheckKind,
		CheckSpec:     spec,
		Source:        source,
		CreatedByType: actor.Type,
		CreatedByID:   createdBy,
	})
}

// ListCriteria returns every criterion of an issue, archived ones included, in order.
func (s *VerificationService) ListCriteria(ctx context.Context, ws, issue pgtype.UUID) ([]db.AcceptanceCriterion, error) {
	return s.Queries.ListIssueAcceptanceCriteria(ctx, db.ListIssueAcceptanceCriteriaParams{WorkspaceID: ws, IssueID: issue})
}

// UpdateCriterion changes the text of a criterion or archives it. Neither is allowed
// once the issue's outcome has settled, or while its pending outcome already has a check
// on the criterion: the verifier judged the text it had.
func (s *VerificationService) UpdateCriterion(ctx context.Context, ws, issue, criterion pgtype.UUID, actor GovernanceActor, patch CriterionPatch) (db.AcceptanceCriterion, error) {
	if !s.outcomesEnabled(ctx) {
		return db.AcceptanceCriterion{}, ErrOutcomesOff
	}
	var out db.AcceptanceCriterion
	err := s.inTx(ctx, func(q *db.Queries) error {
		if _, err := q.GetAcceptanceCriterion(ctx, db.GetAcceptanceCriterionParams{ID: criterion, WorkspaceID: ws, IssueID: issue}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrCriterionNotFound
			}
			return fmt.Errorf("load criterion: %w", err)
		}
		current, err := q.GetCurrentWorkOutcomeForIssue(ctx, db.GetCurrentWorkOutcomeForIssueParams{WorkspaceID: ws, IssueID: issue})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return fmt.Errorf("load current outcome: %w", err)
		case current.SettledAt.Valid:
			return ErrOutcomeSettled
		default:
			checks, err := q.CountCriterionChecksOnCurrentOutcome(ctx, db.CountCriterionChecksOnCurrentOutcomeParams{CriterionID: criterion, WorkspaceID: ws})
			if err != nil {
				return fmt.Errorf("count checks: %w", err)
			}
			if checks > 0 {
				return ErrCriterionChecked
			}
		}
		if patch.Text != nil {
			text := strings.TrimSpace(*patch.Text)
			if text == "" || len([]rune(text)) > criterionTextMax {
				return fmt.Errorf("%w: text must be 1 to %d characters", ErrCriterionInvalid, criterionTextMax)
			}
			out, err = q.UpdateAcceptanceCriterionText(ctx, db.UpdateAcceptanceCriterionTextParams{Text: text, ID: criterion, WorkspaceID: ws})
			if err != nil {
				return mapNoRows(err, ErrCriterionNotFound, "update criterion text")
			}
		}
		if patch.Archive {
			out, err = q.ArchiveAcceptanceCriterion(ctx, db.ArchiveAcceptanceCriterionParams{ID: criterion, WorkspaceID: ws})
			if err != nil {
				return mapNoRows(err, ErrCriterionNotFound, "archive criterion")
			}
		}
		if patch.Text == nil && !patch.Archive {
			out, err = q.GetAcceptanceCriterion(ctx, db.GetAcceptanceCriterionParams{ID: criterion, WorkspaceID: ws, IssueID: issue})
			if err != nil {
				return mapNoRows(err, ErrCriterionNotFound, "load criterion")
			}
		}
		return nil
	})
	return out, err
}

// ApplyTemplateCriteria writes the criteria a squad template declares, when the squad
// receives the issue. A criterion whose text the issue already has is skipped, so a
// second receipt adds nothing. It returns how many it wrote.
func (s *VerificationService) ApplyTemplateCriteria(ctx context.Context, ws, issue pgtype.UUID, templates []TemplateCriterion) (int, error) {
	written := 0
	err := s.inTx(ctx, func(q *db.Queries) error {
		if _, err := q.LockIssueForCriterionWrite(ctx, db.LockIssueForCriterionWriteParams{ID: issue, WorkspaceID: ws}); err != nil {
			return mapNoRows(err, ErrCriterionNotFound, "lock issue")
		}
		existing, err := q.ListIssueAcceptanceCriteria(ctx, db.ListIssueAcceptanceCriteriaParams{WorkspaceID: ws, IssueID: issue})
		if err != nil {
			return fmt.Errorf("list criteria: %w", err)
		}
		// An archived criterion counts as present: archiving one by hand must not bring it back
		// on the next receipt.
		have := map[string]bool{}
		open := 0
		for _, c := range existing {
			have[c.Text] = true
			if !c.ArchivedAt.Valid {
				open++
			}
		}
		for _, tc := range templates {
			if have[strings.TrimSpace(tc.Text)] {
				continue
			}
			spec := make(map[string]any, len(tc.CheckSpec)+1)
			for k, v := range tc.CheckSpec {
				spec[k] = v
			}
			if tc.Role != "" {
				spec["role"] = tc.Role
			}
			if _, err := s.insertCriterion(ctx, q, ws, issue, GovernanceActor{Type: GovernanceActorSystem}, criterionSourceTemplate, CriterionInput{
				Text: tc.Text, CheckKind: tc.CheckKind, CheckSpec: spec,
			}); err != nil {
				return err
			}
			written++
		}
		return nil
	})
	return written, err
}

// asNumber reads a JSON number, whether the spec came from a decoded body (float64) or
// was built in Go.
func asNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// validateCriterionSpec checks a criterion's kind and its check_spec (spec 2.5) and
// returns the spec as stored.
func validateCriterionSpec(kind string, spec map[string]any) ([]byte, error) {
	if spec == nil {
		spec = map[string]any{}
	}
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: "+format, append([]any{ErrCriterionInvalid}, args...)...)
	}
	switch kind {
	case CheckKindManual:
	case CheckKindAgentJudgment:
		rubric, _ := spec["rubric"].(string)
		if strings.TrimSpace(rubric) == "" || len([]rune(rubric)) > criterionRubricMax {
			return nil, invalid("agent_judgment needs a rubric of 1 to %d characters", criterionRubricMax)
		}
	case CheckKindCommand:
		run, _ := spec["run"].(string)
		if strings.TrimSpace(run) == "" {
			return nil, invalid("command needs a run")
		}
		if exp, ok := spec["expect_exit"]; ok {
			if f, isNum := asNumber(exp); !isNum || f != math.Trunc(f) {
				return nil, invalid("expect_exit must be an integer")
			}
		}
	case CheckKindArtifact:
		k, _ := spec["kind"].(string)
		if strings.TrimSpace(k) == "" {
			return nil, invalid("artifact needs a knowledge kind")
		}
	case CheckKindMetric:
		key, _ := spec["metric_key"].(string)
		if strings.TrimSpace(key) == "" {
			return nil, invalid("metric needs a metric_key")
		}
		if scope, _ := spec["scope"].(string); scope != "company" {
			return nil, invalid("metric scope must be company")
		}
		if op, _ := spec["op"].(string); op != ">=" {
			return nil, invalid("metric op must be >=")
		}
		if _, ok := asNumber(spec["value"]); !ok {
			return nil, invalid("metric needs a numeric value")
		}
		days, ok := asNumber(spec["within_days"])
		if !ok || days != math.Trunc(days) || days < 1 || days > 365 {
			return nil, invalid("metric within_days must be an integer from 1 to 365")
		}
	default:
		return nil, invalid("unknown check_kind %q", kind)
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, invalid("check_spec: %v", err)
	}
	return raw, nil
}

// Record writes one check on an outcome. The outcome must not have settled: a settled
// conclusion changes only through Override. An agent may check a criterion only through
// the verification task it was given for that criterion, and never if it did the work
// or wrote the criterion.
func (s *VerificationService) Record(ctx context.Context, ws pgtype.UUID, actor GovernanceActor, outcomeID pgtype.UUID, in VerificationInput) (db.OutcomeVerification, error) {
	if !s.outcomesEnabled(ctx) {
		return db.OutcomeVerification{}, ErrOutcomesOff
	}
	var out db.OutcomeVerification
	err := s.inTx(ctx, func(q *db.Queries) error {
		o, err := q.GetWorkOutcomeInWorkspace(ctx, db.GetWorkOutcomeInWorkspaceParams{ID: outcomeID, WorkspaceID: ws})
		if err != nil {
			return mapNoRows(err, ErrOutcomeNotFound, "load outcome")
		}
		if o.SettledAt.Valid {
			return ErrOutcomeSettled
		}
		if err := checkVerificationInput(in, false); err != nil {
			return err
		}
		var criterion db.AcceptanceCriterion
		if in.CriterionID.Valid {
			criterion, err = q.GetAcceptanceCriterion(ctx, db.GetAcceptanceCriterionParams{ID: in.CriterionID, WorkspaceID: ws, IssueID: o.IssueID})
			if err != nil {
				return mapNoRows(err, ErrCriterionNotFound, "load criterion")
			}
		}
		verifierType := verifierTypeMember
		var verifierID pgtype.UUID
		var taskID pgtype.UUID
		if actor.Type == GovernanceActorAgent {
			if err := s.authorizeAgentCheck(ctx, q, ws, actor, o, criterion, in); err != nil {
				return err
			}
			verifierType = verifierTypeAgent
			verifierID = actor.ID
			taskID = in.TaskID
		} else if actor.ID.Valid {
			verifierID = actor.ID
		}
		out, err = q.InsertOutcomeVerification(ctx, verificationParams(ws, o.ID, in, verifierType, verifierID, taskID))
		return err
	})
	if err != nil {
		return out, err
	}
	if out.VerifierType == verifierTypeAgent && out.CriterionID.Valid && s.Jury != nil {
		if err := s.Jury.OnVote(ctx, out); err != nil {
			return out, fmt.Errorf("jury vote: %w", err)
		}
	}
	return out, nil
}

// authorizeAgentCheck refuses an agent that is not the verifier of this criterion on
// this outcome, and an agent that did the work or wrote the criterion.
func (s *VerificationService) authorizeAgentCheck(ctx context.Context, q *db.Queries, ws pgtype.UUID, actor GovernanceActor, o db.WorkOutcome, criterion db.AcceptanceCriterion, in VerificationInput) error {
	if !in.TaskID.Valid || !in.CriterionID.Valid {
		return ErrNotVerificationTask
	}
	task, err := q.GetAgentTask(ctx, in.TaskID)
	if err != nil {
		return mapNoRows(err, ErrNotVerificationTask, "load task")
	}
	if task.AgentID != actor.ID {
		return ErrNotVerificationTask
	}
	if (o.AgentID.Valid && o.AgentID == actor.ID) || (criterion.CreatedByType == GovernanceActorAgent && criterion.CreatedByID == actor.ID) {
		return ErrVerifierIsDoer
	}
	if panelID := juryPanelIDOf(task.Context); panelID != "" {
		return authorizeJuror(ctx, q, ws, o, in.CriterionID, actor, panelID)
	}
	assigned, err := q.GetOutcomeCheckTaskForCriterion(ctx, db.GetOutcomeCheckTaskForCriterionParams{
		WorkspaceID: ws, OutcomeID: o.ID, CriterionID: in.CriterionID,
	})
	if err != nil {
		return mapNoRows(err, ErrNotVerificationTask, "load check task")
	}
	if assigned != in.TaskID {
		return ErrNotVerificationTask
	}
	return nil
}

// checkVerificationInput validates the result, severity and note of a check. An override
// may not say unable: a person who cannot judge leaves the conclusion as it is.
func checkVerificationInput(in VerificationInput, override bool) error {
	switch in.Result {
	case "pass", "partial", "fail":
	case "unable":
		if !override {
			break
		}
		return fmt.Errorf("%w: result must be pass, partial or fail", ErrVerificationInvalid)
	default:
		return fmt.Errorf("%w: result must be pass, partial, fail or unable", ErrVerificationInvalid)
	}
	switch in.Severity {
	case "", "none", "low", "medium", "high":
	default:
		return fmt.Errorf("%w: unknown severity %q", ErrVerificationInvalid, in.Severity)
	}
	if len([]rune(in.Note)) > verificationNoteMax {
		return fmt.Errorf("%w: note is longer than %d characters", ErrVerificationInvalid, verificationNoteMax)
	}
	return nil
}

func verificationParams(ws, outcome pgtype.UUID, in VerificationInput, verifierType string, verifierID, taskID pgtype.UUID) db.InsertOutcomeVerificationParams {
	severity := in.Severity
	if severity == "" {
		severity = "none"
	}
	evidence := in.Evidence
	if len(evidence) == 0 {
		evidence = json.RawMessage("[]")
	}
	return db.InsertOutcomeVerificationParams{
		ID:           dbid.NewV7(),
		WorkspaceID:  ws,
		OutcomeID:    outcome,
		CriterionID:  in.CriterionID,
		VerifierType: verifierType,
		VerifierID:   verifierID,
		TaskID:       taskID,
		Result:       in.Result,
		Severity:     severity,
		Note:         in.Note,
		Evidence:     evidence,
	}
}

// Override changes the conclusion of a settled outcome, by a person, within the window.
// It writes a member check on the outcome and the new verdict, and a governance event.
// fail_cause is member for a fail a person made.
func (s *VerificationService) Override(ctx context.Context, ws pgtype.UUID, actor GovernanceActor, outcomeID pgtype.UUID, in VerificationInput) (db.WorkOutcome, error) {
	if !s.outcomesEnabled(ctx) {
		return db.WorkOutcome{}, ErrOutcomesOff
	}
	var out db.WorkOutcome
	err := s.inTx(ctx, func(q *db.Queries) error {
		o, err := q.GetWorkOutcomeInWorkspace(ctx, db.GetWorkOutcomeInWorkspaceParams{ID: outcomeID, WorkspaceID: ws})
		if err != nil {
			return mapNoRows(err, ErrOutcomeNotFound, "load outcome")
		}
		if !o.SettledAt.Valid {
			return ErrOutcomeNotSettled
		}
		now := s.now()
		since := now.Add(-OutcomeOverrideWindow)
		if o.SettledAt.Time.Before(since) {
			return ErrOverrideWindowClosed
		}
		if err := checkVerificationInput(in, true); err != nil {
			return err
		}
		var verifierID pgtype.UUID
		if actor.ID.Valid {
			verifierID = actor.ID
		}
		if _, err := q.InsertOutcomeVerification(ctx, verificationParams(ws, o.ID, in, verifierTypeMember, verifierID, pgtype.UUID{})); err != nil {
			return fmt.Errorf("record override check: %w", err)
		}
		severity := in.Severity
		if severity == "" {
			severity = "none"
		}
		failCause := ""
		if in.Result == "fail" {
			failCause = "member"
		}
		n, err := q.SetWorkOutcomeOverride(ctx, db.SetWorkOutcomeOverrideParams{
			Verdict: in.Result, Severity: severity, FailCause: failCause,
			ID: o.ID, WorkspaceID: ws, Since: pgtype.Timestamptz{Time: since, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("override outcome: %w", err)
		}
		if n == 0 {
			return ErrOverrideWindowClosed
		}
		// An override can make a high fail, so the pair's record moves with it (spec 3.2).
		if err := recordTrack(ctx, q, ws, o.AgentID, o.WorkClass, s.now()); err != nil {
			return err
		}
		changed := o
		changed.Verdict, changed.Severity = in.Result, severity
		if err := (&EnvelopeService{Now: s.now, Flags: s.Flags}).OnHighFail(ctx, q, changed); err != nil {
			return err
		}
		if err := NewGovernanceLog().Record(ctx, q, GovernanceEvent{
			WorkspaceID: ws,
			Kind:        GovernanceKindOutcomeOverridden,
			ActorType:   actor.Type,
			ActorID:     actor.ID,
			SubjectType: "work_outcome",
			SubjectID:   o.ID,
			Summary:     fmt.Sprintf("Outcome %s changed from %s to %s", util.UUIDToString(o.ID), o.Verdict, in.Result),
			Details:     map[string]any{"from": o.Verdict, "to": in.Result, "note": in.Note},
		}); err != nil {
			return fmt.Errorf("record governance event: %w", err)
		}
		out, err = q.GetWorkOutcomeInWorkspace(ctx, db.GetWorkOutcomeInWorkspaceParams{ID: o.ID, WorkspaceID: ws})
		return err
	})
	return out, err
}

// PickVerifier chooses the agent that checks an outcome whose doer is doer, in the order
// of spec 2.5: the workspace's outcome_verifier_agent_id, the people reviewer, the
// Secretary, then each agent of outcome_verifier_pool. The doer is never picked. ok is
// false when no one is left.
func (s *VerificationService) PickVerifier(ctx context.Context, ws, doer pgtype.UUID) (pgtype.UUID, bool) {
	settings, secretary := s.verifierSettings(ctx, ws)
	var candidates []pgtype.UUID
	if id, ok := parseAgentRef(settings.OutcomeVerifierAgentID); ok {
		candidates = append(candidates, id)
	}
	if reviewer, err := s.Queries.GetReviewerAgent(ctx, ws); err == nil {
		candidates = append(candidates, reviewer.ID)
	}
	if secretary.Valid {
		candidates = append(candidates, secretary)
	}
	for _, raw := range settings.OutcomeVerifierPool {
		if id, ok := parseAgentRef(raw); ok {
			candidates = append(candidates, id)
		}
	}
	for _, id := range candidates {
		if id == doer {
			continue
		}
		agent, err := s.Queries.GetAgent(ctx, id)
		if err != nil || agent.WorkspaceID != ws || agent.ArchivedAt.Valid || !agent.RuntimeID.Valid {
			continue
		}
		return id, true
	}
	return pgtype.UUID{}, false
}

// outcomeVerifierSettings is the part of workspace.settings the verifier reads.
type outcomeVerifierSettings struct {
	OutcomeVerifierAgentID string   `json:"outcome_verifier_agent_id"`
	OutcomeVerifierPool    []string `json:"outcome_verifier_pool"`
}

// verifierSettings reads the verifier settings and the workspace's Secretary. A read that
// fails leaves the settings empty, so the next candidate is tried.
func (s *VerificationService) verifierSettings(ctx context.Context, ws pgtype.UUID) (outcomeVerifierSettings, pgtype.UUID) {
	var out outcomeVerifierSettings
	w, err := s.Queries.GetWorkspace(ctx, ws)
	if err != nil {
		slog.Warn("durable verifier: read workspace failed", "workspace_id", util.UUIDToString(ws), "error", err)
		return out, pgtype.UUID{}
	}
	if len(w.Settings) > 0 {
		if err := json.Unmarshal(w.Settings, &out); err != nil {
			slog.Warn("durable verifier: workspace settings are not an object", "workspace_id", util.UUIDToString(ws), "error", err)
		}
	}
	return out, w.SecretaryAgentID
}

// parseAgentRef reads an agent id from a setting; a blank or malformed one is absent.
func parseAgentRef(s string) (pgtype.UUID, bool) {
	if strings.TrimSpace(s) == "" {
		return pgtype.UUID{}, false
	}
	id, err := util.ParseUUID(s)
	if err != nil {
		return pgtype.UUID{}, false
	}
	return id, true
}

// ScheduleChecks gives every current outcome its checks (see Schedule). One outcome that
// fails is logged and the rest still run, so one bad issue cannot stall the job.
func (s *VerificationService) ScheduleChecks(ctx context.Context) (int, error) {
	total := 0
	var after pgtype.Timestamptz
	var afterID pgtype.UUID
	for {
		page, err := s.Queries.ListSchedulableOutcomes(ctx, db.ListSchedulableOutcomesParams{
			AfterAt: after, AfterID: afterID, RowLimit: outcomeBatchSize,
		})
		if err != nil {
			return total, fmt.Errorf("list schedulable outcomes: %w", err)
		}
		for _, o := range page {
			if err := s.Schedule(ctx, o); err != nil {
				slog.Error("durable checks: schedule outcome failed", "outcome_id", util.UUIDToString(o.ID), "error", err)
				continue
			}
			total++
		}
		if len(page) < outcomeBatchSize {
			return total, nil
		}
		last := page[len(page)-1]
		after, afterID = last.CreatedAt, last.ID
	}
}

// Schedule gives one outcome what it needs to be checked (spec 2.5):
//   - an outcome with no criteria gets the implicit agent_judgment criterion;
//   - an artifact criterion passes as soon as a document of its kind is published from
//     the issue;
//   - agent_judgment and command criteria go to one verification task, for the verifier
//     PickVerifier chooses; with no verifier they are recorded as unable.
//
// Manual criteria wait for a person; metric criteria are checked hourly by CheckMetrics.
// Each step is idempotent, so the job calls Schedule for an outcome again each minute
// until every criterion is handled.
func (s *VerificationService) Schedule(ctx context.Context, o db.WorkOutcome) error {
	if !s.outcomesEnabled(ctx) {
		return nil
	}
	if !o.Sampled || o.SettledAt.Valid || o.SupersededBy.Valid {
		return nil
	}
	criteria, err := s.Queries.ListIssueAcceptanceCriteria(ctx, db.ListIssueAcceptanceCriteriaParams{WorkspaceID: o.WorkspaceID, IssueID: o.IssueID})
	if err != nil {
		return fmt.Errorf("list criteria: %w", err)
	}
	open := slices.DeleteFunc(slices.Clone(criteria), func(c db.AcceptanceCriterion) bool { return c.ArchivedAt.Valid })
	if len(open) == 0 {
		implicit, err := s.addImplicitCriterion(ctx, o.WorkspaceID, o.IssueID)
		if err != nil {
			return err
		}
		open = []db.AcceptanceCriterion{implicit}
	}
	handled, err := s.handledCriteria(ctx, o)
	if err != nil {
		return err
	}
	var agentChecks []db.AcceptanceCriterion
	for _, c := range open {
		if handled[util.UUIDToString(c.ID)] {
			continue
		}
		switch c.CheckKind {
		case CheckKindArtifact:
			if err := s.checkArtifact(ctx, o, c); err != nil {
				return err
			}
		case CheckKindManual:
			s.admitManualCheck(ctx, o)
		case CheckKindAgentJudgment, CheckKindCommand:
			if s.Jury != nil {
				eligible, err := s.Jury.Eligible(ctx, o, c)
				if err != nil {
					return err
				}
				if eligible {
					if err := s.Jury.Open(ctx, o, c); err != nil {
						return err
					}
					continue
				}
			}
			agentChecks = append(agentChecks, c)
		}
	}
	if len(agentChecks) == 0 {
		return nil
	}
	return s.assignAgentChecks(ctx, o, agentChecks)
}

// addImplicitCriterion writes the criterion a sampled outcome gets when its issue has none.
func (s *VerificationService) addImplicitCriterion(ctx context.Context, ws, issue pgtype.UUID) (db.AcceptanceCriterion, error) {
	var out db.AcceptanceCriterion
	err := s.inTx(ctx, func(q *db.Queries) error {
		if _, err := q.LockIssueForCriterionWrite(ctx, db.LockIssueForCriterionWriteParams{ID: issue, WorkspaceID: ws}); err != nil {
			return mapNoRows(err, ErrCriterionNotFound, "lock issue")
		}
		all, err := q.ListIssueAcceptanceCriteria(ctx, db.ListIssueAcceptanceCriteriaParams{WorkspaceID: ws, IssueID: issue})
		if err != nil {
			return fmt.Errorf("list criteria: %w", err)
		}
		for _, c := range all {
			if !c.ArchivedAt.Valid {
				out = c
				return nil
			}
		}
		out, err = s.insertCriterion(ctx, q, ws, issue, GovernanceActor{Type: GovernanceActorSystem}, criterionSourceTemplate, CriterionInput{
			Text: "Resolves the request", CheckKind: CheckKindAgentJudgment, CheckSpec: map[string]any{"rubric": implicitRubric},
		})
		return err
	})
	return out, err
}

// handledCriteria is the set of criteria of an outcome that already have an assignment
// or a check. The key is the criterion id.
func (s *VerificationService) handledCriteria(ctx context.Context, o db.WorkOutcome) (map[string]bool, error) {
	handled := map[string]bool{}
	assigned, err := s.Queries.ListOutcomeCheckTaskCriteria(ctx, db.ListOutcomeCheckTaskCriteriaParams{OutcomeID: o.ID, WorkspaceID: o.WorkspaceID})
	if err != nil {
		return nil, fmt.Errorf("list assigned checks: %w", err)
	}
	for _, id := range assigned {
		handled[util.UUIDToString(id)] = true
	}
	checked, err := s.Queries.ListLatestCriterionVerifications(ctx, o.ID)
	if err != nil {
		return nil, fmt.Errorf("list checks: %w", err)
	}
	for _, v := range checked {
		handled[util.UUIDToString(v.CriterionID)] = true
	}
	panels, err := s.Queries.ListJuryPanelsForOutcome(ctx, db.ListJuryPanelsForOutcomeParams{WorkspaceID: o.WorkspaceID, OutcomeID: o.ID})
	if err != nil {
		return nil, fmt.Errorf("list jury panels: %w", err)
	}
	for _, p := range panels {
		handled[util.UUIDToString(p.CriterionID)] = true
	}
	return handled, nil
}

// checkArtifact passes an artifact criterion once a published document of its kind is
// linked to the issue. Until then nothing is written, and the next run looks again.
func (s *VerificationService) checkArtifact(ctx context.Context, o db.WorkOutcome, c db.AcceptanceCriterion) error {
	var spec struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(c.CheckSpec, &spec); err != nil || spec.Kind == "" {
		return fmt.Errorf("artifact criterion %s has no kind", util.UUIDToString(c.ID))
	}
	published, err := s.Queries.HasPublishedIssueKnowledge(ctx, db.HasPublishedIssueKnowledgeParams{
		WorkspaceID: o.WorkspaceID, IssueID: o.IssueID, Kind: spec.Kind,
	})
	if err != nil {
		return fmt.Errorf("check published artifact: %w", err)
	}
	if !published {
		return nil
	}
	_, err = s.Queries.InsertOutcomeVerification(ctx, verificationParams(o.WorkspaceID, o.ID, VerificationInput{
		CriterionID: c.ID, Result: "pass", Note: "A document of kind " + spec.Kind + " is published for this issue.",
	}, verifierTypeSystem, pgtype.UUID{}, pgtype.UUID{}))
	return err
}

// assignAgentChecks gives the agent criteria of an outcome to one verifier. The task is
// marked with the outcome in the same transaction as the criterion rows, so a retry finds
// the task and completes the rows instead of making a second task.
func (s *VerificationService) assignAgentChecks(ctx context.Context, o db.WorkOutcome, criteria []db.AcceptanceCriterion) error {
	verifier, ok := s.PickVerifier(ctx, o.WorkspaceID, o.AgentID)
	if !ok {
		for _, c := range criteria {
			_, err := s.Queries.InsertOutcomeVerification(ctx, verificationParams(o.WorkspaceID, o.ID, VerificationInput{
				CriterionID: c.ID, Result: "unable", Note: "No verifier is available for this outcome.",
			}, verifierTypeSystem, pgtype.UUID{}, pgtype.UUID{}))
			if err != nil {
				return fmt.Errorf("record unable check: %w", err)
			}
		}
		return nil
	}
	if s.Tasks == nil {
		return errors.New("durable checks: no task service to give an agent its check")
	}
	issue, err := s.Queries.GetIssue(ctx, o.IssueID)
	if err != nil {
		return fmt.Errorf("load issue: %w", err)
	}
	task, err := s.Tasks.EnqueueVerificationTask(ctx, issue, verifier)
	if errors.Is(err, ErrDuplicatePendingTask) {
		pending, perr := s.Queries.GetPendingAgentTaskOnIssue(ctx, db.GetPendingAgentTaskOnIssueParams{IssueID: o.IssueID, AgentID: verifier})
		if perr != nil {
			return fmt.Errorf("find pending verification task: %w", perr)
		}
		if other := durableOutcomeOf(pending); other != "" && other != util.UUIDToString(o.ID) {
			// The verifier's pending task belongs to an earlier outcome of this issue. It
			// is checked again once that task is done.
			return nil
		}
		task = pending
	} else if err != nil {
		return fmt.Errorf("enqueue verification task: %w", err)
	}

	return s.inTx(ctx, func(q *db.Queries) error {
		if _, err := q.SetAgentTaskDurableContext(ctx, db.SetAgentTaskDurableContextParams{
			OutcomeID: util.UUIDToString(o.ID), ID: task.ID,
		}); err != nil {
			return fmt.Errorf("mark verification task: %w", err)
		}
		for _, c := range criteria {
			if err := q.InsertOutcomeCheckTask(ctx, db.InsertOutcomeCheckTaskParams{
				WorkspaceID: o.WorkspaceID, OutcomeID: o.ID, CriterionID: c.ID, TaskID: task.ID,
			}); err != nil {
				return fmt.Errorf("assign check: %w", err)
			}
		}
		return nil
	})
}

// durableOutcomeOf reads the outcome a task was marked with, "" when it is not a
// verification task.
func durableOutcomeOf(task db.AgentTaskQueue) string {
	var ctxMap struct {
		DurableKind string `json:"durable_kind"`
		OutcomeID   string `json:"outcome_id"`
	}
	if len(task.Context) == 0 || json.Unmarshal(task.Context, &ctxMap) != nil || ctxMap.DurableKind != "verification" {
		return ""
	}
	return ctxMap.OutcomeID
}

// CheckMetrics decides the metric criteria of current outcomes. A reading in the window
// that reaches the value passes the criterion; past the window without one it fails with
// the metric as verifier. It returns how many checks it wrote. It runs hourly (spec 2.5).
//
// One batch of the open criteria is read per run. A criterion still inside its window
// stays open and comes up again on the next run; a deployment with more open metric
// criteria than metricCheckBatch would need keyset paging here.
func (s *VerificationService) CheckMetrics(ctx context.Context, now time.Time) (int, error) {
	if !s.outcomesEnabled(ctx) {
		return 0, nil
	}
	rows, err := s.Queries.ListOpenSystemCriteria(ctx, db.ListOpenSystemCriteriaParams{CheckKind: CheckKindMetric, RowLimit: metricCheckBatch})
	if err != nil {
		return 0, fmt.Errorf("list metric criteria: %w", err)
	}
	written := 0
	for _, r := range rows {
		ok, err := s.decideMetric(ctx, now, r)
		if err != nil {
			slog.Error("durable metric check failed", "criterion_id", util.UUIDToString(r.CriterionID), "error", err)
			continue
		}
		if ok {
			written++
		}
	}
	return written, nil
}

// decideMetric writes the verdict of one metric criterion when it has one.
func (s *VerificationService) decideMetric(ctx context.Context, now time.Time, r db.ListOpenSystemCriteriaRow) (bool, error) {
	var spec struct {
		MetricKey  string  `json:"metric_key"`
		Value      float64 `json:"value"`
		WithinDays int     `json:"within_days"`
	}
	if err := json.Unmarshal(r.CheckSpec, &spec); err != nil {
		return false, fmt.Errorf("metric spec: %w", err)
	}
	since := r.CriterionCreatedAt.Time
	best, err := s.Queries.LatestMetricReadingSince(ctx, db.LatestMetricReadingSinceParams{
		WorkspaceID: r.WorkspaceID, MetricKey: spec.MetricKey, Since: pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("read metric: %w", err)
	}
	if best.Valid {
		if f, err := best.Float64Value(); err == nil && f.Valid && f.Float64 >= spec.Value {
			return s.writeMetricCheck(ctx, r, "pass", fmt.Sprintf("%s reached %v.", spec.MetricKey, spec.Value))
		}
	}
	if now.Before(since.Add(time.Duration(spec.WithinDays) * 24 * time.Hour)) {
		return false, nil
	}
	return s.writeMetricCheck(ctx, r, "fail", fmt.Sprintf("%s did not reach %v within %d days.", spec.MetricKey, spec.Value, spec.WithinDays))
}

func (s *VerificationService) writeMetricCheck(ctx context.Context, r db.ListOpenSystemCriteriaRow, result, note string) (bool, error) {
	_, err := s.Queries.InsertOutcomeVerification(ctx, verificationParams(r.WorkspaceID, r.OutcomeID, VerificationInput{
		CriterionID: r.CriterionID, Result: result, Note: note,
	}, verifierTypeMetric, pgtype.UUID{}, pgtype.UUID{}))
	if err != nil {
		return false, fmt.Errorf("record metric check: %w", err)
	}
	return true, nil
}

// inTx runs fn in a transaction bound to the service's queries.
func (s *VerificationService) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := fn(s.Queries.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// mapNoRows turns a missing row into the given error and wraps any other failure.
func mapNoRows(err error, missing error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return missing
	}
	return fmt.Errorf("%s: %w", what, err)
}

// GovernanceKindOutcomeOverridden is written when a person changes a settled conclusion
// (spec 2.6). The kind joins the governance log's set below, not the upstream list.
const GovernanceKindOutcomeOverridden = "outcome_overridden"

func init() {
	governanceKinds[GovernanceKindOutcomeOverridden] = true
}
