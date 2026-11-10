package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// Per-item update routes (contract P5): check, file diff, apply and fork. The
// {id} in these routes is the skill or plugin package id (the ref id). Only an
// item that tracks a source can be checked or applied; a forked item keeps its
// content and no longer follows the source.

const (
	hubUpdateOnOverwrite = "overwrite"
	hubUpdateOnFork      = "fork"
)

// HubUpdateCheckResponse is the body of GET /api/hub/items/{kind}/{id}/update.
// current_digest is the digest the item was last imported or applied from.
type HubUpdateCheckResponse struct {
	UpdateAvailable bool            `json:"update_available"`
	CurrentDigest   string          `json:"current_digest"`
	RemoteDigest    string          `json:"remote_digest"`
	LocalModified   bool            `json:"local_modified"`
	Files           []HubUpdateFile `json:"files"`
}

// HubUpdateDiffResponse is the body of GET .../update/diff. Old is the local
// copy, New the remote. Both are null when the file is binary or over 1 MiB.
type HubUpdateDiffResponse struct {
	Path   string  `json:"path"`
	Old    *string `json:"old"`
	New    *string `json:"new"`
	Binary bool    `json:"binary"`
}

// HubUpdateApplyRequest is the body of POST .../update/apply. OnLocalChanges is
// required when the item has local changes: overwrite replaces them with the
// remote version, fork keeps them and detaches the item from its source.
type HubUpdateApplyRequest struct {
	OnLocalChanges  string `json:"on_local_changes"`
	ConfirmWarnings bool   `json:"confirm_warnings"`
}

// HubItemVersionResponse is the body of apply and fork.
type HubItemVersionResponse struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Version  string `json:"version,omitempty"`
	Tracking string `json:"tracking"`
}

// hubItemRoute parses the kind and ref id of an item route and checks that the
// caller is a member of the workspace the request names.
func (h *Handler) hubItemRoute(w http.ResponseWriter, r *http.Request) (string, string, pgtype.UUID, bool) {
	kind := chi.URLParam(r, "kind")
	if kind != "skill" && kind != "plugin" {
		writeError(w, http.StatusBadRequest, "kind must be skill or plugin")
		return "", "", pgtype.UUID{}, false
	}
	refID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "id")
	if !ok {
		return "", "", pgtype.UUID{}, false
	}
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found"); !ok {
		return "", "", pgtype.UUID{}, false
	}
	return workspaceID, kind, refID, true
}

// requireHubItemEditor applies the edit rule of the item's kind: a skill is edited
// by its creator, an owner or an admin (the skill rule); a plugin is changed by an
// owner or an admin. It returns the item's name, for the activity feed.
func (h *Handler) requireHubItemEditor(w http.ResponseWriter, r *http.Request, workspaceID, kind string, refID pgtype.UUID) (string, bool) {
	ws := parseUUID(workspaceID)
	if kind == "skill" {
		skill, err := h.Queries.GetSkillInWorkspace(r.Context(), db.GetSkillInWorkspaceParams{ID: refID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "item not found")
			return "", false
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load the item")
			return "", false
		}
		if !h.canManageSkill(w, r, skill) {
			return "", false
		}
		return skill.Name, true
	}
	if _, ok := h.requireHubSourceAdmin(w, r, workspaceID); !ok {
		return "", false
	}
	pkg, err := h.Queries.GetWorkspacePluginPackage(r.Context(), db.GetWorkspacePluginPackageParams{WorkspaceID: ws, ID: refID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "item not found")
		return "", false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load the item")
		return "", false
	}
	return pkg.Name, true
}

// loadTrackedHubItem returns the item and the source it tracks. An item that does
// not track a source is refused with a 409 that names why.
func (h *Handler) loadTrackedHubItem(w http.ResponseWriter, r *http.Request, ws pgtype.UUID, kind string, refID pgtype.UUID) (db.HubItem, db.HubSource, bool) {
	item, err := h.hubItemForRef(r.Context(), ws, kind, refID)
	if errors.Is(err, errHubItemNotFound) {
		writeError(w, http.StatusNotFound, "item not found")
		return db.HubItem{}, db.HubSource{}, false
	}
	if err != nil {
		slog.Warn("hub update: load item failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to load the item")
		return db.HubItem{}, db.HubSource{}, false
	}
	if item.Tracking != "tracking" {
		writeErrorCode(w, http.StatusConflict, "not_tracked", "only an item that tracks a source has updates")
		return db.HubItem{}, db.HubSource{}, false
	}
	if !item.SourceID.Valid {
		writeErrorCode(w, http.StatusConflict, "no_source", "this item's source no longer exists")
		return db.HubItem{}, db.HubSource{}, false
	}
	source, err := h.Queries.GetHubSourceInWorkspace(r.Context(), db.GetHubSourceInWorkspaceParams{ID: item.SourceID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusConflict, "no_source", "this item's source no longer exists")
		return db.HubItem{}, db.HubSource{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load the source")
		return db.HubItem{}, db.HubSource{}, false
	}
	return item, source, true
}

// writeHubCheckFailure reports a failed remote check: a missing item is 404, a
// changed kind is 409, and a fetch failure takes the import's status and message.
func writeHubCheckFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, errHubItemNotFound):
		writeError(w, http.StatusNotFound, "item not found")
	case errors.Is(err, errHubKindChanged):
		writeErrorCode(w, http.StatusConflict, "kind_changed", err.Error())
	default:
		status, msg := hubImportFailure(r.Context(), err)
		slog.Warn("hub update: remote check failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, status, msg)
	}
}

// GetHubItemUpdate (GET /api/hub/items/{kind}/{id}/update) checks one item against
// its remote now and returns the file-level change list. Any member may read it.
func (h *Handler) GetHubItemUpdate(w http.ResponseWriter, r *http.Request) {
	workspaceID, kind, refID, ok := h.hubItemRoute(w, r)
	if !ok {
		return
	}
	ws := parseUUID(workspaceID)
	item, source, ok := h.loadTrackedHubItem(w, r, ws, kind, refID)
	if !ok {
		return
	}
	ctx := r.Context()
	check, err := h.hubCheckItem(ctx, ws, source, item)
	if err != nil {
		writeHubCheckFailure(w, r, err)
		return
	}
	if err := h.saveHubCheck(ctx, item, check); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record the check")
		return
	}
	writeJSON(w, http.StatusOK, HubUpdateCheckResponse{
		UpdateAvailable: check.updateAvailable,
		CurrentDigest:   item.SourceDigest,
		RemoteDigest:    check.remote.digest,
		LocalModified:   check.localModified,
		Files:           hubFileChanges(check.local, check.remote.files),
	})
}

// GetHubItemUpdateDiff (GET .../update/diff?path=) returns one file's local and
// remote content. The path must be a safe supporting-file path; a file present on
// neither side is 404.
func (h *Handler) GetHubItemUpdateDiff(w http.ResponseWriter, r *http.Request) {
	workspaceID, kind, refID, ok := h.hubItemRoute(w, r)
	if !ok {
		return
	}
	filePath := r.URL.Query().Get("path")
	if !validateFilePath(filePath) {
		writeError(w, http.StatusBadRequest, "invalid file path")
		return
	}
	ws := parseUUID(workspaceID)
	item, source, ok := h.loadTrackedHubItem(w, r, ws, kind, refID)
	if !ok {
		return
	}
	check, err := h.hubCheckItem(r.Context(), ws, source, item)
	if err != nil {
		writeHubCheckFailure(w, r, err)
		return
	}
	oldIdx, newIdx := hubFileIndex(check.local), hubFileIndex(check.remote.files)
	oldFile, hasOld := oldIdx[filePath]
	newFile, hasNew := newIdx[filePath]
	if !hasOld && !hasNew {
		writeError(w, http.StatusNotFound, "file not found in either version")
		return
	}
	resp := HubUpdateDiffResponse{Path: filePath}
	resp.Binary = (hasOld && hubDiffBinary(oldFile.Content)) || (hasNew && hubDiffBinary(newFile.Content))
	if !resp.Binary {
		if hasOld {
			s := string(oldFile.Content)
			resp.Old = &s
		}
		if hasNew {
			s := string(newFile.Content)
			resp.New = &s
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// ApplyHubItemUpdate (POST .../update/apply) brings an item up to its remote.
//
// The remote is fetched again here: nothing the client sent is trusted except the
// choice of on_local_changes. The fresh bundle is scanned, then written through the
// same paths the import uses: a skill is overwritten in place (its name and id are
// kept), and a plugin's new version is published. A plugin version that already
// exists with the same content is not published again.
func (h *Handler) ApplyHubItemUpdate(w http.ResponseWriter, r *http.Request) {
	workspaceID, kind, refID, ok := h.hubItemRoute(w, r)
	if !ok {
		return
	}
	name, ok := h.requireHubItemEditor(w, r, workspaceID, kind, refID)
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var req HubUpdateApplyRequest
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, hubImportBodyLimit)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	switch req.OnLocalChanges {
	case "", hubUpdateOnOverwrite, hubUpdateOnFork:
	default:
		writeError(w, http.StatusBadRequest, "on_local_changes must be overwrite or fork")
		return
	}
	ctx := r.Context()
	ws := parseUUID(workspaceID)
	item, source, ok := h.loadTrackedHubItem(w, r, ws, kind, refID)
	if !ok {
		return
	}
	if req.OnLocalChanges == hubUpdateOnFork {
		h.forkTrackedHubItem(w, r, item, name)
		return
	}
	if h.HubStorage == nil {
		writeError(w, http.StatusServiceUnavailable, "file storage is not configured")
		return
	}

	check, err := h.hubCheckItem(ctx, ws, source, item)
	if err != nil {
		writeHubCheckFailure(w, r, err)
		return
	}
	if check.localModified && req.OnLocalChanges == "" {
		writeErrorCode(w, http.StatusConflict, "local_changes", "this item has local changes; choose overwrite to replace them, or fork to keep them")
		return
	}
	if err := h.saveHubCheck(ctx, item, check); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record the check")
		return
	}
	if !check.updateAvailable {
		writeJSON(w, http.StatusOK, HubItemVersionResponse{
			Kind:     kind,
			ID:       uuidToString(refID),
			Version:  hubItemVersion(item.LatestVersion, item.BundleDigest),
			Tracking: item.Tracking,
		})
		return
	}

	scan := hub.Scan(check.remote.bundle)
	if scan.Status == hub.ScanBlocked {
		writeErrorCode(w, http.StatusConflict, "scan_blocked", "the new version failed the security scan and cannot be applied")
		return
	}
	if scan.Status == hub.ScanWarn && !req.ConfirmWarnings {
		writeErrorCode(w, http.StatusConflict, "warnings_unconfirmed", "the new version has warnings; confirm them to apply it")
		return
	}

	c := &hubCommit{
		workspaceID: workspaceID,
		ws:          ws,
		userID:      userID,
		user:        parseUUID(userID),
		detected:    check.remote.detected,
		bundle:      check.remote.bundle,
		zipBytes:    check.remote.zipBytes,
		scan:        scan,
		sourceID:    source.ID,
		meta: hubStagedMeta{
			SourceType:   source.Type,
			SourceURL:    source.Url,
			SourceRef:    check.remote.meta.SourceRef,
			BundleDigest: check.remote.digest,
		},
	}
	activity := hubActivityEntry{
		WorkspaceID: ws,
		Action:      hubActionUpdateApplied,
		Kind:        kind,
		RefID:       refID,
		ItemName:    name,
		SourceID:    source.ID,
		Detail: map[string]any{
			"from_digest":      item.SourceDigest,
			"to_digest":        check.remote.digest,
			"from_version":     item.LatestVersion,
			"on_local_changes": req.OnLocalChanges,
			"local_modified":   check.localModified,
		},
	}
	if kind == "skill" {
		h.applyHubSkillUpdate(w, r, c, item, check, activity)
		return
	}
	h.applyHubPluginUpdate(w, r, c, item, check, activity)
}

// applyHubSkillUpdate overwrites the skill with the fresh bundle, keeping its id
// and name, and records the item at the new digest.
func (h *Handler) applyHubSkillUpdate(w http.ResponseWriter, r *http.Request, c *hubCommit, item db.HubItem, check hubCheck, activity hubActivityEntry) {
	ctx := r.Context()
	refID := uuidToString(item.RefID)
	if !h.checkHubCommitQuota(ctx, w, c.ws, 0, c.bundle.TotalSize-item.SizeBytes) {
		return
	}
	content, config, files := hubSkillInputs(c)
	bundleKey, err := h.hubStoreBundle(ctx, c, "skill", refID)
	if err != nil {
		slog.Warn("hub update: bundle upload failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to store the bundle")
		return
	}
	resp, err := h.overwriteSkillWithFiles(ctx, skillOverwriteInput{
		WorkspaceID:   c.ws,
		TargetSkillID: item.RefID,
		UserID:        c.userID,
		ExpectedName:  check.name,
		// The caller checked the rule for editing this skill, so the overwrite's
		// own creator-only check is not applied a second time.
		AllowOverwrite: func(string, db.Skill) bool { return true },
		Description:    c.detected.Description,
		Content:        content,
		Config:         config,
		Files:          files,
		VersionSource:  skillVersionSourceRefresh,
	})
	if err != nil {
		status, reason := skillImportOverwriteFailure(err)
		if status == http.StatusInternalServerError {
			slog.Warn("hub update: overwrite skill failed", append(logger.RequestAttrs(r), "error", err)...)
		}
		writeError(w, status, reason)
		return
	}

	item, ok := h.recordHubUpdate(w, r, c, item, bundleKey, "")
	if !ok {
		return
	}
	actorType, actorID := h.resolveActor(r, c.userID, c.workspaceID)
	h.publish(protocol.EventSkillUpdated, c.workspaceID, actorType, actorID, map[string]any{"skill": resp})
	h.finishHubUpdate(w, r, activity, item, c.detected.Kind, hubItemVersion("", c.meta.BundleDigest))
}

// applyHubPluginUpdate publishes the fresh bundle as a new version of the plugin.
// Published versions are immutable, so a version string that already exists is
// accepted only when its content is the same; otherwise the upstream author must
// publish a new version.
func (h *Handler) applyHubPluginUpdate(w http.ResponseWriter, r *http.Request, c *hubCommit, item db.HubItem, check hubCheck, activity hubActivityEntry) {
	if !h.requirePluginsV1(w, r) {
		return
	}
	ctx := r.Context()
	refID := uuidToString(item.RefID)
	parsed := check.remote.plugin
	pkg, err := h.Queries.GetWorkspacePluginPackage(ctx, db.GetWorkspacePluginPackageParams{WorkspaceID: c.ws, ID: item.RefID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load the plugin")
		return
	}
	if parsed.Manifest.Key != pkg.PluginKey {
		writeErrorCode(w, http.StatusConflict, "key_changed", "the plugin's manifest key changed upstream; a key is the plugin's identity, so import it as a new plugin")
		return
	}
	version := parsed.Manifest.Version
	versions, err := h.Queries.ListPluginPackageVersions(ctx, pkg.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list published versions")
		return
	}
	publish := true
	for _, v := range versions {
		if v.Version != version {
			continue
		}
		if v.Digest != service.BundleDigest(*parsed) {
			writeErrorCode(w, http.StatusConflict, "version_conflict", "version "+version+" is already published with different content; the upstream author must publish a new version")
			return
		}
		publish = false
		break
	}

	if !h.checkHubCommitQuota(ctx, w, c.ws, 0, c.bundle.TotalSize-item.SizeBytes) {
		return
	}
	if publish {
		if _, err := h.PluginService.PublishBundle(ctx, c.ws, c.user, c.zipBytes); err != nil {
			writePluginError(w, err, "failed to publish the Plugin")
			return
		}
	}
	bundleKey, err := h.hubStoreBundle(ctx, c, "plugin", refID)
	if err != nil {
		slog.Warn("hub update: plugin bundle upload failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "the plugin was published but its bundle could not be stored")
		return
	}
	item, ok := h.recordHubUpdate(w, r, c, item, bundleKey, version)
	if !ok {
		return
	}
	h.finishHubUpdate(w, r, activity, item, c.detected.Kind, version)
}

// recordHubUpdate writes the item at the applied bundle and marks it up to date, in
// one transaction. Re-applying the same bundle is safe: the snapshot key is by
// digest and the version blob is idempotent.
func (h *Handler) recordHubUpdate(w http.ResponseWriter, r *http.Request, c *hubCommit, item db.HubItem, bundleKey, version string) (db.HubItem, bool) {
	ctx := r.Context()
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeHubCommitFailure(w, r, err)
		return db.HubItem{}, false
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	updated, err := h.recordHubCommitItem(ctx, qtx, c, item.RefID, version, bundleKey)
	if err != nil {
		writeHubCommitFailure(w, r, err)
		return db.HubItem{}, false
	}
	if err := qtx.UpdateHubItemSyncState(ctx, db.UpdateHubItemSyncStateParams{
		UpdateAvailable: false,
		RemoteDigest:    c.meta.BundleDigest,
		LocalModified:   false,
		ID:              updated.ID,
		WorkspaceID:     updated.WorkspaceID,
	}); err != nil {
		writeHubCommitFailure(w, r, err)
		return db.HubItem{}, false
	}
	if err := tx.Commit(ctx); err != nil {
		writeHubCommitFailure(w, r, err)
		return db.HubItem{}, false
	}
	return updated, true
}

// finishHubUpdate records the apply in the activity feed and answers with the new version.
func (h *Handler) finishHubUpdate(w http.ResponseWriter, r *http.Request, activity hubActivityEntry, item db.HubItem, kind hub.Kind, version string) {
	activity.Detail["to_version"] = version
	h.recordHubActivity(r.Context(), r, activity)
	writeJSON(w, http.StatusOK, HubItemVersionResponse{
		Kind:     string(kind),
		ID:       uuidToString(item.RefID),
		Version:  version,
		Tracking: item.Tracking,
	})
}

// forkTrackedHubItem detaches a tracking item from its source. The content is not
// touched; the item stops following the source and gets no further update checks.
func (h *Handler) forkTrackedHubItem(w http.ResponseWriter, r *http.Request, item db.HubItem, name string) {
	ctx := r.Context()
	n, err := h.Queries.MarkHubItemForked(ctx, db.MarkHubItemForkedParams{ID: item.ID, WorkspaceID: item.WorkspaceID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fork the item")
		return
	}
	if n == 0 {
		// Another request changed the tracking first. Answer with the state it left.
		current, err := h.Queries.GetHubItemByRef(ctx, db.GetHubItemByRefParams{WorkspaceID: item.WorkspaceID, Kind: item.Kind, RefID: item.RefID})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fork the item")
			return
		}
		if current.Tracking != "forked" {
			writeErrorCode(w, http.StatusConflict, "not_tracked", "only an item that tracks a source can be forked")
			return
		}
		item = current
	} else {
		h.recordHubActivity(ctx, r, hubActivityEntry{
			WorkspaceID: item.WorkspaceID,
			Action:      hubActionItemForked,
			Kind:        item.Kind,
			RefID:       item.RefID,
			ItemName:    name,
			SourceID:    item.SourceID,
		})
		item.Tracking = "forked"
	}
	writeJSON(w, http.StatusOK, HubItemVersionResponse{
		Kind:     item.Kind,
		ID:       uuidToString(item.RefID),
		Tracking: item.Tracking,
	})
}

// ForkHubItem (POST /api/hub/items/{kind}/{id}/fork) detaches a tracking item from
// its source and keeps its content. Forking an item that is already forked is a
// no-op. A manual item has no source to detach from, so it is refused.
func (h *Handler) ForkHubItem(w http.ResponseWriter, r *http.Request) {
	workspaceID, kind, refID, ok := h.hubItemRoute(w, r)
	if !ok {
		return
	}
	name, ok := h.requireHubItemEditor(w, r, workspaceID, kind, refID)
	if !ok {
		return
	}
	ws := parseUUID(workspaceID)
	item, err := h.hubItemForRef(r.Context(), ws, kind, refID)
	if errors.Is(err, errHubItemNotFound) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load the item")
		return
	}
	switch item.Tracking {
	case "forked":
		writeJSON(w, http.StatusOK, HubItemVersionResponse{Kind: kind, ID: uuidToString(refID), Tracking: "forked"})
		return
	case "tracking":
	default:
		writeErrorCode(w, http.StatusConflict, "not_tracked", "only an item that tracks a source can be forked")
		return
	}
	h.forkTrackedHubItem(w, r, item, name)
}
