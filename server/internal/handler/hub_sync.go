package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/logger"
	skillpkg "github.com/multica-ai/multica/server/internal/skill"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/plugincontract"
)

// Source sync and item update checks (spec 5.6, contract P5).
//
// A tracking item's remote is refetched through the same importer and the same
// SafeFetcher the import used, from the source's URL and the ref the item was
// fetched at. Three digests describe an item:
//
//   - source_digest: the raw digest of the remote bundle at the last import or
//     apply. "Update available" is a fresh raw digest that differs from it.
//   - the bundle snapshot (bundle_key): that same remote bundle, stored. Local
//     edits are judged against it, through the canonical layout below.
//   - the canonical digest: the files a skill or plugin actually holds (a skill is
//     SKILL.md plus its text supporting files, a plugin is its manifest plus its
//     files). A remote bundle and a local copy are compared in this layout, so a
//     binary asset or a path prefix that import drops is not reported as a change.

const (
	hubSyncConcurrency = 4
	hubSyncDeadline    = 3 * time.Minute
	hubDiffMaxBytes    = 1 << 20

	hubChangeUpdate    = "update_available"
	hubChangeUnchanged = "unchanged"
	hubChangeError     = "error"

	hubTriggerManual = "manual"
	hubTriggerAuto   = "auto"
)

// errHubKindChanged means the remote bundle is now a different kind (skill or plugin)
// from the item it was imported as.
var errHubKindChanged = errors.New("hub: the remote now provides a different kind of item")

// hubSourceIsDue reports whether an enabled source with an auto-sync interval has
// waited at least that interval since its last sync. The sweep query in SQL
// (ListDueHubSources) implements the same rule.
func hubSourceIsDue(enabled bool, intervalSec int32, lastSyncAt *time.Time, now time.Time) bool {
	if !enabled || intervalSec <= 0 {
		return false
	}
	if lastSyncAt == nil {
		return true
	}
	return !now.Before(lastSyncAt.Add(time.Duration(intervalSec) * time.Second))
}

// hubRemote is a freshly fetched bundle, judged the way a stored item is judged.
type hubRemote struct {
	bundle   *hub.Bundle
	meta     hub.SourceMeta
	detected hub.Detected
	// digest is hub.Digest of the raw normalized bundle: the figure an import records.
	digest   string
	zipBytes []byte
	// files is the canonical layout a local copy holds (see the file comment).
	files []hub.FileEntry
	// plugin is the parsed package for a plugin bundle; nil for a skill.
	plugin *plugincontract.Bundle
}

// hubRemoteFromBundle detects a fetched bundle and lays it out canonically. A
// plugin must also parse as a package, since that is what a publish stores.
func hubRemoteFromBundle(bundle *hub.Bundle, meta hub.SourceMeta) (hubRemote, error) {
	detected, err := hub.Detect(bundle)
	if err != nil {
		return hubRemote{}, err
	}
	zipBytes, err := hub.BuildZip(bundle)
	if err != nil {
		return hubRemote{}, fmt.Errorf("build zip: %w", err)
	}
	remote := hubRemote{
		bundle:   bundle,
		meta:     meta,
		detected: detected,
		digest:   hub.Digest(bundle),
		zipBytes: zipBytes,
	}
	switch detected.Kind {
	case hub.KindSkill:
		content, files := hubSkillParts(bundle, detected.SkillMDPath)
		remote.files = []hub.FileEntry{bundleFileEntry(skillpkg.ContentFilename, []byte(content))}
		for _, f := range files {
			remote.files = append(remote.files, bundleFileEntry(f.Path, []byte(f.Content)))
		}
	case hub.KindPlugin:
		parsed, err := plugincontract.ParseBundle(zipBytes)
		if err != nil {
			return hubRemote{}, fmt.Errorf("%w: %v", hub.ErrInvalidManifest, err)
		}
		remote.plugin = &parsed
		remote.files = hubPluginFiles(parsed.Canonical, parsedPluginFiles(parsed))
	}
	return remote, nil
}

// parsedPluginFiles adapts a parsed package's files to hub entries.
func parsedPluginFiles(parsed plugincontract.Bundle) []hub.FileEntry {
	out := make([]hub.FileEntry, 0, len(parsed.Files))
	for _, f := range parsed.Files {
		out = append(out, bundleFileEntry(f.Path, f.Content))
	}
	return out
}

// hubPluginFiles is a plugin version's canonical layout: the canonical manifest, then
// the package files. Shared by the local and the remote side, so they compare equal.
func hubPluginFiles(manifest []byte, files []hub.FileEntry) []hub.FileEntry {
	out := []hub.FileEntry{bundleFileEntry(plugincontract.ManifestFilename, manifest)}
	for _, f := range files {
		if f.Path == plugincontract.ManifestFilename {
			continue
		}
		out = append(out, f)
	}
	return out
}

// hubFetchRemote refetches an item's bundle from its source, the way the import
// fetched it. sourceRef is the ref the item was fetched at.
func (h *Handler) hubFetchRemote(ctx context.Context, source db.HubSource, sourceRef string) (hubRemote, error) {
	importer, ref, err := hubImporterFor(hub.SourceRef{Type: hub.SourceType(source.Type), URL: source.Url, Ref: sourceRef})
	if err != nil {
		return hubRemote{}, err
	}
	bundle, meta, err := importer.Fetch(ctx, hubImportFetcher, ref)
	if err != nil {
		return hubRemote{}, err
	}
	return hubRemoteFromBundle(bundle, meta)
}

// hubLocal is an item's current local copy in the canonical layout.
type hubLocal struct {
	name  string
	files []hub.FileEntry
}

// hubLoadLocal reads the item's current skill or the newest published plugin version.
func (h *Handler) hubLoadLocal(ctx context.Context, ws pgtype.UUID, item db.HubItem) (hubLocal, error) {
	switch item.Kind {
	case "skill":
		skill, err := h.Queries.GetSkillInWorkspace(ctx, db.GetSkillInWorkspaceParams{ID: item.RefID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			return hubLocal{}, errHubItemNotFound
		}
		if err != nil {
			return hubLocal{}, err
		}
		rows, err := h.Queries.ListSkillFiles(ctx, skill.ID)
		if err != nil {
			return hubLocal{}, err
		}
		return hubLocal{name: skill.Name, files: hubSkillBundleFiles(skill.Content, rows)}, nil
	case "plugin":
		pkg, err := h.Queries.GetWorkspacePluginPackage(ctx, db.GetWorkspacePluginPackageParams{WorkspaceID: ws, ID: item.RefID})
		if errors.Is(err, pgx.ErrNoRows) {
			return hubLocal{}, errHubItemNotFound
		}
		if err != nil {
			return hubLocal{}, err
		}
		versions, err := h.Queries.ListLatestPluginPackageVersionsByIDs(ctx, db.ListLatestPluginPackageVersionsByIDsParams{
			WorkspaceID: ws,
			PackageIds:  []pgtype.UUID{item.RefID},
		})
		if err != nil {
			return hubLocal{}, err
		}
		if len(versions) == 0 {
			return hubLocal{}, errHubItemNotFound
		}
		rows, err := h.Queries.ListHubPluginVersionFiles(ctx, versions[0].ID)
		if err != nil {
			return hubLocal{}, err
		}
		return hubLocal{name: pkg.Name, files: hubPluginBundleFiles(versions[0].Manifest, rows)}, nil
	}
	return hubLocal{}, errHubItemNotFound
}

// hubBaseline returns the canonical layout of the bundle the item was last imported
// or applied from. ok is false when no such snapshot is recorded (an item created
// from the database, or a snapshot that cannot be read).
func (h *Handler) hubBaseline(ctx context.Context, item db.HubItem) ([]hub.FileEntry, bool) {
	if item.BundleKey == "" || item.SourceDigest == "" || h.HubStorage == nil {
		return nil, false
	}
	rc, err := h.HubStorage.GetReader(ctx, item.BundleKey)
	if err != nil {
		slog.Warn("hub: snapshot unreadable; comparing with the remote instead", "item_id", uuidToString(item.ID), "error", err)
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(rc, hub.DefaultLimits.MaxArchiveBytes+1))
	rc.Close()
	if err != nil {
		return nil, false
	}
	bundle, err := hub.ExtractZip(bytes.NewReader(data), int64(len(data)), hub.DefaultLimits)
	if err != nil {
		slog.Warn("hub: snapshot is not a bundle; comparing with the remote instead", "item_id", uuidToString(item.ID), "error", err)
		return nil, false
	}
	base, err := hubRemoteFromBundle(bundle, hub.SourceMeta{})
	if err != nil {
		return nil, false
	}
	return base.files, true
}

// hubCheck is the outcome of checking one item against its remote.
type hubCheck struct {
	name            string
	remote          hubRemote
	local           []hub.FileEntry
	updateAvailable bool
	localModified   bool
}

// hubCheckItem refetches an item's remote and compares it with the local copy and
// the recorded snapshot. It writes nothing; callers persist the result.
func (h *Handler) hubCheckItem(ctx context.Context, ws pgtype.UUID, source db.HubSource, item db.HubItem) (hubCheck, error) {
	remote, err := h.hubFetchRemote(ctx, source, item.SourceRef)
	if err != nil {
		return hubCheck{}, err
	}
	if string(remote.detected.Kind) != item.Kind {
		return hubCheck{}, errHubKindChanged
	}
	local, err := h.hubLoadLocal(ctx, ws, item)
	if err != nil {
		return hubCheck{}, err
	}
	check := hubCheck{name: local.name, remote: remote, local: local.files}
	localDigest := hub.Digest(&hub.Bundle{Files: local.files})
	if base, ok := h.hubBaseline(ctx, item); ok {
		check.localModified = localDigest != hub.Digest(&hub.Bundle{Files: base})
		check.updateAvailable = remote.digest != item.SourceDigest
		return check, nil
	}
	// Without a snapshot the local copy and the remote are the only two versions
	// there are. Any difference is an update or a local change, and both ask for
	// a choice before anything is overwritten.
	differs := localDigest != hub.Digest(&hub.Bundle{Files: remote.files})
	check.localModified = differs
	check.updateAvailable = differs
	return check, nil
}

// saveHubCheck records a check's result on the item.
func (h *Handler) saveHubCheck(ctx context.Context, item db.HubItem, check hubCheck) error {
	return h.Queries.UpdateHubItemSyncState(ctx, db.UpdateHubItemSyncStateParams{
		UpdateAvailable: check.updateAvailable,
		RemoteDigest:    check.remote.digest,
		LocalModified:   check.localModified,
		ID:              item.ID,
		WorkspaceID:     item.WorkspaceID,
	})
}

// hubCheckMessage is the client-safe reason an item check failed.
func hubCheckMessage(ctx context.Context, err error) string {
	switch {
	case errors.Is(err, errHubItemNotFound):
		return "the item no longer exists"
	case errors.Is(err, errHubKindChanged):
		return err.Error()
	}
	_, msg := hubImportFailure(ctx, err)
	return msg
}

// HubSyncItemResponse is one item's line in a sync result.
type HubSyncItemResponse struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	Change string `json:"change"`
	Error  string `json:"error,omitempty"`
}

// HubSourceSyncResponse is the body of POST /api/hub/sources/{id}/sync. Checked
// counts every item the sync attempted, so checked = updates_available + unchanged
// + the items that errored.
type HubSourceSyncResponse struct {
	Status           string                `json:"status"`
	Checked          int                   `json:"checked"`
	UpdatesAvailable int                   `json:"updates_available"`
	Unchanged        int                   `json:"unchanged"`
	Error            string                `json:"error,omitempty"`
	Items            []HubSyncItemResponse `json:"items"`
}

// syncHubSourceItems checks every tracking item of a source against its remote,
// with bounded concurrency and an overall deadline. One item's failure is its own
// error line; it never stops the others. The caller holds the source's sync lock.
func (h *Handler) syncHubSourceItems(ctx context.Context, source db.HubSource) (HubSourceSyncResponse, error) {
	items, err := h.Queries.ListTrackedHubItemsBySource(ctx, db.ListTrackedHubItemsBySourceParams{
		WorkspaceID: source.WorkspaceID,
		SourceID:    source.ID,
	})
	if err != nil {
		return HubSourceSyncResponse{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, hubSyncDeadline)
	defer cancel()

	results := make([]HubSyncItemResponse, len(items))
	sem := make(chan struct{}, hubSyncConcurrency)
	var wg sync.WaitGroup
	for i, item := range items {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			results[i] = HubSyncItemResponse{Kind: item.Kind, ID: uuidToString(item.RefID), Change: hubChangeError,
				Error: "the sync deadline passed before this item was checked"}
			continue
		}
		wg.Add(1)
		go func(i int, item db.HubItem) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = h.syncHubItem(ctx, source, item)
		}(i, item)
	}
	wg.Wait()

	resp := HubSourceSyncResponse{Status: "ok", Checked: len(results), Items: results}
	failed := 0
	for _, r := range results {
		switch r.Change {
		case hubChangeUpdate:
			resp.UpdatesAvailable++
		case hubChangeUnchanged:
			resp.Unchanged++
		default:
			failed++
		}
	}
	if failed > 0 {
		resp.Status = "error"
		resp.Error = fmt.Sprintf("%d of %d items could not be checked", failed, len(results))
	}
	if err := h.Queries.UpdateHubSourceSyncState(ctx, db.UpdateHubSourceSyncStateParams{
		Status:      resp.Status,
		Error:       resp.Error,
		ID:          source.ID,
		WorkspaceID: source.WorkspaceID,
	}); err != nil {
		return HubSourceSyncResponse{}, fmt.Errorf("record source sync: %w", err)
	}
	return resp, nil
}

// syncHubItem checks one item and records the result. A panic in the check is
// reported as that item's error, so it cannot take the other items down.
func (h *Handler) syncHubItem(ctx context.Context, source db.HubSource, item db.HubItem) (res HubSyncItemResponse) {
	res = HubSyncItemResponse{Kind: item.Kind, ID: uuidToString(item.RefID)}
	defer func() {
		if p := recover(); p != nil {
			slog.Error("hub sync: item check panicked", "item_id", uuidToString(item.ID), "panic", p)
			res.Change = hubChangeError
			res.Error = "the item could not be checked"
		}
	}()
	itemCtx, cancel := context.WithTimeout(ctx, hubImportFetchTimeout)
	defer cancel()
	check, err := h.hubCheckItem(itemCtx, source.WorkspaceID, source, item)
	if err != nil {
		res.Change = hubChangeError
		res.Error = hubCheckMessage(itemCtx, err)
		return res
	}
	res.Name = check.name
	if err := h.saveHubCheck(ctx, item, check); err != nil {
		slog.Warn("hub sync: save check failed", "item_id", uuidToString(item.ID), "error", err)
		res.Change = hubChangeError
		res.Error = "the check could not be recorded"
		return res
	}
	res.Change = hubChangeUnchanged
	if check.updateAvailable {
		res.Change = hubChangeUpdate
	}
	return res
}

// hubSourceSyncEntry is the activity entry for a source sync.
func hubSourceSyncEntry(source db.HubSource, resp HubSourceSyncResponse, trigger string) hubActivityEntry {
	return hubActivityEntry{
		WorkspaceID: source.WorkspaceID,
		Action:      hubActionSourceSynced,
		ItemName:    source.Name,
		SourceID:    source.ID,
		Detail: map[string]any{
			"trigger":           trigger,
			"status":            resp.Status,
			"checked":           resp.Checked,
			"updates_available": resp.UpdatesAvailable,
			"unchanged":         resp.Unchanged,
			"errors":            resp.Checked - resp.UpdatesAvailable - resp.Unchanged,
		},
	}
}

// SyncHubSource (POST /api/hub/sources/{id}/sync) checks every tracking item of a
// source now. Owners and admins only. A sync already running for the same source
// (here or on another instance) is refused with sync_in_progress rather than queued.
func (h *Handler) SyncHubSource(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.requireHubSourceAdmin(w, r, workspaceID); !ok {
		return
	}
	source, ok := h.loadHubSource(w, r, workspaceID, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start sync")
		return
	}
	// The advisory lock is held by this transaction; ending it releases the lock.
	defer tx.Rollback(ctx)
	locked, err := h.Queries.WithTx(tx).TryHubSourceSyncLock(ctx, uuidToString(source.ID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start sync")
		return
	}
	if !locked {
		writeErrorCode(w, http.StatusConflict, "sync_in_progress", "this source is already being synced")
		return
	}
	resp, err := h.syncHubSourceItems(ctx, source)
	if err != nil {
		slog.Warn("hub sync failed", append(logger.RequestAttrs(r), "source_id", uuidToString(source.ID), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to sync the source")
		return
	}
	h.recordHubActivity(ctx, r, hubSourceSyncEntry(source, resp, hubTriggerManual))
	writeJSON(w, http.StatusOK, resp)
}

// SyncHubSourceAuto runs one scheduled sync of a source, if it is still due once
// the lock is held. A sync already running is skipped: the next sweep retries it
// while the source stays due. Only a setup failure is returned as an error.
func (h *Handler) SyncHubSourceAuto(ctx context.Context, sourceID string) error {
	id, err := util.ParseUUID(sourceID)
	if err != nil {
		return fmt.Errorf("hub auto sync: invalid source id: %w", err)
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("hub auto sync: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	locked, err := h.Queries.WithTx(tx).TryHubSourceSyncLock(ctx, uuidToString(id))
	if err != nil {
		return fmt.Errorf("hub auto sync: lock: %w", err)
	}
	if !locked {
		return nil
	}
	// Read under the lock: a manual sync may have just finished this source.
	source, err := h.Queries.GetHubSourceByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("hub auto sync: load source: %w", err)
	}
	var lastSync *time.Time
	if source.LastSyncAt.Valid {
		t := source.LastSyncAt.Time
		lastSync = &t
	}
	if !hubSourceIsDue(source.Enabled, source.AutoSyncIntervalSec, lastSync, time.Now()) {
		return nil
	}
	resp, err := h.syncHubSourceItems(ctx, source)
	if err != nil {
		return fmt.Errorf("hub auto sync: %w", err)
	}
	h.recordHubSystemActivity(ctx, hubSourceSyncEntry(source, resp, hubTriggerAuto))
	return nil
}

// recordHubSystemActivity writes an activity entry for work no person started, such
// as a scheduled sync. Like recordHubActivity it is best effort.
func (h *Handler) recordHubSystemActivity(ctx context.Context, e hubActivityEntry) {
	detail := e.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		slog.Warn("hub activity: encode detail failed", "action", e.Action, "error", err)
		return
	}
	var kind pgtype.Text
	if e.Kind != "" {
		kind = pgtype.Text{String: e.Kind, Valid: true}
	}
	if err := h.Queries.InsertHubActivity(ctx, db.InsertHubActivityParams{
		WorkspaceID: e.WorkspaceID,
		ActorType:   "system",
		Action:      e.Action,
		Kind:        kind,
		RefID:       e.RefID,
		ItemName:    e.ItemName,
		SourceID:    e.SourceID,
		Detail:      detailJSON,
	}); err != nil {
		slog.Warn("hub activity: record failed", "action", e.Action, "error", err)
	}
}

// markHubItemLocalModified flags a tracking item whose skill or plugin was edited
// here. Best effort: the next check recomputes the flag, so a failure only delays
// the notice, and the edit that triggered it has already been saved.
func (h *Handler) markHubItemLocalModified(ctx context.Context, ws pgtype.UUID, kind string, refID pgtype.UUID) {
	if err := h.Queries.MarkHubItemLocalModified(ctx, db.MarkHubItemLocalModifiedParams{
		WorkspaceID: ws,
		Kind:        kind,
		RefID:       refID,
	}); err != nil {
		slog.Warn("hub: mark local edit failed", "kind", kind, "ref_id", uuidToString(refID), "error", err)
	}
}

// hubDiffBinary reports whether a file is shown as binary: not UTF-8 text, or too
// large to show inline.
func hubDiffBinary(content []byte) bool {
	return len(content) > hubDiffMaxBytes || !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0
}

// hubFileIndex maps a file list by path.
func hubFileIndex(files []hub.FileEntry) map[string]hub.FileEntry {
	out := make(map[string]hub.FileEntry, len(files))
	for _, f := range files {
		out[f.Path] = f
	}
	return out
}

// HubUpdateFile is one line of an update check's file list.
type HubUpdateFile struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	OldSize *int64 `json:"old_size"`
	NewSize *int64 `json:"new_size"`
}

// hubFileChanges compares the local copy (old) with the remote (new), by path.
func hubFileChanges(local, remote []hub.FileEntry) []HubUpdateFile {
	oldByPath, newByPath := hubFileIndex(local), hubFileIndex(remote)
	paths := make([]string, 0, len(oldByPath)+len(newByPath))
	seen := map[string]bool{}
	for p := range oldByPath {
		seen[p] = true
		paths = append(paths, p)
	}
	for p := range newByPath {
		if !seen[p] {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)

	out := make([]HubUpdateFile, 0, len(paths))
	for _, p := range paths {
		o, hasOld := oldByPath[p]
		n, hasNew := newByPath[p]
		file := HubUpdateFile{Path: p}
		if hasOld {
			size := o.Size
			file.OldSize = &size
		}
		if hasNew {
			size := n.Size
			file.NewSize = &size
		}
		switch {
		case hasOld && !hasNew:
			file.Status = "removed"
		case !hasOld && hasNew:
			file.Status = "added"
		case bytes.Equal(o.Content, n.Content):
			file.Status = "unchanged"
		default:
			file.Status = "modified"
		}
		out = append(out, file)
	}
	return out
}
