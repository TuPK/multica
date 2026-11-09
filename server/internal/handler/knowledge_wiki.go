package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// GetKnowledgeWiki (GET /api/knowledge/bases/{key}/wiki) reports a base's Wiki:
// whether it is on, how many pages it has and how many documents feed it.
func (h *Handler) GetKnowledgeWiki(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	out, err := h.Knowledge.WikiOverviewFor(r.Context(), parseUUID(workspaceID), h.knowledgeReadActor(r, workspaceID), chi.URLParam(r, "key"))
	if err != nil {
		writeKnowledgeError(w, r, workspaceID, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ListKnowledgeWikiPages (GET /api/knowledge/bases/{key}/wiki/pages?page=&page_size=&type=)
// lists a base's Wiki pages without their text.
func (h *Handler) ListKnowledgeWikiPages(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("page_size"))
	out, err := h.Knowledge.WikiPagesFor(r.Context(), parseUUID(workspaceID), h.knowledgeReadActor(r, workspaceID), chi.URLParam(r, "key"), page, size, q.Get("type"))
	if err != nil {
		writeKnowledgeError(w, r, workspaceID, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// GetKnowledgeWikiPage (GET /api/knowledge/bases/{key}/wiki/pages/{slug...})
// returns one Wiki page with its text. A slug has slashes, so it is the
// wildcard of the route.
func (h *Handler) GetKnowledgeWikiPage(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	out, err := h.Knowledge.WikiPageFor(r.Context(), parseUUID(workspaceID), h.knowledgeReadActor(r, workspaceID), chi.URLParam(r, "key"), chi.URLParam(r, "*"))
	if err != nil {
		writeKnowledgeError(w, r, workspaceID, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// GenerateKnowledgeWiki (POST /api/knowledge/bases/{key}/wiki/generate) turns
// Wiki on for a base and queues the pages of its documents. Owners and admins.
func (h *Handler) GenerateKnowledgeWiki(w http.ResponseWriter, r *http.Request) {
	workspaceID, key, ok := h.adminKnowledgeBaseKey(w, r)
	if !ok {
		return
	}
	out, err := h.Knowledge.GenerateWiki(r.Context(), parseUUID(workspaceID), key)
	if err != nil {
		writeKnowledgeAdminError(w, r, workspaceID, err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}
