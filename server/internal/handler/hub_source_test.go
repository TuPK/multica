package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Test rows are named after the test so they are unique across the shared
// database. Every row a test creates through the handler is removed again by
// its id; fixture rows are removed by the fixture.

func hubTestLabel(t *testing.T, s string) string {
	return t.Name() + " " + s
}

func hubTestRequest(method, path string, body any, workspaceID, userID string) *http.Request {
	req := newRequest(method, path, body)
	req.Header.Set("X-Workspace-ID", workspaceID)
	req.Header.Set("X-User-ID", userID)
	return req
}

func createHubSourceForTest(t *testing.T, workspaceID, name, sourceType, rawURL string) HubSourceResponse {
	t.Helper()
	var out HubSourceResponse
	testutil.Call(t, testHandler.CreateHubSource,
		hubTestRequest(http.MethodPost, "/api/hub/sources", map[string]any{
			"name": name, "type": sourceType, "url": rawURL,
		}, workspaceID, testUserID)).
		Want(http.StatusCreated).JSON(&out)
	dbfx.Cleanup(t, "DELETE FROM hub_source WHERE id = $1", out.ID)
	return out
}

func insertHubSkillForTest(t *testing.T, workspaceID, name string) string {
	t.Helper()
	return dbfx.Insert(t, "skill", testutil.Cols{"workspace_id": workspaceID, "name": name})
}

func insertHubItemForTest(t *testing.T, workspaceID, kind, refID, sourceID string) string {
	t.Helper()
	return dbfx.Insert(t, "hub_item", testutil.Cols{
		"workspace_id": workspaceID,
		"kind":         kind,
		"ref_id":       refID,
		"source_id":    sourceID,
		"tracking":     "tracking",
	})
}

type hubInUseBody struct {
	Error string           `json:"error"`
	InUse []hubSourceInUse `json:"in_use"`
}

func TestHubSource_CRUD(t *testing.T) {
	src := createHubSourceForTest(t, testWorkspaceID, hubTestLabel(t, "acme"), "github", "https://github.com/acme/skills")
	if src.Type != "github" || src.URL != "https://github.com/acme/skills" || !src.Enabled ||
		src.ItemCount != 0 || src.LastSyncAt != nil || src.CreatedAt == "" {
		t.Fatalf("created source = %+v", src)
	}
	if len(src.Config) != 0 {
		t.Fatalf("config = %v, want empty object", src.Config)
	}

	// Config and the sync interval are stored as given.
	var configured HubSourceResponse
	testutil.Call(t, testHandler.CreateHubSource,
		hubTestRequest(http.MethodPost, "/api/hub/sources", map[string]any{
			"name": hubTestLabel(t, "configured"), "type": "skills_sh",
			"url": "https://skills.sh/acme", "config": map[string]any{"branch": "main"},
			"enabled": false, "auto_sync_interval_sec": 3600,
		}, testWorkspaceID, testUserID)).
		Want(http.StatusCreated).JSON(&configured)
	dbfx.Cleanup(t, "DELETE FROM hub_source WHERE id = $1", configured.ID)
	if configured.Config["branch"] != "main" || configured.Enabled || configured.AutoSyncIntervalSec != 3600 {
		t.Fatalf("configured source = %+v", configured)
	}

	// Two items point at the first source.
	skillA := insertHubSkillForTest(t, testWorkspaceID, hubTestLabel(t, "skill a"))
	skillB := insertHubSkillForTest(t, testWorkspaceID, hubTestLabel(t, "skill b"))
	insertHubItemForTest(t, testWorkspaceID, "skill", skillA, src.ID)
	insertHubItemForTest(t, testWorkspaceID, "skill", skillB, src.ID)

	var listed []HubSourceResponse
	testutil.Call(t, testHandler.ListHubSources,
		hubTestRequest(http.MethodGet, "/api/hub/sources", nil, testWorkspaceID, testUserID)).
		Want(http.StatusOK).JSON(&listed)
	found := false
	for _, s := range listed {
		if s.ID == src.ID {
			found = true
			if s.ItemCount != 2 {
				t.Fatalf("listed item_count = %d, want 2", s.ItemCount)
			}
		}
	}
	if !found {
		t.Fatalf("created source %s missing from list", src.ID)
	}

	var patched HubSourceResponse
	testutil.Call(t, testHandler.UpdateHubSource,
		testutil.WithURLParams(hubTestRequest(http.MethodPatch, "/api/hub/sources/"+src.ID,
			map[string]any{"name": hubTestLabel(t, "renamed"), "enabled": false, "auto_sync_interval_sec": 600},
			testWorkspaceID, testUserID), "id", src.ID)).
		Want(http.StatusOK).JSON(&patched)
	if patched.Name != hubTestLabel(t, "renamed") || patched.Enabled || patched.AutoSyncIntervalSec != 600 ||
		patched.URL != src.URL || patched.ItemCount != 2 {
		t.Fatalf("patched source = %+v", patched)
	}

	// The type is part of the source's identity and cannot change.
	testutil.Call(t, testHandler.UpdateHubSource,
		testutil.WithURLParams(hubTestRequest(http.MethodPatch, "/api/hub/sources/"+src.ID,
			map[string]any{"type": "registry"}, testWorkspaceID, testUserID), "id", src.ID)).
		Want(http.StatusBadRequest)

	testutil.Call(t, testHandler.DeleteHubSource,
		testutil.WithURLParams(hubTestRequest(http.MethodDelete, "/api/hub/sources/"+src.ID, nil, testWorkspaceID, testUserID), "id", src.ID)).
		Want(http.StatusNoContent)
	if n := dbfx.Count(t, "SELECT count(*) FROM hub_source WHERE id = $1", src.ID); n != 0 {
		t.Fatalf("source rows after delete = %d", n)
	}
}

func TestHubSource_ValidationRejectsBadInput(t *testing.T) {
	long := strings.Repeat("n", hubSourceNameMaxRunes+1)
	cases := []struct {
		label string
		body  map[string]any
	}{
		{"unknown type", map[string]any{"name": "x", "type": "ftp", "url": "https://a.example"}},
		{"blank name", map[string]any{"name": "   ", "type": "github", "url": "https://a.example/x"}},
		{"name too long", map[string]any{"name": long, "type": "github", "url": "https://a.example/x"}},
		{"non-http url", map[string]any{"name": "x", "type": "github", "url": "ftp://a.example/x"}},
		{"unparseable url", map[string]any{"name": "x", "type": "registry", "url": "not a url"}},
		{"network type without url", map[string]any{"name": "x", "type": "url_zip", "url": ""}},
		{"upload with url", map[string]any{"name": "x", "type": "upload", "url": "https://a.example/x.zip"}},
		{"negative interval", map[string]any{"name": "x", "type": "clawhub", "url": "https://a.example", "auto_sync_interval_sec": -1}},
		{"config is not an object", map[string]any{"name": "x", "type": "github", "url": "https://a.example/x", "config": []string{"a"}}},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			testutil.Call(t, testHandler.CreateHubSource,
				hubTestRequest(http.MethodPost, "/api/hub/sources", tc.body, testWorkspaceID, testUserID)).
				Want(http.StatusBadRequest)
		})
	}
}

func TestHubSource_DuplicateTypeAndURLIsConflict(t *testing.T) {
	ws := dbfx.Workspace(t, hubTestLabel(t, "workspace"), strings.ToLower(strings.ReplaceAll(t.Name(), "_", "-")))
	dbfx.Member(t, ws, testUserID, "owner")
	createHubSourceForTest(t, ws, hubTestLabel(t, "first"), "registry", "https://registry.example/index.json")

	testutil.Call(t, testHandler.CreateHubSource,
		hubTestRequest(http.MethodPost, "/api/hub/sources", map[string]any{
			"name": hubTestLabel(t, "second"), "type": "registry", "url": "https://registry.example/index.json",
		}, ws, testUserID)).
		Want(http.StatusConflict)

	// The same URL under another type is a different source.
	createHubSourceForTest(t, ws, hubTestLabel(t, "other type"), "github", "https://registry.example/index.json")

	// Upload has no URL, so a workspace holds one upload source.
	createHubSourceForTest(t, ws, hubTestLabel(t, "upload"), "upload", "")
	testutil.Call(t, testHandler.CreateHubSource,
		hubTestRequest(http.MethodPost, "/api/hub/sources", map[string]any{
			"name": hubTestLabel(t, "second upload"), "type": "upload",
		}, ws, testUserID)).
		Want(http.StatusConflict)
}

func TestHubSource_CrossWorkspaceIsolation(t *testing.T) {
	other := dbfx.Workspace(t, hubTestLabel(t, "other workspace"), strings.ToLower(strings.ReplaceAll(t.Name(), "_", "-")))
	dbfx.Member(t, other, testUserID, "owner")

	src := createHubSourceForTest(t, testWorkspaceID, hubTestLabel(t, "home"), "github", "https://github.com/home/skills")
	skill := insertHubSkillForTest(t, testWorkspaceID, hubTestLabel(t, "home skill"))
	insertHubItemForTest(t, testWorkspaceID, "skill", skill, src.ID)

	var listed []HubSourceResponse
	testutil.Call(t, testHandler.ListHubSources,
		hubTestRequest(http.MethodGet, "/api/hub/sources", nil, other, testUserID)).
		Want(http.StatusOK).JSON(&listed)
	for _, s := range listed {
		if s.ID == src.ID {
			t.Fatalf("source from another workspace listed")
		}
	}

	testutil.Call(t, testHandler.UpdateHubSource,
		testutil.WithURLParams(hubTestRequest(http.MethodPatch, "/api/hub/sources/"+src.ID,
			map[string]any{"name": "hijacked"}, other, testUserID), "id", src.ID)).
		Want(http.StatusNotFound)
	testutil.Call(t, testHandler.DeleteHubSource,
		testutil.WithURLParams(hubTestRequest(http.MethodDelete, "/api/hub/sources/"+src.ID+"?items=delete", nil, other, testUserID), "id", src.ID)).
		Want(http.StatusNotFound)

	if n := dbfx.Count(t, "SELECT count(*) FROM hub_source WHERE id = $1 AND name = $2", src.ID, hubTestLabel(t, "home")); n != 1 {
		t.Fatalf("source changed or removed from its own workspace")
	}
	if n := dbfx.Count(t, "SELECT count(*) FROM hub_item WHERE source_id = $1", src.ID); n != 1 {
		t.Fatalf("items of the source changed from another workspace: %d", n)
	}
}

func TestHubSource_MembersCannotManageSources(t *testing.T) {
	memberID := dbfx.User(t, hubTestLabel(t, "member"), "hub-source-member-"+strings.ReplaceAll(t.Name(), "/", "-")+"@multica.ai")
	dbfx.Member(t, testWorkspaceID, memberID, "member")

	testutil.Call(t, testHandler.ListHubSources,
		hubTestRequest(http.MethodGet, "/api/hub/sources", nil, testWorkspaceID, memberID)).
		Want(http.StatusOK)
	testutil.Call(t, testHandler.CreateHubSource,
		hubTestRequest(http.MethodPost, "/api/hub/sources", map[string]any{
			"name": hubTestLabel(t, "nope"), "type": "github", "url": "https://github.com/nope/skills",
		}, testWorkspaceID, memberID)).
		Want(http.StatusForbidden)
}

func TestHubSource_DeleteKeepDetachesItems(t *testing.T) {
	src := createHubSourceForTest(t, testWorkspaceID, hubTestLabel(t, "keep"), "github", "https://github.com/keep/skills")
	skill := insertHubSkillForTest(t, testWorkspaceID, hubTestLabel(t, "kept skill"))
	item := insertHubItemForTest(t, testWorkspaceID, "skill", skill, src.ID)

	testutil.Call(t, testHandler.DeleteHubSource,
		testutil.WithURLParams(hubTestRequest(http.MethodDelete, "/api/hub/sources/"+src.ID+"?items=keep", nil, testWorkspaceID, testUserID), "id", src.ID)).
		Want(http.StatusNoContent)

	if n := dbfx.Count(t, "SELECT count(*) FROM hub_source WHERE id = $1", src.ID); n != 0 {
		t.Fatalf("source still present after keep delete")
	}
	var detached bool
	var tracking string
	dbfx.QueryRow(t, "SELECT source_id IS NULL, tracking FROM hub_item WHERE id = $1", item).Scan(&detached, &tracking)
	if !detached || tracking != "manual" {
		t.Fatalf("item after keep delete: detached=%v tracking=%q", detached, tracking)
	}
	if n := dbfx.Count(t, "SELECT count(*) FROM skill WHERE id = $1", skill); n != 1 {
		t.Fatalf("skill removed by a keep delete")
	}
}

func TestHubSource_DeleteWithItemsBlockedByUsage(t *testing.T) {
	src := createHubSourceForTest(t, testWorkspaceID, hubTestLabel(t, "blocked"), "github", "https://github.com/blocked/skills")
	inAgent := insertHubSkillForTest(t, testWorkspaceID, hubTestLabel(t, "in agent"))
	inSquad := insertHubSkillForTest(t, testWorkspaceID, hubTestLabel(t, "in squad"))
	free := insertHubSkillForTest(t, testWorkspaceID, hubTestLabel(t, "free"))
	for _, skill := range []string{inAgent, inSquad, free} {
		insertHubItemForTest(t, testWorkspaceID, "skill", skill, src.ID)
	}

	agent := dbfx.Agent(t, hubTestLabel(t, "agent"), testRuntimeID)
	dbfx.InsertNoID(t, "agent_skill", testutil.Cols{"agent_id": agent, "skill_id": inAgent},
		"agent_id = $1 AND skill_id = $2", agent, inAgent)
	leader := dbfx.Agent(t, hubTestLabel(t, "squad leader"), testRuntimeID)
	squad := dbfx.Squad(t, hubTestLabel(t, "squad"), leader)
	dbfx.InsertNoID(t, "squad_skill", testutil.Cols{"workspace_id": testWorkspaceID, "squad_id": squad, "skill_id": inSquad},
		"squad_id = $1 AND skill_id = $2", squad, inSquad)

	var body hubInUseBody
	testutil.Call(t, testHandler.DeleteHubSource,
		testutil.WithURLParams(hubTestRequest(http.MethodDelete, "/api/hub/sources/"+src.ID+"?items=delete", nil, testWorkspaceID, testUserID), "id", src.ID)).
		Want(http.StatusConflict).JSON(&body)
	names := map[string]bool{}
	for _, item := range body.InUse {
		if item.Kind != "skill" {
			t.Fatalf("in_use kind = %q", item.Kind)
		}
		names[item.Name] = true
	}
	if !names[hubTestLabel(t, "in agent")] || !names[hubTestLabel(t, "in squad")] || names[hubTestLabel(t, "free")] {
		t.Fatalf("in_use = %+v", body.InUse)
	}

	// Refused: nothing is deleted.
	for _, skill := range []string{inAgent, inSquad, free} {
		if n := dbfx.Count(t, "SELECT count(*) FROM skill WHERE id = $1", skill); n != 1 {
			t.Fatalf("skill %s deleted by a refused removal", skill)
		}
	}
	if n := dbfx.Count(t, "SELECT count(*) FROM hub_item WHERE source_id = $1", src.ID); n != 3 {
		t.Fatalf("items after refused removal = %d, want 3", n)
	}
	if n := dbfx.Count(t, "SELECT count(*) FROM hub_source WHERE id = $1", src.ID); n != 1 {
		t.Fatalf("source removed by a refused removal")
	}
}

func TestHubSource_DeleteWithItemsRemovesUnusedSkillsAndPlugins(t *testing.T) {
	src := createHubSourceForTest(t, testWorkspaceID, hubTestLabel(t, "removable"), "url_zip", "https://files.example/bundle.zip")
	skill := insertHubSkillForTest(t, testWorkspaceID, hubTestLabel(t, "unused skill"))
	skillItem := insertHubItemForTest(t, testWorkspaceID, "skill", skill, src.ID)
	pluginKey := "hubtest." + strings.ReplaceAll(t.Name(), "/", "-")
	plugin := dbfx.Insert(t, "plugin_package", testutil.Cols{
		"workspace_id": testWorkspaceID, "plugin_key": pluginKey, "name": hubTestLabel(t, "unused plugin"),
	})
	insertHubItemForTest(t, testWorkspaceID, "plugin", plugin, src.ID)

	// An unrelated source in the same workspace must not be touched.
	keep := createHubSourceForTest(t, testWorkspaceID, hubTestLabel(t, "bystander"), "clawhub", "https://clawhub.example/other")
	keepSkill := insertHubSkillForTest(t, testWorkspaceID, hubTestLabel(t, "bystander skill"))
	keepItem := insertHubItemForTest(t, testWorkspaceID, "skill", keepSkill, keep.ID)

	testutil.Call(t, testHandler.DeleteHubSource,
		testutil.WithURLParams(hubTestRequest(http.MethodDelete, "/api/hub/sources/"+src.ID+"?items=delete", nil, testWorkspaceID, testUserID), "id", src.ID)).
		Want(http.StatusNoContent)

	if n := dbfx.Count(t, "SELECT count(*) FROM skill WHERE id = $1", skill); n != 0 {
		t.Fatalf("unused skill still present")
	}
	if n := dbfx.Count(t, "SELECT count(*) FROM plugin_package WHERE id = $1", plugin); n != 0 {
		t.Fatalf("unused plugin package still present")
	}
	if n := dbfx.Count(t, "SELECT count(*) FROM hub_item WHERE id = $1 OR id = $2", skillItem, keepItem); n != 1 {
		t.Fatalf("hub items after removal = %d, want only the bystander's", n)
	}
	if n := dbfx.Count(t, "SELECT count(*) FROM hub_source WHERE id = $1", src.ID); n != 0 {
		t.Fatalf("source still present after items=delete")
	}
	if n := dbfx.Count(t, "SELECT count(*) FROM hub_source WHERE id = $1", keep.ID); n != 1 {
		t.Fatalf("bystander source removed")
	}
	if n := dbfx.Count(t, "SELECT count(*) FROM skill WHERE id = $1", keepSkill); n != 1 {
		t.Fatalf("bystander skill removed")
	}
}
