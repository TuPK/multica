package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/service"
)

// shadowListLimit is how many runs a seat's list returns, newest first.
const shadowListLimit = 20

// ShadowRunReportResponse is a run's report (durable spec 9.4). NotLosingLower is the Wilson
// lower bound of (shadow + tie) / pairs.
type ShadowRunReportResponse struct {
	Pairs                int     `json:"pairs"`
	Shadow               int     `json:"shadow"`
	Tie                  int     `json:"tie"`
	Primary              int     `json:"primary"`
	Unable               int     `json:"unable"`
	NotLosingLower       float64 `json:"not_losing_lower"`
	PrimaryCostAvgMicros int64   `json:"primary_cost_avg_micros"`
	ShadowCostAvgMicros  int64   `json:"shadow_cost_avg_micros"`
}

// ShadowRunListItem is one run in a seat's list, with its report.
type ShadowRunListItem struct {
	ShadowRunResponse
	Purpose         string                  `json:"purpose"`
	WorkClassPrefix string                  `json:"work_class_prefix"`
	CreatedAt       string                  `json:"created_at"`
	CompletedAt     *string                 `json:"completed_at"`
	Report          ShadowRunReportResponse `json:"report"`
}

// ListSeatShadowRuns returns a seat's recent shadow runs with their reports
// (GET /api/agents/{id}/shadow). Only a member reads them: a run's cost and verdicts are the
// person's to see, not the seat's.
func (h *Handler) ListSeatShadowRuns(w http.ResponseWriter, r *http.Request) {
	ws, actor, ok := h.seatDossierCaller(w, r)
	if !ok {
		return
	}
	if actor.Type != service.GovernanceActorMember {
		writeError(w, http.StatusForbidden, "only a member can read shadow runs")
		return
	}
	seat, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "agent id")
	if !ok {
		return
	}
	views, err := h.shadowService().ListSeatShadowRuns(r.Context(), ws, seat, shadowListLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list shadow runs")
		return
	}
	out := make([]ShadowRunListItem, 0, len(views))
	for _, v := range views {
		out = append(out, ShadowRunListItem{
			ShadowRunResponse: shadowRunResponse(v.Config),
			Purpose:           v.Config.Purpose,
			WorkClassPrefix:   v.Config.WorkClassPrefix,
			CreatedAt:         timestampToString(v.Config.CreatedAt),
			CompletedAt:       timestampPtr(v.Config.CompletedAt),
			Report: ShadowRunReportResponse{
				Pairs: v.Report.Pairs, Shadow: v.Report.Shadow, Tie: v.Report.Tie,
				Primary: v.Report.Primary, Unable: v.Report.Unable,
				NotLosingLower:       v.Report.NotLosingLower,
				PrimaryCostAvgMicros: v.Report.PrimaryCostAvgMicros,
				ShadowCostAvgMicros:  v.Report.ShadowCostAvgMicros,
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": out})
}
