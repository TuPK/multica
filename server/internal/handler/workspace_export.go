package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/storage"
	"github.com/multica-ai/multica/server/internal/workspaceexport"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// WorkspaceExportResponse is one export of a workspace (platform spec 5.11).
// DownloadURL is set only for a done export that has not expired, and only when
// the store can sign a link.
type WorkspaceExportResponse struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	RequestedBy string     `json:"requested_by"`
	Status      string     `json:"status"`
	SizeBytes   int64      `json:"size_bytes"`
	Error       string     `json:"error"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
	DownloadURL string     `json:"download_url,omitempty"`
}

func workspaceExportResponse(e workspaceexport.Export, now time.Time) WorkspaceExportResponse {
	return WorkspaceExportResponse{
		ID:          e.ID,
		WorkspaceID: e.WorkspaceID,
		RequestedBy: e.RequestedBy,
		Status:      workspaceexport.EffectiveStatus(e, now),
		SizeBytes:   e.SizeBytes,
		Error:       e.Error,
		CreatedAt:   e.CreatedAt,
		StartedAt:   e.StartedAt,
		FinishedAt:  e.FinishedAt,
		ExpiresAt:   e.ExpiresAt,
	}
}

// exportOwner resolves the requester and writes a 403 unless it is the owner.
// The router also limits these routes to the owner; this is the backstop for a
// handler called without the middleware.
func (h *Handler) exportOwner(w http.ResponseWriter, r *http.Request) (db.Member, bool) {
	member, ok := h.workspaceMember(w, r, workspaceIDFromURL(r, "id"))
	if !ok {
		return db.Member{}, false
	}
	if member.Role != "owner" {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return db.Member{}, false
	}
	return member, true
}

// CreateWorkspaceExport serves POST /api/workspaces/{id}/exports. It queues an
// export for the job to build. A workspace has one export queued or running at
// a time, and creates at most one in 24 hours.
func (h *Handler) CreateWorkspaceExport(w http.ResponseWriter, r *http.Request) {
	member, ok := h.exportOwner(w, r)
	if !ok {
		return
	}
	workspaceID := uuidToString(member.WorkspaceID)

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		slog.Warn("workspace export: begin failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "failed to create export")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.Queries.WithTx(tx)

	var latest *workspaceexport.Latest
	last, err := qtx.GetLatestWorkspaceExport(r.Context(), member.WorkspaceID)
	switch {
	case err == nil:
		latest = &workspaceexport.Latest{Status: last.Status, CreatedAt: last.CreatedAt.Time}
	case !errors.Is(err, pgx.ErrNoRows):
		slog.Warn("workspace export: read latest failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "failed to create export")
		return
	}
	switch err := workspaceexport.Admit(time.Now(), latest); {
	case errors.Is(err, workspaceexport.ErrInProgress):
		writeErrorCode(w, http.StatusConflict, "export_in_progress", "an export of this workspace is already queued or running")
		return
	case errors.Is(err, workspaceexport.ErrDailyLimit):
		writeErrorCode(w, http.StatusTooManyRequests, "export_daily_limit", "a workspace can create one export every 24 hours")
		return
	}

	row, err := qtx.CreateWorkspaceExport(r.Context(), db.CreateWorkspaceExportParams{
		ID:          pgtype.UUID{Bytes: uuid.New(), Valid: true},
		WorkspaceID: member.WorkspaceID,
		RequestedBy: member.UserID,
	})
	if err != nil {
		// A concurrent request won the slot: the unique index on queued and
		// running rows refused this insert.
		if isUniqueViolation(err) {
			writeErrorCode(w, http.StatusConflict, "export_in_progress", "an export of this workspace is already queued or running")
			return
		}
		slog.Warn("workspace export: insert failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "failed to create export")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		if isUniqueViolation(err) {
			writeErrorCode(w, http.StatusConflict, "export_in_progress", "an export of this workspace is already queued or running")
			return
		}
		slog.Warn("workspace export: commit failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "failed to create export")
		return
	}

	slog.Info("workspace export queued", append(logger.RequestAttrs(r), "workspace_id", workspaceID, "export_id", uuidToString(row.ID))...)
	writeJSON(w, http.StatusAccepted, workspaceExportResponse(workspaceexport.FromRow(row), time.Now()))
}

// ListWorkspaceExports serves GET /api/workspaces/{id}/exports, newest first.
func (h *Handler) ListWorkspaceExports(w http.ResponseWriter, r *http.Request) {
	member, ok := h.exportOwner(w, r)
	if !ok {
		return
	}
	rows, err := h.Queries.ListWorkspaceExports(r.Context(), member.WorkspaceID)
	if err != nil {
		slog.Warn("workspace export: list failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to list exports")
		return
	}
	now := time.Now()
	out := make([]WorkspaceExportResponse, len(rows))
	for i, row := range rows {
		out[i] = workspaceExportResponse(workspaceexport.FromRow(row), now)
	}
	writeJSON(w, http.StatusOK, map[string]any{"exports": out})
}

// GetWorkspaceExport serves GET /api/workspaces/{id}/exports/{exportId}. A done
// export that has not expired carries a signed download URL valid until it
// expires; an expired one has no URL.
func (h *Handler) GetWorkspaceExport(w http.ResponseWriter, r *http.Request) {
	member, ok := h.exportOwner(w, r)
	if !ok {
		return
	}
	exportID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "exportId"), "export id")
	if !ok {
		return
	}
	row, err := h.Queries.GetWorkspaceExport(r.Context(), db.GetWorkspaceExportParams{ID: exportID, WorkspaceID: member.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "export not found")
		return
	}
	if err != nil {
		slog.Warn("workspace export: read failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to read export")
		return
	}

	now := time.Now()
	e := workspaceexport.FromRow(row)
	resp := workspaceExportResponse(e, now)
	if window, ok := workspaceexport.DownloadWindow(e, now); ok && e.StorageKey != "" {
		url, err := h.exportDownloadURL(r, e.StorageKey, window)
		if err != nil {
			slog.Warn("workspace export: sign failed", append(logger.RequestAttrs(r), "error", err, "export_id", e.ID)...)
			writeError(w, http.StatusInternalServerError, "failed to create download link")
			return
		}
		resp.DownloadURL = url
	}
	writeJSON(w, http.StatusOK, resp)
}

// exportDownloadURL signs a link to the stored archive. A store that cannot
// sign (local disk) yields no link.
func (h *Handler) exportDownloadURL(r *http.Request, key string, ttl time.Duration) (string, error) {
	presigner, ok := h.Storage.(storage.DownloadPresigner)
	if !ok {
		return "", nil
	}
	url, err := presigner.PresignGetWithContentDisposition(r.Context(), key, ttl, storage.AttachmentContentDisposition("workspace-export.zip"))
	if errors.Is(err, storage.ErrPresignUnsupported) {
		return "", nil
	}
	return url, err
}
