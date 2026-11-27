package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The Reach Map over HTTP (durable spec 11, F10). Reads are for members. Closing a gap is for
// an owner or an admin, as a person (the routes in router_fork.go refuse agents and other roles).
// Both answer 404 while durable_open is off.

// ReachSurfaceResponse is one surface of the map.
type ReachSurfaceResponse struct {
	Surface         string   `json:"surface"`
	State           string   `json:"state"`
	Effects30d      int32    `json:"effects_30d"`
	AgentEffects30d int32    `json:"agent_effects_30d"`
	HumanMinutes30d float64  `json:"human_minutes_30d"`
	GapAsks30d      int32    `json:"gap_asks_30d"`
	TopAskIDs       []string `json:"top_ask_ids"`
	GapIssueID      *string  `json:"gap_issue_id"`
}

// ReachResponse is the newest snapshot of the map, surfaces in order of human minutes. AgentShare30d
// is the share of effects the agents made in the window, in percent (spec 11.4).
type ReachResponse struct {
	SnapshotDate  *string                `json:"snapshot_date"`
	AgentShare30d float64                `json:"agent_share_30d"`
	Surfaces      []ReachSurfaceResponse `json:"surfaces"`
}

// CloseReachGapRequest asks for the gap issue of one surface. A missing squad_id picks the squad
// that has asked most about the surface.
type CloseReachGapRequest struct {
	Surface string `json:"surface"`
	SquadID string `json:"squad_id"`
}

// reachService wires the Reach Map to the handler's stores.
func (h *Handler) reachService() *service.ReachService {
	return &service.ReachService{
		Queries: h.Queries, Issues: h.IssueService, Verification: h.outcomeService(), Flags: h.FeatureFlags,
	}
}

func reachSurfaceResponse(r db.ReachSnapshot) ReachSurfaceResponse {
	out := ReachSurfaceResponse{
		Surface: r.Surface, State: r.State, Effects30d: r.Effects30d, AgentEffects30d: r.AgentEffects30d,
		GapAsks30d: r.GapAsks30d, TopAskIDs: make([]string, 0, len(r.TopAskIds)),
	}
	if f, err := r.HumanMinutes30d.Float64Value(); err == nil && f.Valid {
		out.HumanMinutes30d = f.Float64
	}
	for _, id := range r.TopAskIds {
		out.TopAskIDs = append(out.TopAskIDs, util.UUIDToString(id))
	}
	if r.GapIssueID.Valid {
		id := util.UUIDToString(r.GapIssueID)
		out.GapIssueID = &id
	}
	return out
}

// GetReachMap returns the newest snapshot of the workspace's Reach Map. With no snapshot yet it
// returns an empty map.
func (h *Handler) GetReachMap(w http.ResponseWriter, r *http.Request) {
	if !featureflags.DurableOpenEnabled(r.Context(), h.FeatureFlags) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace_id")
	if !ok {
		return
	}
	day, rows, found, err := h.reachService().Latest(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read the reach map")
		return
	}
	out := ReachResponse{Surfaces: make([]ReachSurfaceResponse, 0, len(rows))}
	var effects, agent int64
	for _, row := range rows {
		effects += int64(row.Effects30d)
		agent += int64(row.AgentEffects30d)
		out.Surfaces = append(out.Surfaces, reachSurfaceResponse(row))
	}
	if effects > 0 {
		out.AgentShare30d = float64(agent) * 100 / float64(effects)
	}
	if found {
		date := day.Format(time.DateOnly)
		out.SnapshotDate = &date
	}
	writeJSON(w, http.StatusOK, out)
}

// CloseReachGap opens the gap issue of a surface, or returns the one already open.
func (h *Handler) CloseReachGap(w http.ResponseWriter, r *http.Request) {
	if !featureflags.DurableOpenEnabled(r.Context(), h.FeatureFlags) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	actor, ok := h.companyActorFor(w, r, workspaceID)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	var req CloseReachGapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	squad := pgtype.UUID{}
	if strings.TrimSpace(req.SquadID) != "" {
		squad, ok = parseUUIDOrBadRequest(w, req.SquadID, "squad_id")
		if !ok {
			return
		}
	}
	issue, err := h.reachService().CloseGap(r.Context(), wsUUID, actor, req.Surface, squad)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"issue_id": util.UUIDToString(issue.ID)})
	case errors.Is(err, service.ErrReachOff):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, service.ErrReachSurfaceInvalid):
		writeErrorCode(w, http.StatusBadRequest, "reach_surface_invalid", err.Error())
	case errors.Is(err, service.ErrReachSurfaceUnknown):
		writeErrorCode(w, http.StatusNotFound, "reach_surface_unknown", err.Error())
	case errors.Is(err, service.ErrReachNoSquad):
		writeErrorCode(w, http.StatusConflict, "reach_no_squad", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "failed to open the gap issue")
	}
}
