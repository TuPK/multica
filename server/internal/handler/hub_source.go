package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// Hub sources are the places skills and plugins are pulled from (spec 5.2).
// Every route is workspace-scoped; reads need membership, writes need owner or
// admin, and agents never manage sources.

const hubSourceNameMaxRunes = 160

// hubSourceNetworkTypes are fetched from a URL. The upload type has no URL: its
// content arrives through the import routes.
var hubSourceNetworkTypes = map[string]bool{
	"github":    true,
	"skills_sh": true,
	"clawhub":   true,
	"url_zip":   true,
	"registry":  true,
}

var hubSourceTypes = map[string]bool{
	"github":    true,
	"skills_sh": true,
	"clawhub":   true,
	"url_zip":   true,
	"registry":  true,
	"upload":    true,
}

type HubSourceResponse struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Type                string         `json:"type"`
	URL                 string         `json:"url"`
	Config              map[string]any `json:"config"`
	Enabled             bool           `json:"enabled"`
	AutoSyncIntervalSec int32          `json:"auto_sync_interval_sec"`
	LastSyncAt          *string        `json:"last_sync_at"`
	LastSyncStatus      string         `json:"last_sync_status"`
	LastSyncError       string         `json:"last_sync_error"`
	ItemCount           int64          `json:"item_count"`
	CreatedAt           string         `json:"created_at"`
	UpdatedAt           string         `json:"updated_at"`
}

type CreateHubSourceRequest struct {
	Name                string         `json:"name"`
	Type                string         `json:"type"`
	URL                 string         `json:"url"`
	Config              map[string]any `json:"config"`
	Enabled             *bool          `json:"enabled"`
	AutoSyncIntervalSec *int32         `json:"auto_sync_interval_sec"`
}

// UpdateHubSourceRequest is a partial update. The type is fixed at creation:
// a source's URL and type are its identity, and the import routes rely on it.
type UpdateHubSourceRequest struct {
	Name                *string         `json:"name"`
	Type                *string         `json:"type"`
	URL                 *string         `json:"url"`
	Config              *map[string]any `json:"config"`
	Enabled             *bool           `json:"enabled"`
	AutoSyncIntervalSec *int32          `json:"auto_sync_interval_sec"`
}

// hubSourceInUse names an item whose skill or plugin is still used, so a
// source removal that would delete it is refused.
type hubSourceInUse struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

func hubSourceToResponse(s db.HubSource, itemCount int64) HubSourceResponse {
	return HubSourceResponse{
		ID:                  uuidToString(s.ID),
		Name:                s.Name,
		Type:                s.Type,
		URL:                 s.Url,
		Config:              hubSourceConfig(s.Config),
		Enabled:             s.Enabled,
		AutoSyncIntervalSec: s.AutoSyncIntervalSec,
		LastSyncAt:          timestampToPtr(s.LastSyncAt),
		LastSyncStatus:      s.LastSyncStatus,
		LastSyncError:       s.LastSyncError,
		ItemCount:           itemCount,
		CreatedAt:           timestampToString(s.CreatedAt),
		UpdatedAt:           timestampToString(s.UpdatedAt),
	}
}

func hubSourceConfig(raw []byte) map[string]any {
	config := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &config)
	}
	if config == nil {
		return map[string]any{}
	}
	return config
}

// validateHubSourceFields returns the trimmed name and the URL, or a message
// for a 400. The URL must be an http(s) URL for network types and empty for upload.
func validateHubSourceFields(sourceType, name, rawURL string, autoSyncIntervalSec int32) (string, string, string) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > hubSourceNameMaxRunes {
		return "", "", "name must be 1 to 160 characters"
	}
	if !hubSourceTypes[sourceType] {
		return "", "", "invalid source type"
	}
	if autoSyncIntervalSec < 0 {
		return "", "", "auto_sync_interval_sec must not be negative"
	}
	rawURL = strings.TrimSpace(rawURL)
	if sourceType == "upload" {
		if rawURL != "" {
			return "", "", "upload sources have no url"
		}
		return name, "", ""
	}
	if rawURL == "" {
		return "", "", "url is required for this source type"
	}
	if len(rawURL) > 2048 {
		return "", "", "url must be 2048 characters or fewer"
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", "", "url must be an http or https URL"
	}
	return name, rawURL, ""
}

// requireHubSourceAdmin gates writes: owner or admin, and never an agent.
func (h *Handler) requireHubSourceAdmin(w http.ResponseWriter, r *http.Request, workspaceID string) (db.Member, bool) {
	member, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin")
	if !ok {
		return db.Member{}, false
	}
	if actorType, _ := h.resolveActor(r, requestUserID(r), workspaceID); actorType == "agent" {
		writeError(w, http.StatusForbidden, "agents cannot manage skill sources")
		return db.Member{}, false
	}
	return member, true
}

// loadHubSource resolves a source id within the request's workspace. Another
// workspace's source is reported as not found.
func (h *Handler) loadHubSource(w http.ResponseWriter, r *http.Request, workspaceID, id string) (db.HubSource, bool) {
	sourceID, ok := parseUUIDOrBadRequest(w, id, "id")
	if !ok {
		return db.HubSource{}, false
	}
	source, err := h.Queries.GetHubSourceInWorkspace(r.Context(), db.GetHubSourceInWorkspaceParams{
		ID:          sourceID,
		WorkspaceID: parseUUID(workspaceID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "source not found")
		return db.HubSource{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load source")
		return db.HubSource{}, false
	}
	return source, true
}

// ListHubSources (GET /api/hub/sources) lists the workspace's sources with the
// number of items each one produced.
func (h *Handler) ListHubSources(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found"); !ok {
		return
	}
	rows, err := h.Queries.ListHubSourcesWithItemCount(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sources")
		return
	}
	resp := make([]HubSourceResponse, len(rows))
	for i, row := range rows {
		resp[i] = hubSourceToResponse(db.HubSource{
			ID:                  row.ID,
			WorkspaceID:         row.WorkspaceID,
			Name:                row.Name,
			Type:                row.Type,
			Url:                 row.Url,
			Config:              row.Config,
			Enabled:             row.Enabled,
			AutoSyncIntervalSec: row.AutoSyncIntervalSec,
			LastSyncAt:          row.LastSyncAt,
			LastSyncStatus:      row.LastSyncStatus,
			LastSyncError:       row.LastSyncError,
			CreatedBy:           row.CreatedBy,
			CreatedAt:           row.CreatedAt,
			UpdatedAt:           row.UpdatedAt,
		}, row.ItemCount)
	}
	writeJSON(w, http.StatusOK, resp)
}

// CreateHubSource (POST /api/hub/sources) adds a source. A second source with
// the same type and URL is a conflict.
func (h *Handler) CreateHubSource(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	member, ok := h.requireHubSourceAdmin(w, r, workspaceID)
	if !ok {
		return
	}
	var req CreateHubSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var interval int32
	if req.AutoSyncIntervalSec != nil {
		interval = *req.AutoSyncIntervalSec
	}
	name, rawURL, msg := validateHubSourceFields(req.Type, req.Name, req.URL, interval)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	config, err := json.Marshal(nonNilConfig(req.Config))
	if err != nil {
		writeError(w, http.StatusBadRequest, "config must be a JSON object")
		return
	}

	source, err := h.Queries.CreateHubSource(r.Context(), db.CreateHubSourceParams{
		WorkspaceID:         parseUUID(workspaceID),
		Name:                name,
		Type:                req.Type,
		Url:                 rawURL,
		Config:              config,
		Enabled:             enabled,
		AutoSyncIntervalSec: interval,
		CreatedBy:           member.UserID,
	})
	if isUniqueViolation(err) {
		writeError(w, http.StatusConflict, "a source with this type and URL already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create source")
		return
	}
	h.recordHubActivity(r.Context(), r, hubActivityEntry{
		WorkspaceID: source.WorkspaceID,
		Action:      hubActionSourceCreated,
		ItemName:    source.Name,
		SourceID:    source.ID,
		Detail:      map[string]any{"type": source.Type, "url": source.Url},
	})
	writeJSON(w, http.StatusCreated, hubSourceToResponse(source, 0))
}

// UpdateHubSource (PATCH /api/hub/sources/{id}) changes a source's name, URL,
// config, enabled flag or sync interval.
func (h *Handler) UpdateHubSource(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.requireHubSourceAdmin(w, r, workspaceID); !ok {
		return
	}
	source, ok := h.loadHubSource(w, r, workspaceID, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var req UpdateHubSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Type != nil && *req.Type != source.Type {
		writeError(w, http.StatusBadRequest, "the source type cannot be changed")
		return
	}

	name, rawURL, config, enabled, interval := source.Name, source.Url, hubSourceConfig(source.Config), source.Enabled, source.AutoSyncIntervalSec
	if req.Name != nil {
		name = *req.Name
	}
	if req.URL != nil {
		rawURL = *req.URL
	}
	if req.Config != nil {
		config = *req.Config
	}
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.AutoSyncIntervalSec != nil {
		interval = *req.AutoSyncIntervalSec
	}
	name, rawURL, msg := validateHubSourceFields(source.Type, name, rawURL, interval)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	configJSON, err := json.Marshal(nonNilConfig(config))
	if err != nil {
		writeError(w, http.StatusBadRequest, "config must be a JSON object")
		return
	}

	updated, err := h.Queries.UpdateHubSource(r.Context(), db.UpdateHubSourceParams{
		ID:                  source.ID,
		WorkspaceID:         source.WorkspaceID,
		Name:                name,
		Url:                 rawURL,
		Config:              configJSON,
		Enabled:             enabled,
		AutoSyncIntervalSec: interval,
	})
	if isUniqueViolation(err) {
		writeError(w, http.StatusConflict, "a source with this type and URL already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update source")
		return
	}
	count, err := h.Queries.CountHubItemsBySource(r.Context(), db.CountHubItemsBySourceParams{
		WorkspaceID: updated.WorkspaceID,
		SourceID:    updated.ID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count source items")
		return
	}
	h.recordHubActivity(r.Context(), r, hubActivityEntry{
		WorkspaceID: updated.WorkspaceID,
		Action:      hubActionSourceUpdated,
		ItemName:    updated.Name,
		SourceID:    updated.ID,
		Detail:      map[string]any{"enabled": updated.Enabled, "auto_sync_interval_sec": updated.AutoSyncIntervalSec},
	})
	writeJSON(w, http.StatusOK, hubSourceToResponse(updated, count))
}

// DeleteHubSource (DELETE /api/hub/sources/{id}?items=keep|delete) removes a
// source. items=keep (the default) detaches its items: they stay, as manual
// entries. items=delete removes the skills and plugins it produced, except when
// any of them is still in use, which is a 409 listing them and deletes nothing.
func (h *Handler) DeleteHubSource(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.requireHubSourceAdmin(w, r, workspaceID); !ok {
		return
	}
	source, ok := h.loadHubSource(w, r, workspaceID, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	mode := r.URL.Query().Get("items")
	switch mode {
	case "", "keep":
		h.removeHubSourceKeepingItems(w, r, source)
	case "delete":
		h.removeHubSourceWithItems(w, r, source)
	default:
		writeError(w, http.StatusBadRequest, "items must be keep or delete")
	}
}

func (h *Handler) removeHubSourceKeepingItems(w http.ResponseWriter, r *http.Request, source db.HubSource) {
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.Queries.WithTx(tx)
	if err := qtx.DetachHubItemsBySource(r.Context(), db.DetachHubItemsBySourceParams{
		WorkspaceID: source.WorkspaceID,
		SourceID:    source.ID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to detach source items")
		return
	}
	if err := qtx.DeleteHubSource(r.Context(), db.DeleteHubSourceParams{
		ID:          source.ID,
		WorkspaceID: source.WorkspaceID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete source")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit source deletion")
		return
	}
	h.recordHubSourceDeleted(r, source, "keep")
	w.WriteHeader(http.StatusNoContent)
}

// recordHubSourceDeleted writes the source_deleted entry. Best effort, like every
// activity write: the deletion has already happened.
func (h *Handler) recordHubSourceDeleted(r *http.Request, source db.HubSource, items string) {
	h.recordHubActivity(r.Context(), r, hubActivityEntry{
		WorkspaceID: source.WorkspaceID,
		Action:      hubActionSourceDeleted,
		ItemName:    source.Name,
		SourceID:    source.ID,
		Detail:      map[string]any{"items": items},
	})
}

// removeHubSourceWithItems deletes the source's items with it. The in-use check
// runs before anything is deleted, so a refusal changes nothing. Plugins are
// removed through the plugin service, which has its own transaction; each one
// drops its hub row as it goes, so a failure partway leaves a source that a retry
// finishes. Skills and the remaining rows go in one transaction.
func (h *Handler) removeHubSourceWithItems(w http.ResponseWriter, r *http.Request, source db.HubSource) {
	ctx := r.Context()
	items, err := h.Queries.ListHubItemsBySource(ctx, db.ListHubItemsBySourceParams{
		WorkspaceID: source.WorkspaceID,
		SourceID:    source.ID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list source items")
		return
	}
	var skillIDs, pluginIDs []pgtype.UUID
	for _, item := range items {
		switch item.Kind {
		case "skill":
			skillIDs = append(skillIDs, item.RefID)
		case "plugin":
			pluginIDs = append(pluginIDs, item.RefID)
		}
	}

	inUse := []hubSourceInUse{}
	if len(skillIDs) > 0 {
		used, err := h.Queries.ListSkillsInUseByIDs(ctx, db.ListSkillsInUseByIDsParams{
			WorkspaceID: source.WorkspaceID,
			SkillIds:    skillIDs,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to check skill usage")
			return
		}
		for _, s := range used {
			inUse = append(inUse, hubSourceInUse{Kind: "skill", ID: uuidToString(s.ID), Name: s.Name})
		}
	}
	if len(pluginIDs) > 0 {
		used, err := h.Queries.ListPluginPackagesInUseByIDs(ctx, db.ListPluginPackagesInUseByIDsParams{
			WorkspaceID: source.WorkspaceID,
			PackageIds:  pluginIDs,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to check plugin usage")
			return
		}
		for _, p := range used {
			inUse = append(inUse, hubSourceInUse{Kind: "plugin", ID: uuidToString(p.ID), Name: p.Name})
		}
	}
	if len(inUse) > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":  "some items are still in use; remove them from agents and squads first",
			"in_use": inUse,
		})
		return
	}

	for _, item := range items {
		if item.Kind != "plugin" {
			continue
		}
		if err := h.PluginService.DeletePackage(ctx, source.WorkspaceID, uuidToString(item.RefID)); err != nil {
			writePluginError(w, err, "failed to delete the published Plugin")
			return
		}
		if err := h.Queries.DeleteHubItem(ctx, db.DeleteHubItemParams{
			ID:          item.ID,
			WorkspaceID: source.WorkspaceID,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to remove plugin item")
			return
		}
	}

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	deletedSkillIDs := make([]string, 0, len(skillIDs))
	for _, skillID := range skillIDs {
		// The same statements as DeleteSkill, batched into this transaction.
		if err := qtx.DeleteSkillLabelAssignmentsBySkill(ctx, skillID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to remove skill label assignments")
			return
		}
		if err := qtx.DeleteSkill(ctx, db.DeleteSkillParams{ID: skillID, WorkspaceID: source.WorkspaceID}); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete skill")
			return
		}
		deletedSkillIDs = append(deletedSkillIDs, uuidToString(skillID))
	}
	if err := qtx.DeleteHubItemsBySource(ctx, db.DeleteHubItemsBySourceParams{
		WorkspaceID: source.WorkspaceID,
		SourceID:    source.ID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete source items")
		return
	}
	if err := qtx.DeleteHubSource(ctx, db.DeleteHubSourceParams{
		ID:          source.ID,
		WorkspaceID: source.WorkspaceID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete source")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit source deletion")
		return
	}

	actorType, actorID := h.resolveActor(r, requestUserID(r), uuidToString(source.WorkspaceID))
	for _, skillID := range deletedSkillIDs {
		h.publish(protocol.EventSkillDeleted, uuidToString(source.WorkspaceID), actorType, actorID, map[string]any{"skill_id": skillID})
	}
	h.recordHubSourceDeleted(r, source, "delete")
	w.WriteHeader(http.StatusNoContent)
}

// nonNilConfig turns an absent config into an empty object, so the column always
// holds a JSON object.
func nonNilConfig(config map[string]any) map[string]any {
	if config == nil {
		return map[string]any{}
	}
	return config
}
