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
	state issueworkflow.State
}

// compat reports whether the request runs a workflow of schema 1 through its lifted
// graph: its stored state keeps the schema 1 form and its texts the schema 1 templates.
func (r *flowRun) compat() bool { return r.def.Graph.Compat == issueworkflow.CompatV1 }

// round is the flow_round of the tasks of the current entry into the step.
func (r *flowRun) round() int { return issueworkflow.EntryRound(r.def.Graph, r.state) }

// stepStatus is the board status the request has while it is at its step.
func (r *flowRun) stepStatus() string {
	if step, ok := r.def.Graph.Step(r.state.Step); ok {
		return step.Status.Key
	}
	return r.state.Step
}

// loadFlowRun reads a request's flow state and its workflow. errFlowNotManaged
// when the issue has no flow, or the workflow is gone or is no longer a flow.
func (h *Handler) loadFlowRun(ctx context.Context, q *db.Queries, issue db.Issue) (*flowRun, error) {
	state, ok := readFlowState(issue)
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
	if err != nil || !def.HasFlow() {
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

// ensureFlowStartedOn is EnsureFlowStarted for a request whose creator chose the
// workflow it runs on. A nil choice means the squad's active workflow.
func (h *Handler) ensureFlowStartedOn(ctx context.Context, issue db.Issue, chosen *db.IssueWorkflow, trial bool) (db.Issue, error) {
	if !h.flowEngineEnabled(ctx) {
		return issue, nil
	}
	return service.StartFlowIfEligibleOn(ctx, h.Queries, issue, chosen, trial)
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
		if task, ok := flowGraphTask(run.def.Graph, ts.Step, ts.Key); ok {
			ts.Kind = task.Kind
		}
		switch resolver.Category(ctx, q, t.Status) {
		case issuestatus.CategoryDone:
			ts.Closed = true
		case issuestatus.CategoryClosed:
			ts.Closed, ts.Cancelled = true, true
		}
		if ts.Closed && ts.Step == run.state.Step && ts.Round == run.round() {
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
// still at the step and seq the caller decided from; the stored seq is one more.
// It reports false when the state moved, and the caller drops its decision.
func (h *Handler) saveFlowState(ctx context.Context, qtx *db.Queries, run *flowRun, next issueworkflow.State) (bool, error) {
	next.Seq = run.state.Seq + 1
	raw, err := encodeFlowState(run.def.Graph, next)
	if err != nil {
		return false, err
	}
	updated, err := qtx.SetIssueFlow(ctx, db.SetIssueFlowParams{
		Flow:         raw,
		ID:           run.issue.ID,
		WorkspaceID:  run.issue.WorkspaceID,
		ExpectedStep: run.state.Step,
		ExpectedSeq:  int32(run.state.Seq),
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

// routeFlowRequest follows a decision: it writes the next state with the step
// left added to the trail, logs the transition, moves the request, posts the
// stop comment and closes the request when it ends. by is the user who chose the
// exit of a manual step, for the checkpoint. It returns false, writing nothing,
// when another writer moved the request first.
func (h *Handler) routeFlowRequest(ctx context.Context, qtx *db.Queries, run *flowRun, d issueworkflow.GraphDecision, rows []flowTaskRow, note flowRouteNote, e *flowEffects, by string) (bool, error) {
	old := run.state
	oldRound := issueworkflow.EntryRound(run.def.Graph, old)
	now := flowNow()
	next := issueworkflow.ApplyDecision(run.def.Graph, old, d, issueworkflow.ApplyMeta{Now: now, By: by})
	applied := next
	to := d.Status
	if d.StopCode != "" && run.compat() {
		// A schema 1 request parks on the blocked step.
		next.Step = to
	}
	next.EnteredAt = now
	if d.StopCode == "" {
		next.BlockedFrom = ""
	}
	_, terminal := run.def.Graph.Terminal(d.To)
	closing := terminal
	switch issuestatus.Category(ctx, qtx, run.issue.WorkspaceID, to) {
	case issuestatus.CategoryDone, issuestatus.CategoryClosed:
		closing = true
		next.ClosedAt = now
	}
	saved, err := h.saveFlowState(ctx, qtx, run, next)
	if err != nil || !saved {
		return false, err
	}
	if err := logFlowTransition(ctx, qtx, run, old, applied, d); err != nil {
		return false, err
	}
	if err := moveFlowRequest(ctx, qtx, run, to, e); err != nil {
		return false, err
	}
	if kind := stopKindOf(run.def.Graph, d); kind != "" {
		if err := h.postStopComment(ctx, qtx, run, old, oldRound, d, kind, rows, note, e); err != nil {
			return false, err
		}
	}
	if !run.compat() {
		if err := supersedeFlowTasks(ctx, qtx, rows, d.Cancel, e); err != nil {
			return false, err
		}
	}
	if closing {
		if err := closeFlowRequestSideEffects(ctx, qtx, run, e); err != nil {
			return false, err
		}
		if run.compat() {
			if err := cancelFlowTasks(ctx, qtx, rows, nil, e); err != nil {
				return false, err
			}
		}
	}
	return true, nil
}

// supersedeFlowTasks cancels the open task rows a decision names ("step/key") and
// marks them superseded: only the engine writes that outcome (spec 2.5, E23). A
// join ignores such a row.
func supersedeFlowTasks(ctx context.Context, qtx *db.Queries, rows []flowTaskRow, refs []string, e *flowEffects) error {
	for _, ref := range refs {
		for i, r := range rows {
			if r.state.Closed || issueworkflow.RowRef(r.state.Step, r.state.Key) != ref {
				continue
			}
			prev := r.issue.Status
			moved, err := qtx.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{ID: r.issue.ID, Status: issuestatus.Cancelled, WorkspaceID: r.issue.WorkspaceID})
			if err != nil {
				return fmt.Errorf("cancel task %s: %w", r.state.Key, err)
			}
			if err := qtx.SetIssueOutcome(ctx, db.SetIssueOutcomeParams{
				ID: r.issue.ID, WorkspaceID: r.issue.WorkspaceID,
				Outcome: pgtype.Text{String: issueworkflow.OutcomeSuperseded, Valid: true},
			}); err != nil {
				return err
			}
			moved.Outcome = pgtype.Text{String: issueworkflow.OutcomeSuperseded, Valid: true}
			rows[i].issue = moved
			rows[i].state.Closed, rows[i].state.Cancelled = true, true
			e.moves = append(e.moves, statusMove{prev: prev, issue: moved})
			e.cancelTasksOf = append(e.cancelTasksOf, r.issue.ID)
		}
	}
	return nil
}

// logFlowTransition records why a request left a step: the entry of the trail
// (written with the state) and the activity row that carries the facts the edges
// read (spec 4.13). It runs in the transaction of the state write.
func logFlowTransition(ctx context.Context, qtx *db.Queries, run *flowRun, old issueworkflow.State, applied issueworkflow.State, d issueworkflow.GraphDecision) error {
	details := map[string]any{
		"from":  old.Step,
		"edge":  d.TransitionID,
		"loops": applied.Loops,
		"facts": d.Facts,
	}
	details["activation"] = 0
	if n := len(applied.Trail); n > 0 {
		details["activation"] = applied.Trail[n-1].Activation
	}
	if d.StopCode != "" {
		details["stop"] = d.StopCode
		if d.Reason != "" {
			details["reason"] = d.Reason
		}
	} else {
		details["to"] = d.To
	}
	if d.JoinResult != "" {
		details["join"] = d.JoinResult
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = qtx.CreateActivity(ctx, db.CreateActivityParams{
		ID: dbid.NewV7(), WorkspaceID: run.issue.WorkspaceID, IssueID: run.issue.ID,
		ActorType: pgtype.Text{String: "system", Valid: true}, Action: "flow_transition", Details: raw,
	})
	return err
}

const (
	stopKind1       = "stop1"
	stopKind2       = "stop2"
	stopKind3       = "stop3"
	stopKindStalled = "stalled"
	stopKindHuman   = "human"
)

// stopKindOf is the message a decision posts: the one its stop code declares, or
// the notice of the step the request enters.
func stopKindOf(g issueworkflow.Graph, d issueworkflow.GraphDecision) string {
	switch {
	case d.StopCode == issueworkflow.DocStopStalled:
		return stopKindStalled
	case d.StopCode != "":
		if stop, ok := g.Stop(d.StopCode); ok {
			return stop.Message
		}
	}
	return d.Notice
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

// stallReasonText says in words why the engine stalled a request.
var stallReasonText = map[string]string{
	issueworkflow.ReasonStepMissing:              "The step the request is at is no longer in the workflow.",
	issueworkflow.ReasonNoEdge:                   "No way out of the step fits the state of the request.",
	issueworkflow.ReasonConditionInvalid:         "A condition of the step cannot be evaluated.",
	issueworkflow.ReasonInstancesTooFew:          "The list the step fans out over has fewer items than the step needs.",
	issueworkflow.ReasonFlowCapReached:           "The request reached the limit of steps or task issues of the workflow.",
	issueworkflow.ReasonReleaseWithoutCheckpoint: "Work of an agent has not been seen by a person, so the request cannot leave the workspace.",
	issueworkflow.ReasonInputMissing:             "A task needs the output of a step the request has not run.",
	issueworkflow.ReasonLeadVerdictInvalid:       "The lead reported a verdict the step does not have.",
	issueworkflow.ReasonStopUnknown:              "A way out of the step names a stop the workflow does not declare.",
	issueworkflow.ReasonPoolTooSmall:             "No one in the pool of the role can take the task.",
}

// postStopComment writes the system comment that tells people why a request
// stopped, or entered a step with a notice, from the flow's own message.
func (h *Handler) postStopComment(ctx context.Context, qtx *db.Queries, run *flowRun, old issueworkflow.State, oldRound int, d issueworkflow.GraphDecision, kind string, rows []flowTaskRow, note flowRouteNote, e *flowEffects) error {
	if header := stopCommentHeader(kind); header != "" && run.compat() {
		posted, err := stopCommentByLead(ctx, qtx, run.issue, old, header)
		if err != nil {
			return err
		}
		if posted {
			return nil
		}
	}
	template := run.def.Graph.Doc.Messages[kind]
	if strings.TrimSpace(template) == "" {
		template = defaultStopMessages[kind]
	}
	if strings.TrimSpace(template) == "" {
		return nil
	}
	prefix := h.getIssuePrefix(ctx, run.issue.WorkspaceID)
	resolver := issuestatus.NewResolver(run.issue.WorkspaceID)
	if kind == stopKind1 && len(note.Missing) == 0 {
		note = h.blockedTaskNote(ctx, qtx, prefix, old.Step, oldRound, rows, note)
	}
	if note.Reason == "" && !run.compat() {
		note.Reason = stallReasonText[d.Reason]
	}
	missing := ""
	if len(note.Missing) > 0 {
		missing = "- " + strings.Join(note.Missing, "\n- ")
	}
	text := issueworkflow.RenderMessage(template, map[string]string{
		"requester": h.requesterMention(ctx, run.issue),
		"request":   prefix + "-" + fmt.Sprint(run.issue.Number),
		"step":      resolver.Name(ctx, qtx, h.flowStepStatusKey(run, old.Step)),
		"rework":    fmt.Sprint(old.Loops[issueworkflow.LoopRework]),
		"tasks":     taskLines(prefix, old.Step, oldRound, rows),
		"missing":   missing,
		"reason":    note.Reason,
		"loop":      d.Loop,
		"stop":      d.StopCode,
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

// flowStepStatusKey is the board status of a step, which names it for people.
func (h *Handler) flowStepStatusKey(run *flowRun, step string) string {
	if s, ok := run.def.Graph.Step(step); ok && s.Status.Key != "" {
		return s.Status.Key
	}
	return step
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
func stopCommentByLead(ctx context.Context, qtx *db.Queries, request db.Issue, old issueworkflow.State, header string) (bool, error) {
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
func (h *Handler) blockedTaskNote(ctx context.Context, qtx *db.Queries, prefix string, step string, round int, rows []flowTaskRow, note flowRouteNote) flowRouteNote {
	var keys []string
	for _, r := range rows {
		if r.state.Step != step || r.state.Round != round || r.state.Outcome != issueworkflow.OutcomeBlocked {
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
func taskLines(prefix string, step string, round int, rows []flowTaskRow) string {
	var lines []string
	for _, r := range rows {
		if r.state.Step != step || r.state.Round != round {
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
			if r.state.Step == run.state.Step && r.state.Round == run.round() && !r.state.Closed {
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
	if run.state.ClosedAt != "" || run.state.Stop != nil {
		return noop, nil
	}
	rows, err := h.loadFlowTasks(ctx, qtx, run)
	if err != nil {
		return advanceOutcome{}, err
	}
	var effects flowEffects
	if run.issue.Status != run.stepStatus() {
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
	if reopened, err := h.reopenCancelledWork(ctx, qtx, run, rows, &effects); err != nil {
		return advanceOutcome{}, err
	} else if len(reopened) > 0 {
		if err := tx.Commit(ctx); err != nil {
			return advanceOutcome{}, fmt.Errorf("commit flow advance: %w", err)
		}
		h.publishFlowEffects(ctx, run.issue, &effects)
		return advanceOutcome{action: FlowAction{Kind: "reopened_tasks", Step: run.state.Step, Tasks: reopened}}, nil
	}
	decision := decideFlow(run, taskStates(rows))
	switch decision.Kind {
	case issueworkflow.DecisionRoute:
		routed, err := h.routeFlowRequest(ctx, qtx, run, decision, rows, flowRouteNote{}, &effects, "")
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
		if decision.StopCode != "" {
			kind = "stopped"
		}
		return advanceOutcome{
			action:     FlowAction{Kind: kind, Step: flowTarget(decision), Tasks: []string{}},
			progressed: decision.StopCode == "",
		}, nil
	case issueworkflow.DecisionOpen:
		// The request is unreviewed from the moment a task of an agent produces work, until
		// the next human checkpoint (spec 4.8).
		if opened := issueworkflow.ApplyOpened(run.def.Graph, run.state, decision.Open); opened.Unreviewed && !run.state.Unreviewed {
			next := run.state
			next.Unreviewed = true
			saved, err := h.saveFlowState(ctx, qtx, run, next)
			if err != nil {
				return advanceOutcome{}, err
			}
			if !saved {
				return noop, nil
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return advanceOutcome{}, fmt.Errorf("commit flow advance: %w", err)
		}
		return h.openFlowTasks(ctx, run, rows, decision.Open)
	}
	return noop, nil
}

// flowDecideObserver sees every decision the engine makes from the stored state
// and the task rows. It is nil in production; a test sets it to compare the
// decision with the frozen schema 1 oracle on the real states the database
// produces (spec 9.3 item 7). The engine has no second path: the observer only
// reads.
var flowDecideObserver func(def issueworkflow.Definition, state issueworkflow.State, tasks []issueworkflow.TaskState, d issueworkflow.GraphDecision)

// flowRequestFacts are the columns of the request that conditions can read.
func flowRequestFacts(issue db.Issue) issueworkflow.RequestFacts {
	facts := issueworkflow.RequestFacts{Priority: issue.Priority, CreatorType: issue.CreatorType}
	if issue.ProjectID.Valid {
		facts.ProjectID = uuidToString(issue.ProjectID)
	}
	return facts
}

// decideFlow decides what to do next for the request by the graph of its
// workflow.
func decideFlow(run *flowRun, tasks []issueworkflow.TaskState) issueworkflow.GraphDecision {
	g := run.def.Graph
	st := run.state
	d := issueworkflow.DecideGraph(g, st, tasks, issueworkflow.BuildGraphFacts(g, st, tasks, flowRequestFacts(run.issue)))
	if flowDecideObserver != nil {
		flowDecideObserver(run.def, run.state, tasks, d)
	}
	return d
}

// flowTarget is the board status a route decision moves the request to: the
// step or terminal it enters, or the status of the stop it parks in.
func flowTarget(d issueworkflow.GraphDecision) string {
	if d.StopCode != "" {
		return d.Status
	}
	return d.To
}

// cancelledForItsFiles reports whether a task row is work that a person cancelled
// although its definition makes a file part of the work. A cancelled gate is a
// rejection and other work needs nothing, so neither is reopened. An outcome on a
// cancelled task means a person let the step go on without it (skip_cancelled).
func cancelledForItsFiles(run *flowRun, r flowTaskRow) bool {
	if !r.state.Closed || !r.state.Cancelled || r.state.Kind != issueworkflow.KindWork || r.issue.Outcome.Valid {
		return false
	}
	task, ok := flowGraphTask(run.def.Graph, r.state.Step, r.state.Key)
	return ok && task.NeedsFiles()
}

// reopenCancelledWork sets back to todo the work tasks of the current step and
// round that a person cancelled while the step still requires their file (E7), so
// the step cannot pass without it. Each gets a new run and a flow_task_reopened
// entry. It returns the keys it reopened.
func (h *Handler) reopenCancelledWork(ctx context.Context, qtx *db.Queries, run *flowRun, rows []flowTaskRow, e *flowEffects) ([]string, error) {
	var keys []string
	for i, r := range rows {
		if r.state.Step != run.state.Step || r.state.Round != run.round() || !cancelledForItsFiles(run, r) {
			continue
		}
		prev := r.issue.Status
		reopened, err := qtx.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{ID: r.issue.ID, Status: issuestatus.Todo, WorkspaceID: r.issue.WorkspaceID})
		if err != nil {
			return nil, fmt.Errorf("reopen task %s: %w", r.state.Key, err)
		}
		details, _ := json.Marshal(map[string]any{"task": r.state.Key, "step": r.state.Step, "round": r.state.Round, "reason": "cancelled_work_requires_files"})
		if _, err := qtx.CreateActivity(ctx, db.CreateActivityParams{
			ID: dbid.NewV7(), WorkspaceID: run.issue.WorkspaceID, IssueID: r.issue.ID,
			ActorType: pgtype.Text{String: "system", Valid: true}, Action: "flow_task_reopened", Details: details,
		}); err != nil {
			return nil, err
		}
		rows[i].issue = reopened
		rows[i].state.Closed, rows[i].state.Cancelled = false, false
		e.moves = append(e.moves, statusMove{prev: prev, issue: reopened})
		e.reopened = append(e.reopened, reopened)
		keys = append(keys, r.state.Key)
	}
	return keys, nil
}

// openFlowTasks creates the task rows a decision opens. The agents are checked
// first: a task whose agent cannot take work stops the request instead of waiting
// forever.
func (h *Handler) openFlowTasks(ctx context.Context, run *flowRun, rows []flowTaskRow, open []issueworkflow.OpenTask) (advanceOutcome, error) {
	if run.compat() {
		// A flow lifted from schema 1 keeps the task definitions, texts and the agent of each task.
		var missing []issueworkflow.Task
		for _, o := range open {
			if task, ok := run.def.V1Task(run.state.Step, o.Key); ok {
				missing = append(missing, task)
			}
		}
		return h.openCompatTasks(ctx, run, rows, missing)
	}
	return h.openGraphTasks(ctx, run, rows, open)
}

// flowTaskCreate is a task issue the engine opens.
type flowTaskCreate struct {
	Title       string
	Description string
	Stage       int32
	Step        string
	Key         string
	Round       int
	AssigneeTyp string
	AssigneeID  pgtype.UUID
}

// createFlowTask opens one task issue. It reports false when another run of the
// engine opened the same task first.
func (h *Handler) createFlowTask(ctx context.Context, run *flowRun, prefix string, c flowTaskCreate) (bool, error) {
	params := service.IssueCreateParams{
		WorkspaceID:    run.issue.WorkspaceID,
		Title:          c.Title,
		Description:    pgtype.Text{String: c.Description, Valid: true},
		Status:         issuestatus.Todo,
		Priority:       run.issue.Priority,
		CreatorType:    run.issue.CreatorType,
		CreatorID:      run.issue.CreatorID,
		ParentIssueID:  run.issue.ID,
		ProjectID:      run.issue.ProjectID,
		Stage:          pgtype.Int4{Int32: c.Stage, Valid: true},
		AllowDuplicate: true,
		FlowStep:       pgtype.Text{String: c.Step, Valid: true},
		FlowTask:       pgtype.Text{String: c.Key, Valid: true},
		FlowRound:      pgtype.Int4{Int32: int32(c.Round), Valid: true},
	}
	if c.AssigneeTyp != "" {
		params.AssigneeType = pgtype.Text{String: c.AssigneeTyp, Valid: true}
		params.AssigneeID = c.AssigneeID
	}
	_, err := h.IssueService.Create(ctx, params, service.IssueCreateOpts{
		BroadcastPayload: func(issue db.Issue, atts []db.Attachment, labels []db.IssueLabel) map[string]any {
			resp := issueToResponse(issue, prefix)
			h.fillStatusCategory(ctx, issue.WorkspaceID, &resp)
			return map[string]any{"issue": resp}
		},
	})
	if err != nil {
		if isFlowTaskOnce(err) {
			return false, nil
		}
		return false, fmt.Errorf("open task %s: %w", c.Key, err)
	}
	return true, nil
}

// openCompatTasks opens the tasks of a step of a flow lifted from schema 1.
func (h *Handler) openCompatTasks(ctx context.Context, run *flowRun, rows []flowTaskRow, missing []issueworkflow.Task) (advanceOutcome, error) {
	step := run.state.Step
	for _, task := range missing {
		why, err := h.flowAgentUnavailable(ctx, run, task.Handler.ID)
		if err != nil {
			return advanceOutcome{}, err
		}
		if why != "" {
			return h.stopForUnavailableAgent(ctx, run, task.Key, task.Title, task.Handler.ID, why)
		}
	}
	// A gate is never given to an agent that did work in the same round.
	workers := map[pgtype.UUID]string{}
	for _, r := range rows {
		if r.state.Round == run.round() && r.state.Kind == issueworkflow.KindWork && r.issue.AssigneeID.Valid {
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
		created, err := h.createFlowTask(ctx, run, prefix, flowTaskCreate{
			Title:       fmt.Sprintf("%s · %s · v%d", resolver.Name(ctx, h.Queries, step), task.Title, run.round()+1),
			Description: h.flowTaskDescription(run, rows, task, requestRef, prefix),
			Stage:       stage, Step: step, Key: task.Key, Round: run.round(),
			AssigneeTyp: "agent", AssigneeID: agentID,
		})
		if err != nil {
			return advanceOutcome{}, err
		}
		if created {
			opened = append(opened, task.Key)
		}
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
	inputs, rejected := flowTaskLinks(run.def, rows, prefix, run.state.Step, run.round())
	return issueworkflow.TaskDescription(issueworkflow.TaskDescriptionInput{
		Task: task, RequestIdentifier: requestRef, Round: run.round(), Inputs: inputs, Rejected: rejected,
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

// flowAgentUnavailable says why the agent of a task cannot take it now, or "" when
// it can: the agent is archived or gone, it left the squad that owns the workflow,
// or it became the lead of that squad, who plans and does not take tasks (E14). A
// workflow no squad owns has no roster to check.
func (h *Handler) flowAgentUnavailable(ctx context.Context, run *flowRun, agentRef string) (string, error) {
	agentID, err := util.ParseUUID(agentRef)
	agent, lookupErr := h.Queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: agentID, WorkspaceID: run.issue.WorkspaceID})
	if err != nil || lookupErr != nil || agent.ArchivedAt.Valid {
		return "is archived or no longer in this workspace", nil
	}
	if !run.row.SquadID.Valid {
		return "", nil
	}
	squad, err := h.Queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: run.row.SquadID, WorkspaceID: run.issue.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if squad.LeaderID == agentID {
		return "is the lead of the squad, who plans and does not take tasks", nil
	}
	member, err := h.Queries.IsSquadMember(ctx, db.IsSquadMemberParams{SquadID: squad.ID, MemberType: "agent", MemberID: agentID})
	if err != nil {
		return "", err
	}
	if !member {
		return "is no longer a member of the squad", nil
	}
	return "", nil
}

// stopForUnavailableAgent blocks a request whose next task cannot be given to
// its agent (why says what is wrong with it). The advance reports it instead of failing.
func (h *Handler) stopForUnavailableAgent(ctx context.Context, current *flowRun, key, title, agent, why string) (advanceOutcome, error) {
	out, err := h.stallFlow(ctx, current, issueworkflow.ReasonAgentUnavailable, fmt.Sprintf("The agent for the task %q (%s) %s, so the task cannot open.", key, title, why), "")
	if err != nil {
		return advanceOutcome{}, err
	}
	out.action.Tasks = []string{key}
	out.action.Code = "flow_agent_unavailable"
	out.action.AgentID = agent
	return out, nil
}

// stallFlow stops a request that cannot move on without a person: it goes to blocked with the stalled
// stop and a comment that gives the reason; code is the issueworkflow.Reason* the transition log records. blockTask, when set, is the key of an open task of the
// current step that is set to blocked first, so the stop shows which task was stuck. It does
// nothing when the request moved or stopped meanwhile.
func (h *Handler) stallFlow(ctx context.Context, current *flowRun, code, reason, blockTask string) (advanceOutcome, error) {
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
	if run.state.Step != current.state.Step || run.state.Stop != nil || run.state.ClosedAt != "" {
		return advanceOutcome{action: FlowAction{Kind: "noop", Step: run.state.Step, Tasks: []string{}}}, nil
	}
	rows, err := h.loadFlowTasks(ctx, qtx, run)
	if err != nil {
		return advanceOutcome{}, err
	}
	var effects flowEffects
	if blockTask != "" {
		for i, r := range rows {
			if r.state.Key != blockTask || r.state.Step != run.state.Step || r.state.Round != run.round() || r.state.Closed {
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
	routed, err := h.routeFlowRequest(ctx, qtx, run, issueworkflow.StalledDecision(code), rows, flowRouteNote{Reason: reason}, &effects, "")
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

// canRehearseSquad reports whether the user is a member who manages the squad the
// issue is given to, the people who may start a rehearsal of one of its versions.
func (h *Handler) canRehearseSquad(ctx context.Context, userID, workspaceID string, assigneeType pgtype.Text, assigneeID pgtype.UUID) bool {
	if assigneeType.String != "squad" || !assigneeID.Valid {
		return false
	}
	member, err := h.getWorkspaceMember(ctx, userID, workspaceID)
	if err != nil {
		return false
	}
	squad, err := h.Queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: assigneeID, WorkspaceID: member.WorkspaceID})
	return err == nil && canManageSquad(member, squad)
}
