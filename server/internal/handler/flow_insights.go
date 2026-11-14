package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// flowInsightsMaxRequests is how many of the newest requests of a version the numbers are
// computed from (spec 16.6).
const flowInsightsMaxRequests = 500

// workflowManagedBy reports whether the member manages the workflow: the admin of a workflow no
// squad owns, otherwise the member who manages its squad.
func (h *Handler) workflowManagedBy(ctx context.Context, member db.Member, wsUUID pgtype.UUID, row db.IssueWorkflow) bool {
	if !row.SquadID.Valid {
		return isWorkspaceAdmin(member)
	}
	squad, err := h.Queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: row.SquadID, WorkspaceID: wsUUID})
	return err == nil && canManageSquad(member, squad)
}

// GetFlowInsights answers with the operating numbers of one version of a flow: where its
// newest 500 requests ended, the steps and edges they went through, the rounds of its loops and
// why they stalled. It reads, and only a member who manages the squad of the version may ask.
func (h *Handler) GetFlowInsights(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !featureflags.FlowAuthoringEnabled(ctx, h.FeatureFlags) {
		writeErrorCode(w, http.StatusNotFound, "flow_authoring_disabled", "flow authoring is not enabled")
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}
	member, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found")
	if !ok {
		return
	}
	if h.refuseFlowAgent(w, r, member, workspaceID, "flow_forbidden", "only the members who manage the squad can read the insights of a workflow") {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workflow id")
	if !ok {
		return
	}
	includeTrial := false
	if raw := r.URL.Query().Get("include_trial"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "include_trial must be true or false")
			return
		}
		includeTrial = parsed
	}
	row, err := h.Queries.GetIssueWorkflow(ctx, db.GetIssueWorkflowParams{ID: id, WorkspaceID: wsUUID})
	if err != nil || len(row.FlowConfig) == 0 {
		writeErrorCode(w, http.StatusNotFound, "workflow_not_found", "flow workflow not found")
		return
	}
	if !h.workflowManagedBy(ctx, member, wsUUID, row) {
		writeErrorCode(w, http.StatusForbidden, "flow_forbidden", "only the members who manage the squad can read the insights of a workflow")
		return
	}
	def, err := issueworkflow.Decode(row)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}

	// One more than is shown tells whether the list was cut.
	requests, err := h.Queries.ListFlowInsightRequests(ctx, db.ListFlowInsightRequestsParams{
		WorkspaceID: wsUUID, WorkflowID: uuidToString(id), IncludeTrial: includeTrial, PageLimit: flowInsightsMaxRequests + 1,
	})
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	truncated := len(requests) > flowInsightsMaxRequests
	if truncated {
		requests = requests[:flowInsightsMaxRequests]
	}
	states := make([]issueworkflow.State, 0, len(requests))
	ids := make([]pgtype.UUID, 0, len(requests))
	for _, req := range requests {
		state, err := decodeFlowState(req.Flow)
		if err != nil {
			continue
		}
		states = append(states, state)
		ids = append(ids, req.ID)
	}
	var closes []issueworkflow.ManualClose
	if len(ids) > 0 {
		rows, err := h.Queries.ListFlowManualCloses(ctx, db.ListFlowManualClosesParams{WorkspaceID: wsUUID, RequestIds: ids})
		if err != nil {
			writeFlowError(w, r, err)
			return
		}
		for _, c := range rows {
			closes = append(closes, issueworkflow.ManualClose{Step: c.Step, Task: c.Task})
		}
	}
	report := issueworkflow.Insights(states, closes).WithLoopLimits(def.Graph.Doc.Loops)
	report.Truncated = truncated
	writeJSON(w, http.StatusOK, report)
}
