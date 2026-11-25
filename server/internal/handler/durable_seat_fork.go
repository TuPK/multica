package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The seat dossier over HTTP (durable spec 9.2). The seat's own agent and any member read and
// write its entries. Only a member retires an entry a member wrote. Every route answers 404
// while durable_seats is off.

// SeatDossierEntryResponse is one entry of a seat's dossier.
type SeatDossierEntryResponse struct {
	ID        string          `json:"id"`
	AgentID   string          `json:"agent_id"`
	Kind      string          `json:"kind"`
	Text      string          `json:"text"`
	WorkClass string          `json:"work_class"`
	Evidence  json.RawMessage `json:"evidence"`
	Origin    string          `json:"origin"`
	Status    string          `json:"status"`
	CreatedAt string          `json:"created_at"`
	RetiredAt *string         `json:"retired_at"`
}

// SeatDossierEntryInput is one entry a write adds.
type SeatDossierEntryInput struct {
	Kind      string                    `json:"kind"`
	Text      string                    `json:"text"`
	WorkClass string                    `json:"work_class"`
	Evidence  []service.DossierEvidence `json:"evidence"`
}

// AddSeatDossierRequest adds up to three entries to a seat.
type AddSeatDossierRequest struct {
	Entries []SeatDossierEntryInput `json:"entries"`
}

// RetireSeatDossierRequest retires an entry; the reason is kept in the governance log.
type RetireSeatDossierRequest struct {
	Reason string `json:"reason"`
}

func seatDossierEntryResponse(e db.SeatDossierEntry) SeatDossierEntryResponse {
	return SeatDossierEntryResponse{
		ID:        util.UUIDToString(e.ID),
		AgentID:   util.UUIDToString(e.AgentID),
		Kind:      e.Kind,
		Text:      e.Text,
		WorkClass: e.WorkClass,
		Evidence:  json.RawMessage(e.Evidence),
		Origin:    e.Origin,
		Status:    e.Status,
		CreatedAt: timestampToString(e.CreatedAt),
		RetiredAt: timestampPtr(e.RetiredAt),
	}
}

// seatDossierCaller checks the flag, then resolves the workspace and the caller. The caller is
// the seat's agent or a member; the service decides what each may do.
func (h *Handler) seatDossierCaller(w http.ResponseWriter, r *http.Request) (pgtype.UUID, service.GovernanceActor, bool) {
	if h.FeatureFlags == nil || !featureflags.DurableSeatsEnabled(r.Context(), h.FeatureFlags) {
		writeError(w, http.StatusNotFound, "not found")
		return pgtype.UUID{}, service.GovernanceActor{}, false
	}
	workspaceID := h.resolveWorkspaceID(r)
	c, ok := h.initiativeCallerFor(w, r, workspaceID)
	if !ok {
		return pgtype.UUID{}, service.GovernanceActor{}, false
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return pgtype.UUID{}, service.GovernanceActor{}, false
	}
	return wsUUID, service.GovernanceActor{Type: c.actor.Type, ID: c.actor.ID}, true
}

func (h *Handler) seatDossierService() *service.DossierService {
	return &service.DossierService{Queries: h.Queries, TxStarter: h.TxStarter}
}

// writeSeatDossierError maps the dossier's rules to status codes.
func writeSeatDossierError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrDossierForbidden), errors.Is(err, service.ErrDossierMemberOnly):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrDossierNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrDossierFull):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrDossierInvalid):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "failed to update the seat dossier")
	}
}

// ListSeatDossier returns a seat's entries (GET /api/agents/{id}/dossier). status is active
// (the default), retired or all; limit is 1 to 200, default 100.
func (h *Handler) ListSeatDossier(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.seatDossierCaller(w, r)
	if !ok {
		return
	}
	seat, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "agent id")
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	switch status {
	case "", "active":
		status = "active"
	case "retired":
	case "all":
		status = ""
	default:
		writeError(w, http.StatusBadRequest, "status must be active, retired or all")
		return
	}
	limit := int32(100)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			writeError(w, http.StatusBadRequest, "limit must be 1 to 200")
			return
		}
		limit = int32(n)
	}
	entries, err := h.seatDossierService().List(r.Context(), ws, seat, status, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list the seat dossier")
		return
	}
	out := make([]SeatDossierEntryResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, seatDossierEntryResponse(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": out})
}

// AddSeatDossierEntries adds up to three entries to a seat (POST /api/agents/{id}/dossier).
func (h *Handler) AddSeatDossierEntries(w http.ResponseWriter, r *http.Request) {
	ws, actor, ok := h.seatDossierCaller(w, r)
	if !ok {
		return
	}
	seat, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "agent id")
	if !ok {
		return
	}
	var req AddSeatDossierRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in := make([]service.DossierEntryInput, 0, len(req.Entries))
	for _, e := range req.Entries {
		in = append(in, service.DossierEntryInput{Kind: e.Kind, Text: e.Text, WorkClass: e.WorkClass, Evidence: e.Evidence})
	}
	entries, err := h.seatDossierService().Add(r.Context(), ws, seat, actor, in)
	if err != nil {
		writeSeatDossierError(w, err)
		return
	}
	out := make([]SeatDossierEntryResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, seatDossierEntryResponse(e))
	}
	writeJSON(w, http.StatusCreated, map[string]any{"entries": out})
}

// RetireSeatDossierEntry retires an entry of a seat
// (POST /api/agents/{id}/dossier/{entryId}/retire).
func (h *Handler) RetireSeatDossierEntry(w http.ResponseWriter, r *http.Request) {
	ws, actor, ok := h.seatDossierCaller(w, r)
	if !ok {
		return
	}
	seat, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "agent id")
	if !ok {
		return
	}
	entryID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "entryId"), "entry id")
	if !ok {
		return
	}
	var req RetireSeatDossierRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	e, err := h.seatDossierService().Retire(r.Context(), ws, seat, entryID, actor, req.Reason)
	if err != nil {
		writeSeatDossierError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, seatDossierEntryResponse(e))
}

// seatChangeActor is who a seat change is logged under: the agent that made the call, or the
// member who did.
func (h *Handler) seatChangeActor(r *http.Request, workspaceID string) service.GovernanceActor {
	actorType, actorID := h.resolveActor(r, requestUserID(r), workspaceID)
	if actorType == "agent" {
		if id, err := util.ParseUUID(actorID); err == nil {
			return service.GovernanceActor{Type: service.GovernanceActorAgent, ID: id}
		}
	}
	return service.GovernanceActor{Type: service.GovernanceActorMember, ID: parseUUID(requestUserID(r))}
}
