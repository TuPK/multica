package handler

import (
	"net/http"

	"github.com/multica-ai/multica/server/internal/quickstart"
)

// ListSquadPluginCatalog lists the plugins of the team's quickstart bundle, the
// set every machine is asked to have. The squad picker offers them next to the
// plugins runtimes report, so a squad can require one that no runtime has yet;
// its leader then installs it on the project's runtime.
func (h *Handler) ListSquadPluginCatalog(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	bundle, err := quickstart.LoadBundle()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load the plugin catalog")
		return
	}
	out := make([]SquadPluginResponse, 0, len(bundle.Plugins))
	for _, id := range bundle.Plugins {
		out = append(out, SquadPluginResponse{Name: id, InstallHint: "claude plugin install " + id})
	}
	writeJSON(w, http.StatusOK, out)
}
