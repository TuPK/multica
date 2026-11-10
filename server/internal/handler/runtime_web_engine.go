package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// webEngineStatuses are the states a daemon reports for the Web engine (spec 4.13): running,
// not reachable, or no address and token configured on its machine.
var webEngineStatuses = map[string]bool{"running": true, "not_running": true, "not_configured": true}

// maxWebEngineVersion bounds the version string a daemon can store.
const maxWebEngineVersion = 64

// ReportWebEngineStatus (POST /api/daemon/runtimes/{runtimeId}/web-engine) records the Web engine's
// status on the machine that runs the runtime. The daemon sends it on change and on a refresh; the
// server stamps the time, so the card shows when the machine last answered.
func (h *Handler) ReportWebEngineStatus(w http.ResponseWriter, r *http.Request) {
	rt, ok := h.requireDaemonRuntimeAccess(w, r, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	var req struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !webEngineStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "status must be running, not_running or not_configured")
		return
	}
	if len(req.Version) > maxWebEngineVersion {
		writeError(w, http.StatusBadRequest, "version is too long")
		return
	}
	body, err := json.Marshal(map[string]string{
		"status":     req.Status,
		"version":    req.Version,
		"checked_at": time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not encode the status")
		return
	}
	if err := h.Queries.SetAgentRuntimeWebEngine(r.Context(), db.SetAgentRuntimeWebEngineParams{ID: rt.ID, WebEngine: body}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not store the status")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
