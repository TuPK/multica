package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/logger"
	skillpkg "github.com/multica-ai/multica/server/internal/skill"
	"github.com/multica-ai/multica/server/internal/storage"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/plugincontract"
)

// Hub bundles (contract P4). An item's bundle is an immutable zip in HubStorage,
// recorded on its hub row (bundle_key, bundle_digest) and on a version row. An
// item committed through the import path has its bundle from the start. Any other
// skill or plugin gets one the first time its bundle is requested, built from the
// files in the database. A later edit to the skill or plugin does not change a
// stored bundle.

// hubBundleURLTTL is how long a presigned bundle link stays valid.
const hubBundleURLTTL = 15 * time.Minute

// errHubItemNotFound means the skill or plugin named in the request is not in the workspace.
var errHubItemNotFound = errors.New("hub item not found")

// HubItemBundleResponse is the body of GET /api/hub/items/{kind}/{id}/bundle.
// SizeBytes is the bundle's content size, the same figure the item list reports.
type HubItemBundleResponse struct {
	URL       string `json:"url"`
	ExpiresIn int64  `json:"expires_in"`
	Digest    string `json:"digest"`
	SizeBytes int64  `json:"size_bytes"`
	Version   string `json:"version"`
}

// GetHubItemBundle handles GET /api/hub/items/{kind}/{id}/bundle. It answers with a
// presigned download link when the Hub store can sign one, and otherwise with the
// authenticated download route of the same item.
func (h *Handler) GetHubItemBundle(w http.ResponseWriter, r *http.Request) {
	item, ok := h.hubBundleForRequest(w, r)
	if !ok {
		return
	}
	url, err := h.hubBundleURL(r.Context(), item)
	if err != nil {
		slog.Warn("hub bundle: create link failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to create the bundle link")
		return
	}
	writeJSON(w, http.StatusOK, HubItemBundleResponse{
		URL:       url,
		ExpiresIn: int64(hubBundleURLTTL / time.Second),
		Digest:    item.BundleDigest,
		SizeBytes: item.SizeBytes,
		Version:   hubItemVersion(item.LatestVersion, item.BundleDigest),
	})
}

// DownloadHubItemBundle handles GET /api/hub/items/{kind}/{id}/bundle/download. It
// streams the stored zip through the server, for stores that cannot presign.
func (h *Handler) DownloadHubItemBundle(w http.ResponseWriter, r *http.Request) {
	item, ok := h.hubBundleForRequest(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	rc, err := h.HubStorage.GetReader(ctx, item.BundleKey)
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.Is(err, fs.ErrNotExist) || errors.As(err, &noSuchKey) {
			writeError(w, http.StatusNotFound, "bundle not found")
			return
		}
		slog.Warn("hub bundle: read failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to read the bundle")
		return
	}
	defer rc.Close()

	filename := fmt.Sprintf("%s-%s.zip", item.Kind, uuidToString(item.RefID))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", storage.ContentDisposition("application/zip", filename))
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, rc); err != nil {
		// The status is already sent; the client sees a truncated body.
		slog.Warn("hub bundle: stream failed", append(logger.RequestAttrs(r), "error", err)...)
	}
}

// hubBundleURL returns a presigned GET link to the stored bundle, or the item's
// download route when the store cannot presign.
func (h *Handler) hubBundleURL(ctx context.Context, item db.HubItem) (string, error) {
	if presigner, ok := h.HubStorage.(storage.Presigner); ok {
		url, err := presigner.PresignGet(ctx, item.BundleKey, hubBundleURLTTL)
		if err == nil {
			return url, nil
		}
		if !errors.Is(err, storage.ErrPresignUnsupported) {
			return "", err
		}
	}
	return fmt.Sprintf("/api/hub/items/%s/%s/bundle/download", item.Kind, uuidToString(item.RefID)), nil
}

// hubBundleForRequest checks the caller and the item, creates the item's hub row
// and bundle when they are missing, and returns the item with its bundle recorded.
// It writes the error response itself and reports false when it cannot.
func (h *Handler) hubBundleForRequest(w http.ResponseWriter, r *http.Request) (db.HubItem, bool) {
	kind := chi.URLParam(r, "kind")
	if kind != "skill" && kind != "plugin" {
		writeError(w, http.StatusBadRequest, "kind must be skill or plugin")
		return db.HubItem{}, false
	}
	refID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "id")
	if !ok {
		return db.HubItem{}, false
	}
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found"); !ok {
		return db.HubItem{}, false
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return db.HubItem{}, false
	}
	if h.HubStorage == nil {
		writeError(w, http.StatusServiceUnavailable, "file storage is not configured")
		return db.HubItem{}, false
	}
	ctx := r.Context()
	ws := parseUUID(workspaceID)
	user := parseUUID(userID)

	item, err := h.hubItemForRef(ctx, ws, kind, refID)
	switch {
	case errors.Is(err, errHubItemNotFound):
		writeError(w, http.StatusNotFound, "item not found")
		return db.HubItem{}, false
	case err != nil:
		slog.Warn("hub bundle: load item failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to load the item")
		return db.HubItem{}, false
	}
	if item.BundleKey == "" {
		item, err = h.buildHubBundle(ctx, ws, user, item)
		switch {
		case errors.Is(err, errHubItemNotFound):
			writeError(w, http.StatusNotFound, "item not found")
			return db.HubItem{}, false
		case err != nil:
			slog.Warn("hub bundle: build failed", append(logger.RequestAttrs(r), "error", err)...)
			writeError(w, http.StatusInternalServerError, "failed to build the bundle")
			return db.HubItem{}, false
		}
	}
	return item, true
}

// hubItemForRef returns the hub row of a skill or plugin in the workspace. The
// skill or package must still exist: a row left behind by a delete is not served,
// as the list does not show it. A missing row is created first.
// errHubItemNotFound means the skill or package is not in the workspace, or has
// no published version to bundle.
func (h *Handler) hubItemForRef(ctx context.Context, ws pgtype.UUID, kind string, refID pgtype.UUID) (db.HubItem, error) {
	switch kind {
	case "skill":
		skill, err := h.Queries.GetSkillInWorkspace(ctx, db.GetSkillInWorkspaceParams{ID: refID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			return db.HubItem{}, errHubItemNotFound
		}
		if err != nil {
			return db.HubItem{}, err
		}
		item, err := h.Queries.GetHubItemByRef(ctx, db.GetHubItemByRefParams{WorkspaceID: ws, Kind: kind, RefID: refID})
		if err == nil {
			return item, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return db.HubItem{}, err
		}
		if err := h.ensureSkillHubRows(ctx, ws, []hubSkillRef{{id: skill.ID, config: skill.Config}}); err != nil {
			return db.HubItem{}, err
		}
	case "plugin":
		if _, err := h.Queries.GetWorkspacePluginPackage(ctx, db.GetWorkspacePluginPackageParams{WorkspaceID: ws, ID: refID}); errors.Is(err, pgx.ErrNoRows) {
			return db.HubItem{}, errHubItemNotFound
		} else if err != nil {
			return db.HubItem{}, err
		}
		item, err := h.Queries.GetHubItemByRef(ctx, db.GetHubItemByRefParams{WorkspaceID: ws, Kind: kind, RefID: refID})
		if err == nil {
			return item, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return db.HubItem{}, err
		}
		if err := h.ensurePluginHubRows(ctx, ws, []pgtype.UUID{refID}); err != nil {
			return db.HubItem{}, err
		}
	default:
		return db.HubItem{}, errHubItemNotFound
	}
	item, err := h.Queries.GetHubItemByRef(ctx, db.GetHubItemByRefParams{WorkspaceID: ws, Kind: kind, RefID: refID})
	if errors.Is(err, pgx.ErrNoRows) {
		// The package has no published version, so there is nothing to list or bundle.
		return db.HubItem{}, errHubItemNotFound
	}
	return item, err
}

// buildHubBundle builds an item's bundle from the database, stores it and records
// it on the item and as a version row. The record is written only where no bundle
// is recorded yet, so when two requests race, the first one wins and the other
// serves what was recorded.
func (h *Handler) buildHubBundle(ctx context.Context, ws, user pgtype.UUID, item db.HubItem) (db.HubItem, error) {
	var files []hub.FileEntry
	var version string
	switch item.Kind {
	case "skill":
		skill, err := h.Queries.GetSkillInWorkspace(ctx, db.GetSkillInWorkspaceParams{ID: item.RefID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			return db.HubItem{}, errHubItemNotFound
		}
		if err != nil {
			return db.HubItem{}, err
		}
		rows, err := h.Queries.ListSkillFiles(ctx, skill.ID)
		if err != nil {
			return db.HubItem{}, err
		}
		files = hubSkillBundleFiles(skill.Content, rows)
	case "plugin":
		versions, err := h.Queries.ListLatestPluginPackageVersionsByIDs(ctx, db.ListLatestPluginPackageVersionsByIDsParams{
			WorkspaceID: ws,
			PackageIds:  []pgtype.UUID{item.RefID},
		})
		if err != nil {
			return db.HubItem{}, err
		}
		if len(versions) == 0 {
			return db.HubItem{}, errHubItemNotFound
		}
		latest := versions[0]
		rows, err := h.Queries.ListHubPluginVersionFiles(ctx, latest.ID)
		if err != nil {
			return db.HubItem{}, err
		}
		files = hubPluginBundleFiles(latest.Manifest, rows)
		version = latest.Version
	default:
		return db.HubItem{}, fmt.Errorf("unknown hub item kind %q", item.Kind)
	}

	bundle := &hub.Bundle{Files: files}
	for _, f := range files {
		bundle.TotalSize += f.Size
	}
	zipBytes, err := hub.BuildZip(bundle)
	if err != nil {
		return db.HubItem{}, fmt.Errorf("build zip: %w", err)
	}
	digest := hub.Digest(bundle)
	wsID := uuidToString(ws)
	refID := uuidToString(item.RefID)
	key := hubBundleKey(wsID, item.Kind, refID, digest)
	if _, err := h.HubStorage.Upload(ctx, key, zipBytes, "application/zip", ""); err != nil {
		return db.HubItem{}, fmt.Errorf("store bundle: %w", err)
	}

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return db.HubItem{}, err
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	recorded, err := qtx.UpdateHubItemBundleIfUnset(ctx, db.UpdateHubItemBundleIfUnsetParams{
		BundleKey:     key,
		BundleDigest:  digest,
		LatestVersion: version,
		SizeBytes:     bundle.TotalSize,
		FileCount:     int32(len(files)),
		ID:            item.ID,
		WorkspaceID:   ws,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Another request recorded its bundle first; serve that one.
		return h.Queries.GetHubItemByRef(ctx, db.GetHubItemByRefParams{WorkspaceID: ws, Kind: item.Kind, RefID: item.RefID})
	}
	if err != nil {
		return db.HubItem{}, err
	}
	if err := writeHubVersionBlob(ctx, qtx, recorded, user); err != nil {
		return db.HubItem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.HubItem{}, err
	}
	return recorded, nil
}

// hubSkillBundleFiles is a skill's bundle: SKILL.md from the skill content, then
// the supporting files, as the import path lays them out.
func hubSkillBundleFiles(content string, rows []db.SkillFile) []hub.FileEntry {
	files := []hub.FileEntry{bundleFileEntry(skillpkg.ContentFilename, []byte(content))}
	for _, row := range rows {
		if row.Path == skillpkg.ContentFilename {
			continue
		}
		files = append(files, bundleFileEntry(row.Path, []byte(row.Content)))
	}
	return files
}

// hubPluginBundleFiles is a plugin version's bundle: the manifest, which is stored
// on the version row and not as a file, then the package files.
func hubPluginBundleFiles(manifest []byte, rows []db.ListHubPluginVersionFilesRow) []hub.FileEntry {
	files := []hub.FileEntry{bundleFileEntry(plugincontract.ManifestFilename, manifest)}
	for _, row := range rows {
		if row.Path == plugincontract.ManifestFilename {
			continue
		}
		files = append(files, bundleFileEntry(row.Path, row.Content))
	}
	return files
}

func bundleFileEntry(path string, content []byte) hub.FileEntry {
	return hub.FileEntry{Path: path, Size: int64(len(content)), Content: content}
}
