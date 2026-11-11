package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Plugin updates end to end: a plugin imported from a url_zip source is changed
// upstream, checked, and applied through the update routes. The remote is an
// httptest origin, and the package rows are created by the import commit.

// hubUpdatePluginManifest is the manifest of one version of a plugin with the given key.
func hubUpdatePluginManifest(key, version string) string {
	return strings.Replace(packageManifest(version), "com.example.published", key, 1)
}

// hubUpdatePluginFixture imports a plugin version from the remote and returns the
// workspace, the plugin's package id, its hub item id and its manifest key.
func hubUpdatePluginFixture(t *testing.T, remote *hubSyncRemote, path string) (hubPermWorkspace, string, string, string) {
	t.Helper()
	setupHubImportTest(t)
	withPluginsV1Flag(t, testHandler, true)
	withHostCapabilities(t)
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	key := "com.example.hubupd" + hubItemsSuffix()
	remote.set(path, pluginBundleZip(t, hubUpdatePluginManifest(key, "1.0.0"), "console.log('v1');\n"))
	dbfx.Cleanup(t, "DELETE FROM plugin_package_file WHERE version_id IN (SELECT id FROM plugin_package_version WHERE workspace_id = $1)", f.ws)
	dbfx.Cleanup(t, "DELETE FROM plugin_package_version WHERE workspace_id = $1", f.ws)
	dbfx.Cleanup(t, "DELETE FROM plugin_package WHERE workspace_id = $1", f.ws)

	var preview hubImportPreviewBody
	testutil.Call(t, testHandler.PreviewHubImport,
		hubTestRequest(http.MethodPost, "/api/hub/import/preview", map[string]any{
			"source": map[string]any{"type": "url_zip", "url": remote.url(path)},
		}, f.ws, owner)).Want(http.StatusOK).JSON(&preview)
	rec := commitHubImportForTest(t, f.ws, owner, map[string]any{"preview_id": preview.PreviewID})
	body := decodeHubImportCommit(t, rec)
	if rec.Code != http.StatusOK || body.Kind != "plugin" || body.HubItemID == "" {
		t.Fatalf("plugin commit = %d %+v", rec.Code, body)
	}
	return f, body.ID, body.HubItemID, key
}

func hubUpdatePublishedVersions(t *testing.T, packageID, version string) int {
	t.Helper()
	return dbfx.Count(t, "SELECT count(*) FROM plugin_package_version WHERE package_id = $1 AND version = $2", packageID, version)
}

func TestHubPluginUpdateApplyOverwriteConflictsAndFork(t *testing.T) {
	remote := newHubSyncRemote(t)
	const path = "/plugin-update.zip"
	f, packageID, itemID, key := hubUpdatePluginFixture(t, remote, path)
	owner := f.users[hubRoleOwner]

	t.Run("overwrite publishes the new version and records the item", func(t *testing.T) {
		remote.set(path, pluginBundleZip(t, hubUpdatePluginManifest(key, "1.1.0"), "console.log('v1.1');\n"))
		check := checkHubItemForTest(t, f.ws, owner, "plugin", packageID)
		if !check.UpdateAvailable || check.LocalModified {
			t.Fatalf("check = %+v, want an update and no local changes", check)
		}

		var applied HubItemVersionResponse
		applyHubItemForTest(t, f.ws, owner, "plugin", packageID, map[string]any{}).Want(http.StatusOK).JSON(&applied)
		if applied.Version != "1.1.0" || applied.Kind != "plugin" || applied.Tracking != "tracking" {
			t.Fatalf("apply response = %+v", applied)
		}
		if n := hubUpdatePublishedVersions(t, packageID, "1.1.0"); n != 1 {
			t.Fatalf("published 1.1.0 versions = %d, want 1", n)
		}
		var latest string
		dbfx.QueryRow(t, `SELECT latest_version FROM hub_item WHERE id = $1`, itemID).Scan(&latest)
		if latest != "1.1.0" {
			t.Fatalf("hub item latest_version = %q, want 1.1.0", latest)
		}
		if _, update, local, _ := hubItemStateForTest(t, itemID); update || local {
			t.Fatalf("item after apply: update=%v local=%v", update, local)
		}
		if got := hubActivityActionsForTest(t, f.ws, owner); !got["update_applied"] {
			t.Fatalf("activity actions = %v, want update_applied", got)
		}
	})

	t.Run("a changed version string with different content is version_conflict", func(t *testing.T) {
		// The upstream author republishes 1.1.0 with new content: versions are immutable.
		remote.set(path, pluginBundleZip(t, hubUpdatePluginManifest(key, "1.1.0"), "console.log('rebuilt');\n"))
		resp := applyHubItemForTest(t, f.ws, owner, "plugin", packageID, map[string]any{})
		resp.Want(http.StatusConflict)
		if code := decodeErrorCode(t, resp); code != "version_conflict" {
			t.Fatalf("code = %q, want version_conflict", code)
		}
		if n := hubUpdatePublishedVersions(t, packageID, "1.1.0"); n != 1 {
			t.Fatalf("published 1.1.0 versions after the conflict = %d, want 1", n)
		}
	})

	t.Run("a changed manifest key is key_changed and publishes nothing", func(t *testing.T) {
		remote.set(path, pluginBundleZip(t, hubUpdatePluginManifest(key+"x", "1.2.0"), "console.log('other');\n"))
		resp := applyHubItemForTest(t, f.ws, owner, "plugin", packageID, map[string]any{})
		resp.Want(http.StatusConflict)
		if code := decodeErrorCode(t, resp); code != "key_changed" {
			t.Fatalf("code = %q, want key_changed", code)
		}
		if n := hubUpdatePublishedVersions(t, packageID, "1.2.0"); n != 0 {
			t.Fatalf("published 1.2.0 versions = %d, want 0", n)
		}
	})

	t.Run("fork detaches the plugin and keeps it at its version", func(t *testing.T) {
		remote.set(path, pluginBundleZip(t, hubUpdatePluginManifest(key, "1.3.0"), "console.log('v1.3');\n"))
		var forked HubItemVersionResponse
		applyHubItemForTest(t, f.ws, owner, "plugin", packageID, map[string]any{"on_local_changes": "fork"}).
			Want(http.StatusOK).JSON(&forked)
		if forked.Tracking != "forked" {
			t.Fatalf("fork response = %+v", forked)
		}
		if n := hubUpdatePublishedVersions(t, packageID, "1.3.0"); n != 0 {
			t.Fatalf("fork published 1.3.0 %d times, want 0", n)
		}
		var tracking string
		dbfx.QueryRow(t, `SELECT tracking FROM hub_item WHERE id = $1`, itemID).Scan(&tracking)
		if tracking != "forked" {
			t.Fatalf("hub item tracking = %q, want forked", tracking)
		}
		resp := testutil.Call(t, testHandler.GetHubItemUpdate,
			testutil.WithURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/plugin/"+packageID+"/update", nil, f.ws, owner), "kind", "plugin", "id", packageID))
		resp.Want(http.StatusConflict)
		if code := decodeErrorCode(t, resp); code != "not_tracked" {
			t.Fatalf("update check after fork: code = %q, want not_tracked", code)
		}
	})
}
