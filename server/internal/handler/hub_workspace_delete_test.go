package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/storage"
)

// Deleting a workspace removes its Hub rows (no foreign keys do it) and its
// bundles from the Hub store, and leaves a neighbour's untouched.
func TestDeleteWorkspace_RemovesHubRowsAndObjects(t *testing.T) {
	if testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	t.Setenv("LOCAL_UPLOAD_DIR", t.TempDir())
	store := storage.NewLocalStorageFromEnv()
	prev := testHandler.HubStorage
	testHandler.HubStorage = store
	t.Cleanup(func() { testHandler.HubStorage = prev })

	f := newWorkspaceDeletePathFixture(t, "hubcleanup")

	seed := func(workspaceID string) string {
		if _, err := testPool.Exec(ctx, `INSERT INTO hub_source (workspace_id, name, type, url) VALUES ($1, 's', 'github', 'https://github.com/a/b')`, workspaceID); err != nil {
			t.Fatalf("seed hub_source: %v", err)
		}
		if _, err := testPool.Exec(ctx, `INSERT INTO hub_item (workspace_id, kind, ref_id) VALUES ($1, 'skill', gen_random_uuid())`, workspaceID); err != nil {
			t.Fatalf("seed hub_item: %v", err)
		}
		if _, err := testPool.Exec(ctx, `INSERT INTO hub_item_version_blob (workspace_id, item_id, version, bundle_key, digest, size_bytes) VALUES ($1, gen_random_uuid(), '1', 'k', 'd', 1)`, workspaceID); err != nil {
			t.Fatalf("seed hub_item_version_blob: %v", err)
		}
		if _, err := testPool.Exec(ctx, `INSERT INTO hub_activity (workspace_id, actor_type, action) VALUES ($1, 'member', 'source_created')`, workspaceID); err != nil {
			t.Fatalf("seed hub_activity: %v", err)
		}
		key := "hub/" + workspaceID + "/skill/x/1/bundle.zip"
		if _, err := store.Upload(ctx, key, []byte("zip"), "application/zip", "bundle.zip"); err != nil {
			t.Fatalf("seed bundle object: %v", err)
		}
		return key
	}
	victimKey := seed(f.victimID)
	neighbourKey := seed(f.neighbourID)
	t.Cleanup(func() { store.DeleteKeys(context.Background(), []string{victimKey, neighbourKey}) })

	w := httptest.NewRecorder()
	req := withURLParam(newRequest("DELETE", "/api/workspaces/"+f.victimID, nil), "id", f.victimID)
	testHandler.DeleteWorkspace(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("DeleteWorkspace = %d, want 204: %s", w.Code, w.Body.String())
	}

	count := func(table, workspaceID string) int {
		var n int
		if err := testPool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE workspace_id = $1`, workspaceID).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return n
	}
	for _, table := range []string{"hub_source", "hub_item", "hub_item_version_blob", "hub_activity"} {
		if n := count(table, f.victimID); n != 0 {
			t.Errorf("%s kept %d rows of the deleted workspace", table, n)
		}
		if n := count(table, f.neighbourID); n != 1 {
			t.Errorf("%s has %d rows for the neighbour, want 1", table, n)
		}
	}
	if _, err := store.GetReader(ctx, victimKey); err == nil {
		t.Error("the deleted workspace's bundle object survived")
	}
	if r, err := store.GetReader(ctx, neighbourKey); err != nil {
		t.Errorf("the neighbour's bundle object was removed: %v", err)
	} else {
		_ = r.Close()
	}
}
