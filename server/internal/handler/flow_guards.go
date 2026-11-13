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
	"github.com/multica-ai/multica/server/pkg/dbid"
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
	if err != nil || !def.HasFlow() || def.Graph.Compat != issueworkflow.CompatV1 {
		return issueworkflow.Definition{}, false
	}
	return def, true
}

// flowGraphTask finds the task a step of the graph defines.
func flowGraphTask(g issueworkflow.Graph, step, key string) (issueworkflow.DocTask, bool) {
	s, ok := g.Step(step)
	if !ok {
		return issueworkflow.DocTask{}, false
	}
	for _, t := range s.Tasks {
		if t.Key == key {
			return t, true
		}
	}
	return issueworkflow.DocTask{}, false
}

// flowTaskRun finds the request a task the engine opened belongs to, the
// workflow it is pinned to, and the definition of the task. ok is false when a
// link is missing, for instance after the request was detached; the issue is
// then an ordinary one.
func (h *Handler) flowTaskRun(ctx context.Context, task db.Issue) (*flowRun, issueworkflow.DocTask, bool) {
	if !task.FlowTask.Valid || !task.ParentIssueID.Valid {
		return nil, issueworkflow.DocTask{}, false
	}
	parent, err := h.Queries.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{ID: task.ParentIssueID, WorkspaceID: task.WorkspaceID})
	if err != nil {
		return nil, issueworkflow.DocTask{}, false
	}
	run, err := h.loadFlowRun(ctx, h.Queries, parent)
	if err != nil {
		return nil, issueworkflow.DocTask{}, false
	}
	spec, ok := flowGraphTask(run.def.Graph, task.FlowStep.String, task.FlowTask.String)
	if !ok {
		return nil, issueworkflow.DocTask{}, false
	}
	return run, spec, true
}

// flowTaskSpec finds the definition of a task the engine opened.
func (h *Handler) flowTaskSpec(ctx context.Context, task db.Issue) (issueworkflow.DocTask, bool) {
	_, spec, ok := h.flowTaskRun(ctx, task)
	return spec, ok
}

// flowActor is who makes a write: an agent, or a member (with the workspace
// membership that decides whether they manage the squad).
type flowActor struct {
	Type string
	// ID is the id of the agent, or of the user for a member.
	ID     string
	Member *db.Member
}

// flowActorFor resolves the actor of a request.
func (h *Handler) flowActorFor(r *http.Request, userID, workspaceID string) flowActor {
	actorType, actorID := h.resolveActor(r, userID, workspaceID)
	actor := flowActor{Type: actorType, ID: actorID}
	if actorType == "member" {
		if member, err := h.getWorkspaceMember(r.Context(), userID, workspaceID); err == nil {
			actor.Member = &member
		}
	}
	return actor
}

// checkFlowWrite applies the flow rules to one prospective update of prev.
// params carries the status and the assignee the write sets; outcome is what the
// caller reported with it, empty when none. A write to a request that does not
// change its status passes; a write to a task is checked for who makes it.
func (h *Handler) checkFlowWrite(ctx context.Context, prev db.Issue, params db.UpdateIssueParams, outcome string, actor flowActor) error {
	if prev.FlowTask.Valid {
		if !h.flowEngineEnabled(ctx) {
			return nil
		}
		return h.checkFlowTaskWrite(ctx, prev, params, outcome, actor)
	}
	if !params.Status.Valid || params.Status.String == prev.Status {
		return nil
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

func flowTaskAssignedTo(task db.Issue, actor flowActor) bool {
	return task.AssigneeID.Valid && task.AssigneeType.String == actor.Type && uuidToString(task.AssigneeID) == actor.ID
}

func (h *Handler) flowTaskClosed(ctx context.Context, task db.Issue, status string) bool {
	switch issuestatus.Category(ctx, h.Queries, task.WorkspaceID, status) {
	case issuestatus.CategoryDone, issuestatus.CategoryClosed:
		return true
	}
	return false
}

// checkFlowTaskWrite enforces who may write to a task (E1 to E4): the agent it is
// given to, or a member who manages the squad on its behalf. An agent does not
// hand a task to someone else, does not cancel it, and does not change a task
// that is closed; a member who manages the squad reopens it. What the task needs
// to close is checkFlowTaskStatus.
func (h *Handler) checkFlowTaskWrite(ctx context.Context, prev db.Issue, params db.UpdateIssueParams, outcome string, actor flowActor) error {
	run, spec, ok := h.flowTaskRun(ctx, prev)
	if !ok {
		return nil
	}
	taskFields := map[string]any{"task": spec.Key}
	statusChange := params.Status.Valid && params.Status.String != prev.Status
	reassigned := params.AssigneeType != prev.AssigneeType || params.AssigneeID != prev.AssigneeID
	closedBefore := h.flowTaskClosed(ctx, prev, prev.Status)
	changesVerdict := closedBefore && outcome != "" && outcome != prev.Outcome.String

	if reassigned && actor.Type == "agent" {
		return &flowRejection{
			Status: http.StatusConflict, Code: "flow_task_assignee_locked",
			Message: "the workflow gave this task to its agent: only a member who manages the squad can give it to someone else",
			Fields:  taskFields,
		}
	}
	if !statusChange && !changesVerdict {
		return nil
	}
	manages := actor.Member != nil && h.canManageFlowRequest(ctx, *actor.Member, run)
	if !flowTaskAssignedTo(prev, actor) && !manages {
		return &flowRejection{
			Status: http.StatusForbidden, Code: "flow_task_not_assignee",
			Message: "only the agent this task is given to, or a member who manages the squad, can change it",
			Fields:  taskFields,
		}
	}
	if closedBefore && !manages {
		return &flowRejection{
			Status: http.StatusConflict, Code: "flow_task_closed",
			Message: "this task is closed and its verdict stands: a member who manages the squad can reopen it",
			Fields:  taskFields,
		}
	}
	if !statusChange {
		return nil
	}
	to := params.Status.String
	if actor.Type == "agent" && issuestatus.Category(ctx, h.Queries, prev.WorkspaceID, to) == issuestatus.CategoryClosed {
		return &flowRejection{
			Status: http.StatusConflict, Code: "flow_task_cancel_forbidden",
			Message: "an agent does not cancel a task of the workflow: if it cannot go on, run `multica issue status <id> blocked --outcome blocked`",
			Fields:  taskFields,
		}
	}
	return h.checkFlowTaskStatus(ctx, prev, spec, to, outcome, actor)
}

// checkFlowTaskStatus enforces R1 and R2: a task closes (moves to a done status)
// only with the verdict and the file its definition requires, and a task that
// cannot go on says so with the outcome blocked.
func (h *Handler) checkFlowTaskStatus(ctx context.Context, task db.Issue, spec issueworkflow.DocTask, to, outcome string, actor flowActor) error {
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
	if spec.NeedsVerdict() {
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
	if spec.NeedsFiles() {
		attached, err := h.flowTaskAttachmentCount(ctx, task, actor)
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

// flowTaskAttachmentCount counts the files that satisfy the file requirement of
// a task: the ones its assignee attached to it, a file linked to a comment
// included, and the ones the member who closes it for the assignee attached.
func (h *Handler) flowTaskAttachmentCount(ctx context.Context, task db.Issue, actor flowActor) (int64, error) {
	count := func(uploaderType string, uploaderID pgtype.UUID) (int64, error) {
		return h.Queries.CountAttachmentsOnIssueByUploader(ctx, db.CountAttachmentsOnIssueByUploaderParams{
			IssueID:      task.ID,
			WorkspaceID:  task.WorkspaceID,
			UploaderType: uploaderType,
			UploaderID:   uploaderID,
		})
	}
	var total int64
	if task.AssigneeID.Valid {
		n, err := count(task.AssigneeType.String, task.AssigneeID)
		if err != nil {
			return 0, err
		}
		total += n
	}
	if actor.Type == "member" && !flowTaskAssignedTo(task, actor) {
		id, err := util.ParseUUID(actor.ID)
		if err != nil {
			return total, nil
		}
		n, err := count("member", id)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// logFlowManualAction records, after a write, that a member who manages the squad
// closed a task for its assignee or reopened a closed one (flow_manual_close,
// flow_manual_reopen). Best effort, like storeOutcomeAfterMove: the write is done.
func (h *Handler) logFlowManualAction(ctx context.Context, prev, issue db.Issue, outcome string, actor flowActor) {
	if !prev.FlowTask.Valid || actor.Type != "member" || prev.Status == issue.Status || flowTaskAssignedTo(prev, actor) || !h.flowEngineEnabled(ctx) {
		return
	}
	_, spec, ok := h.flowTaskRun(ctx, prev)
	if !ok {
		return
	}
	var action string
	details := map[string]any{"task": spec.Key, "activation": int(prev.FlowRound.Int32), "on_behalf_of": nil}
	if prev.AssigneeID.Valid {
		details["on_behalf_of"] = uuidToString(prev.AssigneeID)
	}
	wasClosed := h.flowTaskClosed(ctx, prev, prev.Status)
	nowClosed := h.flowTaskClosed(ctx, issue, issue.Status)
	switch {
	case wasClosed && !nowClosed:
		action = "flow_manual_reopen"
		details["from"] = prev.Status
	case !wasClosed && issuestatus.Category(ctx, h.Queries, issue.WorkspaceID, issue.Status) == issuestatus.CategoryDone:
		action = "flow_manual_close"
		files, err := h.flowTaskAttachmentCount(ctx, prev, actor)
		if err != nil {
			slog.Warn("flow: count the files of a close on behalf failed", "issue_id", uuidToString(issue.ID), "error", err)
		}
		details["verdict"] = outcome
		details["files"] = files
	default:
		return
	}
	raw, _ := json.Marshal(details)
	actorID, _ := util.ParseUUID(actor.ID)
	if _, err := h.Queries.CreateActivity(ctx, db.CreateActivityParams{
		ID: dbid.NewV7(), WorkspaceID: issue.WorkspaceID, IssueID: issue.ID,
		ActorType: pgtype.Text{String: "member", Valid: true}, ActorID: actorID,
		Action: action, Details: raw,
	}); err != nil {
		slog.Warn("flow: log a manual action failed", "issue_id", uuidToString(issue.ID), "action", action, "error", err)
	}
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
		// The brief of a flow lifted from schema 1 keeps the text of the task as the workflow wrote it.
		spec, ok := run.def.V1Task(issue.FlowStep.String, issue.FlowTask.String)
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
