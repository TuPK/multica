package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Read-only file access for published plugin versions.
//
// The settings page browses what publish stored: the file list of a version,
// and one file's text. Nothing here writes, so a version stays immutable.

// ListPluginPackageVersionFiles — GET /api/workspaces/{id}/plugins/packages/{packageId}/versions/{versionId}/files
func (h *Handler) ListPluginPackageVersionFiles(w http.ResponseWriter, r *http.Request) {
	if !h.requirePluginsV1(w, r) {
		return
	}
	workspaceIDString := workspaceIDFromURL(r, "id")
	workspaceID, ok := parseUUIDOrBadRequest(w, workspaceIDString, "workspace_id")
	if !ok {
		return
	}
	if _, ok := h.workspaceMember(w, r, workspaceIDString); !ok {
		return
	}
	packageID, versionID, ok := pluginPackageVersionParams(w, r)
	if !ok {
		return
	}
	files, err := h.PluginService.ListPackageVersionFiles(r.Context(), workspaceID, packageID, versionID)
	if err != nil {
		writePluginError(w, err, "failed to list the Plugin's files")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

// GetPluginPackageVersionFileContent — GET /api/workspaces/{id}/plugins/packages/{packageId}/versions/{versionId}/files/content?path=<relative path>
func (h *Handler) GetPluginPackageVersionFileContent(w http.ResponseWriter, r *http.Request) {
	if !h.requirePluginsV1(w, r) {
		return
	}
	workspaceIDString := workspaceIDFromURL(r, "id")
	workspaceID, ok := parseUUIDOrBadRequest(w, workspaceIDString, "workspace_id")
	if !ok {
		return
	}
	if _, ok := h.workspaceMember(w, r, workspaceIDString); !ok {
		return
	}
	packageID, versionID, ok := pluginPackageVersionParams(w, r)
	if !ok {
		return
	}
	file, err := h.PluginService.PackageVersionFile(r.Context(), workspaceID, packageID, versionID, r.URL.Query().Get("path"))
	if err != nil {
		writePluginError(w, err, "failed to read the Plugin's file")
		return
	}
	writeJSON(w, http.StatusOK, file)
}

// pluginPackageVersionParams validates the two path ids that name a published
// version. Both are pure UUID inputs, so a malformed one is a 400 before any
// lookup runs.
func pluginPackageVersionParams(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	packageID := chi.URLParam(r, "packageId")
	if _, ok := parseUUIDOrBadRequest(w, packageID, "package_id"); !ok {
		return "", "", false
	}
	versionID := chi.URLParam(r, "versionId")
	if _, ok := parseUUIDOrBadRequest(w, versionID, "version_id"); !ok {
		return "", "", false
	}
	return packageID, versionID, true
}
