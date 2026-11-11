package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/storage"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Bundles must not go stale. A bundle is rebuilt before it is served when the item's
// content no longer hashes to the latest recorded version. The download also checks
// the stored bytes against the recorded digest.

// hubBundleFileOf returns the content of one entry of a served zip.
func hubBundleFileOf(t *testing.T, data []byte, path string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("served bundle is not a zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != path {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		defer rc.Close()
		body, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		return string(body)
	}
	t.Fatalf("served bundle has no %s", path)
	return ""
}

func downloadHubBundleForTest(t *testing.T, ws, userID, kind, refID string) []byte {
	t.Helper()
	rec := testutil.Call(t, testHandler.DownloadHubItemBundle,
		withURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/"+kind+"/"+refID+"/bundle/download", nil, ws, userID), "kind", kind, "id", refID)).
		Want(http.StatusOK)
	return rec.Body.Bytes()
}

func hubBundleVersionRowsForRef(t *testing.T, ws, refID string) int {
	t.Helper()
	return dbfx.Count(t, "SELECT count(*) FROM hub_item_version_blob WHERE workspace_id = $1 AND item_id IN (SELECT id FROM hub_item WHERE workspace_id = $1 AND ref_id = $2)", ws, refID)
}

func TestHubBundleRebuildsAfterSkillEdit(t *testing.T) {
	setupHubImportTest(t)
	ws := testWorkspaceID
	name := "hubitems-stale-" + hubItemsSuffix()
	skill := insertHubItemsSkill(t, ws, name, "---\nname: "+name+"\ndescription: fixture\n---\n# First\n", `'{}'::jsonb`)
	registerHubItemCleanup(t, ws, skill)

	first := getHubItemBundleForTest(t, ws, testUserID, "skill", skill)
	again := getHubItemBundleForTest(t, ws, testUserID, "skill", skill)
	if again.Digest != first.Digest || hubBundleVersionRowsForRef(t, ws, skill) != 1 {
		t.Fatalf("unchanged skill: digest %s -> %s, version rows %d; want no rebuild", first.Digest, again.Digest, hubBundleVersionRowsForRef(t, ws, skill))
	}

	editSkillForTest(t, ws, testUserID, skill, "---\nname: "+name+"\ndescription: fixture\n---\n# Edited\n")
	rebuilt := getHubItemBundleForTest(t, ws, testUserID, "skill", skill)
	if rebuilt.Digest == first.Digest {
		t.Fatal("digest did not change after the skill was edited")
	}
	if rebuilt.Version != rebuilt.Digest {
		t.Fatalf("version = %q, want the new digest", rebuilt.Version)
	}
	if n := hubBundleVersionRowsForRef(t, ws, skill); n != 2 {
		t.Fatalf("version rows after the edit = %d, want 2 (one per recorded bundle)", n)
	}
	var recordedDigest, recordedKey string
	var size int64
	dbfx.QueryRow(t, `SELECT bundle_digest, bundle_key, size_bytes FROM hub_item WHERE workspace_id = $1 AND ref_id = $2`, ws, skill).
		Scan(&recordedDigest, &recordedKey, &size)
	if recordedDigest != rebuilt.Digest || !strings.HasSuffix(recordedKey, "/"+rebuilt.Digest+"/bundle.zip") || size != rebuilt.SizeBytes {
		t.Fatalf("hub row = digest %s key %s size %d; want the rebuilt bundle %+v", recordedDigest, recordedKey, size, rebuilt)
	}

	data := downloadHubBundleForTest(t, ws, testUserID, "skill", skill)
	if got := hubBundleFileOf(t, data, "SKILL.md"); !strings.Contains(got, "# Edited") {
		t.Fatalf("served SKILL.md = %q, want the edited content", got)
	}
	if digest := hub.Digest(mustExtractHubBundle(t, data)); digest != rebuilt.Digest {
		t.Fatalf("served digest = %s, want %s", digest, rebuilt.Digest)
	}
}

func mustExtractHubBundle(t *testing.T, data []byte) *hub.Bundle {
	t.Helper()
	bundle, err := hub.ExtractZip(bytes.NewReader(data), int64(len(data)), hub.DefaultLimits)
	if err != nil {
		t.Fatalf("extract served bundle: %v", err)
	}
	return bundle
}

func TestHubBundleRebuildsAnEditedImportedSkill(t *testing.T) {
	setupHubImportTest(t)
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	remote := newHubSyncRemote(t)
	name := "hubitems-import-" + hubItemsSuffix()
	remote.set("/imported.zip", hubSyncSkillZip(t, name, "imported", "# imported"))
	skillID, _ := importHubSkillForSync(t, f.ws, owner, remote.url("/imported.zip"))

	// The import recorded its bundle. The first request serves it unchanged.
	var importedDigest string
	dbfx.QueryRow(t, `SELECT bundle_digest FROM hub_item WHERE workspace_id = $1 AND ref_id = $2`, f.ws, skillID).Scan(&importedDigest)
	untouched := getHubItemBundleForTest(t, f.ws, owner, "skill", skillID)
	if untouched.Digest != importedDigest {
		t.Fatalf("an unedited import was rebuilt: digest %s, recorded at import %s", untouched.Digest, importedDigest)
	}

	editSkillForTest(t, f.ws, owner, skillID, "---\nname: "+name+"\ndescription: imported\n---\n\n# local edit\n")
	rebuilt := getHubItemBundleForTest(t, f.ws, owner, "skill", skillID)
	if rebuilt.Digest == importedDigest {
		t.Fatal("an edited import kept its import bundle")
	}
	data := downloadHubBundleForTest(t, f.ws, owner, "skill", skillID)
	if got := hubBundleFileOf(t, data, "SKILL.md"); !strings.Contains(got, "# local edit") {
		t.Fatalf("served SKILL.md = %q, want the local edit", got)
	}
}

func TestHubBundleRebuildsWhenPluginHasNewerVersion(t *testing.T) {
	setupHubImportTest(t)
	withHostCapabilities(t)
	ws := testWorkspaceID
	key := "com.example.hubnewer" + strings.ToLower(hubItemsSuffix())
	pkg := insertHubItemsPlugin(t, ws, key, "1.0.0")
	registerHubItemCleanup(t, ws, pkg)

	first := getHubItemBundleForTest(t, ws, testUserID, "plugin", pkg)
	if first.Version != "1.0.0" {
		t.Fatalf("first version = %q, want 1.0.0", first.Version)
	}

	// A newer version is published through the publish path, not through the hub.
	manifest := strings.Replace(packageManifest("1.1.0"), "com.example.published", key, 1)
	if _, err := testHandler.PluginService.PublishBundle(context.Background(), parseUUID(ws), parseUUID(testUserID), pluginBundleZip(t, manifest, "console.log('1.1');\n")); err != nil {
		t.Fatalf("publish 1.1.0: %v", err)
	}

	newer := getHubItemBundleForTest(t, ws, testUserID, "plugin", pkg)
	if newer.Version != "1.1.0" || newer.Digest == first.Digest {
		t.Fatalf("bundle after a newer version = %+v, want version 1.1.0 and a new digest", newer)
	}
	var latest string
	dbfx.QueryRow(t, `SELECT latest_version FROM hub_item WHERE workspace_id = $1 AND ref_id = $2`, ws, pkg).Scan(&latest)
	if latest != "1.1.0" {
		t.Fatalf("hub item latest_version = %q, want 1.1.0", latest)
	}
	data := downloadHubBundleForTest(t, ws, testUserID, "plugin", pkg)
	if got := hubBundleFileOf(t, data, "multica.plugin.json"); !strings.Contains(got, `"version":"1.1.0"`) {
		t.Fatalf("served manifest does not carry 1.1.0: %s", got)
	}
}

// tamperingHubStore returns other bytes for one key, as a store that lost or
// swapped an object would.
type tamperingHubStore struct {
	storage.Storage
	key  string
	data []byte
}

func (s tamperingHubStore) GetReader(ctx context.Context, key string) (io.ReadCloser, error) {
	if key == s.key {
		return io.NopCloser(bytes.NewReader(s.data)), nil
	}
	return s.Storage.GetReader(ctx, key)
}

func TestHubBundleDownloadRefusesBytesThatDoNotMatchTheDigest(t *testing.T) {
	setupHubImportTest(t)
	ws := testWorkspaceID
	name := "hubitems-tamper-" + hubItemsSuffix()
	skill := insertHubItemsSkill(t, ws, name, "---\nname: "+name+"\ndescription: fixture\n---\n# Real\n", `'{}'::jsonb`)
	registerHubItemCleanup(t, ws, skill)
	getHubItemBundleForTest(t, ws, testUserID, "skill", skill)
	var key string
	dbfx.QueryRow(t, `SELECT bundle_key FROM hub_item WHERE workspace_id = $1 AND ref_id = $2`, ws, skill).Scan(&key)

	const secret = "SECRET-TAMPERED-CONTENT"
	other := hubImportZip(t, map[string]string{"SKILL.md": "---\nname: x\ndescription: x\n---\n" + secret + "\n"})
	prev := testHandler.HubStorage
	testHandler.HubStorage = tamperingHubStore{Storage: prev, key: key, data: other}
	t.Cleanup(func() { testHandler.HubStorage = prev })

	rec := testutil.Call(t, testHandler.DownloadHubItemBundle,
		withURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/skill/"+skill+"/bundle/download", nil, ws, testUserID), "kind", "skill", "id", skill)).
		Want(http.StatusInternalServerError)
	if strings.Contains(rec.Body.String(), secret) || strings.Contains(rec.Body.String(), "SKILL.md") {
		t.Fatalf("error body leaks bundle content: %q", rec.Body.String())
	}
}
