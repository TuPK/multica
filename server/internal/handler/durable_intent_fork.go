package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// IntentResponse is one rehearsed action as the API shows it (durable spec 5.2). The
// preview is what the person reads; the call's hash is what an approval binds to.
type IntentResponse struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	AgentID     *string   `json:"agent_id"`
	IssueID     *string   `json:"issue_id"`
	Source      string    `json:"source"`
	Target      string    `json:"target"`
	EffectClass string    `json:"effect_class"`
	State       string    `json:"state"`
	CallSHA256  string    `json:"call_sha256"`
	// Call is the call an approval runs, so the person can edit its text before approving
	// (durable spec 5.3). The server rejects an edit that changes anything else.
	Call         json.RawMessage `json:"call"`
	Preview      json.RawMessage `json:"preview"`
	Rationale    string          `json:"rationale"`
	DecisionNote string          `json:"decision_note"`
	BatchKey     string          `json:"batch_key"`
}

func intentResponseOf(it db.EffectIntent) IntentResponse {
	out := IntentResponse{
		ID:           util.UUIDToString(it.ID),
		CreatedAt:    it.CreatedAt.Time,
		ExpiresAt:    it.ExpiresAt.Time,
		Source:       it.Source,
		Target:       it.Target,
		EffectClass:  it.EffectClass,
		State:        it.State,
		CallSHA256:   it.CallSha256,
		Call:         json.RawMessage(it.Call),
		Preview:      json.RawMessage(it.Preview),
		Rationale:    it.Rationale,
		DecisionNote: it.DecisionNote,
		BatchKey:     it.BatchKey,
	}
	if it.AgentID.Valid {
		s := util.UUIDToString(it.AgentID)
		out.AgentID = &s
	}
	if it.IssueID.Valid {
		s := util.UUIDToString(it.IssueID)
		out.IssueID = &s
	}
	return out
}

// intentError maps the intent service's errors to the API's answers.
func intentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrIntentNotFound):
		writeError(w, http.StatusNotFound, "intent not found")
	case errors.Is(err, service.ErrIntentNotPending):
		writeErrorCode(w, http.StatusConflict, "intent_not_pending", "this intent was already decided")
	case errors.Is(err, service.ErrIntentExpired):
		writeErrorCode(w, http.StatusConflict, "intent_expired", "this intent has expired")
	case errors.Is(err, service.ErrIntentNoteRequired):
		writeErrorCode(w, http.StatusUnprocessableEntity, "note_required", err.Error())
	case errors.Is(err, service.ErrIntentEditNotAllowed), errors.Is(err, service.ErrIntentEditTarget):
		writeErrorCode(w, http.StatusUnprocessableEntity, "edit_not_allowed", err.Error())
	case errors.Is(err, service.ErrEffectsOff):
		writeError(w, http.StatusServiceUnavailable, "durable effects is off")
	default:
		writeError(w, http.StatusInternalServerError, "could not record the decision")
	}
}

// intentDecider returns the member making a decision and checks they may decide on
// this intent: the owner, an admin, or the member whose action it is.
func (h *Handler) intentDecider(w http.ResponseWriter, r *http.Request, workspaceID string, id pgtype.UUID) (db.Member, bool) {
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return db.Member{}, false
	}
	if !h.mayDecideIntent(r, member, id) {
		writeError(w, http.StatusForbidden, "only the owner, an admin, or the member whose action this is can decide on it")
		return db.Member{}, false
	}
	return member, true
}

// mayDecideIntent reports whether member may decide on an intent: the owner, an
// admin, or the member whose awaiting event the intent is.
func (h *Handler) mayDecideIntent(r *http.Request, member db.Member, id pgtype.UUID) bool {
	if member.Role == "owner" || member.Role == "admin" {
		return true
	}
	ev, err := h.Queries.GetEffectEventByIntent(r.Context(), db.GetEffectEventByIntentParams{IntentID: id, WorkspaceID: member.WorkspaceID})
	return err == nil && ev.MemberID == member.UserID
}

// ListIntents returns the workspace's intents, newest first (GET /api/intents).
// state filters by one state; limit is 1 to 200, default 50.
func (h *Handler) ListIntents(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	if h.Intents == nil {
		writeError(w, http.StatusServiceUnavailable, "intents are not configured")
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	limit := int32(50)
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 200")
			return
		}
		limit = int32(n)
	}
	rows, err := h.Intents.List(r.Context(), wsUUID, r.URL.Query().Get("state"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list intents")
		return
	}
	out := make([]IntentResponse, 0, len(rows))
	for _, it := range rows {
		out = append(out, intentResponseOf(it))
	}
	writeJSON(w, http.StatusOK, map[string]any{"intents": out})
}

// ApproveIntent runs a pending intent once (POST /api/intents/{id}/approve). The body
// may carry edited_call, with new text for the message or post, and a note.
func (h *Handler) ApproveIntent(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "id")
	if !ok {
		return
	}
	member, ok := h.intentDecider(w, r, workspaceID, id)
	if !ok {
		return
	}
	var req struct {
		EditedCall *connectedapps.CanonicalCall `json:"edited_call"`
		Note       string                       `json:"note"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if h.Intents == nil {
		writeError(w, http.StatusServiceUnavailable, "intents are not configured")
		return
	}
	it, err := h.Intents.Approve(r.Context(), wsUUID, member.UserID, id, req.EditedCall, strings.TrimSpace(req.Note))
	if err != nil {
		intentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, intentResponseOf(it))
}

// RejectIntent closes a pending intent without running it (POST
// /api/intents/{id}/reject). The note is required and goes to the agent.
func (h *Handler) RejectIntent(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "id")
	if !ok {
		return
	}
	member, ok := h.intentDecider(w, r, workspaceID, id)
	if !ok {
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if h.Intents == nil {
		writeError(w, http.StatusServiceUnavailable, "intents are not configured")
		return
	}
	it, err := h.Intents.Reject(r.Context(), wsUUID, member.UserID, id, req.Note)
	if err != nil {
		intentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, intentResponseOf(it))
}

// ApproveIntentBatch approves several intents in one request (POST
// /api/intents/approve-batch). Each intent is decided on its own: one call and one
// event each. The answer lists each intent's outcome.
func (h *Handler) ApproveIntentBatch(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.IDs) == 0 || len(req.IDs) > 50 {
		writeError(w, http.StatusBadRequest, "ids must list between 1 and 50 intents")
		return
	}
	if h.Intents == nil {
		writeError(w, http.StatusServiceUnavailable, "intents are not configured")
		return
	}
	type result struct {
		ID     string `json:"id"`
		State  string `json:"state,omitempty"`
		Error  string `json:"error,omitempty"`
		Status int    `json:"status"`
	}
	out := make([]result, 0, len(req.IDs))
	for _, raw := range req.IDs {
		id, err := util.ParseUUID(raw)
		if err != nil {
			out = append(out, result{ID: raw, Error: "invalid id", Status: http.StatusBadRequest})
			continue
		}
		if !h.mayDecideIntent(r, member, id) {
			out = append(out, result{ID: raw, Error: "only the owner, an admin, or the member whose action this is can decide on it", Status: http.StatusForbidden})
			continue
		}
		it, err := h.Intents.Approve(r.Context(), wsUUID, member.UserID, id, nil, "")
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, service.ErrIntentNotFound):
				status = http.StatusNotFound
			case errors.Is(err, service.ErrIntentNotPending), errors.Is(err, service.ErrIntentExpired):
				status = http.StatusConflict
			}
			out = append(out, result{ID: raw, Error: err.Error(), Status: status})
			continue
		}
		out = append(out, result{ID: raw, State: it.State, Status: http.StatusOK})
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}
