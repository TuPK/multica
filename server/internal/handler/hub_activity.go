package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The hub activity feed (spec 7.4): one row per skill or plugin hub action,
// newest first. Every member can read it. Writers call recordHubActivity, which
// is best-effort: an audit write never fails the action it describes.

// Hub activity actions. Writers use these names so the feed has one spelling.
const (
	hubActionSourceCreated = "source_created"
	hubActionSourceUpdated = "source_updated"
	hubActionSourceDeleted = "source_deleted"
	hubActionImportCommit  = "import_committed"
	hubActionSourceSynced  = "source_synced"
	hubActionUpdateApplied = "update_applied"
	hubActionItemForked    = "item_forked"
	hubActionItemExported  = "item_exported"
	hubActionFileSaved     = "file_saved"
	hubActionPluginPublish = "plugin_published"
)

const (
	hubActivityDefaultLimit = 50
	hubActivityMaxLimit     = 200
)

// hubActivityEntry is one action to record. The actor is read from the request.
// Zero-valued UUID fields are stored as NULL.
type hubActivityEntry struct {
	WorkspaceID pgtype.UUID
	Action      string
	Kind        string // "skill", "plugin", or "" when the action has no item
	RefID       pgtype.UUID
	ItemName    string
	SourceID    pgtype.UUID
	Detail      map[string]any
}

// recordHubActivity writes one activity row. It logs and returns on failure:
// the caller's action has already happened, and a missing audit row must not
// turn it into an error the user sees.
func (h *Handler) recordHubActivity(ctx context.Context, r *http.Request, e hubActivityEntry) {
	workspaceID := uuidToString(e.WorkspaceID)
	actorType, actorIDText := h.resolveActor(r, requestUserID(r), workspaceID)
	var actorID pgtype.UUID
	if parsed, err := util.ParseUUID(actorIDText); err == nil {
		actorID = parsed
	}
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
		ActorType:   actorType,
		ActorID:     actorID,
		Action:      e.Action,
		Kind:        kind,
		RefID:       e.RefID,
		ItemName:    e.ItemName,
		SourceID:    e.SourceID,
		Detail:      detailJSON,
	}); err != nil {
		slog.Warn("hub activity: record failed", "action", e.Action, "workspace_id", workspaceID, "error", err)
	}
}

// HubActivityEntryResponse is one feed row. actor_name is empty when the actor
// no longer exists.
type HubActivityEntryResponse struct {
	ID        string         `json:"id"`
	Action    string         `json:"action"`
	ActorType string         `json:"actor_type"`
	ActorID   *string        `json:"actor_id"`
	ActorName string         `json:"actor_name"`
	Kind      *string        `json:"kind"`
	RefID     *string        `json:"ref_id"`
	ItemName  string         `json:"item_name"`
	SourceID  *string        `json:"source_id"`
	Detail    map[string]any `json:"detail"`
	CreatedAt string         `json:"created_at"`
}

type HubActivityResponse struct {
	Entries    []HubActivityEntryResponse `json:"entries"`
	NextCursor *string                    `json:"next_cursor"`
}

// GetHubActivity (GET /api/hub/activity) returns one page of the workspace's hub
// activity, newest first. Filters: kind, ref_id, source_id. The cursor is opaque
// to clients; it encodes the created_at and id of the last row returned.
func (h *Handler) GetHubActivity(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found"); !ok {
		return
	}
	query := r.URL.Query()
	limit, ok := parseHubActivityLimit(w, query.Get("limit"))
	if !ok {
		return
	}
	params := db.ListHubActivityParams{
		WorkspaceID: parseUUID(workspaceID),
		RowLimit:    int32(limit + 1), // one extra row tells us whether a next page exists
	}
	if kind := query.Get("kind"); kind != "" {
		if kind != "skill" && kind != "plugin" {
			writeError(w, http.StatusBadRequest, "kind must be skill or plugin")
			return
		}
		params.Kind = pgtype.Text{String: kind, Valid: true}
	}
	if raw := query.Get("ref_id"); raw != "" {
		refID, ok := parseUUIDOrBadRequest(w, raw, "ref_id")
		if !ok {
			return
		}
		params.RefID = refID
	}
	if raw := query.Get("source_id"); raw != "" {
		sourceID, ok := parseUUIDOrBadRequest(w, raw, "source_id")
		if !ok {
			return
		}
		params.SourceID = sourceID
	}
	if raw := query.Get("before"); raw != "" {
		before, err := decodeHubActivityCursor(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid cursor")
			return
		}
		params.BeforeCreatedAt = pgtype.Timestamptz{Time: before.createdAt, Valid: true}
		params.BeforeID = before.id
	}

	rows, err := h.Queries.ListHubActivity(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list hub activity")
		return
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	entries := make([]HubActivityEntryResponse, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, hubActivityToResponse(row))
	}
	resp := HubActivityResponse{Entries: entries}
	if hasMore {
		last := rows[len(rows)-1]
		cursor := encodeHubActivityCursor(last.CreatedAt.Time, uuidToString(last.ID))
		resp.NextCursor = &cursor
	}
	writeJSON(w, http.StatusOK, resp)
}

func hubActivityToResponse(row db.ListHubActivityRow) HubActivityEntryResponse {
	detail := map[string]any{}
	if len(row.Detail) > 0 {
		// The column is written by this package and always an object; a malformed
		// value shows as an empty detail rather than failing the feed.
		_ = json.Unmarshal(row.Detail, &detail)
		if detail == nil {
			detail = map[string]any{}
		}
	}
	var kind *string
	if row.Kind.Valid {
		kind = &row.Kind.String
	}
	return HubActivityEntryResponse{
		ID:        uuidToString(row.ID),
		Action:    row.Action,
		ActorType: row.ActorType,
		ActorID:   uuidToPtr(row.ActorID),
		ActorName: row.ActorName,
		Kind:      kind,
		RefID:     uuidToPtr(row.RefID),
		ItemName:  row.ItemName,
		SourceID:  uuidToPtr(row.SourceID),
		Detail:    detail,
		CreatedAt: timestampToString(row.CreatedAt),
	}
}

func parseHubActivityLimit(w http.ResponseWriter, raw string) (int, bool) {
	if raw == "" {
		return hubActivityDefaultLimit, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		writeError(w, http.StatusBadRequest, "limit must be a positive integer")
		return 0, false
	}
	return min(n, hubActivityMaxLimit), true
}

type hubActivityCursor struct {
	createdAt time.Time
	id        pgtype.UUID
}

// encodeHubActivityCursor is base64url of "<created_at RFC3339Nano>|<id>". The
// URL-safe alphabet keeps the value usable in a query string unescaped.
func encodeHubActivityCursor(createdAt time.Time, id string) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeHubActivityCursor(raw string) (hubActivityCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return hubActivityCursor{}, err
	}
	ts, rawID, found := strings.Cut(string(decoded), "|")
	if !found {
		return hubActivityCursor{}, fmt.Errorf("cursor has no separator")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return hubActivityCursor{}, err
	}
	id, err := util.ParseUUID(rawID)
	if err != nil {
		return hubActivityCursor{}, err
	}
	return hubActivityCursor{createdAt: createdAt, id: id}, nil
}
