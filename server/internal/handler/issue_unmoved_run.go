package handler

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// unmovedRunBrief is the note the assignee gets when its run ended and the issue
// did not move. It tells the agent to change approach, not to stop.
const unmovedRunBrief = "Your previous run ended, but this issue is still in progress and nothing on it moved. " +
	"Do not stop, and do not wait for anyone to choose the next step. Read the acceptance criteria and the " +
	"comments of your earlier runs to see which approaches you already tried. Choose an approach you have not " +
	"tried, as the SDLC development skill describes under \"When a check fails or gives no evidence\", and keep " +
	"going until each criterion passes on evidence or a check proves it false."

// reconcileUnmovedWorkflowRun re-runs the step's handler when its run completed
// without moving the issue. The step's move is the only signal that work
// advanced, so a run that left the issue untouched is run again with a brief to
// try a different approach. Only an agent that is the handler of the issue's
// current workflow step qualifies; a plain issue may legitimately stay in progress.
func (h *Handler) reconcileUnmovedWorkflowRun(ctx context.Context, task *db.AgentTaskQueue) {
	if task == nil || !task.IssueID.Valid || !task.AgentID.Valid {
		return
	}
	issue, err := h.Queries.GetIssue(ctx, task.IssueID)
	if err != nil {
		slog.Warn("unmoved run: load issue failed", "issue_id", util.UUIDToString(task.IssueID), "error", err)
		return
	}
	if issue.Status != "in_progress" || issue.AssigneeType.String != "agent" || issue.AssigneeID != task.AgentID {
		return
	}
	// The run started in a different step than the issue is in now: the run moved it.
	if task.WorkflowStep.Valid && task.WorkflowStep.String != "" && task.WorkflowStep.String != issue.Status {
		return
	}
	def, _, err := issueworkflow.ForProject(ctx, h.Queries, issue.WorkspaceID, issue.ProjectID)
	if err != nil {
		slog.Warn("unmoved run: load workflow failed", "issue_id", util.UUIDToString(issue.ID), "error", err)
		return
	}
	// The step's handler is the one who must move the issue; re-run only that agent.
	if def == nil {
		return
	}
	step, ok := def.Step(issue.Status)
	if !ok || step.Handler.Type != issueworkflow.HandlerAgent || step.Handler.ID != util.UUIDToString(task.AgentID) {
		return
	}
	// A squad request and a parent waiting on sub-issues are advanced by their own
	// flows; re-running their assignee here would repeat work that is not stalled.
	if h.isSquadRequest(ctx, issue) {
		return
	}
	children, err := h.Queries.ListChildIssues(ctx, issue.ID)
	if err != nil || len(children) > 0 {
		return
	}
	if _, err := h.TaskService.EnqueueTaskForIssueWithHandoff(ctx, issue, unmovedRunBrief, pgtype.UUID{}); err != nil {
		slog.Warn("unmoved run: re-run enqueue failed", "issue_id", util.UUIDToString(issue.ID), "error", err)
		return
	}
	slog.Info("unmoved run: assignee re-run with a different-approach brief", "issue_id", util.UUIDToString(issue.ID), "agent_id", util.UUIDToString(task.AgentID))
}
