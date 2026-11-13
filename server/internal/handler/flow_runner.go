package handler

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The flow runner (tier 2, docs/engineering/kitviet-workflow-spec.md section 5) has no logic of its
// own: it calls the same AdvanceFlow the lead calls, when something happens that the lead need not
// be woken for. It runs only while FF_FLOW_RUNNER is on and the flow workflow has flow_auto set.

func (h *Handler) flowRunnerEnabled(ctx context.Context) bool {
	return h.flowEngineEnabled(ctx) && featureflags.FlowRunnerEnabled(ctx, h.FeatureFlags)
}

// isAutoFlowRequest reports whether the issue is an open flow request the runner moves.
func (h *Handler) isAutoFlowRequest(ctx context.Context, issueID pgtype.UUID) bool {
	if !h.flowRunnerEnabled(ctx) {
		return false
	}
	auto, err := h.Queries.IsFlowRequestAuto(ctx, issueID)
	return err == nil && auto
}

// advanceFlowRequests advances each of these issues that is an auto flow request. A failure is only
// logged: the sweep (flow_sweep) runs the request again.
func (h *Handler) advanceFlowRequests(ctx context.Context, requestIDs ...pgtype.UUID) {
	for _, id := range requestIDs {
		if !h.isAutoFlowRequest(ctx, id) {
			continue
		}
		request, err := h.Queries.GetIssue(ctx, id)
		if err != nil {
			slog.Warn("flow runner: load request", "request_id", uuidToString(id), "error", err)
			continue
		}
		h.advanceRequestLogged(ctx, request, "event")
	}
}

// advanceRequestLogged runs AdvanceFlow and writes one structured line of what it did.
func (h *Handler) advanceRequestLogged(ctx context.Context, request db.Issue, source string) bool {
	resp, err := h.AdvanceFlow(ctx, request)
	if err != nil {
		slog.Warn("flow runner: advance failed", "request_id", uuidToString(request.ID), "source", source, "error", err)
		return false
	}
	for _, a := range resp.Actions {
		if a.Kind == "noop" {
			continue
		}
		slog.Info("flow runner: advanced", "request_id", uuidToString(request.ID), "step", resp.State.Step, "action", a.Kind, "source", source)
	}
	return true
}

// advanceFlowAfterRun advances the request of a flow task whose run just ended, so the next step
// does not wait for the next sweep.
func (h *Handler) advanceFlowAfterRun(ctx context.Context, task *db.AgentTaskQueue) {
	if task == nil || !task.IssueID.Valid || !h.flowRunnerEnabled(ctx) {
		return
	}
	issue, err := h.Queries.GetIssue(ctx, task.IssueID)
	if err != nil || !issue.FlowTask.Valid || !issue.ParentIssueID.Valid {
		return
	}
	h.advanceFlowRequests(ctx, issue.ParentIssueID)
}
