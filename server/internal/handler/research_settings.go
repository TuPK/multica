package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/multica-ai/multica/server/internal/research"
)

// Research Lab source settings. Reading the settings (never a key) is for any member. Changing them
// is for owners and admins; agents cannot. The credentials endpoint returns the keys in plaintext and
// is narrower: only a task-scoped agent token (the daemon's `mat_` token, bound to this workspace by
// the auth middleware) and a human owner or admin may read them. A plain member gets 403 and uses
// OPENALEX_API_KEY and the other environment variables on their own machine instead.

// UpdateResearchSettingsRequest is the PUT /api/research/settings body. An absent field is left
// alone; an empty string removes the stored value.
type UpdateResearchSettingsRequest struct {
	OpenAlexAPIKey        *string `json:"openalex_api_key"`
	SemanticScholarAPIKey *string `json:"semantic_scholar_api_key"`
	ContactEmail          *string `json:"contact_email"`
}

func (h *Handler) researchReady(w http.ResponseWriter) bool {
	if h.Research == nil {
		writeError(w, http.StatusServiceUnavailable, "research settings are not available")
		return false
	}
	return true
}

// GetResearchSettings (GET /api/research/settings) returns what is configured, without any key.
func (h *Handler) GetResearchSettings(w http.ResponseWriter, r *http.Request) {
	if !h.researchReady(w) {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := requireUserID(w, r); !ok {
		return
	}
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	ws, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	settings, err := h.Research.Get(r.Context(), ws)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load research settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// UpdateResearchSettings (PUT /api/research/settings) changes the keys and the contact address.
func (h *Handler) UpdateResearchSettings(w http.ResponseWriter, r *http.Request) {
	if !h.researchReady(w) {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := requireUserID(w, r); !ok {
		return
	}
	if isMachineCredentialActor(r) {
		writeError(w, http.StatusForbidden, "only an owner or admin may change research settings; agents cannot")
		return
	}
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}
	if !roleAllowed(member.Role, "owner", "admin") {
		writeError(w, http.StatusForbidden, "only an owner or admin may change research settings")
		return
	}
	ws, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	var req UpdateResearchSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	settings, err := h.Research.Update(r.Context(), ws, research.Update{
		OpenAlexKey: req.OpenAlexAPIKey, S2Key: req.SemanticScholarAPIKey, ContactEmail: req.ContactEmail,
	})
	var verr *research.ValidationError
	switch {
	case errors.As(err, &verr):
		writeError(w, http.StatusBadRequest, verr.Msg)
		return
	case errors.Is(err, research.ErrNotConfigured):
		writeErrorCode(w, http.StatusServiceUnavailable, "research_secret_not_configured", err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "failed to save research settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// GetResearchCredentials (GET /api/research/credentials) returns the keys for `multica research`.
func (h *Handler) GetResearchCredentials(w http.ResponseWriter, r *http.Request) {
	if !h.researchReady(w) {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := requireUserID(w, r); !ok {
		return
	}
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}
	// X-Actor-Source is stamped by the auth middleware and cannot be forged by a client. An agent's task
	// token is bound to one workspace and one agent; a Cloud node credential is not an agent task, so it
	// is treated like the human it acts for.
	agentTask := r.Header.Get("X-Actor-Source") == "task_token" && r.Header.Get("X-Agent-ID") != ""
	if !agentTask && !roleAllowed(member.Role, "owner", "admin") {
		writeError(w, http.StatusForbidden, "only an owner, an admin or an agent task may read research credentials")
		return
	}
	ws, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	creds, err := h.Research.Credentials(r.Context(), ws)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load research credentials")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, creds)
}
