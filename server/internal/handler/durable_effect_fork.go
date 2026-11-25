package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// EffectResponse is one effect event as the API shows it (durable spec 4.4). The
// parameters of the call are never included.
type EffectResponse struct {
	ID            string     `json:"id"`
	CreatedAt     time.Time  `json:"created_at"`
	Source        string     `json:"source"`
	Target        string     `json:"target"`
	EffectClass   string     `json:"effect_class"`
	Reversibility string     `json:"reversibility"`
	Decision      string     `json:"decision"`
	Status        string     `json:"status"`
	Summary       string     `json:"summary"`
	TaskID        *string    `json:"task_id"`
	AgentID       *string    `json:"agent_id"`
	MemberID      *string    `json:"member_id"`
	UndoAvailable bool       `json:"undo_available"`
	UndoUntil     *time.Time `json:"undo_until"`
}

// DeclareEffectResponse answers a declared action: the event, the policy's decision,
// and the intent when the decision is to rehearse (durable spec 4.5).
type DeclareEffectResponse struct {
	EventID  string  `json:"event_id"`
	Decision string  `json:"decision"`
	Status   string  `json:"status"`
	IntentID *string `json:"intent_id"`
}

// effectUndoOpen reports whether an event can be undone now: it is executed, it has
// an undo, and its window has not closed (durable spec 4.6).
func effectUndoOpen(ev db.EffectEvent, now time.Time) bool {
	return ev.Status == "executed" && ev.Undo != nil && ev.UndoUntil.Valid && now.Before(ev.UndoUntil.Time)
}

func effectResponseOf(ev db.EffectEvent, now time.Time) EffectResponse {
	out := EffectResponse{
		ID:            util.UUIDToString(ev.ID),
		CreatedAt:     ev.CreatedAt.Time,
		Source:        ev.Source,
		Target:        ev.Target,
		EffectClass:   ev.EffectClass,
		Reversibility: ev.Reversibility,
		Decision:      ev.Decision,
		Status:        ev.Status,
		Summary:       ev.Summary,
		UndoAvailable: effectUndoOpen(ev, now),
	}
	if ev.TaskID.Valid {
		s := util.UUIDToString(ev.TaskID)
		out.TaskID = &s
	}
	if ev.AgentID.Valid {
		s := util.UUIDToString(ev.AgentID)
		out.AgentID = &s
	}
	if ev.MemberID.Valid {
		s := util.UUIDToString(ev.MemberID)
		out.MemberID = &s
	}
	if ev.UndoUntil.Valid {
		t := ev.UndoUntil.Time
		out.UndoUntil = &t
	}
	return out
}

// DeclareEffect records an action an agent is about to take outside the platform
// (POST /api/effects/declare). The platform cannot stop it; it answers with the
// policy's decision, which the CLI turns into an exit code (durable spec 4.5).
func (h *Handler) DeclareEffect(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	if h.Effects == nil {
		writeError(w, http.StatusServiceUnavailable, "the effect log is not configured")
		return
	}
	var req struct {
		Class         string `json:"class"`
		Reversibility string `json:"reversibility"`
		Target        string `json:"target"`
		Summary       string `json:"summary"`
		Rationale     string `json:"rationale"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validDeclaredClass(req.Class) {
		writeError(w, http.StatusUnprocessableEntity, "class must be one of read, write_internal, write, send, share, delete, spend")
		return
	}
	rev := req.Reversibility
	if rev == "" {
		rev = connectedapps.DefaultReversibility(req.Class)
	}
	if req.Class != connectedapps.EffectRead && !validDeclaredReversibility(rev) {
		writeError(w, http.StatusUnprocessableEntity, "reversibility must be reversible, compensable or irreversible")
		return
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		writeError(w, http.StatusUnprocessableEntity, "target is required")
		return
	}

	in := connectedapps.EffectRequest{
		WS:            wsUUID,
		MemberID:      member.UserID,
		Source:        connectedapps.SourceDeclared,
		Target:        target,
		Class:         req.Class,
		Reversibility: rev,
		Summary:       strings.TrimSpace(req.Summary),
		Prior:         connectedapps.PolicyAllow,
	}
	if task, ok := h.taskFromRequestHeader(r); ok {
		in.TaskID, in.AgentID = task.ID, task.AgentID
	}
	ticket, err := h.Effects.Gate(r.Context(), in)
	if err != nil {
		slog.Error("declare effect: gate", "error", err)
		writeError(w, http.StatusInternalServerError, "could not record the declared action")
		return
	}
	// A rehearsed declaration becomes an intent the person decides on. The platform
	// does not run it: the decision goes back to the agent (durable spec 5.2).
	var intentID *string
	if ticket.Decision == connectedapps.PolicyRehearse && h.Intents != nil {
		id, err := h.Intents.Rehearse(r.Context(), ticket.EventID, in, connectedapps.CanonicalCall{}, req.Rationale)
		if err != nil {
			slog.Error("declare effect: rehearse", "error", err)
			writeError(w, http.StatusInternalServerError, "could not hold the declared action for approval")
			return
		}
		s := util.UUIDToString(id)
		intentID = &s
	}
	writeJSON(w, http.StatusOK, DeclareEffectResponse{
		EventID:  util.UUIDToString(ticket.EventID),
		Decision: ticket.Decision,
		Status:   "declared",
		IntentID: intentID,
	})
}

// ListEffects returns the workspace's effect events, newest first (GET /api/effects).
// Filters: effect_class, source, decision, agent_id, task_id, before (RFC 3339, the
// created_at of the last row of the previous page), limit (1 to 200, default 50).
func (h *Handler) ListEffects(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	q := r.URL.Query()
	params := db.ListEffectEventsParams{WorkspaceID: wsUUID, RowLimit: 50}
	if v := q.Get("effect_class"); v != "" {
		params.EffectClass = pgtype.Text{String: v, Valid: true}
	}
	if v := q.Get("source"); v != "" {
		params.Source = pgtype.Text{String: v, Valid: true}
	}
	if v := q.Get("decision"); v != "" {
		params.Decision = pgtype.Text{String: v, Valid: true}
	}
	if v := q.Get("agent_id"); v != "" {
		id, ok := parseUUIDOrBadRequest(w, v, "agent_id")
		if !ok {
			return
		}
		params.AgentID = id
	}
	if v := q.Get("task_id"); v != "" {
		id, ok := parseUUIDOrBadRequest(w, v, "task_id")
		if !ok {
			return
		}
		params.TaskID = id
	}
	if v := q.Get("before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "before must be an RFC 3339 time")
			return
		}
		params.Before = pgtype.Timestamptz{Time: t, Valid: true}
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 200")
			return
		}
		params.RowLimit = int32(n)
	}
	rows, err := h.Queries.ListEffectEvents(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list effects")
		return
	}
	now := time.Now()
	out := make([]EffectResponse, 0, len(rows))
	for _, ev := range rows {
		out = append(out, effectResponseOf(ev, now))
	}
	writeJSON(w, http.StatusOK, map[string]any{"effects": out})
}

// UndoEffect takes back an executed effect (POST /api/effects/{id}/undo, durable spec
// 4.6). The owner, an admin, or the member who acted may undo, and only inside the
// undo window. The undo runs through the gateway as that member, and records its own
// event.
func (h *Handler) UndoEffect(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "id")
	if !ok {
		return
	}
	if h.Effects == nil || h.ConnectedApps == nil {
		writeError(w, http.StatusServiceUnavailable, "undo is not configured")
		return
	}
	ev, err := h.Queries.GetEffectEvent(r.Context(), db.GetEffectEventParams{ID: id, WorkspaceID: wsUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "effect not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load effect")
		return
	}
	if member.Role != "owner" && member.Role != "admin" && member.UserID != ev.MemberID {
		writeError(w, http.StatusForbidden, "only the owner, an admin, or the member who acted can undo this")
		return
	}
	if ev.Status != "executed" {
		writeErrorCode(w, http.StatusConflict, "effect_not_executed", "only an executed effect can be undone")
		return
	}
	if ev.Undo == nil {
		writeErrorCode(w, http.StatusUnprocessableEntity, "not_undoable", "this effect cannot be taken back")
		return
	}
	if !effectUndoOpen(ev, time.Now()) {
		writeErrorCode(w, http.StatusConflict, "undo_window_closed", "the undo window for this effect has closed")
		return
	}
	var spec connectedapps.UndoSpec
	if err := json.Unmarshal(ev.Undo, &spec); err != nil || spec.Tool == "" {
		writeErrorCode(w, http.StatusUnprocessableEntity, "not_undoable", "this effect cannot be taken back")
		return
	}
	if _, err := h.ConnectedApps.ExecuteUndo(r.Context(), wsUUID, ev.MemberID, ev.Target, spec); err != nil {
		slog.Error("undo effect", "error", err, "effect_id", uuidToString(id))
		writeError(w, http.StatusBadGateway, "the undo call failed; the effect is unchanged")
		return
	}
	if _, err := h.Queries.MarkEffectEventUndone(r.Context(), db.MarkEffectEventUndoneParams{ID: id, WorkspaceID: wsUUID}); err != nil {
		slog.Error("undo effect: mark undone", "error", err, "effect_id", uuidToString(id))
		writeError(w, http.StatusInternalServerError, "the undo ran but the effect could not be marked undone")
		return
	}
	undoEventID := ""
	if undoID, err := h.Queries.InsertEffectEvent(r.Context(), db.InsertEffectEventParams{
		WorkspaceID:   wsUUID,
		MemberID:      member.UserID,
		Source:        connectedapps.SourceConnectedApp,
		Target:        ev.Target,
		EffectClass:   ev.EffectClass,
		Reversibility: ev.Reversibility,
		Decision:      connectedapps.PolicyAllow,
		Status:        "executed",
		Summary:       truncateEffectSummary("Hoàn tác " + uuidToString(id)),
	}); err != nil {
		slog.Error("undo effect: record undo event", "error", err, "effect_id", uuidToString(id))
	} else {
		undoEventID = uuidToString(undoID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"effect_id":     uuidToString(id),
		"status":        "undone",
		"undo_event_id": undoEventID,
	})
}

func validDeclaredClass(class string) bool {
	switch class {
	case connectedapps.EffectRead, connectedapps.EffectWriteInternal, connectedapps.EffectWrite,
		connectedapps.EffectSend, connectedapps.EffectShare, connectedapps.EffectDelete, connectedapps.EffectSpend:
		return true
	}
	return false
}

func validDeclaredReversibility(rev string) bool {
	switch rev {
	case connectedapps.ReversibilityReversible, connectedapps.ReversibilityCompensable, connectedapps.ReversibilityIrreversible:
		return true
	}
	return false
}

func truncateEffectSummary(s string) string {
	if r := []rune(s); len(r) > 500 {
		return string(r[:500])
	}
	return s
}
