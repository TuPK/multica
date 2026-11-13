package handler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
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
	if err != nil || run.state.ClosedAt != "" || run.state.Stop != issueworkflow.StopNone {
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
	since := now
	if t, err := time.Parse(time.RFC3339, run.state.EnteredAt); err == nil {
		since = t
	} else {
		since = run.issue.CreatedAt.Time
	}
	limits := issueworkflow.LimitsOf(run.def.Graph.Doc.Policy)
	log := slog.With("request_id", uuidToString(run.issue.ID), "step", run.state.Step)

	if run.state.Step == issueworkflow.StartStatus {
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
		if r.state.Step != run.state.Step || r.state.Round != run.state.Round() {
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
