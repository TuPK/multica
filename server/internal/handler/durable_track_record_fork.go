package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Track records and verification modes over HTTP (durable spec 3.5). Reads are for
// members and agents. Only an owner or an admin, as a person, pins a mode.

// trackRecordPageMax bounds one track record listing.
const trackRecordPageMax int32 = 200

// TrackRecordResponse is one window of an agent's track record for one work class.
type TrackRecordResponse struct {
	AgentID          string  `json:"agent_id"`
	WorkClass        string  `json:"work_class"`
	WindowDays       int32   `json:"window_days"`
	NPass            int32   `json:"n_pass"`
	NPartial         int32   `json:"n_partial"`
	NFail            int32   `json:"n_fail"`
	NUnverified      int32   `json:"n_unverified"`
	NHighFail        int32   `json:"n_high_fail"`
	ScoreLower       float64 `json:"score_lower"`
	LastHighFailAt   *string `json:"last_high_fail_at"`
	VerificationMode string  `json:"verification_mode"`
	ModePinned       bool    `json:"mode_pinned"`
	ModeChangedAt    *string `json:"mode_changed_at"`
	ComputedAt       string  `json:"computed_at"`
}

// TrackRecordList is one listing of track record rows.
type TrackRecordList struct {
	TrackRecords []TrackRecordResponse `json:"track_records"`
}

// PinTrackRecordRequest pins a verification mode for an agent's work class. A null mode
// unpins it.
type PinTrackRecordRequest struct {
	AgentID   string  `json:"agent_id"`
	WorkClass string  `json:"work_class"`
	Mode      *string `json:"mode"`
}

func trackRecordResponse(r db.TrackRecord) TrackRecordResponse {
	return TrackRecordResponse{
		AgentID:          util.UUIDToString(r.AgentID),
		WorkClass:        r.WorkClass,
		WindowDays:       r.WindowDays,
		NPass:            r.NPass,
		NPartial:         r.NPartial,
		NFail:            r.NFail,
		NUnverified:      r.NUnverified,
		NHighFail:        r.NHighFail,
		ScoreLower:       r.ScoreLower,
		LastHighFailAt:   timestampPtr(r.LastHighFailAt),
		VerificationMode: r.VerificationMode,
		ModePinned:       r.ModePinned,
		ModeChangedAt:    timestampPtr(r.ModeChangedAt),
		ComputedAt:       timestampToString(r.ComputedAt),
	}
}

func (h *Handler) trackRecordService() *service.TrackRecordService {
	return &service.TrackRecordService{Queries: h.Queries, TxStarter: h.TxStarter}
}

// ListTrackRecords lists track record rows, filtered by agent, work class and window.
func (h *Handler) ListTrackRecords(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.outcomeCallerFor(w, r, workspaceID); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	params := db.ListTrackRecordsParams{WorkspaceID: wsUUID, RowLimit: trackRecordPageMax}
	query := r.URL.Query()
	if raw := query.Get("agent_id"); raw != "" {
		agentID, ok := parseUUIDOrBadRequest(w, raw, "agent_id")
		if !ok {
			return
		}
		params.AgentID = agentID
	}
	if raw := query.Get("work_class"); raw != "" {
		if len(raw) > 200 {
			writeError(w, http.StatusBadRequest, "work_class is longer than 200 characters")
			return
		}
		params.WorkClass = pgtype.Text{String: raw, Valid: true}
	}
	if raw := query.Get("window_days"); raw != "" {
		days, err := strconv.Atoi(raw)
		if err != nil || (days != 30 && days != 90) {
			writeError(w, http.StatusBadRequest, "window_days must be 30 or 90")
			return
		}
		params.WindowDays = pgtype.Int4{Int32: int32(days), Valid: true}
	}

	rows, err := h.Queries.ListTrackRecords(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list track records")
		return
	}
	out := make([]TrackRecordResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, trackRecordResponse(row))
	}
	writeJSON(w, http.StatusOK, TrackRecordList{TrackRecords: out})
}

// PinTrackRecord pins or unpins the verification mode of one agent's work class. Only an
// owner or an admin, as a person, may do it, so an agent gets 403.
func (h *Handler) PinTrackRecord(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	caller, ok := h.outcomeCallerFor(w, r, workspaceID)
	if !ok {
		return
	}
	if caller.agent || !caller.admin {
		writeErrorCode(w, http.StatusForbidden, "forbidden", "only the owner or an admin can pin a verification mode")
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	var req PinTrackRecordRequest
	if !decodeInitiativeBody(w, r, &req) {
		return
	}
	agentID, ok := parseUUIDOrBadRequest(w, req.AgentID, "agent_id")
	if !ok {
		return
	}
	workClass := strings.TrimSpace(req.WorkClass)
	if workClass == "" || len(workClass) > 200 {
		writeError(w, http.StatusBadRequest, "work_class must be 1 to 200 characters")
		return
	}

	row, err := h.trackRecordService().Pin(r.Context(), wsUUID, caller.actor, agentID, workClass, req.Mode)
	if err != nil {
		writeOutcomeFailure(w, r, "pin verification mode", err)
		return
	}
	writeJSON(w, http.StatusOK, trackRecordResponse(row))
}
