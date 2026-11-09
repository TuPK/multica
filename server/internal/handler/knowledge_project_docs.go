package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/service"
)

// projectDocsMaxBody bounds an upload: a batch of Markdown files.
const projectDocsMaxBody = 32 << 20

// ListDaemonProjectDocSources (GET /api/daemon/workspaces/{workspaceId}/knowledge/project-sources)
// lists the local directories of the workspace's live projects. It is empty
// while knowledge is not ready.
func (h *Handler) ListDaemonProjectDocSources(w http.ResponseWriter, r *http.Request) {
	workspaceID := chi.URLParam(r, "workspaceId")
	if !h.requireDaemonWorkspaceAccess(w, r, workspaceID) {
		return
	}
	sources, err := h.Knowledge.ProjectDocSources(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeKnowledgeError(w, r, workspaceID, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": sources})
}

type projectDocsRequest struct {
	Files []service.ProjectDocFile `json:"files"`
}

// PostDaemonProjectDocsManifest (POST /api/daemon/workspaces/{workspaceId}/projects/{projectId}/knowledge/manifest)
// takes the complete file list of a project directory and answers with the
// paths whose text the server needs.
func (h *Handler) PostDaemonProjectDocsManifest(w http.ResponseWriter, r *http.Request) {
	workspaceID := chi.URLParam(r, "workspaceId")
	if !h.requireDaemonWorkspaceAccess(w, r, workspaceID) {
		return
	}
	projectID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "projectId"), "project id")
	if !ok {
		return
	}
	var req projectDocsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	need, err := h.Knowledge.ProjectDocsManifest(r.Context(), parseUUID(workspaceID), projectID, req.Files)
	if err != nil {
		writeKnowledgeError(w, r, workspaceID, err)
		return
	}
	if need == nil {
		need = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"need": need})
}

// PostDaemonProjectDocs (POST /api/daemon/workspaces/{workspaceId}/projects/{projectId}/knowledge/docs)
// writes uploaded files into the project's knowledge base.
func (h *Handler) PostDaemonProjectDocs(w http.ResponseWriter, r *http.Request) {
	workspaceID := chi.URLParam(r, "workspaceId")
	if !h.requireDaemonWorkspaceAccess(w, r, workspaceID) {
		return
	}
	projectID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "projectId"), "project id")
	if !ok {
		return
	}
	var req projectDocsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, projectDocsMaxBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := h.Knowledge.IngestProjectDocs(r.Context(), parseUUID(workspaceID), projectID, req.Files)
	if err != nil {
		writeKnowledgeError(w, r, workspaceID, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
