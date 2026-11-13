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

// The operations around Advance: the lead's plan, the switches, a person's
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

// checkSwitchValues validates switch values against the flow's declaration.
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

// PlanFlow records the lead's plan and leaves the plan step: it sets the
// switches the lead chose, then routes the request on, or to stop 1 when the
// lead reports missing grounds. It then advances the request, which opens the
// first tasks.
func (h *Handler) PlanFlow(ctx context.Context, request db.Issue, in FlowPlanInput) (IssueFlowStateResponse, error) {
	tx, qtx, run, err := h.lockFlowRun(ctx, request)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	defer tx.Rollback(ctx)
	if run.state.Step != issueworkflow.StartStatus || run.state.ClosedAt != "" || run.state.Stop != issueworkflow.StopNone {
		return IssueFlowStateResponse{}, wrongStep(run.state.Step, fmt.Sprintf("the request is at %q: only a request at the plan step can be planned", run.state.Step))
	}
	if err := checkSwitchValues(run.def.Flow, in.Switches); err != nil {
		return IssueFlowStateResponse{}, err
	}
	next := run.state
	next.Switches = mergedSwitches(run.def.Flow, run.state.Switches, in.Switches)
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
	// The lead's plan is its verdict: planned, or blocked when the grounds are missing.
	verdict := "planned"
	if in.Blocked != nil {
		verdict = issueworkflow.OutcomeBlocked
	}
	st := issueworkflow.Adapt(run.state)
	decision := issueworkflow.RouteLead(run.def.Graph, st, verdict, issueworkflow.BuildGraphFacts(run.def.Graph, st, nil, flowRequestFacts(run.issue)))
	if decision.Kind == issueworkflow.DecisionRoute {
		routed, err := h.routeFlowRequest(ctx, qtx, run, decision, rows, note, &effects)
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

// mergedSwitches overlays chosen values on the stored ones, keeping a default
// for every declared switch.
func mergedSwitches(cfg *issueworkflow.FlowConfig, stored, chosen map[string]any) map[string]any {
	out := cfg.DefaultSwitches()
	for k, v := range stored {
		out[k] = v
	}
	for k, v := range chosen {
		out[k] = v
	}
	return out
}

// SetFlowSwitch sets one switch. The lead sets switches at the plan step; a
// person who manages the squad may set one at any step, which is logged. A
// task a new value makes expected opens on the next advance.
func (h *Handler) SetFlowSwitch(ctx context.Context, request db.Issue, key string, value any, manager bool) (IssueFlowStateResponse, error) {
	tx, qtx, run, err := h.lockFlowRun(ctx, request)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	defer tx.Rollback(ctx)
	atPlan := run.state.Step == issueworkflow.StartStatus
	if !atPlan && !manager {
		return IssueFlowStateResponse{}, &flowRejection{
			Status: 409, Code: "flow_switch_locked",
			Message: fmt.Sprintf("switches are set at the plan step; the request is at %q", run.state.Step),
			Fields:  map[string]any{"step": run.state.Step},
		}
	}
	if err := checkSwitchValues(run.def.Flow, map[string]any{key: value}); err != nil {
		return IssueFlowStateResponse{}, err
	}
	next := run.state
	next.Switches = mergedSwitches(run.def.Flow, run.state.Switches, map[string]any{key: value})
	saved, err := h.saveFlowState(ctx, qtx, run, next)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	if !saved {
		return IssueFlowStateResponse{}, wrongStep(run.state.Step, "the request moved while the switch was being set; read its state and try again")
	}
	if !atPlan {
		details, _ := json.Marshal(map[string]any{"switch": key, "value": value, "step": run.state.Step})
		if _, err := qtx.CreateActivity(ctx, db.CreateActivityParams{
			ID: dbid.NewV7(), WorkspaceID: run.issue.WorkspaceID, IssueID: run.issue.ID,
			ActorType: pgtype.Text{String: "system", Valid: true},
			Action:    "flow_switch_set", Details: details,
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

// FlowResumeInput is a person's decision to continue a stopped request.
type FlowResumeInput struct {
	// ToStep is where to continue; empty continues at the step the request
	// stopped from.
	ToStep      string
	ResetRework bool
	// SkipCancelled lets the step go on without the work tasks a person cancelled,
	// which the engine otherwise reopens because their files are part of the step.
	SkipCancelled bool
}

// ResumeFlow continues a request that stopped. After stop 1 or a stall the
// tasks that blocked open again and the others stand. After stop 3 the rework
// count starts over and the step's tasks open in a new round. With ToStep the
// request goes there and its tasks open in a new round. With SkipCancelled the
// work tasks a person cancelled in the round that goes on stand cancelled.
func (h *Handler) ResumeFlow(ctx context.Context, request db.Issue, in FlowResumeInput) (IssueFlowStateResponse, error) {
	tx, qtx, run, err := h.lockFlowRun(ctx, request)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	defer tx.Rollback(ctx)
	if run.state.ClosedAt != "" || run.state.Stop == issueworkflow.StopNone {
		return IssueFlowStateResponse{}, wrongStep(run.state.Step, "the request is not stopped; there is nothing to resume")
	}
	target := in.ToStep
	if target == "" {
		target = run.state.BlockedFrom
	}
	if target == "" {
		target = issueworkflow.StartStatus
	}
	switch target {
	case issuestatus.Blocked, issuestatus.Done, issuestatus.Cancelled:
		return IssueFlowStateResponse{}, &flowRejection{Status: 400, Code: "flow_resume_invalid", Message: fmt.Sprintf("a request cannot resume at %q", target)}
	}
	if !flowKnowsStatus(run.def.Graph, target) {
		return IssueFlowStateResponse{}, &flowRejection{Status: 400, Code: "flow_resume_invalid", Message: fmt.Sprintf("the workflow has no step %q", target)}
	}
	rows, err := h.loadFlowTasks(ctx, qtx, run)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	next := run.state
	next.Step = target
	next.Stop = issueworkflow.StopNone
	next.BlockedFrom = ""
	next.EnteredAt = flowNow()
	// A manager who resumes the request has looked at it: a human checkpoint (spec 4.8).
	next.Unreviewed = false
	if in.ResetRework || run.state.Stop == issueworkflow.Stop3 {
		next.Rework = 0
	}
	freshRound := in.ToStep != "" || in.ResetRework || run.state.Stop == issueworkflow.Stop3
	if freshRound {
		highest := -1
		for _, r := range rows {
			if r.state.Round > highest {
				highest = r.state.Round
			}
		}
		next.RoundBase = highest + 1 - next.Rework
		if next.RoundBase < 0 {
			next.RoundBase = 0
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
	if err := moveFlowRequest(ctx, qtx, run, target, &effects); err != nil {
		return IssueFlowStateResponse{}, err
	}
	if !freshRound {
		for _, r := range rows {
			if r.state.Step != target || r.state.Round != run.state.Round() || r.issue.Status != issuestatus.Blocked {
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
			if r.state.Step != target || r.state.Round != run.state.Round() || !cancelledForItsFiles(run, r) {
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
// and cancels every open task. A status the workflow does not know is ignored.
func (h *Handler) followManualMove(ctx context.Context, qtx *db.Queries, run *flowRun, rows []flowTaskRow, e *flowEffects) (bool, FlowAction, error) {
	to := run.issue.Status
	old := run.state
	next := old
	next.EnteredAt = flowNow()
	var action FlowAction
	switch issuestatus.Category(ctx, qtx, run.issue.WorkspaceID, to) {
	case issuestatus.CategoryDone, issuestatus.CategoryClosed:
		next.ClosedAt = next.EnteredAt
		next.Stop = issueworkflow.StopNone
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
		if !flowKnowsStatus(run.def.Graph, to) {
			return false, FlowAction{}, nil
		}
		next.Step = to
		next.Stop = issueworkflow.StopNone
		next.BlockedFrom = ""
		next.Unreviewed = false
		if to == issuestatus.Blocked {
			next.BlockedFrom = old.Step
			next.Stop = issueworkflow.Stop1
		}
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
			next.RoundBase = highest + 1 - next.Rework
			if next.RoundBase < 0 {
				next.RoundBase = 0
			}
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
