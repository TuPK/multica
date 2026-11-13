package handler

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

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// The flow engine (docs/engineering/kitviet-workflow-spec.md, sections 4 and 5).
//
// It lives in package handler because the events it publishes (issue:updated)
// carry a handler.IssueResponse, which the activity log listener reads.
//
// An advance has three phases and keeps no state in memory: it decides inside
// one transaction that holds the request's row lock, opens the missing tasks
// outside it, and publishes after both. Running it again changes nothing, so a
// crash between phases is repaired by the next run, and two runs at once
// converge on one set of tasks (the unique task index and the conditional state
// write).

// flowAdvanceMaxRounds bounds the steps one advance may cross: a step whose
// optional tasks are all off routes straight on.
const flowAdvanceMaxRounds = issueworkflow.MaxFlowSteps + 2

var errFlowNotManaged = &flowRejection{
	Status: 404, Code: "flow_not_managed", Message: "this issue is not run by a workflow flow",
}

func (h *Handler) flowEngineEnabled(ctx context.Context) bool {
	return featureflags.FlowEngineEnabled(ctx, h.FeatureFlags)
}

func flowNow() string { return time.Now().UTC().Format(time.RFC3339) }

// flowRun is a request with the workflow it is pinned to.
type flowRun struct {
	issue db.Issue
	row   db.IssueWorkflow
	def   issueworkflow.Definition
	state issueworkflow.FlowState
}

// loadFlowRun reads a request's flow state and its workflow. errFlowNotManaged
// when the issue has no flow, or the workflow is gone or is no longer a flow.
func (h *Handler) loadFlowRun(ctx context.Context, q *db.Queries, issue db.Issue) (*flowRun, error) {
	state, ok := flowRequestState(issue)
	if !ok {
		return nil, errFlowNotManaged
	}
	id, err := util.ParseUUID(state.WorkflowID)
	if err != nil {
		return nil, errFlowNotManaged
	}
	row, err := q.GetIssueWorkflow(ctx, db.GetIssueWorkflowParams{ID: id, WorkspaceID: issue.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errFlowNotManaged
	}
	if err != nil {
		return nil, err
	}
	def, err := issueworkflow.Decode(row)
	if err != nil || def.Flow == nil {
		return nil, errFlowNotManaged
	}
	return &flowRun{issue: issue, row: row, def: def, state: state}, nil
}

// EnsureFlowStarted puts a request on its squad's flow when it qualifies. It is
// idempotent and cheap for every other issue.
func (h *Handler) EnsureFlowStarted(ctx context.Context, issue db.Issue) (db.Issue, error) {
	if !h.flowEngineEnabled(ctx) {
		return issue, nil
	}
	return service.StartFlowIfEligible(ctx, h.Queries, issue)
}

// flowTaskRow is a task issue with what the engine derives from it.
type flowTaskRow struct {
	issue db.Issue
	state issueworkflow.TaskState
}

// loadFlowTasks reads the tasks of a request. A task counts as closed when its
// issue is done or cancelled and no run of it is still active, so a status
// written before the run ends cannot release the step early.
func (h *Handler) loadFlowTasks(ctx context.Context, q *db.Queries, run *flowRun) ([]flowTaskRow, error) {
	issues, err := q.ListFlowTasks(ctx, db.ListFlowTasksParams{ParentIssueID: run.issue.ID, WorkspaceID: run.issue.WorkspaceID})
	if err != nil {
		return nil, err
	}
	resolver := issuestatus.NewResolver(run.issue.WorkspaceID)
	out := make([]flowTaskRow, 0, len(issues))
	for _, t := range issues {
		ts := issueworkflow.TaskState{
			Step:    t.FlowStep.String,
			Key:     t.FlowTask.String,
			Round:   int(t.FlowRound.Int32),
			Outcome: t.Outcome.String,
		}
		if t.Status == issuestatus.Blocked && ts.Outcome == "" {
			// A system write (a run that failed for good) blocks a task without a verdict: it still
			// reads as a task that cannot go on.
			ts.Outcome = issueworkflow.OutcomeBlocked
		}
		if step, ok := run.def.Step(ts.Step); ok {
			for _, task := range step.Tasks {
				if task.Key == ts.Key {
					ts.Kind = task.Kind
				}
			}
		}
		switch resolver.Category(ctx, q, t.Status) {
		case issuestatus.CategoryDone:
			ts.Closed = true
		case issuestatus.CategoryClosed:
			ts.Closed, ts.Cancelled = true, true
		}
		if ts.Closed && ts.Step == run.state.Step && ts.Round == run.state.Round() {
			active, err := q.ListActiveTasksByIssue(ctx, t.ID)
			if err != nil {
				return nil, err
			}
			if len(active) > 0 {
				ts.Closed = false
			}
		}
		out = append(out, flowTaskRow{issue: t, state: ts})
	}
	return out, nil
}

func taskStates(rows []flowTaskRow) []issueworkflow.TaskState {
	out := make([]issueworkflow.TaskState, len(rows))
	for i, r := range rows {
		out[i] = r.state
	}
	return out
}

// statusMove is a status change the engine wrote, to publish after commit.
type statusMove struct {
	prev  string
	issue db.Issue
}

// flowEffects collects what to publish and start once the transaction commits.
type flowEffects struct {
	moves         []statusMove
	comments      []db.CreateCommentRow
	cancelledRuns []db.AgentTaskQueue
	// cancelTasksOf are task issues cancelled in the transaction; their runs stop after commit.
	cancelTasksOf []pgtype.UUID
	// reopened are task issues set back to todo; each needs a new run.
	reopened []db.Issue
}

// publishFlowEffects sends the events and starts the runs of a committed change.
func (h *Handler) publishFlowEffects(ctx context.Context, request db.Issue, e *flowEffects) {
	ws := uuidToString(request.WorkspaceID)
	prefix := h.getIssuePrefix(ctx, request.WorkspaceID)
	for _, id := range e.cancelTasksOf {
		if err := h.TaskService.CancelTasksForIssue(ctx, id); err != nil {
			slog.Warn("flow: cancel the runs of a cancelled task failed", "issue_id", uuidToString(id), "error", err)
		}
	}
	h.broadcastCancelledWakeups(ctx, request.WorkspaceID, e.cancelledRuns)
	for _, move := range e.moves {
		resp := issueToResponse(move.issue, prefix)
		h.fillStatusCategory(ctx, move.issue.WorkspaceID, &resp)
		h.publish(protocol.EventIssueUpdated, ws, "system", "", map[string]any{
			"issue":              resp,
			"assignee_changed":   false,
			"status_changed":     true,
			"priority_changed":   false,
			"project_changed":    false,
			"prev_status":        move.prev,
			"prev_assignee_type": textToPtr(move.issue.AssigneeType),
			"prev_assignee_id":   uuidToPtr(move.issue.AssigneeID),
			"creator_type":       move.issue.CreatorType,
			"creator_id":         uuidToString(move.issue.CreatorID),
			"source":             "flow",
		})
	}
	for _, created := range e.comments {
		c := created.Comment()
		h.publish(protocol.EventCommentCreated, ws, "system", "", map[string]any{
			"comment": map[string]any{
				"id": uuidToString(c.ID), "issue_id": uuidToString(c.IssueID), "author_type": c.AuthorType,
				"author_id": uuidToString(c.AuthorID), "content": c.Content, "type": c.Type, "revision": c.Revision,
			},
			"issue_title":    request.Title,
			"issue_revision": created.IssueRevision,
		})
	}
	for _, task := range e.reopened {
		if task.AssigneeType.String == "agent" {
			if _, err := h.TaskService.EnqueueTaskForIssue(ctx, task); err != nil {
				slog.Warn("flow: could not start a run for a reopened task", "issue_id", uuidToString(task.ID), "error", err)
			}
		}
	}
	h.publishFlowUpdated(request)
}

func (h *Handler) publishFlowUpdated(request db.Issue) {
	h.publish(protocol.EventIssueFlowUpdated, uuidToString(request.WorkspaceID), "system", "", map[string]any{
		"issue_id": uuidToString(request.ID),
	})
}

// saveFlowState writes the next state under the condition that the request is
// still at the step and rework count the caller decided from. It reports false
// when it is not, and the caller drops its decision.
func (h *Handler) saveFlowState(ctx context.Context, qtx *db.Queries, run *flowRun, next issueworkflow.FlowState) (bool, error) {
	raw, err := json.Marshal(next)
	if err != nil {
		return false, err
	}
	updated, err := qtx.SetIssueFlow(ctx, db.SetIssueFlowParams{
		Flow:           raw,
		ID:             run.issue.ID,
		WorkspaceID:    run.issue.WorkspaceID,
		ExpectedStep:   run.state.Step,
		ExpectedRework: int32(run.state.Rework),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	run.issue = updated
	run.state = next
	return true, nil
}

// moveFlowRequest writes the request's status to the step it entered.
func moveFlowRequest(ctx context.Context, qtx *db.Queries, run *flowRun, to string, e *flowEffects) error {
	if run.issue.Status == to {
		return nil
	}
	prev := run.issue.Status
	moved, err := qtx.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{ID: run.issue.ID, Status: to, WorkspaceID: run.issue.WorkspaceID})
	if err != nil {
		return fmt.Errorf("move the request to %s: %w", to, err)
	}
	run.issue = moved
	e.moves = append(e.moves, statusMove{prev: prev, issue: moved})
	return nil
}

// closeFlowRequestSideEffects is what ending a request does: its wakeups stop.
func closeFlowRequestSideEffects(ctx context.Context, qtx *db.Queries, run *flowRun, e *flowEffects) error {
	stopped, err := service.StopClosedIssueWakeups(ctx, qtx, run.issue)
	if err != nil {
		return err
	}
	e.cancelledRuns = append(e.cancelledRuns, stopped...)
	return nil
}

// cancelFlowTasks cancels the open tasks that match keep (all when nil).
func cancelFlowTasks(ctx context.Context, qtx *db.Queries, rows []flowTaskRow, keep func(flowTaskRow) bool, e *flowEffects) error {
	for _, r := range rows {
		if r.state.Closed || (keep != nil && !keep(r)) {
			continue
		}
		prev := r.issue.Status
		moved, err := qtx.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{ID: r.issue.ID, Status: issuestatus.Cancelled, WorkspaceID: r.issue.WorkspaceID})
		if err != nil {
			return fmt.Errorf("cancel task %s: %w", r.state.Key, err)
		}
		e.moves = append(e.moves, statusMove{prev: prev, issue: moved})
		e.cancelTasksOf = append(e.cancelTasksOf, r.issue.ID)
	}
	return nil
}

// flowRouteNote carries what a stop comment says beyond the tasks.
type flowRouteNote struct {
	// Missing lists what the lead said was missing when it blocked the plan.
	Missing []string
	// Reason is a sentence for the reader, for instance why a task could not open.
	Reason string
}

// routeFlowRequest follows a decision: it writes the next state, moves the
// request, posts the stop comment and closes the request when it ends. It
// returns false, writing nothing, when another writer moved the request first.
func (h *Handler) routeFlowRequest(ctx context.Context, qtx *db.Queries, run *flowRun, d issueworkflow.Decision, rows []flowTaskRow, note flowRouteNote, e *flowEffects) (bool, error) {
	old := run.state
	next := old
	next.Step = d.To
	if d.Rework {
		next.Rework++
	}
	next.Stop = d.Stop
	next.EnteredAt = flowNow()
	next.BlockedFrom = ""
	if d.To == issuestatus.Blocked {
		next.BlockedFrom = old.Step
	}
	closing := false
	switch issuestatus.Category(ctx, qtx, run.issue.WorkspaceID, d.To) {
	case issuestatus.CategoryDone, issuestatus.CategoryClosed:
		closing = true
		next.ClosedAt = next.EnteredAt
	}
	saved, err := h.saveFlowState(ctx, qtx, run, next)
	if err != nil || !saved {
		return false, err
	}
	if err := moveFlowRequest(ctx, qtx, run, d.To, e); err != nil {
		return false, err
	}
	if kind := stopKindOf(d); kind != "" {
		if err := h.postStopComment(ctx, qtx, run, old, kind, rows, note, e); err != nil {
			return false, err
		}
	}
	if closing {
		if err := closeFlowRequestSideEffects(ctx, qtx, run, e); err != nil {
			return false, err
		}
		if err := cancelFlowTasks(ctx, qtx, rows, nil, e); err != nil {
			return false, err
		}
	}
	return true, nil
}

const (
	stopKind1       = "stop1"
	stopKind2       = "stop2"
	stopKind3       = "stop3"
	stopKindStalled = "stalled"
	stopKindHuman   = "human"
)

func stopKindOf(d issueworkflow.Decision) string {
	switch d.Stop {
	case issueworkflow.Stop1:
		return stopKind1
	case issueworkflow.Stop3:
		return stopKind3
	case issueworkflow.StopStalled:
		return stopKindStalled
	}
	if d.HumanStop {
		// Stop 2 is the last human step, in_review. Any other human-only step is a wait for approval.
		if d.To == issuestatus.InReview {
			return stopKind2
		}
		return stopKindHuman
	}
	return ""
}

// defaultStopMessages are used when a flow_config has no text for a stop; the
// catalog flows carry their own.
var defaultStopMessages = map[string]string{
	stopKind1:       "BLOCKED · stop 1\n{requester} {request} stopped at {step}: grounds are missing.\n{missing}\n{tasks}",
	stopKind2:       "IN REVIEW · stop 2\n{requester} {request} is ready for your review.\n{tasks}",
	stopKind3:       "BLOCKED · stop 3\n{requester} {request} used its {rework} rework rounds and was still rejected.\n{tasks}",
	stopKindHuman:   "WAITING · {step}\n{requester} {request} waits for a person at {step}.\n{tasks}",
	stopKindStalled: "BLOCKED · stalled\n{requester} {request} stopped moving at {step}.\n{reason}\n{tasks}",
}

// postStopComment writes the system comment that tells people why a request
// stopped, from the flow's own message for that stop.
func (h *Handler) postStopComment(ctx context.Context, qtx *db.Queries, run *flowRun, old issueworkflow.FlowState, kind string, rows []flowTaskRow, note flowRouteNote, e *flowEffects) error {
	if header := stopCommentHeader(kind); header != "" {
		posted, err := stopCommentByLead(ctx, qtx, run.issue, old, header)
		if err != nil {
			return err
		}
		if posted {
			return nil
		}
	}
	template := ""
	switch kind {
	case stopKind1:
		template = run.def.Flow.Messages.Stop1
	case stopKind2:
		template = run.def.Flow.Messages.Stop2
	case stopKind3:
		template = run.def.Flow.Messages.Stop3
	case stopKindStalled:
		template = run.def.Flow.Messages.Stalled
	case stopKindHuman:
		template = run.def.Flow.Messages.Human
	}
	if strings.TrimSpace(template) == "" {
		template = defaultStopMessages[kind]
	}
	prefix := h.getIssuePrefix(ctx, run.issue.WorkspaceID)
	resolver := issuestatus.NewResolver(run.issue.WorkspaceID)
	if kind == stopKind1 && len(note.Missing) == 0 {
		note = h.blockedTaskNote(ctx, qtx, prefix, old, rows, note)
	}
	missing := ""
	if len(note.Missing) > 0 {
		missing = "- " + strings.Join(note.Missing, "\n- ")
	}
	text := issueworkflow.RenderMessage(template, map[string]string{
		"requester": h.requesterMention(ctx, run.issue),
		"request":   prefix + "-" + fmt.Sprint(run.issue.Number),
		"step":      resolver.Name(ctx, qtx, old.Step),
		"rework":    fmt.Sprint(old.Rework),
		"tasks":     taskLines(prefix, old, rows),
		"missing":   missing,
		"reason":    note.Reason,
	})
	title, body := issueworkflow.SplitMessage(text)
	content := title
	if body != "" {
		content += "\n\n" + body
	}
	created, err := qtx.CreateComment(ctx, db.CreateCommentParams{
		ID: dbid.NewV7(), IssueID: run.issue.ID, WorkspaceID: run.issue.WorkspaceID,
		AuthorType: "system", AuthorID: pgtype.UUID{Valid: true},
		Content: content, Type: "system",
	})
	if err != nil {
		return fmt.Errorf("post the stop comment: %w", err)
	}
	e.comments = append(e.comments, created)
	return nil
}

// stopCommentHeader is the first line of the stop comment for a stop kind, as the
// lead writes it. Stops without one (stalled, human) are always posted by the server.
func stopCommentHeader(kind string) string {
	switch kind {
	case stopKind1:
		return "BLOCKED · stop 1"
	case stopKind2:
		return "IN REVIEW · stop 2"
	case stopKind3:
		return "BLOCKED · stop 3"
	}
	return ""
}

// stopCommentByLead reports whether an agent already wrote the stop comment for the
// stop the request is at now. The lead may write it before it reports the stop, so the
// server must not post a second copy. Only comments made after the request entered the
// step it stopped from count: a comment from an earlier stop is not this stop's comment.
func stopCommentByLead(ctx context.Context, qtx *db.Queries, request db.Issue, old issueworkflow.FlowState, header string) (bool, error) {
	entered, err := time.Parse(time.RFC3339, old.EnteredAt)
	if err != nil {
		return false, nil
	}
	comments, err := qtx.ListCommentsSinceForIssue(ctx, db.ListCommentsSinceForIssueParams{
		IssueID: request.ID, WorkspaceID: request.WorkspaceID,
		CreatedAt: pgtype.Timestamptz{Time: entered, Valid: true}, Limit: 500,
	})
	if err != nil {
		return false, fmt.Errorf("list comments for the stop: %w", err)
	}
	for _, c := range comments {
		if c.AuthorType == "agent" && !c.DeletedAt.Valid && strings.HasPrefix(strings.TrimSpace(c.Content), header) {
			return true, nil
		}
	}
	return false, nil
}

// blockedTaskNote fills a stop 1 that came from tasks which reported they cannot go on. The lead's
// plan carries its own missing list; a task has only its comment, so the first line of the newest
// comment its agent wrote (a gate's verdict line, a collector's missing item) says what is missing.
func (h *Handler) blockedTaskNote(ctx context.Context, qtx *db.Queries, prefix string, state issueworkflow.FlowState, rows []flowTaskRow, note flowRouteNote) flowRouteNote {
	var keys []string
	for _, r := range rows {
		if r.state.Step != state.Step || r.state.Round != state.Round() || r.state.Outcome != issueworkflow.OutcomeBlocked {
			continue
		}
		keys = append(keys, r.state.Key)
		content, err := qtx.LastAgentCommentOnIssue(ctx, r.issue.ID)
		if err != nil {
			continue
		}
		line, _, _ := strings.Cut(strings.TrimSpace(content), "\n")
		if runes := []rune(line); len(runes) > 300 {
			line = string(runes[:300]) + "…"
		}
		if line != "" {
			note.Missing = append(note.Missing, fmt.Sprintf("%s-%d · %s: %s", prefix, r.issue.Number, r.state.Key, line))
		}
	}
	if note.Reason == "" && len(keys) > 0 {
		note.Reason = fmt.Sprintf("The task %s reported it cannot go on; its comment has the details.", strings.Join(keys, ", "))
	}
	return note
}

// requesterMention mentions the member who made the request, or is empty when
// an agent did.
func (h *Handler) requesterMention(ctx context.Context, request db.Issue) string {
	if request.CreatorType != "member" {
		return ""
	}
	user, err := h.Queries.GetUser(ctx, request.CreatorID)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("[@%s](mention://member/%s)", user.Name, uuidToString(request.CreatorID))
}

// taskLines lists the tasks of the step the request stopped at, in the round it
// stopped in, with the verdict each reported.
func taskLines(prefix string, state issueworkflow.FlowState, rows []flowTaskRow) string {
	var lines []string
	for _, r := range rows {
		if r.state.Step != state.Step || r.state.Round != state.Round() {
			continue
		}
		outcome := r.issue.Outcome.String
		if outcome == "" {
			outcome = "-"
		}
		lines = append(lines, fmt.Sprintf("- %s-%d · %s · %s · %s", prefix, r.issue.Number, r.state.Key, r.issue.Status, outcome))
	}
	return strings.Join(lines, "\n")
}

// advanceOutcome is what one pass of Advance did.
type advanceOutcome struct {
	action FlowAction
	// progressed: the request or its tasks changed, so another pass may have more to do.
	progressed bool
}

// AdvanceFlow runs the engine on a request until it has nothing more to do: it
// opens the tasks of the step, waits for them, and follows the step's
// transitions once they are all closed. It is idempotent.
func (h *Handler) AdvanceFlow(ctx context.Context, request db.Issue) (FlowAdvanceResponse, error) {
	var actions []FlowAction
	for i := 0; i < flowAdvanceMaxRounds; i++ {
		out, err := h.advanceOnce(ctx, request)
		if err != nil {
			return FlowAdvanceResponse{}, err
		}
		if out.action.Kind != "noop" {
			actions = append(actions, out.action)
		}
		if !out.progressed {
			break
		}
	}
	return h.flowAdvanceResponse(ctx, request, actions)
}

// flowAdvanceResponse builds the answer of an advance from the stored state.
func (h *Handler) flowAdvanceResponse(ctx context.Context, request db.Issue, actions []FlowAction) (FlowAdvanceResponse, error) {
	fresh, err := h.Queries.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{ID: request.ID, WorkspaceID: request.WorkspaceID})
	if err != nil {
		return FlowAdvanceResponse{}, err
	}
	state, err := h.flowStateResponse(ctx, fresh, issueworkflow.RoleRequest)
	if err != nil {
		return FlowAdvanceResponse{}, err
	}
	if len(actions) == 0 {
		actions = []FlowAction{{Kind: "noop", Step: state.Step, Tasks: []string{}}}
	}
	pending := []FlowPending{}
	run, err := h.loadFlowRun(ctx, h.Queries, fresh)
	if err == nil {
		rows, err := h.loadFlowTasks(ctx, h.Queries, run)
		if err != nil {
			return FlowAdvanceResponse{}, err
		}
		for _, r := range rows {
			if r.state.Step == run.state.Step && r.state.Round == run.state.Round() && !r.state.Closed {
				pending = append(pending, FlowPending{Task: r.state.Key, IssueID: uuidToString(r.issue.ID), Status: r.issue.Status})
			}
		}
	}
	return FlowAdvanceResponse{State: state, Actions: actions, Pending: pending}, nil
}

// advanceOnce is one pass: decide under the row lock, then open missing tasks.
func (h *Handler) advanceOnce(ctx context.Context, request db.Issue) (advanceOutcome, error) {
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return advanceOutcome{}, fmt.Errorf("begin flow advance: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	locked, err := qtx.LockIssueForFlow(ctx, db.LockIssueForFlowParams{ID: request.ID, WorkspaceID: request.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return advanceOutcome{}, errFlowNotManaged
	}
	if err != nil {
		return advanceOutcome{}, err
	}
	run, err := h.loadFlowRun(ctx, qtx, locked)
	if err != nil {
		return advanceOutcome{}, err
	}
	noop := advanceOutcome{action: FlowAction{Kind: "noop", Step: run.state.Step, Tasks: []string{}}}
	if run.state.ClosedAt != "" || run.state.Stop != issueworkflow.StopNone {
		return noop, nil
	}
	rows, err := h.loadFlowTasks(ctx, qtx, run)
	if err != nil {
		return advanceOutcome{}, err
	}
	var effects flowEffects
	if run.issue.Status != run.state.Step {
		followed, action, err := h.followManualMove(ctx, qtx, run, rows, &effects)
		if err != nil {
			return advanceOutcome{}, err
		}
		if followed {
			if err := tx.Commit(ctx); err != nil {
				return advanceOutcome{}, fmt.Errorf("commit flow advance: %w", err)
			}
			h.publishFlowEffects(ctx, run.issue, &effects)
			return advanceOutcome{action: action, progressed: action.Kind != "stopped"}, nil
		}
	}
	decision := issueworkflow.Decide(run.def, run.state, taskStates(rows))
	switch decision.Kind {
	case issueworkflow.DecisionRoute:
		routed, err := h.routeFlowRequest(ctx, qtx, run, decision, rows, flowRouteNote{}, &effects)
		if err != nil {
			return advanceOutcome{}, err
		}
		if !routed {
			// Another writer moved the request first; this decision is stale.
			return noop, nil
		}
		if err := tx.Commit(ctx); err != nil {
			return advanceOutcome{}, fmt.Errorf("commit flow advance: %w", err)
		}
		h.publishFlowEffects(ctx, run.issue, &effects)
		kind := "routed"
		if decision.Stop != issueworkflow.StopNone {
			kind = "stopped"
		}
		return advanceOutcome{
			action:     FlowAction{Kind: kind, Step: decision.To, Tasks: []string{}},
			progressed: decision.Stop == issueworkflow.StopNone,
		}, nil
	case issueworkflow.DecisionOpen:
		if err := tx.Commit(ctx); err != nil {
			return advanceOutcome{}, fmt.Errorf("commit flow advance: %w", err)
		}
		return h.openFlowTasks(ctx, run, rows, decision.Missing)
	}
	return noop, nil
}

// openFlowTasks creates the missing tasks of the request's current step. The
// agents are checked first: a task whose agent cannot take work stops the
// request instead of waiting forever.
func (h *Handler) openFlowTasks(ctx context.Context, run *flowRun, rows []flowTaskRow, missing []issueworkflow.Task) (advanceOutcome, error) {
	step := run.state.Step
	for _, task := range missing {
		agentID, err := util.ParseUUID(task.Handler.ID)
		agent, lookupErr := h.Queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: agentID, WorkspaceID: run.issue.WorkspaceID})
		if err != nil || lookupErr != nil || agent.ArchivedAt.Valid {
			return h.stopForUnavailableAgent(ctx, run, task)
		}
	}
	// A gate is never given to an agent that did work in the same round.
	workers := map[pgtype.UUID]string{}
	for _, r := range rows {
		if r.state.Round == run.state.Round() && r.state.Kind == issueworkflow.KindWork && r.issue.AssigneeID.Valid {
			workers[r.issue.AssigneeID] = r.state.Key
		}
	}
	for _, task := range missing {
		agentID, _ := util.ParseUUID(task.Handler.ID)
		if worker, same := workers[agentID]; same && task.Kind == issueworkflow.KindGate {
			return advanceOutcome{}, &flowRejection{
				Status: 422, Code: "flow_gate_same_agent",
				Message: fmt.Sprintf("the gate %q cannot go to the agent that did the work %q in the same round", task.Key, worker),
				Fields:  map[string]any{"task": task.Key},
			}
		}
	}
	prefix := h.getIssuePrefix(ctx, run.issue.WorkspaceID)
	resolver := issuestatus.NewResolver(run.issue.WorkspaceID)
	stage := int32(1)
	for i, s := range run.def.Steps {
		if s.StatusKey == step {
			stage = int32(i + 1)
		}
	}
	requestRef := prefix + "-" + fmt.Sprint(run.issue.Number)
	var opened []string
	for _, task := range missing {
		agentID, _ := util.ParseUUID(task.Handler.ID)
		title := fmt.Sprintf("%s · %s · v%d", resolver.Name(ctx, h.Queries, step), task.Title, run.state.Round()+1)
		_, err := h.IssueService.Create(ctx, service.IssueCreateParams{
			WorkspaceID:    run.issue.WorkspaceID,
			Title:          title,
			Description:    pgtype.Text{String: h.flowTaskDescription(run, rows, task, requestRef, prefix), Valid: true},
			Status:         issuestatus.Todo,
			Priority:       run.issue.Priority,
			AssigneeType:   pgtype.Text{String: "agent", Valid: true},
			AssigneeID:     agentID,
			CreatorType:    run.issue.CreatorType,
			CreatorID:      run.issue.CreatorID,
			ParentIssueID:  run.issue.ID,
			ProjectID:      run.issue.ProjectID,
			Stage:          pgtype.Int4{Int32: stage, Valid: true},
			AllowDuplicate: true,
			FlowStep:       pgtype.Text{String: step, Valid: true},
			FlowTask:       pgtype.Text{String: task.Key, Valid: true},
			FlowRound:      pgtype.Int4{Int32: int32(run.state.Round()), Valid: true},
		}, service.IssueCreateOpts{
			BroadcastPayload: func(issue db.Issue, atts []db.Attachment, labels []db.IssueLabel) map[string]any {
				resp := issueToResponse(issue, prefix)
				h.fillStatusCategory(ctx, issue.WorkspaceID, &resp)
				return map[string]any{"issue": resp}
			},
		})
		if err != nil {
			if isFlowTaskOnce(err) {
				continue
			}
			return advanceOutcome{}, fmt.Errorf("open task %s: %w", task.Key, err)
		}
		opened = append(opened, task.Key)
	}
	if len(opened) == 0 {
		return advanceOutcome{action: FlowAction{Kind: "noop", Step: step, Tasks: []string{}}}, nil
	}
	h.publishFlowUpdated(run.issue)
	return advanceOutcome{action: FlowAction{Kind: "opened_tasks", Step: step, Tasks: opened}, progressed: true}, nil
}

// isFlowTaskOnce reports whether err is the unique index of the task identity:
// another run opened this task first.
func isFlowTaskOnce(err error) bool {
	return isUniqueViolation(err) && strings.Contains(err.Error(), "idx_issue_flow_task_once")
}

// flowTaskDescription renders the description of a task issue: the task's own
// instructions, the files of the step before, and on a rework round the gates
// that rejected the last one.
func (h *Handler) flowTaskDescription(run *flowRun, rows []flowTaskRow, task issueworkflow.Task, requestRef, prefix string) string {
	inputs, rejected := flowTaskLinks(run.def, rows, prefix, run.state.Step, run.state.Round())
	return issueworkflow.TaskDescription(issueworkflow.TaskDescriptionInput{
		Task: task, RequestIdentifier: requestRef, Round: run.state.Round(), Inputs: inputs, Rejected: rejected,
	})
}

// flowTaskLinks names what a task at a step and round reads: the tasks of the
// nearest earlier step that ran in the round (its inputs), and on a rework round
// the gates of the round before that did not approve.
func flowTaskLinks(def issueworkflow.Definition, rows []flowTaskRow, prefix, step string, round int) (inputs, rejected []issueworkflow.TaskInput) {
	ref := func(r flowTaskRow) issueworkflow.TaskInput {
		return issueworkflow.TaskInput{Identifier: fmt.Sprintf("%s-%d", prefix, r.issue.Number), Key: r.state.Key}
	}
	index := -1
	for i, s := range def.Steps {
		if s.StatusKey == step {
			index = i
		}
	}
	for i := index - 1; i >= 0 && len(inputs) == 0; i-- {
		for _, r := range rows {
			if r.state.Step == def.Steps[i].StatusKey && r.state.Round == round {
				inputs = append(inputs, ref(r))
			}
		}
	}
	if round > 0 {
		for _, r := range rows {
			if r.state.Round == round-1 && r.state.Kind == issueworkflow.KindGate && (r.state.Cancelled || r.state.Outcome != issueworkflow.OutcomeApproved) {
				rejected = append(rejected, ref(r))
			}
		}
	}
	return inputs, rejected
}

// stopForUnavailableAgent blocks a request whose next task cannot be given to
// its agent (archived or gone). The advance reports it instead of failing.
func (h *Handler) stopForUnavailableAgent(ctx context.Context, current *flowRun, task issueworkflow.Task) (advanceOutcome, error) {
	out, err := h.stallFlow(ctx, current, fmt.Sprintf("The agent for the task %q (%s) is archived or no longer in this workspace, so the task cannot open.", task.Key, task.Title), "")
	if err != nil {
		return advanceOutcome{}, err
	}
	out.action.Tasks = []string{task.Key}
	out.action.Code = "flow_agent_unavailable"
	out.action.AgentID = task.Handler.ID
	return out, nil
}

// stallFlow stops a request that cannot move on without a person: it goes to blocked with the stalled
// stop and a comment that gives the reason. blockTask, when set, is the key of an open task of the
// current step that is set to blocked first, so the stop shows which task was stuck. It does
// nothing when the request moved or stopped meanwhile.
func (h *Handler) stallFlow(ctx context.Context, current *flowRun, reason, blockTask string) (advanceOutcome, error) {
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return advanceOutcome{}, err
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	locked, err := qtx.LockIssueForFlow(ctx, db.LockIssueForFlowParams{ID: current.issue.ID, WorkspaceID: current.issue.WorkspaceID})
	if err != nil {
		return advanceOutcome{}, err
	}
	run, err := h.loadFlowRun(ctx, qtx, locked)
	if err != nil {
		return advanceOutcome{}, err
	}
	if run.state.Step != current.state.Step || run.state.Stop != issueworkflow.StopNone || run.state.ClosedAt != "" {
		return advanceOutcome{action: FlowAction{Kind: "noop", Step: run.state.Step, Tasks: []string{}}}, nil
	}
	rows, err := h.loadFlowTasks(ctx, qtx, run)
	if err != nil {
		return advanceOutcome{}, err
	}
	var effects flowEffects
	if blockTask != "" {
		for i, r := range rows {
			if r.state.Key != blockTask || r.state.Step != run.state.Step || r.state.Round != run.state.Round() || r.state.Closed {
				continue
			}
			prev := r.issue.Status
			blocked, err := qtx.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{ID: r.issue.ID, Status: issuestatus.Blocked, WorkspaceID: r.issue.WorkspaceID})
			if err != nil {
				return advanceOutcome{}, fmt.Errorf("block task %s: %w", r.state.Key, err)
			}
			if err := qtx.SetIssueOutcome(ctx, db.SetIssueOutcomeParams{ID: r.issue.ID, WorkspaceID: r.issue.WorkspaceID, Outcome: pgtype.Text{String: issueworkflow.OutcomeBlocked, Valid: true}}); err != nil {
				return advanceOutcome{}, err
			}
			blocked.Outcome = pgtype.Text{String: issueworkflow.OutcomeBlocked, Valid: true}
			rows[i].issue = blocked
			effects.moves = append(effects.moves, statusMove{prev: prev, issue: blocked})
		}
	}
	routed, err := h.routeFlowRequest(ctx, qtx, run, issueworkflow.Decision{
		Kind: issueworkflow.DecisionRoute, To: issuestatus.Blocked, Stop: issueworkflow.StopStalled,
	}, rows, flowRouteNote{Reason: reason}, &effects)
	if err != nil {
		return advanceOutcome{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return advanceOutcome{}, err
	}
	if !routed {
		return advanceOutcome{action: FlowAction{Kind: "noop", Step: run.state.Step, Tasks: []string{}}}, nil
	}
	h.publishFlowEffects(ctx, run.issue, &effects)
	return advanceOutcome{action: FlowAction{Kind: "stopped", Step: issuestatus.Blocked, Tasks: []string{}}}, nil
}
