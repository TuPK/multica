package handler

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/hub"
	skillpkg "github.com/multica-ai/multica/server/internal/skill"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/plugincontract"
)

// GET /api/hub/export streams every hub item of the workspace as one zip: a
// manifest.json, and each item's bundle as {kind}/{safe-name}.zip. Bundles are
// built from the database on each export, so the export does not depend on any
// stored bundle. Only owners and admins may export; the bundles hold skill text
// and plugin code, which is what a source-management role is trusted with.

// hubExportBundleMaxBytes caps one item's uncompressed content. An item over it
// is left out and logged, and the rest of the export still streams. A variable
// so a test can lower it.
var hubExportBundleMaxBytes int64 = 64 << 20

type hubExportItem struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
	Path    string `json:"path"`
}

type hubExportManifest struct {
	ExportedAt  string          `json:"exported_at"`
	WorkspaceID string          `json:"workspace_id"`
	Items       []hubExportItem `json:"items"`
}

// hubExportBuild is one item's bundle, zipped, with the fields the manifest lists.
type hubExportBuild struct {
	name    string
	version string
	digest  string
	zip     []byte
}

// ExportHubItems (GET /api/hub/export) streams the workspace's hub items as a zip.
// The manifest is written last, so it lists exactly the items that were written.
// Once the first byte is out a failure cannot change the status, so an item that
// cannot be built is logged and skipped; a failed write to the client ends the
// stream and is not recorded.
func (h *Handler) ExportHubItems(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.requireHubSourceAdmin(w, r, workspaceID); !ok {
		return
	}
	ws := parseUUID(workspaceID)
	items, err := h.Queries.ListHubItemsByWorkspace(ctx, ws)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list hub items")
		return
	}

	now := time.Now().UTC()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="hub-export-%s.zip"`, now.Format("20060102-150405")))
	w.WriteHeader(http.StatusOK)

	zw := zip.NewWriter(w)
	flusher, _ := w.(http.Flusher)
	names := newHubExportNames()
	manifest := hubExportManifest{
		ExportedAt:  now.Format(time.RFC3339),
		WorkspaceID: workspaceID,
		Items:       []hubExportItem{},
	}
	for _, item := range items {
		built, err := h.buildHubExportItem(ctx, ws, item)
		if err != nil {
			slog.Warn("hub export: skipping item", "kind", item.Kind, "ref_id", uuidToString(item.RefID), "error", err)
			continue
		}
		name := names.assign(item.Kind, built.name, uuidToString(item.RefID))
		path := item.Kind + "/" + name + ".zip"
		entry, err := zw.Create(path)
		if err != nil {
			return
		}
		if _, err := entry.Write(built.zip); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
		manifest.Items = append(manifest.Items, hubExportItem{
			Kind:    item.Kind,
			Name:    built.name,
			Version: built.version,
			Digest:  built.digest,
			Path:    path,
		})
	}

	manifestEntry, err := zw.Create("manifest.json")
	if err != nil {
		return
	}
	if err := json.NewEncoder(manifestEntry).Encode(manifest); err != nil {
		return
	}
	if err := zw.Close(); err != nil {
		return
	}
	if flusher != nil {
		flusher.Flush()
	}
	h.recordHubActivity(ctx, r, hubActivityEntry{
		WorkspaceID: ws,
		Action:      hubActionItemExported,
		Detail:      map[string]any{"count": len(manifest.Items)},
	})
}

func (h *Handler) buildHubExportItem(ctx context.Context, ws pgtype.UUID, item db.HubItem) (hubExportBuild, error) {
	switch item.Kind {
	case "skill":
		return h.buildHubSkillExport(ctx, ws, item)
	case "plugin":
		return h.buildHubPluginExport(ctx, ws, item)
	}
	return hubExportBuild{}, fmt.Errorf("unknown item kind %q", item.Kind)
}

// buildHubSkillExport lays a skill out the way the import reads one: SKILL.md at
// the root, with the skill's other files beside it.
func (h *Handler) buildHubSkillExport(ctx context.Context, ws pgtype.UUID, item db.HubItem) (hubExportBuild, error) {
	skill, err := h.Queries.GetSkillInWorkspace(ctx, db.GetSkillInWorkspaceParams{ID: item.RefID, WorkspaceID: ws})
	if err != nil {
		return hubExportBuild{}, fmt.Errorf("load skill: %w", err)
	}
	files, err := h.Queries.ListSkillFiles(ctx, skill.ID)
	if err != nil {
		return hubExportBuild{}, fmt.Errorf("list skill files: %w", err)
	}
	bundle := &hub.Bundle{}
	hubExportAddFile(bundle, skillpkg.ContentFilename, []byte(skill.Content))
	for _, f := range files {
		hubExportAddFile(bundle, f.Path, []byte(f.Content))
	}
	return hubExportBundleOf(bundle, skill.Name, item.LatestVersion, "")
}

// buildHubPluginExport packs the newest published version of a plugin. The
// version's manifest is virtual unless a real file claims its path, so it is
// rendered through the plugin service, which owns that rule.
func (h *Handler) buildHubPluginExport(ctx context.Context, ws pgtype.UUID, item db.HubItem) (hubExportBuild, error) {
	pkg, err := h.Queries.GetWorkspacePluginPackage(ctx, db.GetWorkspacePluginPackageParams{WorkspaceID: ws, ID: item.RefID})
	if err != nil {
		return hubExportBuild{}, fmt.Errorf("load plugin: %w", err)
	}
	versions, err := h.Queries.ListPluginPackageVersions(ctx, pkg.ID)
	if err != nil {
		return hubExportBuild{}, fmt.Errorf("list plugin versions: %w", err)
	}
	if len(versions) == 0 {
		return hubExportBuild{}, errors.New("plugin has no published version")
	}
	version := versions[0] // ListPluginPackageVersions is newest first

	rows, err := h.Queries.ListPluginPackageFileContents(ctx, version.ID)
	if err != nil {
		return hubExportBuild{}, fmt.Errorf("list plugin files: %w", err)
	}
	bundle := &hub.Bundle{}
	hasManifest := false
	for _, row := range rows {
		if row.Path == plugincontract.ManifestFilename {
			hasManifest = true
		}
		hubExportAddFile(bundle, row.Path, row.Content)
	}
	if !hasManifest {
		manifest, err := h.PluginService.PackageVersionFile(ctx, ws, uuidToString(pkg.ID), uuidToString(version.ID), plugincontract.ManifestFilename)
		if err != nil {
			return hubExportBuild{}, fmt.Errorf("render plugin manifest: %w", err)
		}
		if manifest.Content == nil {
			return hubExportBuild{}, errors.New("plugin manifest is not text")
		}
		hubExportAddFile(bundle, plugincontract.ManifestFilename, []byte(*manifest.Content))
	}
	return hubExportBundleOf(bundle, pkg.Name, version.Version, version.Digest)
}

// hubExportAddFile appends one file and keeps the bundle's total size current.
func hubExportAddFile(bundle *hub.Bundle, path string, content []byte) {
	bundle.Files = append(bundle.Files, hub.FileEntry{Path: path, Size: int64(len(content)), Content: content})
	bundle.TotalSize += int64(len(content))
}

// hubExportBundleOf applies the size cap and zips the bundle. A plugin keeps the
// digest its publish recorded; a skill has none stored, so its digest is computed
// from the bundle.
func hubExportBundleOf(bundle *hub.Bundle, name, version, storedDigest string) (hubExportBuild, error) {
	if bundle.TotalSize > hubExportBundleMaxBytes {
		return hubExportBuild{}, fmt.Errorf("bundle is %d bytes, over the %d byte export cap", bundle.TotalSize, hubExportBundleMaxBytes)
	}
	zipped, err := hub.BuildZip(bundle)
	if err != nil {
		return hubExportBuild{}, fmt.Errorf("zip bundle: %w", err)
	}
	digest := storedDigest
	if digest == "" {
		digest = hub.Digest(bundle)
	}
	return hubExportBuild{name: name, version: version, digest: digest, zip: zipped}, nil
}

// hubExportNames gives each item a unique, slash-free file name within its kind.
type hubExportNames struct {
	used map[string]bool
}

func newHubExportNames() *hubExportNames {
	return &hubExportNames{used: map[string]bool{}}
}

// assign returns the file name (without extension) for an item. The first item
// with a given safe name keeps it; a later one with the same name gets its ref
// id appended, which is unique within the kind.
func (n *hubExportNames) assign(kind, name, refID string) string {
	base := hubExportSafeName(name)
	candidate := base
	if n.used[kind+"/"+candidate] {
		candidate = base + "-" + refID
	}
	n.used[kind+"/"+candidate] = true
	return candidate
}

// hubExportSafeName keeps ASCII letters, digits, '-', '_' and '.', turns every
// other rune into '-', and trims leading and trailing '-' and '.'. The result has
// no separator, so it cannot escape its folder, and no leading dot, so it is not
// hidden.
func hubExportSafeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	s := strings.Trim(b.String(), "-.")
	if len(s) > 80 {
		s = strings.Trim(s[:80], "-.")
	}
	if s == "" {
		return "item"
	}
	return s
}
