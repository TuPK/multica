package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
)

// The autonomy envelope over HTTP (durable spec 3.5). Reads are for members. Only an owner
// or an admin, as a person, sets or clears it, so an agent gets 403.

// SquadEnvelopeResponse is a squad's envelope.
type SquadEnvelopeResponse struct {
	SquadID          string  `json:"squad_id"`
	FloorLevel       int32   `json:"floor_level"`
	CeilingLevel     int32   `json:"ceiling_level"`
	Reason           string  `json:"reason"`
	SetBy            string  `json:"set_by"`
	LastAutoChangeAt *string `json:"last_auto_change_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// SquadReputationResponse is the reputation the envelope reads, over 90 days.
type SquadReputationResponse struct {
	N              int      `json:"n"`
	ScoreLower     float64  `json:"score_lower"`
	WindowDays     int      `json:"window_days"`
	HighFail30     bool     `json:"high_fail_30"`
	HighFail90     bool     `json:"high_fail_90"`
	AcceptanceRate *float64 `json:"acceptance_rate"`
}

// SquadEnvelopeView is the GET body: the envelope, or null, and the squad's reputation.
type SquadEnvelopeView struct {
	Envelope   *SquadEnvelopeResponse  `json:"envelope"`
	Reputation SquadReputationResponse `json:"reputation"`
}

// SetSquadEnvelopeRequest sets a squad's envelope. Every field is required.
type SetSquadEnvelopeRequest struct {
	FloorLevel   *int   `json:"floor_level"`
	CeilingLevel *int   `json:"ceiling_level"`
	Reason       string `json:"reason"`
}

func (h *Handler) envelopeService() *service.EnvelopeService {
	return &service.EnvelopeService{Queries: h.Queries, TxStarter: h.TxStarter, Flags: h.FeatureFlags}
}

func squadEnvelopeView(v service.EnvelopeView) SquadEnvelopeView {
	out := SquadEnvelopeView{Reputation: SquadReputationResponse{
		N:          v.Stats.N,
		ScoreLower: v.Stats.ScoreLower,
		WindowDays: 90,
		HighFail30: v.Stats.HighFail30,
		HighFail90: v.Stats.HighFail90,
	}}
	if v.Stats.HasAcceptance {
		rate := v.Stats.AcceptanceRate
		out.Reputation.AcceptanceRate = &rate
	}
	if e := v.Envelope; e != nil {
		out.Envelope = &SquadEnvelopeResponse{
			SquadID:          util.UUIDToString(e.SquadID),
			FloorLevel:       e.FloorLevel,
			CeilingLevel:     e.CeilingLevel,
			Reason:           e.Reason,
			SetBy:            util.UUIDToString(e.SetBy),
			LastAutoChangeAt: timestampPtr(e.LastAutoChangeAt),
			UpdatedAt:        timestampToString(e.UpdatedAt),
		}
	}
	return out
}

// GetSquadEnvelope returns a squad's envelope and reputation, for members.
func (h *Handler) GetSquadEnvelope(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	squadID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "squad id")
	if !ok {
		return
	}
	view, err := h.envelopeService().Get(r.Context(), wsUUID, squadID)
	if errors.Is(err, service.ErrAutonomySquadNotFound) {
		writeError(w, http.StatusNotFound, "squad not found")
		return
	}
	if err != nil {
		slog.Warn("read squad envelope failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to read the envelope")
		return
	}
	writeJSON(w, http.StatusOK, squadEnvelopeView(view))
}

// SetSquadEnvelope sets a squad's envelope. Owners and admins only, as a person.
func (h *Handler) SetSquadEnvelope(w http.ResponseWriter, r *http.Request) {
	if isMachineCredentialActor(r) {
		writeError(w, http.StatusForbidden, "this endpoint is only available to human actors")
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	actor, ok := h.governanceBossFor(w, r, workspaceID)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	squadID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "squad id")
	if !ok {
		return
	}
	var req SetSquadEnvelopeRequest
	if !decodeInitiativeBody(w, r, &req) {
		return
	}
	if req.FloorLevel == nil || req.CeilingLevel == nil {
		writeError(w, http.StatusBadRequest, "floor_level and ceiling_level are required")
		return
	}
	view, err := h.envelopeService().Set(r.Context(), wsUUID, squadID, *req.FloorLevel, *req.CeilingLevel, req.Reason, actor)
	switch {
	case errors.Is(err, service.ErrOutcomesOff):
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	case errors.Is(err, service.ErrEnvelopeInvalid), errors.Is(err, service.ErrAutonomyReasonRequired):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, service.ErrAutonomySquadNotFound):
		writeError(w, http.StatusNotFound, "squad not found")
		return
	case errors.Is(err, service.ErrGovernanceActorNotBoss):
		writeError(w, http.StatusForbidden, err.Error())
		return
	case err != nil:
		slog.Warn("set squad envelope failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to set the envelope")
		return
	}
	writeJSON(w, http.StatusOK, squadEnvelopeView(view))
}

// ClearSquadEnvelope removes a squad's envelope. Owners and admins only, as a person.
func (h *Handler) ClearSquadEnvelope(w http.ResponseWriter, r *http.Request) {
	if isMachineCredentialActor(r) {
		writeError(w, http.StatusForbidden, "this endpoint is only available to human actors")
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	actor, ok := h.governanceBossFor(w, r, workspaceID)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	squadID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "squad id")
	if !ok {
		return
	}
	err := h.envelopeService().Clear(r.Context(), wsUUID, squadID, actor)
	switch {
	case errors.Is(err, service.ErrOutcomesOff):
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	case errors.Is(err, service.ErrEnvelopeNotFound):
		writeError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, service.ErrGovernanceActorNotBoss):
		writeError(w, http.StatusForbidden, err.Error())
		return
	case err != nil:
		slog.Warn("clear squad envelope failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to clear the envelope")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
