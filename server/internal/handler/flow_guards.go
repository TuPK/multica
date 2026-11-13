package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The checks the server makes on writes to an issue that belongs to a flow
// (docs/engineering/kitviet-workflow-spec.md, section 4.2). The same function
// runs on the single update and the batch update, so a path cannot dodge a rule.

// flowRejection is a write the flow rules refuse. The body is the shape of
// writeIssueWorkflowError: a message an agent can read, a code, and fields.
type flowRejection struct {
	Status  int
	Code    string
	Message string
	Fields  map[string]any
}

func (e *flowRejection) Error() string { return e.Message }

// writeFlowRejection answers a flow rejection. It reports false when err is not
// one, so the caller can fall through to its generic 500.
func writeFlowRejection(w http.ResponseWriter, err error) bool {
	var rej *flowRejection
	if !errors.As(err, &rej) {
		return false
	}
	body := map[string]any{"error": rej.Message, "code": rej.Code}
	for k, v := range rej.Fields {
		body[k] = v
	}
	writeJSON(w, rej.Status, body)
	return true
}

// allowedClosingOutcomes are the verdicts a gate closes with.
var allowedClosingOutcomes = []string{issueworkflow.OutcomeApproved, issueworkflow.OutcomeRejected}

// flowRequestState reads the flow state of a request, or ok false when the
// issue does not run on a flow.
func flowRequestState(issue db.Issue) (issueworkflow.FlowState, bool) {
	if len(issue.Flow) == 0 {
		return issueworkflow.FlowState{}, false
	}
	var state issueworkflow.FlowState
	if err := json.Unmarshal(issue.Flow, &state); err != nil || state.Step == "" {
		return issueworkflow.FlowState{}, false
	}
	return state, true
}

// flowDefinition loads the workflow a request is pinned to.
func (h *Handler) flowDefinition(ctx context.Context, workspaceID pgtype.UUID, state issueworkflow.FlowState) (issueworkflow.Definition, bool) {
	id, err := util.ParseUUID(state.WorkflowID)
	if err != nil {
		return issueworkflow.Definition{}, false
	}
	row, err := h.Queries.GetIssueWorkflow(ctx, db.GetIssueWorkflowParams{ID: id, WorkspaceID: workspaceID})
	if err != nil {
		return issueworkflow.Definition{}, false
	}
	def, err := issueworkflow.Decode(row)
	if err != nil || def.Flow == nil {
		return issueworkflow.Definition{}, false
	}
	return def, true
}

// flowTaskSpec finds the definition of a task the engine opened: its request
// pins the workflow and the workflow names the task. ok is false when a link is
// missing, for instance after the request was detached; the issue is then an
// ordinary one.
func (h *Handler) flowTaskSpec(ctx context.Context, task db.Issue) (issueworkflow.Task, bool) {
	if !task.FlowTask.Valid || !task.ParentIssueID.Valid {
		return issueworkflow.Task{}, false
	}
	parent, err := h.Queries.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{ID: task.ParentIssueID, WorkspaceID: task.WorkspaceID})
	if err != nil {
		return issueworkflow.Task{}, false
	}
	state, ok := flowRequestState(parent)
	if !ok {
		return issueworkflow.Task{}, false
	}
	def, ok := h.flowDefinition(ctx, task.WorkspaceID, state)
	if !ok {
		return issueworkflow.Task{}, false
	}
	step, ok := def.Step(task.FlowStep.String)
	if !ok {
		return issueworkflow.Task{}, false
	}
	for _, t := range step.Tasks {
		if t.Key == task.FlowTask.String {
			return t, true
		}
	}
	return issueworkflow.Task{}, false
}

// flowActor is who makes a write: an agent, or a member (with the workspace
// membership that decides whether they manage the squad).
type flowActor struct {
	Type   string
	Member *db.Member
}

// flowActorFor resolves the actor of a request.
func (h *Handler) flowActorFor(r *http.Request, userID, workspaceID string) flowActor {
	actorType, _ := h.resolveActor(r, userID, workspaceID)
	actor := flowActor{Type: actorType}
	if actorType == "member" {
		if member, err := h.getWorkspaceMember(r.Context(), userID, workspaceID); err == nil {
			actor.Member = &member
		}
	}
	return actor
}

// checkFlowWrite applies the flow rules to one prospective update of prev.
// params carries the status the write sets; outcome is what the caller reported
// with it, empty when none. Writes that do not change the status pass.
func (h *Handler) checkFlowWrite(ctx context.Context, prev db.Issue, params db.UpdateIssueParams, outcome string, actor flowActor) error {
	if !params.Status.Valid || params.Status.String == prev.Status {
		return nil
	}
	if prev.FlowTask.Valid {
		if !h.flowEngineEnabled(ctx) {
			return nil
		}
		return h.checkFlowTaskStatus(ctx, prev, params.Status.String, outcome)
	}
	if _, ok := flowRequestState(prev); ok {
		if !h.flowEngineEnabled(ctx) {
			return nil
		}
		return h.checkFlowRequestMove(ctx, prev, params.Status.String, actor)
	}
	return nil
}

// checkFlowRequestMove enforces R3 and R4 on a status write to a request that a
// flow runs: an agent never moves it, and nobody leaves or enters a human-only
// step but a member who manages the squad. A member who manages the squad may
// move it by hand; the engine follows the move.
func (h *Handler) checkFlowRequestMove(ctx context.Context, request db.Issue, to string, actor flowActor) error {
	run, err := h.loadFlowRun(ctx, h.Queries, request)
	if err != nil || run.state.ClosedAt != "" {
		// A request whose flow is gone or has ended is an ordinary issue.
		return nil
	}
	if actor.Type == "agent" {
		humanOnly := false
		if step, ok := run.def.Step(run.state.Step); ok && step.HumanOnly {
			humanOnly = true
		}
		if step, ok := run.def.Step(to); ok && step.HumanOnly {
			humanOnly = true
		}
		if humanOnly {
			return &flowRejection{
				Status: http.StatusForbidden, Code: "flow_human_only",
				Message: "this step is for a person: an agent cannot move the request out of it or into it",
				Fields:  map[string]any{"step": run.state.Step},
			}
		}
		return &flowRejection{
			Status: http.StatusConflict, Code: "flow_managed_request",
			Message: "the workflow moves this request: do not set its status; run `multica issue flow advance <id>` instead",
			Fields:  map[string]any{"step": run.state.Step, "hint": "multica issue flow advance"},
		}
	}
	if actor.Member == nil || !h.canManageFlowRequest(ctx, *actor.Member, run) {
		return &flowRejection{
			Status: http.StatusForbidden, Code: "flow_forbidden",
			Message: "only a member who manages the squad can move a request that a workflow runs",
		}
	}
	return nil
}

// canManageFlowRequest reports whether the member manages the squad that owns
// the request's workflow, or else the squad the request is given to.
func (h *Handler) canManageFlowRequest(ctx context.Context, member db.Member, run *flowRun) bool {
	if roleAllowed(member.Role, "owner", "admin") {
		return true
	}
	squadID := run.row.SquadID
	if !squadID.Valid && run.issue.AssigneeType.String == "squad" {
		squadID = run.issue.AssigneeID
	}
	if !squadID.Valid {
		return false
	}
	squad, err := h.Queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: squadID, WorkspaceID: run.issue.WorkspaceID})
	return err == nil && canManageSquad(member, squad)
}

// checkFlowChildCreate enforces R5: an agent does not open sub-issues under a
// request a workflow runs; the engine opens them. A member may.
func (h *Handler) checkFlowChildCreate(ctx context.Context, workspaceID, parentID pgtype.UUID) error {
	if !h.flowEngineEnabled(ctx) {
		return nil
	}
	parent, err := h.Queries.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{ID: parentID, WorkspaceID: workspaceID})
	if err != nil {
		return nil
	}
	state, ok := flowRequestState(parent)
	if !ok || state.ClosedAt != "" {
		return nil
	}
	return &flowRejection{
		Status: http.StatusConflict, Code: "flow_children_managed",
		Message: "the workflow opens the sub-issues of this request: do not create one; run `multica issue flow advance <id>`",
		Fields:  map[string]any{"step": state.Step},
	}
}

// syncFlowAfterMove makes the flow state follow a status a person set by hand
// on a request. A failure is logged: the move is done, and the next advance
// repeats the sync.
func (h *Handler) syncFlowAfterMove(ctx context.Context, request db.Issue) {
	if _, ok := flowRequestState(request); !ok || !h.flowEngineEnabled(ctx) {
		return
	}
	if _, err := h.advanceOnce(ctx, request); err != nil {
		slog.Warn("flow: follow a manual move failed", "issue_id", uuidToString(request.ID), "error", err)
	}
}

// checkFlowTaskStatus enforces R1 and R2: a task closes (moves to a done status)
// only with the verdict and the file its definition requires, and a task that
// cannot go on says so with the outcome blocked.
func (h *Handler) checkFlowTaskStatus(ctx context.Context, task db.Issue, to, outcome string) error {
	if !task.FlowTask.Valid {
		return nil
	}
	spec, ok := h.flowTaskSpec(ctx, task)
	if !ok {
		return nil
	}
	taskFields := func(extra map[string]any) map[string]any {
		fields := map[string]any{"task": spec.Key}
		for k, v := range extra {
			fields[k] = v
		}
		return fields
	}
	if to == issuestatus.Blocked {
		if outcome != "" && outcome != issueworkflow.OutcomeBlocked {
			return &flowRejection{
				Status: http.StatusUnprocessableEntity, Code: "flow_outcome_invalid",
				Message: "a task that is set to blocked reports the outcome blocked, not " + outcome +
					": run `multica issue status <id> blocked --outcome blocked`",
				Fields: taskFields(map[string]any{"allowed_outcomes": []string{issueworkflow.OutcomeBlocked}}),
			}
		}
		return nil
	}
	if issuestatus.Category(ctx, h.Queries, task.WorkspaceID, to) != issuestatus.CategoryDone {
		return nil
	}
	if outcome == issueworkflow.OutcomeBlocked {
		return &flowRejection{
			Status: http.StatusUnprocessableEntity, Code: "flow_outcome_invalid",
			Message: "a task with the outcome blocked cannot be done: run `multica issue status <id> blocked --outcome blocked`",
			Fields:  taskFields(map[string]any{"allowed_outcomes": allowedClosingOutcomes}),
		}
	}
	if spec.Kind == issueworkflow.KindGate || spec.Needs(issueworkflow.RequireOutcome) {
		switch outcome {
		case "":
			return &flowRejection{
				Status: http.StatusUnprocessableEntity, Code: "flow_outcome_required",
				Message: "this task closes only with a verdict: run `multica issue status <id> done --outcome approved` or `--outcome rejected`",
				Fields:  taskFields(map[string]any{"allowed_outcomes": allowedClosingOutcomes}),
			}
		case issueworkflow.OutcomeApproved, issueworkflow.OutcomeRejected:
		default:
			return &flowRejection{
				Status: http.StatusUnprocessableEntity, Code: "flow_outcome_invalid",
				Message: "this task closes with the outcome approved or rejected, not " + outcome,
				Fields:  taskFields(map[string]any{"allowed_outcomes": allowedClosingOutcomes}),
			}
		}
	}
	if spec.Needs(issueworkflow.RequireAttachment) {
		attached, err := h.agentAttachmentCount(ctx, task)
		if err != nil {
			return err
		}
		if attached == 0 {
			return &flowRejection{
				Status: http.StatusUnprocessableEntity, Code: "flow_attachment_required",
				Message: "this task closes only with a file you attached to it: run `multica issue comment add <id> --attachment <file>` first",
				Fields:  taskFields(nil),
			}
		}
	}
	return nil
}

// agentAttachmentCount counts the files the task's assignee attached to it. A
// file linked to a comment keeps the issue's id, so it counts too.
func (h *Handler) agentAttachmentCount(ctx context.Context, task db.Issue) (int64, error) {
	if task.AssigneeType.String != "agent" || !task.AssigneeID.Valid {
		return 0, nil
	}
	return h.Queries.CountAgentAttachmentsOnIssue(ctx, db.CountAgentAttachmentsOnIssueParams{
		IssueID:     task.ID,
		WorkspaceID: task.WorkspaceID,
		UploaderID:  task.AssigneeID,
	})
}

// recordIssueOutcome stores the outcome a status change reported, or clears the
// one an earlier change left when this change reports none: an outcome belongs
// to the move that set it. It returns the issue as stored.
func (h *Handler) recordIssueOutcome(ctx context.Context, issue db.Issue, outcome string) (db.Issue, error) {
	if issue.Outcome.Valid == (outcome != "") && issue.Outcome.String == outcome {
		return issue, nil
	}
	if err := h.Queries.SetIssueOutcome(ctx, db.SetIssueOutcomeParams{
		ID:          issue.ID,
		WorkspaceID: issue.WorkspaceID,
		Outcome:     pgtype.Text{String: outcome, Valid: outcome != ""},
	}); err != nil {
		return issue, err
	}
	issue.Outcome = pgtype.Text{String: outcome, Valid: outcome != ""}
	return issue, nil
}

// storeOutcomeAfterMove records the outcome of a status change that was just
// written. A failure is logged, not returned: the move is done, and a task whose
// outcome is missing counts as rejected, never as approved.
func (h *Handler) storeOutcomeAfterMove(ctx context.Context, issue db.Issue, outcome string) db.Issue {
	stored, err := h.recordIssueOutcome(ctx, issue, outcome)
	if err != nil {
		slog.Warn("store issue outcome failed", "issue_id", uuidToString(issue.ID), "error", err)
		return issue
	}
	return stored
}

// cancelQueuedRunsOfClosedFlowTask cancels the queued runs of a flow task that
// has just closed. The engine counts a task open while any of its runs is still
// queued, so a closed task would otherwise hold its request until a runtime
// claims a run nobody needs. A run that has started is left to finish, since its
// agent may still be writing the verdict. Best effort, like storeOutcomeAfterMove:
// a failure is logged and the close stands.
func (h *Handler) cancelQueuedRunsOfClosedFlowTask(ctx context.Context, issue db.Issue, actor service.TaskCancellationActor) {
	if !issue.FlowTask.Valid {
		return
	}
	category := issuestatus.NewResolver(issue.WorkspaceID).Category(ctx, h.Queries, issue.Status)
	if category != issuestatus.CategoryDone && category != issuestatus.CategoryClosed {
		return
	}
	runs, err := h.Queries.ListActiveTasksByIssue(ctx, issue.ID)
	if err != nil {
		slog.Warn("list runs of closed flow task failed", "issue_id", uuidToString(issue.ID), "error", err)
		return
	}
	for _, run := range runs {
		if run.Status != "queued" {
			continue
		}
		if _, err := h.TaskService.CancelTaskByUser(ctx, run.ID, actor); err != nil {
			slog.Warn("cancel queued run of closed flow task failed", "task_id", uuidToString(run.ID), "error", err)
		}
	}
}

// applyFlowStatusRules keeps the project workflow out of a flow's way. A task
// the engine opened is exempt from it: it takes the built-in statuses whatever
// the project lists. A request a flow runs takes the statuses of its own flow
// workflow, and no handoff of the project applies to it. handled is true when
// the issue is one of these and the project workflow must not be applied.
func (h *Handler) applyFlowStatusRules(ctx context.Context, prev db.Issue, params *db.UpdateIssueParams) (handled bool, err error) {
	if !h.flowEngineEnabled(ctx) {
		return false, nil
	}
	if prev.FlowTask.Valid {
		return true, nil
	}
	run, loadErr := h.loadFlowRun(ctx, h.Queries, prev)
	if loadErr != nil || run.state.ClosedAt != "" {
		return false, nil
	}
	if params.Status.Valid && params.Status.String != prev.Status {
		if err := issueworkflow.CheckStatus(&run.def, params.Status.String); err != nil {
			return true, err
		}
	}
	return true, nil
}

// claimFlowBrief renders the brief of a run on a task of a flow, or on the
// request a flow runs (the lead's). It is nil for any other issue, and for a
// flow that is closed or no longer readable, which then read the project's brief
// as before.
func (h *Handler) claimFlowBrief(ctx context.Context, issue db.Issue) *TaskProjectWorkflowData {
	if !h.flowEngineEnabled(ctx) {
		return nil
	}
	prefix := h.getIssuePrefix(ctx, issue.WorkspaceID)
	identifier := func(i db.Issue) string { return prefix + "-" + strconv.Itoa(int(i.Number)) }
	resolver := issuestatus.NewResolver(issue.WorkspaceID)
	if issue.FlowTask.Valid && issue.ParentIssueID.Valid {
		parent, err := h.Queries.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{ID: issue.ParentIssueID, WorkspaceID: issue.WorkspaceID})
		if err != nil {
			return nil
		}
		run, err := h.loadFlowRun(ctx, h.Queries, parent)
		if err != nil || run.state.ClosedAt != "" {
			return nil
		}
		spec, ok := h.flowTaskSpec(ctx, issue)
		if !ok {
			return nil
		}
		rows, err := h.loadFlowTasks(ctx, h.Queries, run)
		if err != nil {
			return nil
		}
		round := int(issue.FlowRound.Int32)
		inputs, rejected := flowTaskLinks(run.def, rows, prefix, issue.FlowStep.String, round)
		return &TaskProjectWorkflowData{Brief: issueworkflow.FlowTaskBrief(issueworkflow.FlowTaskBriefInput{
			WorkflowName:      run.def.Name,
			RequestIdentifier: identifier(parent),
			TaskIdentifier:    identifier(issue),
			StepName:          resolver.Name(ctx, h.Queries, issue.FlowStep.String),
			Task:              spec,
			Round:             round,
			Inputs:            inputs,
			Rejected:          rejected,
		})}
	}
	run, err := h.loadFlowRun(ctx, h.Queries, issue)
	if err != nil || run.state.ClosedAt != "" {
		return nil
	}
	return &TaskProjectWorkflowData{Brief: issueworkflow.FlowLeadBrief(issueworkflow.FlowLeadBriefInput{
		WorkflowName:      run.def.Name,
		RequestIdentifier: identifier(issue),
		Step:              run.state.Step,
		StepName:          resolver.Name(ctx, h.Queries, run.state.Step),
		PlanStep:          run.state.Step == issueworkflow.StartStatus,
		Stop:              run.state.Stop,
		Switches:          run.def.Flow.Switches,
		Values:            run.state.Switches,
	})}
}
