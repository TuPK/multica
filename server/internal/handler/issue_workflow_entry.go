package handler

import (
	"context"
	"errors"
	"fmt"
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

// Project workflows on the issue write paths (MUL-7420).
//
// Two rules apply to an issue in a project:
//   - when the project uses a named workflow, its status must be one the
//     workflow lists; a move into such a project keeps an equivalent status
//     instead of failing. Default accepts every status;
//   - entering a column the project hands off, through an ordinary status
//     change of an issue that stays in its project, assigns the issue to that
//     handler and starts its run with the step's instructions. A write that
//     sets the assignee itself keeps its explicit choice. A project move
//     enters the column it lands on only when the caller asks for it.
//
// Handoffs reuse assignment wholesale: the resolved handler goes through the
// same permission gate as a hand-picked assignee, and the run starts through
// the same WillEnqueueRun predicate, with the step's brief as its handoff note.

// workflowHandoff is a handoff an issue write triggers.
type workflowHandoff struct {
	def     issueworkflow.Definition
	step    issueworkflow.Step
	project db.Project
	// rework is set when the platform turned a return from review or QA into a
	// block, because the issue had already been returned too often.
	rework *reworkLoop
}

// applyIssueWorkflow enforces prev's (possibly new) project workflow on one
// prospective update and may rewrite params: the status of an issue moving
// into the project, and the assignee when the write enters a handoff step.
// enterOnMove makes a project move enter the step the issue lands on.
// run is the calling agent's run on this issue (nil for members): it may only
// move the issue from the step it works on. outcome is the outcome the handler
// reported for a status change, empty when it reported none. It returns the
// handoff that write triggers, or nil.
func (h *Handler) applyIssueWorkflow(ctx context.Context, prev db.Issue, params *db.UpdateIssueParams, statusExplicit, assigneeTouched, enterOnMove bool, run *db.AgentTaskQueue, outcome string) (*workflowHandoff, error) {
	def, project, err := issueworkflow.ForIssue(ctx, h.Queries, prev.WorkspaceID, params.ProjectID, params.AssigneeType, params.AssigneeID)
	if err != nil {
		return nil, err
	}
	// Keeping the current status is never a rejected write, even when the
	// workflow no longer lists it.
	statusChanging := params.Status.Valid && params.Status.String != prev.Status
	if def == nil {
		if !statusChanging {
			return nil, nil
		}
		outside, err := issueworkflow.OutsideProject(ctx, h.Queries, prev.WorkspaceID)
		if err != nil {
			return nil, err
		}
		return nil, issueworkflow.CheckStatusOutsideProject(outside, params.Status.String)
	}
	projectMoving := params.ProjectID != prev.ProjectID
	if projectMoving && !statusExplicit && !def.IsDefault && !def.Has(prev.Status) {
		target := issueworkflow.MoveTarget(*def, prev.Status, func(key string) string {
			return issuestatus.Category(ctx, h.Queries, prev.WorkspaceID, key)
		})
		params.Status = pgtype.Text{String: target, Valid: true}
	}
	// A move into a Default project keeps the issue's status, as it is never
	// mapped; a named workflow must list the status the issue lands on.
	if params.Status.Valid && (statusChanging || (projectMoving && !def.IsDefault)) {
		if err := issueworkflow.CheckStatus(def, params.Status.String); err != nil {
			return nil, err
		}
	}
	if statusExplicit && statusChanging && outcome != "" {
		if err := h.checkOutcomeRoute(ctx, *def, prev, outcome, params.Status.String); err != nil {
			return nil, err
		}
	}
	// Someone else moved the issue while the run worked: the run's decision was
	// made for a step the issue has left, and applying it would undo that move.
	// Default without handoffs keeps the pre-workflow behavior, so workspaces
	// that never adopt workflows see no change.
	if workflowStepMoved(def, prev.Status, run) &&
		statusExplicit && params.Status.Valid && params.Status.String != prev.Status {
		return nil, h.stepMovedError(ctx, prev, run.WorkflowStep.String)
	}
	// An agent run finishes the issue only from the last step before done: the
	// steps the workflow still has after its own are handed to the next handler.
	if run != nil && !def.IsDefault && statusExplicit && params.Status.Valid &&
		params.Status.String == issuestatus.Done && prev.Status != issuestatus.Done {
		if next := def.NextStatus(prev.Status); next != "" && next != issuestatus.Done {
			return nil, h.stepSkippedError(ctx, prev, next)
		}
	}
	if assigneeTouched || prev.TriageState.Valid {
		return nil, nil
	}
	entering := ""
	switch {
	case projectMoving && enterOnMove:
		entering = prev.Status
		if params.Status.Valid {
			entering = params.Status.String
		}
	// A project move is administrative unless the caller opted in, and
	// re-selecting the current status is a no-op: neither hands the issue off.
	case !projectMoving && statusExplicit && params.Status.Valid && params.Status.String != prev.Status:
		entering = params.Status.String
	default:
		return nil, nil
	}
	var rework *reworkLoop
	if loop, err := h.reworkLoopFor(ctx, def, prev, entering, projectMoving, run); err != nil {
		return nil, err
	} else if loop != nil {
		rework = loop
		entering = issuestatus.Blocked
		params.Status = pgtype.Text{String: entering, Valid: true}
	}
	step, ok := def.Step(entering)
	if !ok || !step.HandsOff() {
		return nil, nil
	}
	// A request a squad leader is working is not a step of the workflow when
	// someone else moves it: the leader splits it into issues, and those issues
	// go through the steps. The leader's own run moving the request does hand it
	// to the step's handler, so the request does not stall with the leader after
	// the leader sends it on.
	if h.isSquadRequest(ctx, prev) && !h.isRunOfRequestLeader(ctx, prev, run) {
		return nil, nil
	}
	assigneeType, assigneeID, ok := issueworkflow.ResolveHandler(step, project, prev.CreatorType, prev.CreatorID)
	if !ok {
		return nil, nil
	}
	params.AssigneeType = pgtype.Text{String: assigneeType, Valid: true}
	params.AssigneeID = assigneeID
	return &workflowHandoff{def: *def, step: step, project: project, rework: rework}, nil
}

// reworkReturnLimit is the number of returns from review or QA to In Progress
// after which the next one blocks the issue instead. A third return means the
// same findings did not yield to the developer's fixes, so the cause is
// probably not the developer's to find (sdlc-delivery-core, "rework-loop").
const reworkReturnLimit = 3

// reworkLoop is a return from review or QA the platform turned into a block.
type reworkLoop struct {
	from    string
	returns int
	agentID pgtype.UUID
}

// reworkLoopFor decides whether an agent run's return of an issue from review
// or QA to In Progress is the one that ends the loop. It applies only to a
// workflow that hands Blocked to an agent (the lead) and whose review or QA
// step sends rejected work back to In Progress, and only to a step handler's own
// move, never a member's. The count restarts after each block.
func (h *Handler) reworkLoopFor(ctx context.Context, def *issueworkflow.Definition, prev db.Issue, entering string, projectMoving bool, run *db.AgentTaskQueue) (*reworkLoop, error) {
	if run == nil || projectMoving || entering != issuestatus.InProgress {
		return nil, nil
	}
	if prev.Status != issuestatus.InReview && prev.Status != "qa" {
		return nil, nil
	}
	from, ok := def.Step(prev.Status)
	if !ok || from.BackStatusKey() != entering || from.Handler.Type != issueworkflow.HandlerAgent ||
		from.Handler.ID != util.UUIDToString(run.AgentID) {
		return nil, nil
	}
	if blocked, ok := def.Step(issuestatus.Blocked); !ok || blocked.Handler.Type != issueworkflow.HandlerAgent {
		return nil, nil
	}
	n, err := h.Queries.CountWorkflowReturns(ctx, db.CountWorkflowReturnsParams{
		IssueID:      prev.ID,
		FromStatuses: []string{issuestatus.InReview, "qa"},
		ToStatus:     issuestatus.InProgress,
	})
	if err != nil {
		return nil, err
	}
	if int(n)+1 < reworkReturnLimit {
		return nil, nil
	}
	return &reworkLoop{from: prev.Status, returns: int(n) + 1, agentID: run.AgentID}, nil
}

// postReworkLoopReport writes the blocker report the lead reads, as the agent
// whose return the platform turned into a block. The report follows the form of
// sdlc-delivery-core, so the lead routes it like any other blocker. The lead is
// already woken by the block, so the comment does not wake it a second time.
func (h *Handler) postReworkLoopReport(ctx context.Context, issue db.Issue, hand *workflowHandoff) {
	if hand == nil || hand.rework == nil {
		return
	}
	stage := "review"
	if hand.rework.from == "qa" {
		stage = "QA"
	}
	content := fmt.Sprintf("BLOCKED\n"+
		"Blocker-class: rework-loop\n"+
		"Blocked-from: in_progress\n"+
		"Symptom: this issue has been sent back from %s to In Progress %d times since it was last blocked. The platform blocked it on the latest return instead of sending it back again. The %s verdicts and the developer's reports on this issue show the findings.\n"+
		"Tried: the developer's fixes after each return, in the earlier reports on this issue.\n"+
		"Question: what is the root cause of the findings that keep coming back (the design, an acceptance criterion, or how it is verified), and what change of approach ends the loop?\n"+
		"Resume-check: the findings of the latest %s verdict are fixed and the next %s of this issue passes\n",
		stage, hand.rework.returns, stage, stage, stage)
	_, err := h.Queries.CreateComment(ctx, db.CreateCommentParams{
		ID:                 dbid.NewV7(),
		IssueID:            issue.ID,
		WorkspaceID:        issue.WorkspaceID,
		AuthorType:         "agent",
		AuthorID:           hand.rework.agentID,
		Content:            content,
		Type:               "comment",
		SuppressedAgentIds: []pgtype.UUID{issue.AssigneeID},
	})
	if err != nil {
		slog.Warn("workflow: could not post the rework-loop report", "issue_id", util.UUIDToString(issue.ID), "error", err)
		return
	}
}

// workflowStepMoved shares the step guard with the claim brief so a run is
// never told to advance a step its status changes cannot act on.
func workflowStepMoved(def *issueworkflow.Definition, status string, run *db.AgentTaskQueue) bool {
	return def != nil && run != nil && run.WorkflowStep.Valid && run.WorkflowStep.String != status &&
		(!def.IsDefault || def.HandsOffAnywhere())
}

// workflowHandoffPayload describes a handoff on the issue:updated event, so
// the activity log can record the status change and the handoff as one
// timeline entry.
func workflowHandoffPayload(issue db.Issue, hand *workflowHandoff, note string, runStarted bool) map[string]string {
	out := map[string]string{
		"workflow": hand.def.Name,
		"to_type":  issue.AssigneeType.String,
		"to_id":    uuidToString(issue.AssigneeID),
		"run":      strconv.FormatBool(runStarted),
	}
	if runStarted && note != "" {
		out["note"] = note
	}
	return out
}

// checkOutcomeRoute refuses a status change that contradicts the outcome a
// handler reported. When the step has transitions that route the outcome, the
// status written must be the route's target. An outcome no transition matches
// leaves the status to the write, as it always was.
func (h *Handler) checkOutcomeRoute(ctx context.Context, def issueworkflow.Definition, prev db.Issue, outcome, requested string) error {
	step, ok := def.Step(prev.Status)
	if !ok || len(step.Transitions) == 0 {
		return nil
	}
	facts, err := h.issueRouteFacts(ctx, prev, outcome)
	if err != nil {
		return err
	}
	routed, ok := step.Route(facts)
	if !ok || routed.ToStatusKey == requested {
		return nil
	}
	return &issueworkflow.OutcomeRouteError{Outcome: outcome, StatusKey: prev.Status, Expected: routed.ToStatusKey, Requested: requested}
}

// issueRouteFacts describes an issue as it leaves its step, for routing.
func (h *Handler) issueRouteFacts(ctx context.Context, issue db.Issue, outcome string) (issueworkflow.Facts, error) {
	labels, err := h.Queries.ListLabelsByIssue(ctx, db.ListLabelsByIssueParams{IssueID: issue.ID, WorkspaceID: issue.WorkspaceID})
	if err != nil {
		return issueworkflow.Facts{}, err
	}
	returns, err := h.Queries.CountWorkflowReturns(ctx, db.CountWorkflowReturnsParams{
		IssueID:      issue.ID,
		FromStatuses: []string{issuestatus.InReview, "qa"},
		ToStatus:     issuestatus.InProgress,
	})
	if err != nil {
		return issueworkflow.Facts{}, err
	}
	facts := issueworkflow.Facts{
		Outcome:      outcome,
		Priority:     issue.Priority,
		ProjectID:    uuidToString(issue.ProjectID),
		ReworkCount:  int(returns),
		CreatorType:  issue.CreatorType,
		AssigneeType: issue.AssigneeType.String,
	}
	for _, label := range labels {
		facts.Labels = append(facts.Labels, label.Name)
	}
	return facts, nil
}

// writeIssueWorkflowError answers a workflow rejection. It reports false when
// err is not one, so the caller can fall through to its generic 500.
func writeIssueWorkflowError(w http.ResponseWriter, err error) bool {
	var notIn *issueworkflow.NotInWorkflowError
	if errors.As(err, &notIn) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":            notIn.Error(),
			"code":             "status_not_in_workflow",
			"workflow_name":    notIn.WorkflowName,
			"allowed_statuses": notIn.Allowed,
		})
		return true
	}
	var routed *issueworkflow.OutcomeRouteError
	if errors.As(err, &routed) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":           routed.Error(),
			"code":            "workflow_outcome_mismatch",
			"step":            routed.StatusKey,
			"expected_status": routed.Expected,
		})
		return true
	}
	var skipped *issueworkflow.StepSkippedError
	if errors.As(err, &skipped) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":       skipped.Error(),
			"code":        "workflow_step_skipped",
			"step":        skipped.Step,
			"next_status": skipped.Next,
		})
		return true
	}
	var moved *issueworkflow.StepMovedError
	if errors.As(err, &moved) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":          moved.Error(),
			"code":           "workflow_step_moved",
			"step":           moved.Step,
			"current_status": moved.Current,
		})
		return true
	}
	return false
}

// agentRunOnIssue returns the calling agent's run when it works on this issue,
// nil for members and for runs on other issues. The task id is the
// server-trusted X-Task-ID resolveActor vouched for.
func (h *Handler) agentRunOnIssue(r *http.Request, actorType string, issue db.Issue) *db.AgentTaskQueue {
	if actorType != "agent" {
		return nil
	}
	taskID, err := util.ParseUUID(r.Header.Get("X-Task-ID"))
	if err != nil {
		return nil
	}
	task, err := h.Queries.GetAgentTask(r.Context(), taskID)
	if err != nil || task.IssueID != issue.ID {
		return nil
	}
	return &task
}

// handedToAgent names the agent a write hands the issue to: its new agent
// assignee, the leader of its new squad assignee, or a workflow step's
// handler even when that handler already owned the issue. Empty when the
// write leaves the issue with whoever had it.
func (h *Handler) handedToAgent(ctx context.Context, prev db.Issue, params db.UpdateIssueParams, handoff bool) string {
	if !params.AssigneeID.Valid {
		return ""
	}
	if !handoff && params.AssigneeType == prev.AssigneeType && params.AssigneeID == prev.AssigneeID {
		return ""
	}
	switch params.AssigneeType.String {
	case "agent":
		return uuidToString(params.AssigneeID)
	case "squad":
		squad, err := h.Queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: params.AssigneeID, WorkspaceID: prev.WorkspaceID})
		if err == nil && squad.LeaderID.Valid {
			return uuidToString(squad.LeaderID)
		}
	}
	return ""
}

// advanceRunStep moves a run's workflow step along with a status change the
// run made itself, so its next change is judged from where it put the issue.
func (h *Handler) advanceRunStep(ctx context.Context, run *db.AgentTaskQueue, prev, issue db.Issue) {
	if run == nil || !run.WorkflowStep.Valid || prev.Status == issue.Status {
		return
	}
	if err := h.Queries.SetTaskWorkflowStep(ctx, db.SetTaskWorkflowStepParams{
		ID:           run.ID,
		WorkflowStep: pgtype.Text{String: issue.Status, Valid: true},
	}); err != nil {
		slog.Warn("workflow: advance run step failed", "task_id", uuidToString(run.ID), "error", err)
	}
}

// stepSkippedError explains a refused move to done: where the issue is and
// the step the workflow still has to pass through.
func (h *Handler) stepSkippedError(ctx context.Context, issue db.Issue, next string) error {
	resolver := issuestatus.NewResolver(issue.WorkspaceID)
	return &issueworkflow.StepSkippedError{
		IssueIdentifier: h.getIssuePrefix(ctx, issue.WorkspaceID) + "-" + strconv.Itoa(int(issue.Number)),
		Step:            issue.Status,
		StepName:        resolver.Name(ctx, h.Queries, issue.Status),
		Next:            next,
		NextName:        resolver.Name(ctx, h.Queries, next),
	}
}

// stepMovedError explains a refused status change: where the run's step was,
// where the issue is now, and who moved it.
func (h *Handler) stepMovedError(ctx context.Context, issue db.Issue, step string) error {
	resolver := issuestatus.NewResolver(issue.WorkspaceID)
	out := &issueworkflow.StepMovedError{
		IssueIdentifier: h.getIssuePrefix(ctx, issue.WorkspaceID) + "-" + strconv.Itoa(int(issue.Number)),
		Step:            step,
		StepName:        resolver.Name(ctx, h.Queries, step),
		Current:         issue.Status,
		CurrentName:     resolver.Name(ctx, h.Queries, issue.Status),
		MovedBy:         h.lastIssueStatusMover(ctx, issue),
	}
	return out
}

func (h *Handler) lastIssueStatusMover(ctx context.Context, issue db.Issue) string {
	if last, err := h.Queries.GetLastIssueStatusChange(ctx, issue.ID); err == nil && last.ActorID.Valid {
		return h.actorDisplayName(ctx, issue.WorkspaceID, last.ActorType.String, last.ActorID)
	}
	return ""
}

// workflowHandoffBrief renders the brief a handoff's agent or squad will
// read, for the timeline and the handoff confirmation. Members run nothing,
// so they get none. The run itself reads the brief its claim renders.
func (h *Handler) workflowHandoffBrief(ctx context.Context, issue db.Issue, hand *workflowHandoff) string {
	if hand == nil || !issue.AssigneeType.Valid {
		return ""
	}
	if t := issue.AssigneeType.String; t != "agent" && t != "squad" {
		return ""
	}
	in := h.workflowBriefInput(ctx, issue, hand.def, hand.project)
	in.StatusKey = hand.step.StatusKey
	return issueworkflow.Brief(in)
}

// workflowBriefInput describes an issue at its current status under its
// project's workflow, for the assignee's brief.
func (h *Handler) workflowBriefInput(ctx context.Context, issue db.Issue, def issueworkflow.Definition, project db.Project) issueworkflow.BriefInput {
	resolver := issuestatus.NewResolver(issue.WorkspaceID)
	return issueworkflow.BriefInput{
		Workflow:        def,
		ProjectName:     project.Title,
		StatusKey:       issue.Status,
		IssueIdentifier: h.getIssuePrefix(ctx, issue.WorkspaceID) + "-" + strconv.Itoa(int(issue.Number)),
		StatusName: func(key string) string {
			return resolver.Name(ctx, h.Queries, key)
		},
		HandlerName: func(step issueworkflow.Step) string {
			if !step.HandsOff() {
				return ""
			}
			return h.stepHandlerName(ctx, issue, project, step)
		},
	}
}

// actorDisplayName names an agent, squad or member for a brief. It falls back
// to the actor type so a lookup failure never blocks a handoff.
func (h *Handler) actorDisplayName(ctx context.Context, wsUUID pgtype.UUID, actorType string, id pgtype.UUID) string {
	switch actorType {
	case "agent":
		if agent, err := h.Queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: id, WorkspaceID: wsUUID}); err == nil {
			return agent.Name
		}
	case "squad":
		if squad, err := h.Queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: id, WorkspaceID: wsUUID}); err == nil {
			return squad.Name
		}
	case "member":
		if user, err := h.Queries.GetUser(ctx, id); err == nil {
			return user.Name
		}
	}
	return actorType
}

// stopPreviousAssigneeRuns cancels the active runs of whoever a handoff took
// the issue from — an agent's runs, or every run made on a squad's behalf —
// when the caller asked for it. A handoff back to the same handler keeps its
// run, and the agent making the handoff keeps its own: that run is the one
// making this write.
func (h *Handler) stopPreviousAssigneeRuns(ctx context.Context, prev, issue db.Issue, actorType, actorID string) {
	if issue.AssigneeType == prev.AssigneeType && issue.AssigneeID == prev.AssigneeID {
		return
	}
	tasks := h.assigneeActiveRuns(ctx, prev)
	if len(tasks) == 0 {
		return
	}
	actor := service.TaskCancellationActor{Type: actorType}
	if id, err := parseUUIDString(actorID); err == nil {
		actor.ID = id
		actor.Name = h.actorDisplayName(ctx, issue.WorkspaceID, actorType, id)
	}
	for _, task := range tasks {
		if actorType == "agent" && uuidToString(task.AgentID) == actorID {
			continue
		}
		if _, err := h.TaskService.CancelTaskByUser(ctx, task.ID, actor); err != nil {
			slog.Warn("workflow: cancel previous run failed",
				"issue_id", uuidToString(issue.ID), "task_id", uuidToString(task.ID), "error", err)
		}
	}
}

// assigneeActiveRuns lists the active runs of an issue's assignee on it: the
// agent's own runs, or every run made on a squad's behalf. Members run
// nothing.
func (h *Handler) assigneeActiveRuns(ctx context.Context, issue db.Issue) []db.AgentTaskQueue {
	if !issue.AssigneeID.Valid {
		return nil
	}
	assigneeType := issue.AssigneeType.String
	if assigneeType != "agent" && assigneeType != "squad" {
		return nil
	}
	tasks, err := h.Queries.ListActiveTasksByIssue(ctx, issue.ID)
	if err != nil {
		slog.Warn("workflow: list active tasks failed", "issue_id", uuidToString(issue.ID), "error", err)
		return nil
	}
	var out []db.AgentTaskQueue
	for _, task := range tasks {
		if (assigneeType == "agent" && task.AgentID == issue.AssigneeID) ||
			(assigneeType == "squad" && task.SquadID == issue.AssigneeID) {
			out = append(out, task)
		}
	}
	return out
}

// claimProjectWorkflow renders the issue's project workflow for a task claim,
// or nil when there is nothing to say beyond the runtime's own status rules:
// no project, or Default with no handoffs. The issue's assignee gets the
// step's instructions and commands; any other agent working on the issue
// gets the observer brief, so it does not move a step it does not own.
//
// legacyRuntimeRules is true for a daemon that does not advertise
// protocol.DaemonCapabilityWorkflowStatusRulesV1; see BriefInput.
func (h *Handler) claimProjectWorkflow(ctx context.Context, issue db.Issue, task *db.AgentTaskQueue, legacyRuntimeRules bool) *TaskProjectWorkflowData {
	def, project, err := issueworkflow.ForIssue(ctx, h.Queries, issue.WorkspaceID, issue.ProjectID, issue.AssigneeType, issue.AssigneeID)
	if err != nil {
		slog.Warn("task claim: failed to load project workflow for brief injection",
			"issue_id", uuidToString(issue.ID), "error", err)
		return nil
	}
	if def == nil || (def.IsDefault && !def.HandsOffAnywhere()) {
		return nil
	}
	in := h.workflowBriefInput(ctx, issue, *def, project)
	in.LegacyRuntimeRules = legacyRuntimeRules
	stepMoved := workflowStepMoved(def, issue.Status, task)
	if stepMoved {
		in.BoundStep = task.WorkflowStep.String
		in.MovedBy = h.lastIssueStatusMover(ctx, issue)
	} else if !taskRunsForAssignee(issue, task) {
		in.Observer = true
		if issue.AssigneeType.Valid && issue.AssigneeID.Valid {
			in.AssigneeName = h.actorDisplayName(ctx, issue.WorkspaceID, issue.AssigneeType.String, issue.AssigneeID)
		}
	}
	return &TaskProjectWorkflowData{Brief: issueworkflow.Brief(in), stepMoved: stepMoved}
}

// taskRunsForAssignee reports whether a run works on the issue as its
// assignee: the assigned agent's own run, or a run made for the assigned
// squad.
func taskRunsForAssignee(issue db.Issue, task *db.AgentTaskQueue) bool {
	if task == nil || !issue.AssigneeID.Valid {
		return false
	}
	switch issue.AssigneeType.String {
	case "agent":
		return task.AgentID == issue.AssigneeID
	case "squad":
		return task.SquadID == issue.AssigneeID
	}
	return false
}

// handoffRunStale reports whether a run queued by a workflow handoff has lost
// its work. Such a run is owed to the issue only while it sits on a step that
// hands off to the run's assignee. Once the issue is back in backlog, on a step
// no handler takes, or with another assignee, nothing hands the work to this
// run, and it must not start. Comment, mention, wakeup and delegated runs are
// not handoffs and are left alone.
func (h *Handler) handoffRunStale(ctx context.Context, issue db.Issue, task *db.AgentTaskQueue, wakeup bool) bool {
	if task == nil || !task.WorkflowStep.Valid || wakeup || task.DelegatedFromTaskID.Valid ||
		task.TriggerCommentID.Valid || len(task.CoalescedCommentIds) > 0 {
		return false
	}
	def, _, err := issueworkflow.ForIssue(ctx, h.Queries, issue.WorkspaceID, issue.ProjectID, issue.AssigneeType, issue.AssigneeID)
	if err != nil || def == nil || (def.IsDefault && !def.HandsOffAnywhere()) {
		return false
	}
	step, ok := def.Step(issue.Status)
	return !ok || !step.HandsOff() || !taskRunsForAssignee(issue, task)
}

// stepHandlerName describes who a step hands the issue to, for briefs.
func (h *Handler) stepHandlerName(ctx context.Context, issue db.Issue, project db.Project, step issueworkflow.Step) string {
	t, id, ok := issueworkflow.ResolveHandler(step, project, issue.CreatorType, issue.CreatorID)
	if !ok {
		switch step.Handler.Type {
		case issueworkflow.HandlerProjectLead:
			return "the project lead (none set)"
		case issueworkflow.HandlerCreator:
			return "the issue creator"
		}
		return ""
	}
	name := h.actorDisplayName(ctx, issue.WorkspaceID, t, id)
	switch step.Handler.Type {
	case issueworkflow.HandlerProjectLead:
		return name + " (project lead)"
	case issueworkflow.HandlerCreator:
		return name + " (issue creator)"
	}
	return name
}

// isSquadRequest reports whether the issue is a request to a squad: it has no
// parent and is assigned to the squad or to the leader of a live squad.
// isRunOfRequestLeader reports whether run is the agent running as the leader
// of the squad that holds the request. For a squad assignee the assignee id is
// the squad, so the run's agent must be checked against the squad's leader.
func (h *Handler) isRunOfRequestLeader(ctx context.Context, issue db.Issue, run *db.AgentTaskQueue) bool {
	if run == nil {
		return false
	}
	if issue.AssigneeType.String != "squad" {
		return run.AgentID == issue.AssigneeID
	}
	squad, err := h.Queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{
		ID:          issue.AssigneeID,
		WorkspaceID: issue.WorkspaceID,
	})
	return err == nil && squad.LeaderID == run.AgentID
}

func (h *Handler) isSquadRequest(ctx context.Context, issue db.Issue) bool {
	if issue.ParentIssueID.Valid || !issue.AssigneeID.Valid {
		return false
	}
	// A leader that opened the issue itself routes its own work through the
	// steps; only a request given to the leader is held by it.
	if issue.CreatorType == "agent" && uuidToString(issue.CreatorID) == uuidToString(issue.AssigneeID) {
		return false
	}
	switch issue.AssigneeType.String {
	case "squad":
		return true
	case "agent":
		leads, err := h.Queries.IsSquadLeaderInWorkspace(ctx, db.IsSquadLeaderInWorkspaceParams{
			WorkspaceID: issue.WorkspaceID,
			LeaderID:    issue.AssigneeID,
		})
		return err == nil && leads
	}
	return false
}
