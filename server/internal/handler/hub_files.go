package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	skillpkg "github.com/multica-ai/multica/server/internal/skill"
)

// Saving a skill file through the Hub (PUT /api/hub/items/skill/{ref_id}/files).
// The body names the etag the file was read with, and a save whose etag is stale
// is refused with 409 file_conflict, carrying the current etag. Everything else
// is the skill file route's own write (see saveSkillSupportingFile and
// saveSkillMarkdown in skill.go), so the activity feed and the local-edit flag
// behave the same whichever route a save takes.

// HubSkillFileSaveRequest is the body of PUT /api/hub/items/skill/{ref_id}/files.
// Etag is the updated_at the file was loaded with. "" means the file should not
// exist yet.
type HubSkillFileSaveRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Etag    string `json:"etag"`
}

// HubSkillFileSaveResponse is the saved file's path and its new etag.
type HubSkillFileSaveResponse struct {
	Path string `json:"path"`
	Etag string `json:"etag"`
}

// SaveHubItemFile handles PUT /api/hub/items/{kind}/{id}/files. Only skills have
// files a client writes; a plugin's files are read-only. The edit rule is the
// skill rule: its creator, an owner or an admin.
func (h *Handler) SaveHubItemFile(w http.ResponseWriter, r *http.Request) {
	if chi.URLParam(r, "kind") != "skill" {
		writeError(w, http.StatusBadRequest, "only skill files can be saved; plugin files are read-only")
		return
	}
	skill, ok := h.loadSkillForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	if !h.canManageSkill(w, r, skill) {
		return
	}

	var req HubSkillFileSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validateFilePath(req.Path) {
		writeError(w, http.StatusBadRequest, "invalid file path")
		return
	}

	if skillpkg.IsReservedContentPath(req.Path) {
		etag, ok := h.saveSkillMarkdown(w, r, skill, req.Content, req.Etag)
		if !ok {
			return
		}
		writeJSON(w, http.StatusOK, HubSkillFileSaveResponse{Path: skillpkg.ContentFilename, Etag: etag})
		return
	}
	expected := req.Etag
	sf, etag, ok := h.saveSkillSupportingFile(w, r, skill, req.Path, req.Content, &expected)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, HubSkillFileSaveResponse{Path: sf.Path, Etag: etag})
}
