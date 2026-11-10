package handler

import (
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// hubActivityFixture is a workspace with an owner and a feed to read. Every test
// builds its own, so the rows it sees are its own and nothing else's.
type hubActivityFixture struct {
	ws    string
	owner string
}

func newHubActivityFixture(t *testing.T) hubActivityFixture {
	t.Helper()
	ws := dbfx.Workspace(t, hubTestLabel(t, "workspace"), hubTestSlug(t))
	dbfx.Cleanup(t, "DELETE FROM hub_activity WHERE workspace_id = $1", ws)
	owner := dbfx.User(t, hubTestLabel(t, "owner"), hubTestEmail(t))
	dbfx.Member(t, ws, owner, hubRoleOwner)
	return hubActivityFixture{ws: ws, owner: owner}
}

// insertHubActivityRow writes one feed row at the given time.
func insertHubActivityRow(t *testing.T, ws, actorID string, at time.Time, cols testutil.Cols) string {
	t.Helper()
	row := testutil.Cols{
		"workspace_id": ws,
		"actor_type":   "member",
		"action":       "source_created",
		"item_name":    "",
		"detail":       `{}`,
		"created_at":   at,
	}
	if actorID != "" {
		row["actor_id"] = actorID
	}
	for k, v := range cols {
		row[k] = v
	}
	return dbfx.Insert(t, "hub_activity", row)
}

func getHubActivity(t *testing.T, f hubActivityFixture, query string) *testutil.Response {
	t.Helper()
	return testutil.Call(t, testHandler.GetHubActivity,
		hubTestRequest(http.MethodGet, "/api/hub/activity"+query, nil, f.ws, f.owner))
}

func TestHubActivity_PagesNewestFirstWithOpaqueCursor(t *testing.T) {
	f := newHubActivityFixture(t)
	base := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		insertHubActivityRow(t, f.ws, "", base.Add(time.Duration(i)*time.Minute), testutil.Cols{
			"item_name": names5[i],
		})
	}

	var page1 HubActivityResponse
	getHubActivity(t, f, "?limit=2").Want(http.StatusOK).JSON(&page1)
	if got := activityNames(page1); got != "item-4,item-3" {
		t.Fatalf("page 1 = %s, want item-4,item-3", got)
	}
	if page1.NextCursor == nil {
		t.Fatalf("page 1 has more rows but next_cursor is null")
	}

	var page2 HubActivityResponse
	getHubActivity(t, f, "?limit=2&before="+*page1.NextCursor).Want(http.StatusOK).JSON(&page2)
	if got := activityNames(page2); got != "item-2,item-1" {
		t.Fatalf("page 2 = %s, want item-2,item-1", got)
	}

	var page3 HubActivityResponse
	getHubActivity(t, f, "?limit=2&before="+*page2.NextCursor).Want(http.StatusOK).JSON(&page3)
	if got := activityNames(page3); got != "item-0" {
		t.Fatalf("page 3 = %s, want item-0", got)
	}
	if page3.NextCursor != nil {
		t.Fatalf("last page next_cursor = %q, want null", *page3.NextCursor)
	}
}

// names5 are the item names the pagination test writes, oldest first.
var names5 = []string{"item-0", "item-1", "item-2", "item-3", "item-4"}

func activityNames(resp HubActivityResponse) string {
	out := ""
	for i, e := range resp.Entries {
		if i > 0 {
			out += ","
		}
		out += e.ItemName
	}
	return out
}

func TestHubActivity_FiltersAndActorName(t *testing.T) {
	f := newHubActivityFixture(t)
	ada := dbfx.User(t, hubTestLabel(t, "Ada Hub"), hubTestEmail(t))
	base := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	skillRef := hubTestUUID(t)
	pluginRef := hubTestUUID(t)
	sourceID := hubTestUUID(t)
	insertHubActivityRow(t, f.ws, ada, base, testutil.Cols{
		"action": "import_committed", "kind": "skill", "ref_id": skillRef,
		"source_id": sourceID, "item_name": "skill-one",
	})
	insertHubActivityRow(t, f.ws, "", base.Add(time.Minute), testutil.Cols{
		"action": "plugin_published", "kind": "plugin", "ref_id": pluginRef, "item_name": "plugin-one",
	})

	var bySkill HubActivityResponse
	getHubActivity(t, f, "?kind=skill").Want(http.StatusOK).JSON(&bySkill)
	if len(bySkill.Entries) != 1 || bySkill.Entries[0].ItemName != "skill-one" {
		t.Fatalf("kind=skill entries = %+v", bySkill.Entries)
	}
	entry := bySkill.Entries[0]
	if entry.ActorName != hubTestLabel(t, "Ada Hub") || entry.ActorType != "member" {
		t.Fatalf("actor = %q (%s), want the user's name as a member", entry.ActorName, entry.ActorType)
	}
	if entry.ActorID == nil || *entry.ActorID != ada || entry.SourceID == nil || *entry.SourceID != sourceID {
		t.Fatalf("actor_id/source_id = %v/%v", entry.ActorID, entry.SourceID)
	}
	if entry.Kind == nil || *entry.Kind != "skill" || entry.RefID == nil || *entry.RefID != skillRef {
		t.Fatalf("kind/ref_id = %v/%v", entry.Kind, entry.RefID)
	}

	var byRef HubActivityResponse
	getHubActivity(t, f, "?ref_id="+pluginRef).Want(http.StatusOK).JSON(&byRef)
	if len(byRef.Entries) != 1 || byRef.Entries[0].Action != "plugin_published" || byRef.Entries[0].ActorID != nil {
		t.Fatalf("ref_id entries = %+v", byRef.Entries)
	}
	if byRef.Entries[0].ActorName != "" {
		t.Fatalf("actor-less row has actor_name %q", byRef.Entries[0].ActorName)
	}

	var bySource HubActivityResponse
	getHubActivity(t, f, "?source_id="+sourceID).Want(http.StatusOK).JSON(&bySource)
	if len(bySource.Entries) != 1 || bySource.Entries[0].ItemName != "skill-one" {
		t.Fatalf("source_id entries = %+v", bySource.Entries)
	}
}

func TestHubActivity_RejectsBadParams(t *testing.T) {
	f := newHubActivityFixture(t)
	bad := []string{
		"?limit=0",
		"?limit=many",
		"?kind=bundle",
		"?ref_id=not-a-uuid",
		"?source_id=not-a-uuid",
		"?before=!!!",
		"?before=" + base64.RawURLEncoding.EncodeToString([]byte("no-separator")),
		"?before=" + base64.RawURLEncoding.EncodeToString([]byte("2026-03-01T00:00:00Z|not-a-uuid")),
	}
	for _, query := range bad {
		getHubActivity(t, f, query).Want(http.StatusBadRequest)
	}
}

// An oversized limit is clamped rather than refused, and an empty feed encodes
// entries as [] so clients never see null.
func TestHubActivity_EmptyFeedAndOversizedLimit(t *testing.T) {
	f := newHubActivityFixture(t)
	var resp HubActivityResponse
	getHubActivity(t, f, "?limit=100000").Want(http.StatusOK).JSON(&resp)
	if resp.Entries == nil || len(resp.Entries) != 0 || resp.NextCursor != nil {
		t.Fatalf("empty feed = %+v, want entries [] and next_cursor null", resp)
	}
}

func TestHubActivity_RecordWritesActorFromRequest(t *testing.T) {
	f := newHubActivityFixture(t)
	req := hubTestRequest(http.MethodPost, "/api/hub/import/commit", nil, f.ws, f.owner)
	testHandler.recordHubActivity(req.Context(), req, hubActivityEntry{
		WorkspaceID: parseUUID(f.ws),
		Action:      hubActionImportCommit,
		Kind:        "skill",
		RefID:       parseUUID(hubTestUUID(t)),
		ItemName:    "recorded",
		Detail:      map[string]any{"count": 1},
	})

	var resp HubActivityResponse
	getHubActivity(t, f, "").Want(http.StatusOK).JSON(&resp)
	if len(resp.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(resp.Entries))
	}
	entry := resp.Entries[0]
	if entry.Action != hubActionImportCommit || entry.ItemName != "recorded" ||
		entry.ActorType != "member" || entry.ActorID == nil || *entry.ActorID != f.owner {
		t.Fatalf("recorded entry = %+v", entry)
	}
	if entry.Detail["count"] != float64(1) {
		t.Fatalf("detail = %v, want count 1", entry.Detail)
	}
}

func TestHubActivity_RecordIsBestEffort(t *testing.T) {
	f := newHubActivityFixture(t)
	req := hubTestRequest(http.MethodPost, "/api/hub/import/commit", nil, f.ws, f.owner)
	// A zero workspace id is NULL, which the table refuses. The call must log and
	// return, not panic or report anything to the caller.
	testHandler.recordHubActivity(req.Context(), req, hubActivityEntry{
		Action: hubActionSourceCreated,
	})

	var resp HubActivityResponse
	getHubActivity(t, f, "").Want(http.StatusOK).JSON(&resp)
	if len(resp.Entries) != 0 {
		t.Fatalf("a refused write left %d rows in the workspace", len(resp.Entries))
	}
}
