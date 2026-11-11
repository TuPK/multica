package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Deleting a skill or a plugin package removes its hub row, its version rows and
// its bundle objects. The hub rows are not foreign-keyed, so the handler does it.

// hubDeleteBundleKey requests a skill's or plugin's bundle so it has a hub row and
// a stored zip, and returns the stored key.
func hubDeleteBundleKey(t *testing.T, ws, kind, refID string) string {
	t.Helper()
	getHubItemBundleForTest(t, ws, testUserID, kind, refID)
	var key string
	dbfx.QueryRow(t, `SELECT bundle_key FROM hub_item WHERE workspace_id = $1 AND ref_id = $2`, ws, refID).Scan(&key)
	if key == "" {
		t.Fatalf("%s %s has no bundle key after a bundle request", kind, refID)
	}
	return key
}

func assertHubObjectGone(t *testing.T, key string) {
	t.Helper()
	rc, err := testHandler.HubStorage.GetReader(context.Background(), key)
	if err == nil {
		rc.Close()
		t.Fatalf("bundle object %s still exists", key)
	}
}

func TestHubDeleteSkillRemovesItsHubRowsAndBundles(t *testing.T) {
	setupHubImportTest(t)
	ws := testWorkspaceID
	name := "hubitems-delete-" + hubItemsSuffix()
	skill := insertHubItemsSkill(t, ws, name, "---\nname: "+name+"\ndescription: fixture\n---\n# Gone\n", `'{}'::jsonb`)
	key := hubDeleteBundleKey(t, ws, "skill", skill)
	if n := hubBundleVersionRowsForRef(t, ws, skill); n != 1 {
		t.Fatalf("version rows before the delete = %d, want 1", n)
	}

	testutil.Call(t, testHandler.DeleteSkill,
		withURLParams(hubTestRequest(http.MethodDelete, "/api/skills/"+skill, nil, ws, testUserID), "id", skill)).
		Want(http.StatusNoContent)

	if n := dbfx.Count(t, "SELECT count(*) FROM hub_item WHERE workspace_id = $1 AND ref_id = $2", ws, skill); n != 0 {
		t.Fatalf("hub item rows after the delete = %d, want 0", n)
	}
	if n := hubBundleVersionRowsForRef(t, ws, skill); n != 0 {
		t.Fatalf("version rows after the delete = %d, want 0", n)
	}
	assertHubObjectGone(t, key)
}

func TestHubDeletePluginPackageRemovesItsHubRowsAndBundles(t *testing.T) {
	setupHubImportTest(t)
	withPluginsV1Flag(t, testHandler, true)
	ws := testWorkspaceID
	pkg := insertHubItemsPlugin(t, ws, "com.example.hubdelete"+hubItemsSuffix(), "3.0.0")
	key := hubDeleteBundleKey(t, ws, "plugin", pkg)

	recorder := httptest.NewRecorder()
	testHandler.DeletePluginPackage(recorder, pluginHandlerRequest(http.MethodDelete, "/plugins/packages", nil,
		map[string]string{"id": ws, "packageId": pkg}))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("delete package status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	if n := dbfx.Count(t, "SELECT count(*) FROM hub_item WHERE workspace_id = $1 AND ref_id = $2", ws, pkg); n != 0 {
		t.Fatalf("hub item rows after the delete = %d, want 0", n)
	}
	if n := hubBundleVersionRowsForRef(t, ws, pkg); n != 0 {
		t.Fatalf("version rows after the delete = %d, want 0", n)
	}
	assertHubObjectGone(t, key)
}

// The hub item keeps no name. The name is read from the skill, so a rename needs no
// hub write: the item keeps its row and its bundle, and the list shows one row.
func TestHubItemSurvivesSkillRename(t *testing.T) {
	setupHubImportTest(t)
	ws := testWorkspaceID
	name := "hubitems-rename-" + hubItemsSuffix()
	skill := insertHubItemsSkill(t, ws, name, "---\nname: "+name+"\ndescription: fixture\n---\n# Rename\n", `'{}'::jsonb`)
	registerHubItemCleanup(t, ws, skill)
	before := getHubItemBundleForTest(t, ws, testUserID, "skill", skill)
	var rowBefore string
	dbfx.QueryRow(t, `SELECT id::text FROM hub_item WHERE workspace_id = $1 AND ref_id = $2`, ws, skill).Scan(&rowBefore)

	renamed := name + "-renamed"
	testutil.Call(t, testHandler.UpdateSkill,
		testutil.WithURLParams(hubTestRequest(http.MethodPut, "/api/skills/"+skill, map[string]any{"name": renamed}, ws, testUserID), "id", skill)).
		Want(http.StatusOK)
	var storedName string
	dbfx.QueryRow(t, `SELECT name FROM skill WHERE id = $1`, skill).Scan(&storedName)
	if storedName != renamed {
		t.Fatalf("skill name = %q, want %q", storedName, renamed)
	}

	var rowAfter string
	dbfx.QueryRow(t, `SELECT id::text FROM hub_item WHERE workspace_id = $1 AND ref_id = $2`, ws, skill).Scan(&rowAfter)
	if rowAfter != rowBefore {
		t.Fatalf("hub item row changed on rename: %s -> %s", rowBefore, rowAfter)
	}
	if after := getHubItemBundleForTest(t, ws, testUserID, "skill", skill); after.Digest != before.Digest {
		t.Fatalf("a rename changed the bundle digest: %s -> %s", before.Digest, after.Digest)
	}
	var matches int
	for _, item := range listHubItemsForTest(t, ws, testUserID) {
		if item.RefID == skill {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("list rows for the renamed skill = %d, want 1", matches)
	}
}
