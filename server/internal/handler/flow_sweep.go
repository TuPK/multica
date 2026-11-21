package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/projectaccess"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

// The sweep is the runner's safety net (spec 5.2 point 2 and 5.6): every pass it starts the requests
// that missed the engine, advances the requests whose event was missed, and acts on what is stuck: a
// task nobody works on, a run that never ends, a lead that never plans. It keeps no state between
// passes except where the last page ended, so a restart only costs fairness.

// flowSweepPage is the number of requests one pass reads.
const flowSweepPage = 200

// The brief of a retried run. A task is closed with the commands in its description; the lead of a
// request has no task to close: it plans, or reports the missing grounds.
const (
	flowRetryNote     = "An earlier run on this task ended without closing it. Read the task again, take a different approach, and close it with the commands in its description."
	flowRetryLeadNote = "An earlier run on this request ended without a plan. Read the request again, post your plan, then run `multica issue flow plan <id>`. If the grounds are missing, run it with `--blocked`, `--missing` and `--reason` instead."
)

var flowSweepCursor struct {
	sync.Mutex
	workspace, id pgtype.UUID
}

func zeroUUID() pgtype.UUID { return pgtype.UUID{Valid: true} }

// SweepFlowRequests is one pass of the flow_sweep job. It returns how many requests it changed.
func (h *Handler) SweepFlowRequests(ctx context.Context) (int, error) {
	if !h.flowRunnerEnabled(ctx) {
		return 0, nil
	}
	changed := 0
	unstarted, err := h.Queries.ListUnstartedFlowRequests(ctx, 50)
	if err != nil {
		return 0, fmt.Errorf("list unstarted flow requests: %w", err)
	}
	for _, issue := range unstarted {
		started, err := h.EnsureFlowStarted(ctx, issue)
		if err != nil {
			slog.Warn("flow sweep: start request", "request_id", uuidToString(issue.ID), "error", err)
			continue
		}
		if len(started.Flow) > 0 {
			changed++
		}
	}

	flowSweepCursor.Lock()
	after := db.ListAutoFlowRequestsParams{AfterWorkspaceID: flowSweepCursor.workspace, AfterID: flowSweepCursor.id, PageLimit: flowSweepPage}
	flowSweepCursor.Unlock()
	if !after.AfterWorkspaceID.Valid {
		after.AfterWorkspaceID, after.AfterID = zeroUUID(), zeroUUID()
	}
	page, err := h.Queries.ListAutoFlowRequests(ctx, after)
	if err != nil {
		return changed, fmt.Errorf("list auto flow requests: %w", err)
	}
	for _, request := range page {
		if ctx.Err() != nil {
			break
		}
		flowSweepCursor.Lock()
		flowSweepCursor.workspace, flowSweepCursor.id = request.WorkspaceID, request.ID
		flowSweepCursor.Unlock()
		if h.sweepFlowRequest(ctx, request) {
			changed++
		}
	}
	if ctx.Err() == nil && len(page) < flowSweepPage {
		flowSweepCursor.Lock()
		flowSweepCursor.workspace, flowSweepCursor.id = pgtype.UUID{}, pgtype.UUID{}
		flowSweepCursor.Unlock()
	}
	manual, err := h.sweepManualHumanTasks(ctx)
	if err != nil {
		return changed, err
	}
	return changed + manual, nil
}

// flowManualSweepCursor pages the open requests whose runner is off, for the reminders and stalls of
// the human tasks in them (spec 4.9). The runner never moves these requests.
var flowManualSweepCursor struct {
	sync.Mutex
	workspace, id pgtype.UUID
}

// sweepManualHumanTasks looks after the human tasks of the open requests the runner does not move: a
// reminder once a person has waited human_remind_minutes, a stall once the wait is human_stall_minutes.
// Agent tasks and the advance of the request are left to the runner.
func (h *Handler) sweepManualHumanTasks(ctx context.Context) (int, error) {
	flowManualSweepCursor.Lock()
	after := db.ListOpenFlowRequestsParams{AfterWorkspaceID: flowManualSweepCursor.workspace, AfterID: flowManualSweepCursor.id, PageLimit: flowSweepPage}
	flowManualSweepCursor.Unlock()
	if !after.AfterWorkspaceID.Valid {
		after.AfterWorkspaceID, after.AfterID = zeroUUID(), zeroUUID()
	}
	page, err := h.Queries.ListOpenFlowRequests(ctx, after)
	if err != nil {
		return 0, fmt.Errorf("list open flow requests: %w", err)
	}
	changed := 0
	for _, request := range page {
		if ctx.Err() != nil {
			break
		}
		flowManualSweepCursor.Lock()
		flowManualSweepCursor.workspace, flowManualSweepCursor.id = request.WorkspaceID, request.ID
		flowManualSweepCursor.Unlock()
		if h.isAutoFlowRequest(ctx, request.ID) {
			continue
		}
		run, err := h.loadFlowRun(ctx, h.Queries, request)
		if err != nil || run.state.ClosedAt != "" || run.state.Stop != nil {
			continue
		}
		rows, err := h.loadFlowTasks(ctx, h.Queries, run)
		if err != nil {
			slog.Warn("flow sweep: load human tasks", "request_id", uuidToString(request.ID), "error", err)
			continue
		}
		now := time.Now()
		for _, r := range rows {
			if r.state.Step != run.state.Step || r.state.Round != run.round() || r.issue.AssigneeType.String == "agent" {
				continue
			}
			if h.sweepHumanTask(ctx, run, r, now) {
				changed++
			}
		}
	}
	if ctx.Err() == nil && len(page) < flowSweepPage {
		flowManualSweepCursor.Lock()
		flowManualSweepCursor.workspace, flowManualSweepCursor.id = pgtype.UUID{}, pgtype.UUID{}
		flowManualSweepCursor.Unlock()
	}
	return changed, nil
}

// sweepFlowRequest advances one request and then looks for what is stuck in its current step. It
// reports whether it changed anything.
func (h *Handler) sweepFlowRequest(ctx context.Context, request db.Issue) bool {
	resp, err := h.AdvanceFlow(ctx, request)
	if err != nil {
		slog.Warn("flow sweep: advance failed", "request_id", uuidToString(request.ID), "error", err)
		return false
	}
	changed := false
	for _, a := range resp.Actions {
		if a.Kind != "noop" {
			changed = true
			slog.Info("flow sweep: advanced", "request_id", uuidToString(request.ID), "step", resp.State.Step, "action", a.Kind)
		}
	}
	current, err := h.Queries.GetIssue(ctx, request.ID)
	if err != nil {
		return changed
	}
	run, err := h.loadFlowRun(ctx, h.Queries, current)
	if err != nil || run.state.ClosedAt != "" || run.state.Stop != nil {
		return changed
	}
	if h.sweepStuck(ctx, run) {
		changed = true
	}
	return changed
}

// flowRunStats reads the runs of an issue since the step began.
func (h *Handler) flowRunStats(ctx context.Context, issueID pgtype.UUID, since time.Time, now time.Time) (issueworkflow.RunStats, error) {
	row, err := h.Queries.FlowRunStats(ctx, db.FlowRunStatsParams{IssueID: issueID, Since: pgtype.Timestamptz{Time: since, Valid: true}})
	if err != nil {
		return issueworkflow.RunStats{}, err
	}
	stats := issueworkflow.RunStats{Attempts: int(row.Attempts), Active: int(row.Active)}
	if row.RunningSince.Valid {
		stats.RunningFor = now.Sub(row.RunningSince.Time)
	}
	stats.IdleFor = now.Sub(since)
	if row.LastActivity.Valid && row.LastActivity.Time.After(since) {
		stats.IdleFor = now.Sub(row.LastActivity.Time)
	}
	return stats, nil
}

// sweepStuck acts on the plan step (a lead that has not planned) or on the open tasks of the current
// step. It reports whether it changed anything.
func (h *Handler) sweepStuck(ctx context.Context, run *flowRun) bool {
	now := time.Now()
	since := stepEnteredAt(run)
	limits := issueworkflow.LimitsOf(run.def.Graph.Doc.Policy)
	log := slog.With("request_id", uuidToString(run.issue.ID), "step", run.state.Step)

	if step, ok := run.def.Graph.Step(run.state.Step); ok && step.Kind == issueworkflow.StepKindLead {
		// The lead has twice the time of a task, then it is woken again, then the request stalls.
		leadLimits := limits
		leadLimits.Idle *= 2
		stats, err := h.flowRunStats(ctx, run.issue.ID, since, now)
		if err != nil {
			log.Warn("flow sweep: lead run stats", "error", err)
			return false
		}
		return h.applyDeadline(ctx, run, run.issue, "", issueworkflow.DecideDeadline(leadLimits, stats), "The lead did not give a plan and used all its attempts.", log)
	}

	rows, err := h.loadFlowTasks(ctx, h.Queries, run)
	if err != nil {
		log.Warn("flow sweep: load tasks", "error", err)
		return false
	}
	changed := false
	for _, r := range rows {
		if r.state.Step != run.state.Step || r.state.Round != run.round() {
			continue
		}
		if r.issue.AssigneeType.String != "agent" {
			// A person holds the task (or any manager may close it): no run to retry.
			if h.sweepHumanTask(ctx, run, r, now) {
				changed = true
			}
			continue
		}
		stats, err := h.flowRunStats(ctx, r.issue.ID, since, now)
		if err != nil {
			log.Warn("flow sweep: task run stats", "task", r.state.Key, "error", err)
			continue
		}
		decision := issueworkflow.DecideDeadline(limits, stats)
		if r.state.Closed {
			// A closed task whose run never ends holds the step; the hard cap releases it.
			if decision.Kind != issueworkflow.DeadlineCancelRun {
				continue
			}
			decision.Stall = false
		}
		reason := fmt.Sprintf("The task %q (%s) was tried %d times without an agent closing it.", r.state.Key, r.issue.Title, limits.MaxAttempts)
		if h.applyDeadline(ctx, run, r.issue, r.state.Key, decision, reason, log.With("task", r.state.Key)) {
			changed = true
		}
		if decision.Kind == issueworkflow.DeadlineStall || decision.Stall {
			break
		}
	}
	return changed
}

// stepEnteredAt is when the request entered its current step: the moment its runs are counted from.
func stepEnteredAt(run *flowRun) time.Time {
	if t, err := time.Parse(time.RFC3339, run.state.EnteredAt); err == nil {
		return t
	}
	return run.issue.CreatedAt.Time
}

// rescueFlowTasks is what an explicit advance does for the agent tasks of the current step when the
// engine itself had nothing to do. The sweep does this for a flow the runner moves, after the task has
// been idle; a request that a lead moves has no sweep for its tasks, so a task whose runs all ended
// (the daemon restarted under them, say) would sit in todo with no run and no stop for ever.
//
// A task with no live run gets one while it has attempts left. A task that used all its attempts stops the
// request with flow_task_stuck, so the answer carries a stop instead of a noop. A task with a live run is
// left alone: the sweep owns the hard cap. The advance is the person's or the lead's ask to move on, so
// the idle wait of the sweep does not apply.
func (h *Handler) rescueFlowTasks(ctx context.Context, request db.Issue, scope projectaccess.Hidden) ([]FlowAction, error) {
	current, err := h.Queries.GetIssue(ctx, request.ID)
	if err != nil {
		return nil, err
	}
	run, err := h.loadFlowRun(ctx, h.Queries, current)
	if err != nil || run.state.ClosedAt != "" || run.state.Stop != nil {
		return nil, nil
	}
	if step, ok := run.def.Graph.Step(run.state.Step); !ok || step.Kind == issueworkflow.StepKindLead {
		return nil, nil
	}
	rows, err := h.loadFlowTasks(ctx, h.Queries, run)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	since := stepEnteredAt(run)
	limits := issueworkflow.LimitsOf(run.def.Graph.Doc.Policy)
	log := slog.With("request_id", uuidToString(run.issue.ID), "step", run.state.Step)
	var given []string
	for _, r := range rows {
		if r.state.Step != run.state.Step || r.state.Round != run.round() || r.state.Closed || r.issue.AssigneeType.String != "agent" {
			continue
		}
		// A task in a project the viewer cannot see is neither given a run nor named in the answer.
		if !scope.CanSeeProject(r.issue.ProjectID) {
			continue
		}
		stats, err := h.flowRunStats(ctx, r.issue.ID, since, now)
		if err != nil {
			return nil, err
		}
		if stats.Active > 0 {
			continue
		}
		if stats.Attempts >= limits.MaxAttempts {
			reason := fmt.Sprintf("The task %q (%s) was tried %d times without an agent closing it.", r.state.Key, r.issue.Title, limits.MaxAttempts)
			out, err := h.stallFlow(ctx, run, issueworkflow.ReasonTaskStuck, reason, r.state.Key)
			if err != nil {
				return nil, err
			}
			if out.action.Kind != "stopped" {
				return nil, nil
			}
			out.action.Tasks = []string{r.state.Key}
			out.action.Code = "flow_task_stuck"
			return []FlowAction{out.action}, nil
		}
		if err := h.enqueueFlowRun(ctx, r.issue); err != nil {
			log.Warn("flow advance: give the task a run", "task", r.state.Key, "error", err)
			continue
		}
		given = append(given, r.state.Key)
	}
	if len(given) == 0 {
		return nil, nil
	}
	return []FlowAction{{Kind: "redispatched_tasks", Step: run.state.Step, Tasks: given}}, nil
}

// applyDeadline does what DecideDeadline chose for an issue (the request at the plan step, or a task).
func (h *Handler) applyDeadline(ctx context.Context, run *flowRun, issue db.Issue, taskKey string, d issueworkflow.Deadline, reason string, log *slog.Logger) bool {
	switch d.Kind {
	case issueworkflow.DeadlineEnqueue:
		if err := h.enqueueFlowRun(ctx, issue); err != nil {
			log.Warn("flow sweep: enqueue run", "error", err)
			return false
		}
		log.Info("flow sweep: gave the task a run")
		return true
	case issueworkflow.DeadlineCancelRun:
		active, err := h.Queries.ListActiveTasksByIssue(ctx, issue.ID)
		if err != nil {
			log.Warn("flow sweep: list runs", "error", err)
			return false
		}
		for _, task := range active {
			if _, err := h.TaskService.CancelTask(ctx, task.ID); err != nil {
				log.Warn("flow sweep: cancel run", "run_id", uuidToString(task.ID), "error", err)
			}
		}
		log.Info("flow sweep: cancelled a run past the hard cap")
		if d.Stall {
			if out, err := h.stallFlow(ctx, run, issueworkflow.ReasonTaskStuck, reason, taskKey); err != nil {
				log.Warn("flow sweep: stall", "error", err)
			} else {
				return out.action.Kind == "stopped" || len(active) > 0
			}
		}
		return len(active) > 0
	case issueworkflow.DeadlineStall:
		out, err := h.stallFlow(ctx, run, issueworkflow.ReasonTaskStuck, reason, taskKey)
		if err != nil {
			log.Warn("flow sweep: stall", "error", err)
			return false
		}
		log.Info("flow sweep: stalled the request")
		return out.action.Kind == "stopped"
	}
	return false
}

// enqueueFlowRun gives an issue a run: for a task its agent, for the request its squad's leader.
func (h *Handler) enqueueFlowRun(ctx context.Context, issue db.Issue) error {
	if issue.AssigneeType.String == "squad" {
		squad, err := h.Queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: issue.AssigneeID, WorkspaceID: issue.WorkspaceID})
		if err != nil {
			return fmt.Errorf("load squad: %w", err)
		}
		_, err = h.TaskService.EnqueueTaskForSquadLeaderWithHandoff(ctx, issue, squad.LeaderID, squad.ID, flowRetryLeadNote, pgtype.UUID{})
		return err
	}
	_, err := h.TaskService.EnqueueTaskForIssueWithHandoff(ctx, issue, flowRetryNote, pgtype.UUID{})
	return err
}

// sweepHumanTask acts on a task that a person holds (spec 4.9): once it has waited
// human_remind_minutes the person gets one reminder; once it has waited
// human_stall_minutes the request stalls. It reports whether it changed anything.
func (h *Handler) sweepHumanTask(ctx context.Context, run *flowRun, r flowTaskRow, now time.Time) bool {
	if r.state.Closed {
		return false
	}
	policy := run.def.Graph.Doc.Policy.Effective()
	remind, stall := policy.HumanRemindMinutes, policy.HumanStallMinutes
	if doc, ok := flowGraphTask(run.def.Graph, r.state.Step, r.state.Key); ok && doc.Deadline != nil {
		if doc.Deadline.RemindAfterMinutes > 0 {
			remind = doc.Deadline.RemindAfterMinutes
		}
		if doc.Deadline.StallAfterMinutes > 0 {
			stall = doc.Deadline.StallAfterMinutes
		}
	}
	waited := now.Sub(r.issue.CreatedAt.Time)
	log := slog.With("request_id", uuidToString(run.issue.ID), "task", r.state.Key)
	switch {
	case waited >= time.Duration(stall)*time.Minute:
		reason := fmt.Sprintf("The task %q (%s) waited %d minutes for a person and nobody closed it.", r.state.Key, r.issue.Title, stall)
		out, err := h.stallFlow(ctx, run, issueworkflow.ReasonTaskStuck, reason, r.state.Key)
		if err != nil {
			log.Warn("flow sweep: stall a task nobody closed", "error", err)
			return false
		}
		return out.action.Kind == "stopped"
	case waited >= time.Duration(remind)*time.Minute:
		sent, err := h.Queries.CountFlowActivity(ctx, db.CountFlowActivityParams{IssueID: r.issue.ID, Action: "flow_human_reminded"})
		if err != nil || sent > 0 {
			return false
		}
		return h.remindHumanTask(ctx, run, r, log)
	}
	return false
}

// remindHumanTask posts the reminder on the task, mentioning the member it is given
// to, or else the member who made the request, and logs it so it is sent once.
func (h *Handler) remindHumanTask(ctx context.Context, run *flowRun, r flowTaskRow, log *slog.Logger) bool {
	mention := h.requesterMention(ctx, run.issue)
	if r.issue.AssigneeType.String == "member" && r.issue.AssigneeID.Valid {
		if user, err := h.Queries.GetUser(ctx, r.issue.AssigneeID); err == nil {
			mention = fmt.Sprintf("[@%s](mention://member/%s)", user.Name, uuidToString(r.issue.AssigneeID))
		}
	}
	created, err := h.Queries.CreateComment(ctx, db.CreateCommentParams{
		ID: dbid.NewV7(), IssueID: r.issue.ID, WorkspaceID: r.issue.WorkspaceID,
		AuthorType: "system", AuthorID: pgtype.UUID{Valid: true},
		Content: strings.TrimSpace(mention + " This task of the workflow waits for you: close it, or the request stalls."), Type: "system",
	})
	if err != nil {
		log.Warn("flow sweep: remind a person", "error", err)
		return false
	}
	if _, err := h.Queries.CreateActivity(ctx, db.CreateActivityParams{
		ID: dbid.NewV7(), WorkspaceID: r.issue.WorkspaceID, IssueID: r.issue.ID,
		ActorType: pgtype.Text{String: "system", Valid: true}, Action: "flow_human_reminded", Details: []byte("{}"),
	}); err != nil {
		log.Warn("flow sweep: log a reminder", "error", err)
	}
	h.publishFlowEffects(ctx, run.issue, &flowEffects{comments: []db.CreateCommentRow{created}})
	return true
}
