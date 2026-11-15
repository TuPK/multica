package handler

import (
	"encoding/csv"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/logger"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// AuditEventsResponse is the body of both audit list endpoints.
type AuditEventsResponse struct {
	Events     []auditEventJSON `json:"events"`
	NextCursor *string          `json:"next_cursor"`
}

func toAuditEventJSON(e db.AuditEvent) auditEventJSON {
	detail := e.Detail
	if len(detail) == 0 {
		detail = []byte("{}")
	}
	return auditEventJSON{
		ID:          uuidToString(e.ID),
		WorkspaceID: uuidToPtr(e.WorkspaceID),
		ActorType:   e.ActorType,
		ActorID:     textToPtr(e.ActorID),
		ActorLabel:  e.ActorLabel,
		Action:      e.Action,
		TargetType:  e.TargetType,
		TargetID:    e.TargetID,
		TargetLabel: e.TargetLabel,
		Detail:      detail,
		IP:          e.Ip,
		UserAgent:   e.UserAgent,
		RequestID:   e.RequestID,
		CreatedAt:   e.CreatedAt.Time.UTC().Format(time.RFC3339Nano),
	}
}

func writeAuditParamError(w http.ResponseWriter, perr *auditParamError) {
	writeErrorCode(w, http.StatusBadRequest, perr.code, perr.msg)
}

func cursorParams(c *auditCursor) (pgtype.Timestamptz, pgtype.UUID) {
	if c == nil {
		return pgtype.Timestamptz{}, pgtype.UUID{}
	}
	// The cursor was validated when it was decoded.
	return pgtype.Timestamptz{Time: c.createdAt, Valid: true}, parseUUID(c.id)
}

// pageAuditEvents trims the extra row fetched to detect a next page and builds
// the cursor from the last row kept.
func pageAuditEvents(rows []db.AuditEvent, limit int) AuditEventsResponse {
	var next *string
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		c := encodeAuditCursor(last.CreatedAt.Time, uuidToString(last.ID))
		next = &c
	}
	events := make([]auditEventJSON, len(rows))
	for i, r := range rows {
		events[i] = toAuditEventJSON(r)
	}
	return AuditEventsResponse{Events: events, NextCursor: next}
}

// ListWorkspaceAuditEvents serves GET /api/workspaces/{id}/audit-events as JSON
// or, with format=csv, as a download capped at auditCSVMaxRows. The route is
// limited to workspace owners and admins.
func (h *Handler) ListWorkspaceAuditEvents(w http.ResponseWriter, r *http.Request) {
	workspaceUUID, ok := parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
	if !ok {
		return
	}
	p, perr := parseAuditListParams(r.URL.Query(), true)
	if perr != nil {
		writeAuditParamError(w, perr)
		return
	}

	rowLimit := p.limit
	if p.csv {
		rowLimit = auditCSVMaxRows
	}
	curAt, curID := cursorParams(p.cursor)
	arg := db.ListWorkspaceAuditEventsParams{
		WorkspaceID:     workspaceUUID,
		CursorCreatedAt: curAt,
		CursorID:        curID,
		RowLimit:        int32(rowLimit + 1),
	}
	if p.action != nil {
		arg.Action = pgtype.Text{String: *p.action, Valid: true}
	}
	if p.actorID != nil {
		arg.ActorID = pgtype.Text{String: *p.actorID, Valid: true}
	}
	if p.from != nil {
		arg.CreatedFrom = pgtype.Timestamptz{Time: *p.from, Valid: true}
	}
	if p.to != nil {
		arg.CreatedTo = pgtype.Timestamptz{Time: *p.to, Valid: true}
	}
	rows, err := h.Queries.ListWorkspaceAuditEvents(r.Context(), arg)
	if err != nil {
		slog.Warn("list audit events failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to list audit events")
		return
	}

	if !p.csv {
		writeJSON(w, http.StatusOK, pageAuditEvents(rows, p.limit))
		return
	}

	truncated := len(rows) > auditCSVMaxRows
	page := pageAuditEvents(rows, auditCSVMaxRows)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="audit-events.csv"`)
	if truncated {
		w.Header().Set("X-Audit-Truncated", "true")
	}
	w.WriteHeader(http.StatusOK)
	if err := writeAuditCSV(csv.NewWriter(w), page.Events); err != nil {
		slog.Warn("write audit csv failed", append(logger.RequestAttrs(r), "error", err)...)
	}
}

// ListMySecurityEvents serves GET /api/me/security-events: the account-level
// events (no workspace) the caller performed.
func (h *Handler) ListMySecurityEvents(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	uid, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}
	p, perr := parseAuditListParams(r.URL.Query(), false)
	if perr != nil {
		writeAuditParamError(w, perr)
		return
	}
	curAt, curID := cursorParams(p.cursor)
	rows, err := h.Queries.ListUserSecurityEvents(r.Context(), db.ListUserSecurityEventsParams{
		// actor_id is stored as text; the canonical form of the user id.
		ActorID:         uuidToString(uid),
		CursorCreatedAt: curAt,
		CursorID:        curID,
		RowLimit:        int32(p.limit + 1),
	})
	if err != nil {
		slog.Warn("list security events failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to list security events")
		return
	}
	writeJSON(w, http.StatusOK, pageAuditEvents(rows, p.limit))
}
