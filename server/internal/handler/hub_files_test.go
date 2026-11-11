package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Saving a skill file through the Hub: the etag check, the routes it shares with
// the skill file handlers, and the skill rule. Item detail is covered here too.

type hubFileSaveBody struct {
	Path string `json:"path"`
	Etag string `json:"etag"`
}

type hubFileConflictBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
	Etag  string `json:"etag"`
}

// hubSkillForFiles inserts a skill created by owner, so the owner may edit it.
func hubSkillForFiles(t *testing.T, ws, owner string) string {
	t.Helper()
	return dbfx.Insert(t, "skill", testutil.Cols{
		"workspace_id": ws,
		"name":         hubTestLabel(t, "skill"),
		"content":      "Original body.",
		"created_by":   owner,
	})
}

func saveHubSkillFileForTest(t *testing.T, ws, user, skillID, path, content, etag string) *testutil.Response {
	t.Helper()
	return testutil.Call(t, testHandler.SaveHubItemFile,
		testutil.WithURLParams(hubTestRequest(http.MethodPut, "/api/hub/items/skill/"+skillID+"/files",
			map[string]any{"path": path, "content": content, "etag": etag}, ws, user), "kind", "skill", "id", skillID))
}

func getHubItemForTest(t *testing.T, ws, user, kind, refID string) *testutil.Response {
	t.Helper()
	return testutil.Call(t, testHandler.GetHubItem,
		testutil.WithURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/"+kind+"/"+refID, nil, ws, user), "kind", kind, "id", refID))
}

func TestHubSkillFileSave_EtagLifecycle(t *testing.T) {
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	skillID := hubSkillForFiles(t, f.ws, owner)

	// A new file is created with the empty etag.
	var created hubFileSaveBody
	saveHubSkillFileForTest(t, f.ws, owner, skillID, "notes/guide.md", "v1", "").Want(http.StatusOK).JSON(&created)
	if created.Path != "notes/guide.md" || created.Etag == "" {
		t.Fatalf("created = %+v, want the path and a new etag", created)
	}
	dbfx.Cleanup(t, "DELETE FROM skill_file WHERE skill_id = $1", skillID)

	// Saving with the etag just returned succeeds and returns a newer one.
	var updated hubFileSaveBody
	saveHubSkillFileForTest(t, f.ws, owner, skillID, "notes/guide.md", "v2", created.Etag).Want(http.StatusOK).JSON(&updated)
	if updated.Etag == created.Etag {
		t.Fatalf("etag did not change after a save: %q", updated.Etag)
	}

	// The etag is now stale: the save is refused with the current etag.
	var conflict hubFileConflictBody
	saveHubSkillFileForTest(t, f.ws, owner, skillID, "notes/guide.md", "v3", created.Etag).
		Want(http.StatusConflict).JSON(&conflict)
	if conflict.Code != "file_conflict" || conflict.Etag != updated.Etag {
		t.Fatalf("conflict = %+v, want file_conflict with etag %q", conflict, updated.Etag)
	}
	var body string
	dbfx.QueryRow(t, `SELECT content FROM skill_file WHERE skill_id = $1 AND path = 'notes/guide.md'`, skillID).Scan(&body)
	if body != "v2" {
		t.Fatalf("a refused save changed the file: %q", body)
	}

	// An empty etag on a file that exists is a conflict too.
	saveHubSkillFileForTest(t, f.ws, owner, skillID, "notes/guide.md", "v4", "").Want(http.StatusConflict)

	// An empty etag on a file that does not exist creates it; a non-empty one is a conflict.
	saveHubSkillFileForTest(t, f.ws, owner, skillID, "new.md", "fresh", "").Want(http.StatusOK)
	saveHubSkillFileForTest(t, f.ws, owner, skillID, "missing.md", "x", "2020-01-01T00:00:00Z").
		Want(http.StatusConflict)
}

func TestHubSkillFileSave_SkillMarkdownUsesTheSkillEtag(t *testing.T) {
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	skillID := hubSkillForFiles(t, f.ws, owner)

	// The skill's JSON carries its updated_at in whole seconds. That form is accepted
	// as the etag of SKILL.md, so a client that read the skill can save.
	var skill struct {
		UpdatedAt string `json:"updated_at"`
	}
	testutil.Call(t, testHandler.GetSkill,
		testutil.WithURLParams(hubTestRequest(http.MethodGet, "/api/skills/"+skillID, nil, f.ws, owner), "id", skillID)).
		Want(http.StatusOK).JSON(&skill)

	var saved hubFileSaveBody
	saveHubSkillFileForTest(t, f.ws, owner, skillID, "SKILL.md", "Rewritten body.", skill.UpdatedAt).
		Want(http.StatusOK).JSON(&saved)
	if saved.Path != "SKILL.md" || saved.Etag == "" {
		t.Fatalf("saved = %+v, want SKILL.md with a new etag", saved)
	}
	var content string
	dbfx.QueryRow(t, `SELECT content FROM skill WHERE id = $1`, skillID).Scan(&content)
	if content != "Rewritten body." {
		t.Fatalf("skill content = %q", content)
	}

	// The stale etag is refused and nothing is written.
	var conflict hubFileConflictBody
	saveHubSkillFileForTest(t, f.ws, owner, skillID, "SKILL.md", "Lost write.", skill.UpdatedAt).
		Want(http.StatusConflict).JSON(&conflict)
	if conflict.Etag != saved.Etag {
		t.Fatalf("conflict etag = %q, want %q", conflict.Etag, saved.Etag)
	}
	dbfx.QueryRow(t, `SELECT content FROM skill WHERE id = $1`, skillID).Scan(&content)
	if content != "Rewritten body." {
		t.Fatalf("a refused SKILL.md save changed the content: %q", content)
	}

	// The fresh etag from the save succeeds.
	saveHubSkillFileForTest(t, f.ws, owner, skillID, "SKILL.md", "Second body.", saved.Etag).Want(http.StatusOK)
}

func TestHubSkillFileSave_RecordsActivityAndLocalEdit(t *testing.T) {
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	skillID := hubSkillForFiles(t, f.ws, owner)
	dbfx.Insert(t, "hub_item", testutil.Cols{
		"workspace_id": f.ws,
		"kind":         "skill",
		"ref_id":       skillID,
		"tracking":     "tracking",
	})

	saveHubSkillFileForTest(t, f.ws, owner, skillID, "tool.md", "body", "").Want(http.StatusOK)

	var activity int
	dbfx.QueryRow(t, `SELECT count(*) FROM hub_activity WHERE workspace_id = $1 AND action = $2 AND ref_id = $3`,
		f.ws, hubActionFileSaved, skillID).Scan(&activity)
	if activity != 1 {
		t.Fatalf("file_saved activity entries = %d, want 1", activity)
	}
	var localModified bool
	dbfx.QueryRow(t, `SELECT local_modified FROM hub_item WHERE workspace_id = $1 AND ref_id = $2`, f.ws, skillID).Scan(&localModified)
	if !localModified {
		t.Fatal("a tracked skill's file save did not set local_modified")
	}
}

func TestHubSkillFileSave_RefusesWhatTheSkillRuleRefuses(t *testing.T) {
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	member := f.users[hubRoleMember]
	skillID := hubSkillForFiles(t, f.ws, owner)

	// A member who did not create the skill may not edit it.
	saveHubSkillFileForTest(t, f.ws, member, skillID, "tool.md", "body", "").Want(http.StatusForbidden)

	// A skill of another workspace is not found from this one.
	other := dbfx.Workspace(t, hubTestLabel(t, "other"), hubTestSlug(t)+"-other")
	dbfx.Cleanup(t, "DELETE FROM hub_activity WHERE workspace_id = $1", other)
	otherSkill := hubSkillForFiles(t, other, owner)
	saveHubSkillFileForTest(t, f.ws, owner, otherSkill, "tool.md", "body", "").Want(http.StatusNotFound)

	// Only skills take file saves; a path that escapes the skill is refused.
	testutil.Call(t, testHandler.SaveHubItemFile,
		testutil.WithURLParams(hubTestRequest(http.MethodPut, "/api/hub/items/plugin/"+skillID+"/files",
			map[string]any{"path": "x.md", "content": "", "etag": ""}, f.ws, owner), "kind", "plugin", "id", skillID)).
		Want(http.StatusBadRequest)
	saveHubSkillFileForTest(t, f.ws, owner, skillID, "../escape.md", "body", "").Want(http.StatusBadRequest)
}

func TestHubItemDetail_MatchesTheListElement(t *testing.T) {
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	skillID := hubSkillForFiles(t, f.ws, owner)

	var detail HubItemResponse
	getHubItemForTest(t, f.ws, owner, "skill", skillID).Want(http.StatusOK).JSON(&detail)
	if detail.Kind != "skill" || detail.RefID != skillID || detail.Tracking != "manual" {
		t.Fatalf("detail = %+v", detail)
	}

	var list []HubItemResponse
	testutil.Call(t, testHandler.ListHubItems, hubTestRequest(http.MethodGet, "/api/hub/items", nil, f.ws, owner)).
		Want(http.StatusOK).JSON(&list)
	var fromList *HubItemResponse
	for i := range list {
		if list[i].RefID == skillID {
			fromList = &list[i]
		}
	}
	if fromList == nil {
		t.Fatal("the skill is not in the list")
	}
	a, _ := json.Marshal(detail)
	b, _ := json.Marshal(fromList)
	if string(a) != string(b) {
		t.Fatalf("detail and list element differ:\n detail %s\n list   %s", a, b)
	}
}

func TestHubItemDetail_AbsentIsNotFound(t *testing.T) {
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	skillID := hubSkillForFiles(t, f.ws, owner)

	getHubItemForTest(t, f.ws, owner, "skill", "00000000-0000-4000-8000-000000000000").Want(http.StatusNotFound)
	getHubItemForTest(t, f.ws, owner, "plugin", "00000000-0000-4000-8000-000000000000").Want(http.StatusNotFound)
	getHubItemForTest(t, f.ws, owner, "widget", skillID).Want(http.StatusBadRequest)

	other := dbfx.Workspace(t, hubTestLabel(t, "other"), hubTestSlug(t)+"-detail")
	dbfx.Cleanup(t, "DELETE FROM hub_activity WHERE workspace_id = $1", other)
	otherSkill := hubSkillForFiles(t, other, owner)
	getHubItemForTest(t, f.ws, owner, "skill", otherSkill).Want(http.StatusNotFound)
}
