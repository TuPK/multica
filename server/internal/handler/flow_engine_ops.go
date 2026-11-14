package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

// The operations around Advance: the lead's step, the variables, a person's exit,
// resume and detach, and following a status a person set by hand.

// flowKnowsStatus reports whether a board status is one the flow knows: a step, a
// terminal, or the status requests park in at a stop.
func flowKnowsStatus(g issueworkflow.Graph, key string) bool {
	if _, ok := g.Step(key); ok {
		return true
	}
	if _, ok := g.Terminal(key); ok {
		return true
	}
	return key == issuestatus.Blocked
}

// lockFlowRun starts a transaction, locks the request and loads its flow.
func (h *Handler) lockFlowRun(ctx context.Context, request db.Issue) (pgx.Tx, *db.Queries, *flowRun, error) {
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("begin flow transaction: %w", err)
	}
	qtx := h.Queries.WithTx(tx)
	locked, err := qtx.LockIssueForFlow(ctx, db.LockIssueForFlowParams{ID: request.ID, WorkspaceID: request.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		tx.Rollback(ctx)
		return nil, nil, nil, errFlowNotManaged
	}
	if err != nil {
		tx.Rollback(ctx)
		return nil, nil, nil, err
	}
	run, err := h.loadFlowRun(ctx, qtx, locked)
	if err != nil {
		tx.Rollback(ctx)
		return nil, nil, nil, err
	}
	return tx, qtx, run, nil
}

func wrongStep(step, message string) error {
	return &flowRejection{Status: 409, Code: "flow_wrong_step", Message: message, Fields: map[string]any{"step": step}}
}

func switchInvalid(key, message string, allowed any) error {
	return &flowRejection{Status: 422, Code: "flow_switch_invalid", Message: message, Fields: map[string]any{"switch": key, "allowed": allowed}}
}

func varInvalid(key, message string) error {
	return &flowRejection{Status: 422, Code: "flow_var_invalid", Message: message, Fields: map[string]any{"var": key}}
}

// checkSwitchValues validates switch values against the declaration of a flow of
// schema 1.
func checkSwitchValues(cfg *issueworkflow.FlowConfig, values map[string]any) error {
	declared := make([]string, 0, len(cfg.Switches))
	for _, s := range cfg.Switches {
		declared = append(declared, s.Key)
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		sw, ok := cfg.Switch(key)
		if !ok {
			return switchInvalid(key, fmt.Sprintf("the switch %q is not declared; use one of: %v", key, declared), declared)
		}
		if err := sw.CheckValue(values[key]); err != nil {
			allowed := any([]bool{true, false})
			if sw.Type == issueworkflow.SwitchEnum {
				allowed = sw.Values
			}
			return switchInvalid(key, err.Error(), allowed)
		}
	}
	return nil
}

// checkFlowVars validates variable values and returns them in the form the state
// stores. A flow of schema 1 keeps its messages and codes; a schema 2 flow checks
// each value against the declaration of its variable.
func checkFlowVars(run *flowRun, values map[string]any, allowed []string) (map[string]any, error) {
	if run.compat() {
		if err := checkSwitchValues(run.def.Flow, values); err != nil {
			return nil, err
		}
		return values, nil
	}
	out := make(map[string]any, len(values))
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		v, ok := run.def.Graph.Var(key)
		if !ok {
			return nil, varInvalid(key, fmt.Sprintf("the variable %q is not declared", key))
		}
		if allowed != nil && !flowHasString(allowed, key) {
			return nil, &flowRejection{Status: 403, Code: "flow_var_forbidden", Message: fmt.Sprintf("the step does not set the variable %q", key), Fields: map[string]any{"var": key}}
		}
		value, err := issueworkflow.CheckVarValue(v, values[key])
		if err != nil {
			return nil, varInvalid(key, err.Error())
		}
		out[key] = value
	}
	return out, nil
}

func flowHasString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// mergedVars overlays chosen values on the stored ones, keeping the default of
// every declared variable.
func mergedVars(g issueworkflow.Graph, stored, chosen map[string]any) map[string]any {
	out := map[string]any{}
	for _, v := range g.Doc.Vars {
		out[v.Key] = v.Default
	}
	for k, v := range stored {
		out[k] = v
	}
	for k, v := range chosen {
		out[k] = v
	}
	return out
}

// FlowPlanInput is the lead's plan: the switch values it chose, or a report
// that the request lacks grounds.
type FlowPlanInput struct {
	Switches map[string]any
	Blocked  *FlowPlanBlocked
}

// FlowPlanBlocked is the lead's report that grounds are missing (stop 1).
type FlowPlanBlocked struct {
	Missing []string
	Reason  string
}

// FlowCompleteInput is the lead's report on a lead step: its verdict and the
// variables it sets, or that the grounds are missing.
type FlowCompleteInput struct {
	Verdict string
	Vars    map[string]any
	Blocked *FlowPlanBlocked
}

// PlanFlow records the lead's plan: it completes the plan step with the switches
// the lead chose, or with stop 1 when the lead reports missing grounds.
func (h *Handler) PlanFlow(ctx context.Context, request db.Issue, in FlowPlanInput) (IssueFlowStateResponse, error) {
	return h.CompleteFlowStep(ctx, request, "", FlowCompleteInput{Vars: in.Switches, Blocked: in.Blocked})
}

// CompleteFlowStep completes the lead step the request is at (step empty means
// that step): it sets the variables the lead gave and the verdict atomically,
// routes the request on, or to the stop for missing grounds, and then advances
// the request, which opens the first tasks.
func (h *Handler) CompleteFlowStep(ctx context.Context, request db.Issue, step string, in FlowCompleteInput) (IssueFlowStateResponse, error) {
	tx, qtx, run, err := h.lockFlowRun(ctx, request)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	defer tx.Rollback(ctx)
	g := run.def.Graph
	current, isStep := g.Step(run.state.Step)
	if step == "" {
		step = run.state.Step
	}
	if run.state.Step != step || run.state.ClosedAt != "" || run.state.Stop != nil || !isStep || current.Kind != issueworkflow.StepKindLead {
		return IssueFlowStateResponse{}, wrongStep(run.state.Step, fmt.Sprintf("the request is at %q: only a request at the plan step can be planned", run.state.Step))
	}
	var allowed []string
	if !run.compat() && current.Lead != nil {
		allowed = append([]string{}, current.Lead.Sets...)
	}
	if !run.compat() && allowed == nil {
		allowed = []string{}
	}
	chosen, err := checkFlowVars(run, in.Vars, allowed)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	verdict := in.Verdict
	switch {
	case in.Blocked != nil:
		verdict = issueworkflow.OutcomeBlocked
	case verdict == "":
		verdict = "planned"
	}
	if !leadVerdictAllowed(g, current, verdict) {
		return IssueFlowStateResponse{}, &flowRejection{
			Status: 422, Code: "flow_outcome_invalid",
			Message: fmt.Sprintf("the step %q has no verdict %q", current.Key, verdict),
			Fields:  map[string]any{"allowed_outcomes": leadVerdictKeys(g, current)},
		}
	}
	next := run.state.Clone()
	next.Vars = mergedVars(g, run.state.Vars, chosen)
	next.PlanDone = true
	saved, err := h.saveFlowState(ctx, qtx, run, next)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	if !saved {
		return IssueFlowStateResponse{}, wrongStep(run.state.Step, "the request moved while it was being planned; read its state and try again")
	}
	rows, err := h.loadFlowTasks(ctx, qtx, run)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	note := flowRouteNote{}
	if in.Blocked != nil {
		note = flowRouteNote{Missing: in.Blocked.Missing, Reason: in.Blocked.Reason}
	}
	var effects flowEffects
	decision := issueworkflow.RouteLead(g, run.state, verdict, issueworkflow.BuildGraphFacts(g, run.state, nil, flowRequestFacts(run.issue)))
	if decision.Kind == issueworkflow.DecisionRoute {
		routed, err := h.routeFlowRequest(ctx, qtx, run, decision, rows, note, &effects, "")
		if err != nil {
			return IssueFlowStateResponse{}, err
		}
		if !routed {
			return IssueFlowStateResponse{}, wrongStep(run.state.Step, "the request moved while it was being planned; read its state and try again")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return IssueFlowStateResponse{}, fmt.Errorf("commit flow plan: %w", err)
	}
	h.publishFlowEffects(ctx, run.issue, &effects)
	if in.Blocked == nil {
		resp, err := h.AdvanceFlow(ctx, run.issue)
		if err != nil {
			return IssueFlowStateResponse{}, err
		}
		return resp.State, nil
	}
	return h.flowStateResponse(ctx, run.issue, issueworkflow.RoleRequest)
}

// leadVerdictKeys are the verdicts a lead step accepts besides blocked.
func leadVerdictKeys(g issueworkflow.Graph, step issueworkflow.DocStep) []string {
	set := issueworkflow.VerdictSetPlan
	if step.Lead != nil && step.Lead.Verdicts != "" {
		set = step.Lead.Verdicts
	}
	var keys []string
	for _, v := range issueworkflow.TaskVerdicts(g.Doc, issueworkflow.DocTask{Kind: issueworkflow.KindWork, Verdicts: set}) {
		keys = append(keys, v.Key)
	}
	return keys
}

func leadVerdictAllowed(g issueworkflow.Graph, step issueworkflow.DocStep, verdict string) bool {
	return verdict == issueworkflow.OutcomeBlocked || flowHasString(leadVerdictKeys(g, step), verdict)
}

// flowVarSetter is who sets a variable of a request: a member who manages the
// squad (any step, logged), or a role the declaration of the variable names.
type flowVarSetter struct {
	Manager bool
	// Kinds are the built-in roles the caller holds on this request: lead, requester.
	Kinds []string
	// By is the user or agent id, for the log.
	By string
}

// SetFlowSwitch sets one switch of a request of schema 1: the lead sets switches
// at the plan step; a person who manages the squad may set one at any step, which
// is logged.
func (h *Handler) SetFlowSwitch(ctx context.Context, request db.Issue, key string, value any, manager bool) (IssueFlowStateResponse, error) {
	return h.SetFlowVar(ctx, request, key, value, flowVarSetter{Manager: manager, Kinds: []string{issueworkflow.RoleKindLead}})
}

// SetFlowVar sets one variable of a request. The variable must be set by a role
// the setter holds at a step its declaration allows, unless the setter manages the
// squad. A task a new value makes expected opens on the next advance; a variable
// the current step fans out over cannot change once the fan-out has started.
func (h *Handler) SetFlowVar(ctx context.Context, request db.Issue, key string, value any, who flowVarSetter) (IssueFlowStateResponse, error) {
	tx, qtx, run, err := h.lockFlowRun(ctx, request)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	defer tx.Rollback(ctx)
	g := run.def.Graph
	declared, found := g.Var(key)
	if run.compat() && !who.Manager && run.state.Step != issueworkflow.StartStatus {
		return IssueFlowStateResponse{}, &flowRejection{
			Status: 409, Code: "flow_switch_locked",
			Message: fmt.Sprintf("switches are set at the plan step; the request is at %q", run.state.Step),
			Fields:  map[string]any{"step": run.state.Step},
		}
	}
	if found && !who.Manager && !run.compat() {
		held := false
		for _, kind := range who.Kinds {
			if flowHasString(declared.SetBy, kind) {
				held = true
			}
		}
		if !held || !issueworkflow.VarAllowedAt(declared, run.state.Step) {
			return IssueFlowStateResponse{}, &flowRejection{
				Status: 403, Code: "flow_var_forbidden",
				Message: fmt.Sprintf("the variable %q is set by %v at %v; the request is at %q", key, declared.SetBy, declared.SetAt, run.state.Step),
				Fields:  map[string]any{"var": key, "step": run.state.Step},
			}
		}
	}
	chosen, err := checkFlowVars(run, map[string]any{key: value}, nil)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	if err := h.checkVarNotInUse(ctx, qtx, run, key); err != nil {
		return IssueFlowStateResponse{}, err
	}
	next := run.state.Clone()
	next.Vars = mergedVars(g, run.state.Vars, chosen)
	saved, err := h.saveFlowState(ctx, qtx, run, next)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	if !saved {
		return IssueFlowStateResponse{}, wrongStep(run.state.Step, "the request moved while the switch was being set; read its state and try again")
	}
	atSetPoint := !found || (flowHasString(declared.SetBy, issueworkflow.RoleKindLead) && issueworkflow.VarAllowedAt(declared, run.state.Step))
	if run.compat() {
		atSetPoint = run.state.Step == issueworkflow.StartStatus
	}
	if !atSetPoint || who.Manager && !run.compat() {
		action, field := "flow_var_set", "var"
		if run.compat() {
			action, field = "flow_switch_set", "switch"
		}
		details, _ := json.Marshal(map[string]any{field: key, "value": chosen[key], "step": run.state.Step, "by": who.By})
		if _, err := qtx.CreateActivity(ctx, db.CreateActivityParams{
			ID: dbid.NewV7(), WorkspaceID: run.issue.WorkspaceID, IssueID: run.issue.ID,
			ActorType: pgtype.Text{String: "system", Valid: true},
			Action:    action, Details: details,
		}); err != nil {
			return IssueFlowStateResponse{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return IssueFlowStateResponse{}, fmt.Errorf("commit flow switch: %w", err)
	}
	h.publishFlowUpdated(run.issue)
	return h.flowStateResponse(ctx, run.issue, issueworkflow.RoleRequest)
}

// checkVarNotInUse refuses a change of a variable the current step fans a task out
// over, once the first row of that task is open in this entry (spec 4.2a): row i
// must stay the i-th item.
func (h *Handler) checkVarNotInUse(ctx context.Context, qtx *db.Queries, run *flowRun, key string) error {
	step, ok := run.def.Graph.Step(run.state.Step)
	if !ok {
		return nil
	}
	rows, err := h.loadFlowTasks(ctx, qtx, run)
	if err != nil {
		return err
	}
	for _, t := range step.Tasks {
		if t.Instances == nil || t.Instances.FromVar != key {
			continue
		}
		for _, r := range rows {
			if r.state.Step == run.state.Step && r.state.Round == run.round() && issueworkflow.TaskBaseKey(r.state.Key) == t.Key {
				return &flowRejection{
					Status: 409, Code: "flow_var_in_use",
					Message: fmt.Sprintf("the step fans the task %q out over %q and has started: the list cannot change until the request leaves the step", t.Key, key),
					Fields:  map[string]any{"var": key, "task": t.Key},
				}
			}
		}
	}
	return nil
}

// FlowResumeInput is a person's decision to continue a stopped request.
type FlowResumeInput struct {
	// ToStep is where to continue; empty continues at the step the request
	// stopped from.
	ToStep      string
	ResetRework bool
	// ResetLoops are the loops whose count starts over on a schema 2 request; "all"
	// names every loop.
	ResetLoops []string
	// SkipCancelled lets the step go on without the work tasks a person cancelled,
	// which the engine otherwise reopens because their files are part of the step.
	SkipCancelled bool
	// AcknowledgeSkipCheckpoint says the manager knows no person has seen the agent work of a
	// request the engine stalled because of it (spec 4.7, 4.8), and resumes it anyway. The
	// resume then counts as the human checkpoint.
	AcknowledgeSkipCheckpoint bool
	// By is the user who resumes: the one a checkpoint made by the resume names.
	By string
}

// ResumeFlow continues a request that stopped. After stop 1 or a stall the
// tasks that blocked open again and the others stand. After stop 3 the rework
// count starts over and the step's tasks open in a new round. With ToStep the
// request goes there and its tasks open in a new round. With SkipCancelled the
// work tasks a person cancelled in the round that goes on stand cancelled. A
// request of a schema 2 flow re-enters the step: its tasks open in a new entry.
func (h *Handler) ResumeFlow(ctx context.Context, request db.Issue, in FlowResumeInput) (IssueFlowStateResponse, error) {
	tx, qtx, run, err := h.lockFlowRun(ctx, request)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	defer tx.Rollback(ctx)
	if run.state.ClosedAt != "" || run.state.Stop == nil {
		return IssueFlowStateResponse{}, wrongStep(run.state.Step, "the request is not stopped; there is nothing to resume")
	}
	g := run.def.Graph
	target := in.ToStep
	if target == "" {
		target = run.state.BlockedFrom
	}
	if target == "" {
		target = g.Doc.Start
	}
	if _, terminal := g.Terminal(target); terminal || target == issuestatus.Blocked || target == issuestatus.Done || target == issuestatus.Cancelled {
		return IssueFlowStateResponse{}, &flowRejection{Status: 400, Code: "flow_resume_invalid", Message: fmt.Sprintf("a request cannot resume at %q", target)}
	}
	if !flowKnowsStatus(g, target) {
		return IssueFlowStateResponse{}, &flowRejection{Status: 400, Code: "flow_resume_invalid", Message: fmt.Sprintf("the workflow has no step %q", target)}
	}
	rows, err := h.loadFlowTasks(ctx, qtx, run)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	var next issueworkflow.State
	stop3 := run.state.Stop.Code == issueworkflow.StopLoopExhausted
	freshRound := true
	if run.compat() {
		next = run.state.Clone()
		next.Step = target
		next.Stop = nil
		next.BlockedFrom = ""
		next.EnteredAt = flowNow()
		// A manager who resumes the request has looked at it: a human checkpoint (spec 4.8).
		next.Unreviewed = false
		if in.ResetRework || stop3 {
			next.Loops[issueworkflow.LoopRework] = 0
		}
		freshRound = in.ToStep != "" || in.ResetRework || stop3
		if freshRound {
			highest := -1
			for _, r := range rows {
				if r.state.Round > highest {
					highest = r.state.Round
				}
			}
			next.RoundBase = highest + 1 - next.Loops[issueworkflow.LoopRework]
			if next.RoundBase < 0 {
				next.RoundBase = 0
			}
		}
	} else {
		// A request the engine stalled while agent work was unseen cannot go on because a
		// manager pressed resume: only one who says so takes the place of the checkpoint.
		skipsCheckpoint := run.state.Unreviewed && run.state.Stop.Code == issueworkflow.DocStopStalled
		if skipsCheckpoint && !in.AcknowledgeSkipCheckpoint {
			return IssueFlowStateResponse{}, &flowRejection{
				Status: 422, Code: "flow_checkpoint_ack_required",
				Message: "no person has seen the work of an agent on this request: resume with acknowledge_skip_checkpoint to take the place of that check",
				Fields:  map[string]any{"step": target},
			}
		}
		meta := issueworkflow.ApplyMeta{Now: flowNow(), By: in.By}
		next = issueworkflow.ApplyResume(g, run.state, target, meta)
		if skipsCheckpoint {
			next.Checkpoints = append(append([]issueworkflow.Checkpoint(nil), next.Checkpoints...),
				issueworkflow.Checkpoint{Step: target, Activation: next.Activations[target], By: in.By, At: meta.Now})
		}
		next.Unreviewed = false
		next.Loops = copyLoops(next.Loops)
		for _, loop := range g.Doc.Loops {
			if in.ResetRework && loop.Key == issueworkflow.LoopRework || flowHasString(in.ResetLoops, loop.Key) || flowHasString(in.ResetLoops, "all") {
				next.Loops[loop.Key] = 0
			}
		}
	}
	saved, err := h.saveFlowState(ctx, qtx, run, next)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	if !saved {
		return IssueFlowStateResponse{}, wrongStep(run.state.Step, "the request moved while it was being resumed; read its state and try again")
	}
	var effects flowEffects
	if err := moveFlowRequest(ctx, qtx, run, run.stepStatus(), &effects); err != nil {
		return IssueFlowStateResponse{}, err
	}
	if !freshRound {
		for _, r := range rows {
			if r.state.Step != target || r.state.Round != run.round() || r.issue.Status != issuestatus.Blocked {
				continue
			}
			prev := r.issue.Status
			reopened, err := qtx.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{ID: r.issue.ID, Status: issuestatus.Todo, WorkspaceID: r.issue.WorkspaceID})
			if err != nil {
				return IssueFlowStateResponse{}, fmt.Errorf("reopen task %s: %w", r.state.Key, err)
			}
			if err := qtx.SetIssueOutcome(ctx, db.SetIssueOutcomeParams{ID: r.issue.ID, WorkspaceID: r.issue.WorkspaceID}); err != nil {
				return IssueFlowStateResponse{}, err
			}
			reopened.Outcome = pgtype.Text{}
			effects.moves = append(effects.moves, statusMove{prev: prev, issue: reopened})
			effects.reopened = append(effects.reopened, reopened)
		}
	}
	if in.SkipCancelled && !freshRound {
		for _, r := range rows {
			if r.state.Step != target || r.state.Round != run.round() || !cancelledForItsFiles(run, r) {
				continue
			}
			// An outcome on a cancelled task is the mark that the step goes on without it.
			if err := qtx.SetIssueOutcome(ctx, db.SetIssueOutcomeParams{
				ID: r.issue.ID, WorkspaceID: r.issue.WorkspaceID,
				Outcome: pgtype.Text{String: issueworkflow.OutcomeDone, Valid: true},
			}); err != nil {
				return IssueFlowStateResponse{}, err
			}
			details, _ := json.Marshal(map[string]any{"task": r.state.Key, "step": r.state.Step, "round": r.state.Round})
			if _, err := qtx.CreateActivity(ctx, db.CreateActivityParams{
				ID: dbid.NewV7(), WorkspaceID: run.issue.WorkspaceID, IssueID: r.issue.ID,
				ActorType: pgtype.Text{String: "system", Valid: true}, Action: "flow_task_skipped", Details: details,
			}); err != nil {
				return IssueFlowStateResponse{}, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return IssueFlowStateResponse{}, fmt.Errorf("commit flow resume: %w", err)
	}
	h.publishFlowEffects(ctx, run.issue, &effects)
	resp, err := h.AdvanceFlow(ctx, run.issue)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	return resp.State, nil
}

func copyLoops(m map[string]int) map[string]int {
	out := make(map[string]int, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}

// DetachFlow takes a request off its flow. Its tasks become ordinary
// sub-issues, or are cancelled when cancelTasks is set.
func (h *Handler) DetachFlow(ctx context.Context, request db.Issue, cancelTasks bool) error {
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin flow detach: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	locked, err := qtx.LockIssueForFlow(ctx, db.LockIssueForFlowParams{ID: request.ID, WorkspaceID: request.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return errFlowNotManaged
	}
	if err != nil {
		return err
	}
	if _, ok := flowRequestState(locked); !ok {
		return errFlowNotManaged
	}
	var effects flowEffects
	if cancelTasks {
		issues, err := qtx.ListFlowTasks(ctx, db.ListFlowTasksParams{ParentIssueID: locked.ID, WorkspaceID: locked.WorkspaceID})
		if err != nil {
			return err
		}
		resolver := issuestatus.NewResolver(locked.WorkspaceID)
		rows := make([]flowTaskRow, len(issues))
		for i, t := range issues {
			rows[i].issue = t
			switch resolver.Category(ctx, qtx, t.Status) {
			case issuestatus.CategoryDone, issuestatus.CategoryClosed:
				rows[i].state.Closed = true
			}
		}
		if err := cancelFlowTasks(ctx, qtx, rows, nil, &effects); err != nil {
			return err
		}
	}
	if err := qtx.ClearIssueFlowTasks(ctx, db.ClearIssueFlowTasksParams{ParentIssueID: locked.ID, WorkspaceID: locked.WorkspaceID}); err != nil {
		return err
	}
	if err := qtx.ClearIssueFlow(ctx, db.ClearIssueFlowParams{ID: locked.ID, WorkspaceID: locked.WorkspaceID}); err != nil {
		return err
	}
	// The sweep starts every request of a flow squad that has no flow. This entry tells it a person
	// took this one off.
	if _, err := qtx.CreateActivity(ctx, db.CreateActivityParams{
		ID: dbid.NewV7(), WorkspaceID: locked.WorkspaceID, IssueID: locked.ID,
		ActorType: pgtype.Text{String: "system", Valid: true}, Action: "flow_detached", Details: []byte("{}"),
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit flow detach: %w", err)
	}
	locked.Flow = nil
	h.publishFlowEffects(ctx, locked, &effects)
	return nil
}

// followManualMove makes the flow state agree with a status a person set on the
// request by hand. A step of the workflow becomes the current step and the open
// tasks of the step it left are cancelled; done or cancelled ends the request
// and cancels every open task. A status the workflow does not know is ignored, and
// so is one that more than one step of a schema 2 flow has: resume names the step.
func (h *Handler) followManualMove(ctx context.Context, qtx *db.Queries, run *flowRun, rows []flowTaskRow, e *flowEffects) (bool, FlowAction, error) {
	to := run.issue.Status
	old := run.state
	g := run.def.Graph
	next := old.Clone()
	next.EnteredAt = flowNow()
	var action FlowAction
	switch issuestatus.Category(ctx, qtx, run.issue.WorkspaceID, to) {
	case issuestatus.CategoryDone, issuestatus.CategoryClosed:
		next.ClosedAt = next.EnteredAt
		next.Stop = nil
		next.Unreviewed = false
		action = FlowAction{Kind: "stopped", Step: to, Tasks: []string{}}
		if saved, err := h.saveFlowState(ctx, qtx, run, next); err != nil || !saved {
			return false, FlowAction{}, err
		}
		if err := cancelFlowTasks(ctx, qtx, rows, nil, e); err != nil {
			return false, FlowAction{}, err
		}
		if err := closeFlowRequestSideEffects(ctx, qtx, run, e); err != nil {
			return false, FlowAction{}, err
		}
	default:
		step := to
		if !run.compat() {
			var matches []string
			for _, s := range g.Doc.Steps {
				if s.Status.Key == to {
					matches = append(matches, s.Key)
				}
			}
			if len(matches) != 1 {
				return false, FlowAction{}, nil
			}
			step = matches[0]
		} else if !flowKnowsStatus(g, to) {
			return false, FlowAction{}, nil
		}
		next.Step = step
		next.Stop = nil
		next.BlockedFrom = ""
		next.Unreviewed = false
		if run.compat() && to == issuestatus.Blocked {
			next.BlockedFrom = old.Step
			next.Stop = &issueworkflow.StopRecord{Code: issueworkflow.StopGroundsMissing, From: old.Step}
		}
		if run.compat() {
			// Tasks already ran at the step in this round; a new round keeps their
			// identity free for the ones that open now.
			highest, ran := -1, false
			for _, r := range rows {
				if r.state.Round > highest {
					highest = r.state.Round
				}
				if r.state.Step == to && r.state.Round == next.Round() {
					ran = true
				}
			}
			if ran {
				next.RoundBase = highest + 1 - next.Loops[issueworkflow.LoopRework]
				if next.RoundBase < 0 {
					next.RoundBase = 0
				}
			}
		} else {
			next.Activations = copyLoops(next.Activations)
			next.Activations[step]++
		}
		action = FlowAction{Kind: "routed", Step: to, Tasks: []string{}}
		if saved, err := h.saveFlowState(ctx, qtx, run, next); err != nil || !saved {
			return false, FlowAction{}, err
		}
		if err := cancelFlowTasks(ctx, qtx, rows, func(r flowTaskRow) bool { return r.state.Step == old.Step }, e); err != nil {
			return false, FlowAction{}, err
		}
	}
	details, _ := json.Marshal(map[string]any{"from": old.Step, "to": to})
	if _, err := qtx.CreateActivity(ctx, db.CreateActivityParams{
		ID: dbid.NewV7(), WorkspaceID: run.issue.WorkspaceID, IssueID: run.issue.ID,
		ActorType: pgtype.Text{String: "system", Valid: true}, Action: "flow_manual_move", Details: details,
	}); err != nil {
		return false, FlowAction{}, err
	}
	return true, action, nil
}
