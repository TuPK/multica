package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/metrics"
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
// streams the stored zip through the server, for stores that cannot presign. The
// bytes are checked against the recorded digest before anything is sent, so a store
// that returns other bytes cannot pass them off as the item's bundle.
func (h *Handler) DownloadHubItemBundle(w http.ResponseWriter, r *http.Request) {
	item, ok := h.hubBundleForRequest(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	rc, err := h.HubStorage.GetReader(ctx, item.BundleKey)
	if err != nil {
		h.recordHubStorageError(metrics.HubStorageOpGet, err)
		var noSuchKey *types.NoSuchKey
		if errors.Is(err, fs.ErrNotExist) || errors.As(err, &noSuchKey) {
			writeError(w, http.StatusNotFound, "bundle not found")
			return
		}
		slog.Warn("hub bundle: read failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to read the bundle")
		return
	}
	data, err := io.ReadAll(io.LimitReader(rc, hubBundleReadLimit+1))
	rc.Close()
	if err != nil || int64(len(data)) > hubBundleReadLimit {
		slog.Warn("hub bundle: read failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to read the bundle")
		return
	}
	// The log carries digests only, never the bundle's content or its file names.
	if got := hubBundleContentDigest(data); got != item.BundleDigest {
		slog.Error("hub bundle: stored bytes do not match the recorded digest",
			append(logger.RequestAttrs(r),
				"item_id", uuidToString(item.ID), "kind", item.Kind, "bundle_key", item.BundleKey,
				"recorded_digest", item.BundleDigest, "stored_digest", got)...)
		writeError(w, http.StatusInternalServerError, "the stored bundle failed verification")
		return
	}

	filename := fmt.Sprintf("%s-%s.zip", item.Kind, uuidToString(item.RefID))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", storage.ContentDisposition("application/zip", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// hubBundleReadLimit bounds what a download reads from the store. A bundle is at
// most the archive cap the hub builds within.
var hubBundleReadLimit = hub.DefaultLimits.MaxArchiveBytes

// hubBundleVerifyLimits bound the re-read of a stored bundle. The bytes were built
// from content the hub already accepted, so the file caps stay as they are, but two
// limits differ from the import's. The file count is 1024, because a skill bundle
// can hold SKILL.md plus 512 supporting files. The compression ratio is loose,
// because a repetitive skill compresses past the import ratio, and the ratio check
// would refuse a bundle the hub itself wrote.
var hubBundleVerifyLimits = hub.Limits{
	MaxFiles:            1024,
	MaxFileBytes:        hub.DefaultLimits.MaxFileBytes,
	MaxTotalBytes:       hubBundleReadLimit,
	MaxArchiveBytes:     hubBundleReadLimit,
	MaxCompressionRatio: 1 << 16,
}

// hubBundleContentDigest is the content digest of a stored zip: the digest of the
// files it holds, the same figure hub.Digest gives the bundle it was built from.
// It returns "" when the bytes are not a readable bundle.
func hubBundleContentDigest(data []byte) string {
	bundle, err := hub.ExtractZip(bytes.NewReader(data), int64(len(data)), hubBundleVerifyLimits)
	if err != nil {
		return ""
	}
	return hub.Digest(bundle)
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
	item, err = h.currentHubBundle(ctx, ws, user, item)
	switch {
	case errors.Is(err, errHubItemNotFound):
		writeError(w, http.StatusNotFound, "item not found")
		return db.HubItem{}, false
	case err != nil:
		slog.Warn("hub bundle: build failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to build the bundle")
		return db.HubItem{}, false
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

// currentHubBundle returns the item with a bundle that matches its content. An item
// with no bundle gets its first one. A recorded bundle is kept while the content
// still hashes to the latest recorded version. An edited skill, or a plugin with a
// newer version, gets a new bundle, recorded as a new version. The bundle is
// rebuilt before it is served, so a download never returns stale content.
func (h *Handler) currentHubBundle(ctx context.Context, ws, user pgtype.UUID, item db.HubItem) (db.HubItem, error) {
	local, err := h.hubLoadLocal(ctx, ws, item)
	if err != nil {
		return db.HubItem{}, err
	}
	if item.BundleKey == "" {
		return h.recordHubBundle(ctx, ws, user, item, local, false)
	}
	recorded := item.BundleDigest
	latest, err := h.Queries.GetLatestHubItemVersionBlob(ctx, db.GetLatestHubItemVersionBlobParams{WorkspaceID: ws, ItemID: item.ID})
	switch {
	case err == nil:
		recorded = latest.Digest
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return db.HubItem{}, err
	}
	if hub.Digest(&hub.Bundle{Files: local.files}) == recorded {
		return item, nil
	}
	return h.recordHubBundle(ctx, ws, user, item, local, true)
}

// recordHubBundle zips the item's current content, stores it under its digest and
// records it on the item and as a version row, in one transaction. When replace is
// false the record is written only where no bundle is recorded yet, so when two
// requests race on a first build, the first one wins and the other serves what was
// recorded. When replace is true the record is overwritten.
func (h *Handler) recordHubBundle(ctx context.Context, ws, user pgtype.UUID, item db.HubItem, local hubLocal, replace bool) (db.HubItem, error) {
	recorded, err := h.recordHubBundleOnce(ctx, ws, user, item, local, replace)
	result := metrics.HubResultOK
	if err != nil {
		result = metrics.HubResultError
	}
	h.Metrics.RecordHubBundleBuild(result)
	return recorded, err
}

// recordHubBundleOnce builds an item's bundle, stores it and records it on the item.
func (h *Handler) recordHubBundleOnce(ctx context.Context, ws, user pgtype.UUID, item db.HubItem, local hubLocal, replace bool) (db.HubItem, error) {
	bundle := &hub.Bundle{Files: local.files}
	for _, f := range local.files {
		bundle.TotalSize += f.Size
	}
	zipBytes, err := hub.BuildZip(bundle)
	if err != nil {
		return db.HubItem{}, fmt.Errorf("build zip: %w", err)
	}
	digest := hub.Digest(bundle)
	key := hubBundleKey(uuidToString(ws), item.Kind, uuidToString(item.RefID), digest)
	if _, err := h.HubStorage.Upload(ctx, key, zipBytes, "application/zip", ""); err != nil {
		h.recordHubStorageError(metrics.HubStorageOpPut, err)
		return db.HubItem{}, fmt.Errorf("store bundle: %w", err)
	}

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return db.HubItem{}, err
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	var recorded db.HubItem
	if replace {
		recorded, err = qtx.UpdateHubItemBundle(ctx, db.UpdateHubItemBundleParams{
			BundleKey:     key,
			BundleDigest:  digest,
			LatestVersion: local.version,
			SizeBytes:     bundle.TotalSize,
			FileCount:     int32(len(local.files)),
			ID:            item.ID,
			WorkspaceID:   ws,
		})
	} else {
		recorded, err = qtx.UpdateHubItemBundleIfUnset(ctx, db.UpdateHubItemBundleIfUnsetParams{
			BundleKey:     key,
			BundleDigest:  digest,
			LatestVersion: local.version,
			SizeBytes:     bundle.TotalSize,
			FileCount:     int32(len(local.files)),
			ID:            item.ID,
			WorkspaceID:   ws,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// Another request recorded its bundle first; serve that one.
			return h.Queries.GetHubItemByRef(ctx, db.GetHubItemByRefParams{WorkspaceID: ws, Kind: item.Kind, RefID: item.RefID})
		}
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
// on the version row and not as a file, then the package files. The stored manifest
// is JSONB, which does not keep the byte layout the publish canonicalized, so it is
// canonicalized again. Without that, every plugin would read as locally modified
// against the bundle it was imported from.
func hubPluginBundleFiles(storedManifest []byte, rows []db.ListHubPluginVersionFilesRow) ([]hub.FileEntry, error) {
	_, manifest, err := plugincontract.ParseManifest(storedManifest)
	if err != nil {
		return nil, fmt.Errorf("stored plugin manifest: %w", err)
	}
	files := []hub.FileEntry{bundleFileEntry(plugincontract.ManifestFilename, manifest)}
	for _, row := range rows {
		if row.Path == plugincontract.ManifestFilename {
			continue
		}
		files = append(files, bundleFileEntry(row.Path, row.Content))
	}
	return files, nil
}

func bundleFileEntry(path string, content []byte) hub.FileEntry {
	return hub.FileEntry{Path: path, Size: int64(len(content)), Content: content}
}

// recordHubStorageError counts a failed Hub object-store operation. A missing
// object is an answer, not a failure of the store, so it is not counted.
func (h *Handler) recordHubStorageError(operation string, err error) {
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return
	}
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return
	}
	h.Metrics.RecordHubStorageError(operation)
}

// deleteHubObjects removes a deleted workspace's bundles and staged previews
// from the Hub store. Best effort: the rows are already gone, so a failure only
// leaves orphaned objects, which is logged rather than failing the delete.
func (h *Handler) deleteHubObjects(ctx context.Context, workspaceID string) {
	lister, ok := h.HubStorage.(storage.Lister)
	if !ok || h.HubStorage == nil {
		return
	}
	for _, prefix := range []string{"hub/" + workspaceID + "/", "staging/" + workspaceID + "/"} {
		objects, err := lister.ListPrefix(ctx, prefix)
		if err != nil {
			slog.Warn("list hub objects for deleted workspace failed", "error", err, "workspace_id", workspaceID, "prefix", prefix)
			continue
		}
		keys := make([]string, 0, len(objects))
		for _, object := range objects {
			keys = append(keys, object.Key)
		}
		if len(keys) > 0 {
			h.HubStorage.DeleteKeys(ctx, keys)
		}
	}
}

// removeHubItemForRef removes the hub row of a skill or plugin package that was
// deleted, with its version rows and their bundle objects. It runs after the delete
// has committed and is best effort: a failure is logged and the delete still
// succeeds. The list and the bundle route already ignore a row whose item is gone,
// so a leftover row is harmless.
func (h *Handler) removeHubItemForRef(ctx context.Context, ws pgtype.UUID, kind string, refID pgtype.UUID) {
	item, err := h.Queries.GetHubItemByRef(ctx, db.GetHubItemByRefParams{WorkspaceID: ws, Kind: kind, RefID: refID})
	if errors.Is(err, pgx.ErrNoRows) {
		return
	}
	if err != nil {
		slog.Warn("hub: load item of deleted ref failed", "error", err, "kind", kind, "ref_id", uuidToString(refID))
		return
	}
	keys, err := h.Queries.ListHubItemVersionBlobKeys(ctx, db.ListHubItemVersionBlobKeysParams{WorkspaceID: ws, ItemID: item.ID})
	if err != nil {
		slog.Warn("hub: list bundles of deleted ref failed", "error", err, "kind", kind, "ref_id", uuidToString(refID))
		return
	}
	if err := h.Queries.DeleteHubItemVersionBlobs(ctx, db.DeleteHubItemVersionBlobsParams{WorkspaceID: ws, ItemID: item.ID}); err != nil {
		slog.Warn("hub: delete versions of deleted ref failed", "error", err, "kind", kind, "ref_id", uuidToString(refID))
		return
	}
	if err := h.Queries.DeleteHubItem(ctx, db.DeleteHubItemParams{ID: item.ID, WorkspaceID: ws}); err != nil {
		slog.Warn("hub: delete item of deleted ref failed", "error", err, "kind", kind, "ref_id", uuidToString(refID))
		return
	}
	if h.HubStorage == nil {
		return
	}
	if item.BundleKey != "" && !slices.Contains(keys, item.BundleKey) {
		keys = append(keys, item.BundleKey)
	}
	if len(keys) > 0 {
		h.HubStorage.DeleteKeys(ctx, keys)
	}
}
