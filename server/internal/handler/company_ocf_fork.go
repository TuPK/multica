package handler

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/service"
)

// Open Company Format (durable E2). The company export and import endpoints hand a
// request to these functions when it asks for the package format: a GET with
// format=ocf, or a POST whose body is a zip. Both are behind durable_open; a
// request without the flag, or without the format, takes the legacy path unchanged.

// ocfImportBodyLimit caps an uploaded package. It is the service's own limit,
// plus room for the zip framing.
const ocfImportBodyLimit = 65 << 20

// serveOCFExport answers a company export asked for in the package format. It
// returns false when the request is for the legacy JSON export.
func (h *Handler) serveOCFExport(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Query().Get("format") != "ocf" {
		return false
	}
	if !featureflags.DurableOpenEnabled(r.Context(), h.FeatureFlags) {
		writeError(w, http.StatusNotFound, "not found")
		return true
	}
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.companyActorFor(w, r, workspaceID); !ok {
		return true
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return true
	}
	opts := service.ExportOptions{History: r.URL.Query().Get("history") == "true", Now: time.Now()}
	pkg, err := service.ExportOCF(r.Context(), h.Queries, wsUUID, opts)
	if err != nil {
		slog.Warn("export ocf failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "failed to export the company")
		return true
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="company.ocf.zip"`)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, pkg)
	return true
}

// importOCFRequested imports a package uploaded as a zip body and returns true. A
// request whose body is not a zip returns false, and the legacy JSON import takes it.
//
//	POST /api/company/import?dry_run=true&runtime_id=... (Content-Type: application/zip)
func (h *Handler) importOCFRequested(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Content-Type") != "application/zip" {
		return false
	}
	if !featureflags.DurableOpenEnabled(r.Context(), h.FeatureFlags) {
		writeError(w, http.StatusNotFound, "not found")
		return true
	}
	workspaceID := h.resolveWorkspaceID(r)
	actor, ok := h.companyActorFor(w, r, workspaceID)
	if !ok {
		return true
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return true
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, ocfImportBodyLimit))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "the package is too large")
		return true
	}
	opts := service.ImportOptions{
		DryRun:       r.URL.Query().Get("dry_run") == "true",
		ActorID:      actor.ID,
		Kinds:        h.proposalService().Kinds,
		Issues:       h.IssueService,
		Entitlements: h.Entitlements,
	}
	if raw := r.URL.Query().Get("runtime_id"); raw != "" {
		runtimeID, ok := parseUUIDOrBadRequest(w, raw, "runtime_id")
		if !ok {
			return true
		}
		opts.RuntimeID = runtimeID
	}
	report, err := service.ImportOCF(r.Context(), h.Queries, h.TxStarter, wsUUID, bytes.NewReader(body), int64(len(body)), opts)
	if err != nil {
		if writePlanLimitReached(w, err) {
			return true
		}
		slog.Warn("import ocf failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID)...)
		writeError(w, http.StatusBadRequest, err.Error())
		return true
	}
	writeJSON(w, http.StatusOK, report)
	return true
}
