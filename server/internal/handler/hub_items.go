package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/logger"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The Hub's item list (contract P4). Every skill and plugin package of a workspace
// appears in it. An item has a hub row only once something needs one, so the
// rows for existing skills and plugins are created here, on first sight, and
// never again. Bundles are not built here; GET .../bundle builds them on demand.

// HubItemResponse is one row of GET /api/hub/items. Fields that Phase 5 (source
// sync) owns are returned from their columns, which hold their defaults until it writes them.
type HubItemResponse struct {
	ID              string  `json:"id"`
	Kind            string  `json:"kind"`
	RefID           string  `json:"ref_id"`
	SourceID        *string `json:"source_id"`
	SourceRef       string  `json:"source_ref"`
	Tracking        string  `json:"tracking"`
	LatestVersion   string  `json:"latest_version"`
	BundleDigest    string  `json:"bundle_digest"`
	SizeBytes       int64   `json:"size_bytes"`
	FileCount       int32   `json:"file_count"`
	ScanStatus      string  `json:"scan_status"`
	UpdateAvailable bool    `json:"update_available"`
	RemoteDigest    string  `json:"remote_digest"`
	CheckedAt       *string `json:"checked_at"`
	LocalModified   bool    `json:"local_modified"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// hubTrackedOrigins are the skill origins (config.origin.type) that name a remote
// the item can be tracked against. Any other origin is hand-made or local, so the
// item is manual. Upload-origin skills are manual too, as hub import records them.
var hubTrackedOrigins = map[string]bool{
	"github":    true,
	"skills_sh": true,
	"clawhub":   true,
	"url_zip":   true,
	"registry":  true,
}

// hubSkillRef is what the hub needs from a skill to create its row.
type hubSkillRef struct {
	id     pgtype.UUID
	config []byte
}

// ListHubItems handles GET /api/hub/items.
func (h *Handler) ListHubItems(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found"); !ok {
		return
	}
	ctx := r.Context()
	ws := parseUUID(workspaceID)

	skills, err := h.Queries.ListSkillSummariesByWorkspace(ctx, ws)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list items")
		return
	}
	packages, err := h.Queries.ListWorkspacePluginPackages(ctx, ws)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list items")
		return
	}
	rows, err := h.Queries.ListHubItemsByWorkspace(ctx, ws)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list items")
		return
	}

	hasRow := make(map[string]bool, len(rows))
	for _, row := range rows {
		hasRow[row.Kind+":"+uuidToString(row.RefID)] = true
	}
	var missingSkills []hubSkillRef
	live := make(map[string]bool, len(skills)+len(packages))
	for _, skill := range skills {
		key := "skill:" + uuidToString(skill.ID)
		live[key] = true
		if !hasRow[key] {
			missingSkills = append(missingSkills, hubSkillRef{id: skill.ID, config: skill.Config})
		}
	}
	var missingPackages []pgtype.UUID
	for _, pkg := range packages {
		key := "plugin:" + uuidToString(pkg.ID)
		live[key] = true
		if !hasRow[key] {
			missingPackages = append(missingPackages, pkg.ID)
		}
	}

	if len(missingSkills) > 0 || len(missingPackages) > 0 {
		if err := h.ensureSkillHubRows(ctx, ws, missingSkills); err != nil {
			slog.Warn("hub items: create skill rows failed", append(logger.RequestAttrs(r), "error", err)...)
			writeError(w, http.StatusInternalServerError, "failed to list items")
			return
		}
		if err := h.ensurePluginHubRows(ctx, ws, missingPackages); err != nil {
			slog.Warn("hub items: create plugin rows failed", append(logger.RequestAttrs(r), "error", err)...)
			writeError(w, http.StatusInternalServerError, "failed to list items")
			return
		}
		rows, err = h.Queries.ListHubItemsByWorkspace(ctx, ws)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list items")
			return
		}
	}

	resp := make([]HubItemResponse, 0, len(rows))
	for _, row := range rows {
		// A row whose skill or plugin is gone is left over from a delete; it is not listed.
		if !live[row.Kind+":"+uuidToString(row.RefID)] {
			continue
		}
		resp = append(resp, hubItemToResponse(row))
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetHubItem handles GET /api/hub/items/{kind}/{id}: one item in the shape of an
// element of GET /api/hub/items. It applies the same lazy row creation, so an
// item that is listed is also found here, and one that is absent is a 404.
func (h *Handler) GetHubItem(w http.ResponseWriter, r *http.Request) {
	workspaceID, kind, refID, ok := h.hubItemRoute(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	ws := parseUUID(workspaceID)

	switch kind {
	case "skill":
		skill, err := h.Queries.GetSkillInWorkspace(ctx, db.GetSkillInWorkspaceParams{ID: refID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "item not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load item")
			return
		}
		if err := h.ensureSkillHubRows(ctx, ws, []hubSkillRef{{id: skill.ID, config: skill.Config}}); err != nil {
			slog.Warn("hub item: create skill row failed", append(logger.RequestAttrs(r), "error", err)...)
			writeError(w, http.StatusInternalServerError, "failed to load item")
			return
		}
	case "plugin":
		if _, err := h.Queries.GetWorkspacePluginPackage(ctx, db.GetWorkspacePluginPackageParams{WorkspaceID: ws, ID: refID}); errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "item not found")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load item")
			return
		}
		if err := h.ensurePluginHubRows(ctx, ws, []pgtype.UUID{refID}); err != nil {
			slog.Warn("hub item: create plugin row failed", append(logger.RequestAttrs(r), "error", err)...)
			writeError(w, http.StatusInternalServerError, "failed to load item")
			return
		}
	}

	row, err := h.Queries.GetHubItemByRef(ctx, db.GetHubItemByRefParams{WorkspaceID: ws, Kind: kind, RefID: refID})
	if errors.Is(err, pgx.ErrNoRows) {
		// A plugin package with no published version has no row, and no item.
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load item")
		return
	}
	writeJSON(w, http.StatusOK, hubItemToResponse(row))
}

func hubItemToResponse(row db.HubItem) HubItemResponse {
	var sourceID *string
	if row.SourceID.Valid {
		s := uuidToString(row.SourceID)
		sourceID = &s
	}
	return HubItemResponse{
		ID:              uuidToString(row.ID),
		Kind:            row.Kind,
		RefID:           uuidToString(row.RefID),
		SourceID:        sourceID,
		SourceRef:       row.SourceRef,
		Tracking:        row.Tracking,
		LatestVersion:   row.LatestVersion,
		BundleDigest:    row.BundleDigest,
		SizeBytes:       row.SizeBytes,
		FileCount:       row.FileCount,
		ScanStatus:      row.ScanStatus,
		UpdateAvailable: row.UpdateAvailable,
		RemoteDigest:    row.RemoteDigest,
		CheckedAt:       timestampToPtr(row.CheckedAt),
		LocalModified:   row.LocalModified,
		CreatedAt:       timestampToString(row.CreatedAt),
		UpdatedAt:       timestampToString(row.UpdatedAt),
	}
}

// ensureSkillHubRows creates the hub row of each skill that lacks one. The row
// insert ignores a row that already exists, so this is safe to run again or
// concurrently. A skill is tracked only when its origin names a remote and that
// remote is a source of this workspace; otherwise it is manual.
func (h *Handler) ensureSkillHubRows(ctx context.Context, ws pgtype.UUID, skills []hubSkillRef) error {
	if len(skills) == 0 {
		return nil
	}
	ids := make([]pgtype.UUID, len(skills))
	for i, skill := range skills {
		ids[i] = skill.id
	}
	stats, err := h.Queries.ListSkillBundleStatsByIDs(ctx, db.ListSkillBundleStatsByIDsParams{WorkspaceID: ws, SkillIds: ids})
	if err != nil {
		return err
	}
	byID := make(map[string]db.ListSkillBundleStatsByIDsRow, len(stats))
	for _, st := range stats {
		byID[uuidToString(st.SkillID)] = st
	}
	for _, skill := range skills {
		st := byID[uuidToString(skill.id)]
		tracking, sourceID, err := h.hubSkillTracking(ctx, ws, skill.config)
		if err != nil {
			return err
		}
		if err := h.Queries.InsertHubItemIfMissing(ctx, db.InsertHubItemIfMissingParams{
			WorkspaceID:   ws,
			Kind:          "skill",
			RefID:         skill.id,
			SourceID:      sourceID,
			Tracking:      tracking,
			LatestVersion: "",
			SizeBytes:     st.SizeBytes,
			FileCount:     int32(st.FileCount),
		}); err != nil {
			return err
		}
	}
	return nil
}

// ensurePluginHubRows creates the hub row of each plugin package that lacks one.
// A plugin's hub row names its newest published version. A package with no
// version has no row, and has no bundle to list.
func (h *Handler) ensurePluginHubRows(ctx context.Context, ws pgtype.UUID, packageIDs []pgtype.UUID) error {
	if len(packageIDs) == 0 {
		return nil
	}
	versions, err := h.Queries.ListLatestPluginPackageVersionsByIDs(ctx, db.ListLatestPluginPackageVersionsByIDsParams{
		WorkspaceID: ws,
		PackageIds:  packageIDs,
	})
	if err != nil {
		return err
	}
	for _, v := range versions {
		if err := h.Queries.InsertHubItemIfMissing(ctx, db.InsertHubItemIfMissingParams{
			WorkspaceID:   ws,
			Kind:          "plugin",
			RefID:         v.PackageID,
			SourceID:      pgtype.UUID{},
			Tracking:      "manual",
			LatestVersion: v.Version,
			SizeBytes:     v.SizeBytes,
			FileCount:     int32(v.FileCount),
		}); err != nil {
			return err
		}
	}
	return nil
}

// hubSkillTracking reads a skill's origin as the skill import path records it
// (config.origin) and returns the tracking mode and, when the origin matches one
// of this workspace's sources, that source's id.
func (h *Handler) hubSkillTracking(ctx context.Context, ws pgtype.UUID, config []byte) (string, pgtype.UUID, error) {
	var parsed struct {
		Origin struct {
			Type      string `json:"type"`
			SourceURL string `json:"source_url"`
		} `json:"origin"`
	}
	if len(config) > 0 {
		_ = json.Unmarshal(config, &parsed)
	}
	sourceType := parsed.Origin.Type
	if !hubTrackedOrigins[sourceType] {
		return "manual", pgtype.UUID{}, nil
	}
	if sourceURL := parsed.Origin.SourceURL; strings.HasPrefix(sourceURL, "https://") || strings.HasPrefix(sourceURL, "http://") {
		src, err := h.Queries.GetHubSourceByTypeURL(ctx, db.GetHubSourceByTypeURLParams{
			WorkspaceID: ws,
			Type:        sourceType,
			Url:         sourceURL,
		})
		switch {
		case err == nil:
			return "tracking", src.ID, nil
		case !errors.Is(err, pgx.ErrNoRows):
			return "", pgtype.UUID{}, err
		}
	}
	return "tracking", pgtype.UUID{}, nil
}
