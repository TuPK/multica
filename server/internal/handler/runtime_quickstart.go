package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/logger"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	"github.com/multica-ai/multica/server/internal/quickstart"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// Quickstart asks a connected machine to install the team's Claude Code setup.
// The request is a row in runtime_quickstart_request: the web app creates it,
// the daemon claims it on heartbeat, reads the workspace skills it needs, and
// reports steps back. The UI polls by request ID. Windows match the SQL in
// runtime_quickstart.sql.

const (
	quickstartPendingTimeout = 2 * time.Minute
	quickstartRunningTimeout = 10 * time.Minute
	quickstartPendingMessage = "the machine did not pick up the request within 2 minutes; check that its daemon is running"
	quickstartRunningMessage = "the install did not finish within 10 minutes"
)

// QuickstartRequestResponse is the wire shape of one quickstart request.
type QuickstartRequestResponse struct {
	ID        string                    `json:"id"`
	RuntimeID string                    `json:"runtime_id"`
	Status    string                    `json:"status"`
	Steps     []protocol.QuickstartStep `json:"steps"`
	Error     string                    `json:"error,omitempty"`
	CreatedAt time.Time                 `json:"created_at"`
	UpdatedAt time.Time                 `json:"updated_at"`
	initiator pgtype.UUID               `json:"-"`
}

func quickstartResponseFromRow(row db.RuntimeQuickstartRequest, now time.Time) QuickstartRequestResponse {
	resp := QuickstartRequestResponse{
		ID:        uuidToString(row.ID),
		RuntimeID: uuidToString(row.RuntimeID),
		Status:    row.Status,
		Steps:     []protocol.QuickstartStep{},
		Error:     row.Error,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
	if len(row.Steps) > 0 {
		_ = json.Unmarshal(row.Steps, &resp.Steps)
	}
	// Timeouts are applied when the status is read, not written back. The
	// request is only ever moved to a terminal state by the daemon or by the
	// SQL windows, so a late report still lands on a row that is not terminal.
	switch {
	case row.Status == "pending" && now.Sub(row.CreatedAt.Time) > quickstartPendingTimeout:
		resp.Status = "failed"
		resp.Error = quickstartPendingMessage
	case row.Status == "running" && now.Sub(row.CreatedAt.Time) > quickstartRunningTimeout:
		resp.Status = "failed"
		resp.Error = quickstartRunningMessage
	}
	return resp
}

// InitiateRuntimeQuickstart queues a quickstart for a runtime (web app).
func (h *Handler) InitiateRuntimeQuickstart(w http.ResponseWriter, r *http.Request) {
	runtimeID := chi.URLParam(r, "runtimeId")
	rt, member, ok := h.requireRuntimeReadAccess(w, r, obsmetrics.RuntimeLookupSourceRuntimeAPI, runtimeID)
	if !ok {
		return
	}
	if !canEditRuntime(member, rt) {
		writeError(w, http.StatusForbidden, "only runtime owners and workspace admins can run quickstart")
		return
	}

	_, err := h.Queries.GetLiveRuntimeQuickstartRequest(r.Context(), rt.ID)
	switch {
	case err == nil:
		writeError(w, http.StatusConflict, "a quickstart is already running on this machine")
		return
	case !errors.Is(err, pgx.ErrNoRows):
		slog.Error("load live quickstart failed", "error", err, "runtime_id", runtimeID)
		writeError(w, http.StatusInternalServerError, "failed to start quickstart")
		return
	}

	row, err := h.Queries.CreateRuntimeQuickstartRequest(r.Context(), db.CreateRuntimeQuickstartRequestParams{
		RuntimeID:       rt.ID,
		WorkspaceID:     rt.WorkspaceID,
		InitiatorUserID: member.UserID,
	})
	if err != nil {
		slog.Error("create quickstart failed", "error", err, "runtime_id", runtimeID)
		writeError(w, http.StatusInternalServerError, "failed to start quickstart")
		return
	}
	writeJSON(w, http.StatusOK, quickstartResponseFromRow(row, time.Now()))
}

// GetRuntimeQuickstart returns the status of one quickstart request (web app).
func (h *Handler) GetRuntimeQuickstart(w http.ResponseWriter, r *http.Request) {
	runtimeID := chi.URLParam(r, "runtimeId")
	rt, member, ok := h.requireRuntimeReadAccess(w, r, obsmetrics.RuntimeLookupSourceRuntimeAPI, runtimeID)
	if !ok {
		return
	}
	requestID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "requestId"), "requestId")
	if !ok {
		return
	}
	row, err := h.Queries.GetRuntimeQuickstartRequest(r.Context(), requestID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "quickstart not found")
			return
		}
		slog.Warn("load quickstart failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to load quickstart")
		return
	}
	if row.RuntimeID != rt.ID {
		writeError(w, http.StatusNotFound, "quickstart not found")
		return
	}
	if !canEditRuntime(member, rt) && row.InitiatorUserID != member.UserID {
		writeError(w, http.StatusForbidden, "only runtime owners, workspace admins, and the quickstart initiator can view this quickstart")
		return
	}
	writeJSON(w, http.StatusOK, quickstartResponseFromRow(row, time.Now()))
}

// claimPendingQuickstart hands the oldest fresh request for a runtime to the
// daemon and returns it for the heartbeat ack. Nil means nothing is waiting.
func (h *Handler) claimPendingQuickstart(ctx context.Context, runtimeID pgtype.UUID) (*protocol.DaemonHeartbeatPendingQuickstart, error) {
	row, err := h.Queries.ClaimPendingRuntimeQuickstartRequest(ctx, runtimeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	bundle, err := quickstart.LoadBundle()
	if err != nil {
		return nil, err
	}
	return &protocol.DaemonHeartbeatPendingQuickstart{ID: uuidToString(row.ID), Bundle: bundle}, nil
}

// claimedQuickstart loads a request that the daemon of this runtime has
// claimed and is still running. Anything else is refused.
func (h *Handler) claimedQuickstart(w http.ResponseWriter, r *http.Request, rt db.AgentRuntime) (db.RuntimeQuickstartRequest, bool) {
	requestID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "requestId"), "requestId")
	if !ok {
		return db.RuntimeQuickstartRequest{}, false
	}
	row, err := h.Queries.GetRuntimeQuickstartRequest(r.Context(), requestID)
	if err != nil || row.RuntimeID != rt.ID {
		writeError(w, http.StatusNotFound, "quickstart not found")
		return db.RuntimeQuickstartRequest{}, false
	}
	if row.Status != "running" {
		writeError(w, http.StatusConflict, "quickstart is not running")
		return db.RuntimeQuickstartRequest{}, false
	}
	return row, true
}

// GetRuntimeQuickstartSkills returns the workspace copies of the bundle's
// skills to the daemon that claimed the request.
func (h *Handler) GetRuntimeQuickstartSkills(w http.ResponseWriter, r *http.Request) {
	rt, ok := h.requireDaemonRuntimeAccess(w, r, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	if _, ok := h.claimedQuickstart(w, r, rt); !ok {
		return
	}

	bundle, err := quickstart.LoadBundle()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load quickstart bundle")
		return
	}
	out := protocol.QuickstartSkillBundles{Skills: []protocol.QuickstartSkill{}}
	for _, name := range bundle.Skills {
		skill, found, err := h.lookupSkillByName(r.Context(), rt.WorkspaceID, name)
		if err != nil {
			slog.Error("quickstart skill lookup failed", "error", err, "skill", name)
			writeError(w, http.StatusInternalServerError, "failed to load skills")
			return
		}
		if !found {
			out.Missing = append(out.Missing, name)
			continue
		}
		files, err := h.Queries.ListSkillFiles(r.Context(), skill.ID)
		if err != nil {
			slog.Error("quickstart skill files failed", "error", err, "skill", name)
			writeError(w, http.StatusInternalServerError, "failed to load skills")
			return
		}
		item := protocol.QuickstartSkill{Name: skill.Name, Content: skill.Content, Files: []protocol.QuickstartSkillFile{}}
		for _, f := range files {
			item.Files = append(item.Files, protocol.QuickstartSkillFile{Path: f.Path, Content: f.Content})
		}
		out.Skills = append(out.Skills, item)
	}
	writeJSON(w, http.StatusOK, out)
}

// ReportRuntimeQuickstartResult receives the daemon's final steps.
func (h *Handler) ReportRuntimeQuickstartResult(w http.ResponseWriter, r *http.Request) {
	rt, ok := h.requireDaemonRuntimeAccess(w, r, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	row, ok := h.claimedQuickstart(w, r, rt)
	if !ok {
		return
	}

	var req struct {
		Status string                    `json:"status"` // "completed" or "failed"
		Steps  []protocol.QuickstartStep `json:"steps"`
		Error  string                    `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	steps, err := json.Marshal(req.Steps)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid steps")
		return
	}

	switch req.Status {
	case "completed":
		_, err = h.Queries.CompleteRuntimeQuickstartRequest(r.Context(), db.CompleteRuntimeQuickstartRequestParams{
			ID: row.ID, Steps: steps,
		})
	case "failed":
		_, err = h.Queries.FailRuntimeQuickstartRequest(r.Context(), db.FailRuntimeQuickstartRequestParams{
			ID: row.ID, Steps: steps, Error: req.Error,
		})
	default:
		writeError(w, http.StatusBadRequest, "status must be completed or failed")
		return
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.Error("persist quickstart result failed", "error", err, "quickstart_id", uuidToString(row.ID))
		writeError(w, http.StatusInternalServerError, "failed to persist quickstart result")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
