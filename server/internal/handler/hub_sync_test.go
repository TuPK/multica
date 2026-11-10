package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Phase 5 behaviour against a remote served by an httptest origin. The remote
// is mutable, so a test can change what the source serves between syncs.

// hubSyncRemote serves zips by path and lets a test change or remove them.
type hubSyncRemote struct {
	srv    *httptest.Server
	mu     sync.Mutex
	routes map[string][]byte
}

func newHubSyncRemote(t *testing.T) *hubSyncRemote {
	t.Helper()
	remote := &hubSyncRemote{routes: map[string][]byte{}}
	remote.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remote.mu.Lock()
		body, ok := remote.routes[r.URL.Path]
		remote.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(remote.srv.Close)
	return remote
}

func (r *hubSyncRemote) set(path string, body []byte) {
	r.mu.Lock()
	r.routes[path] = body
	r.mu.Unlock()
}

func (r *hubSyncRemote) drop(path string) {
	r.mu.Lock()
	delete(r.routes, path)
	r.mu.Unlock()
}

func (r *hubSyncRemote) url(path string) string { return r.srv.URL + path }

// hubSyncSkillZip is a skill bundle with a description and a body the test can vary.
func hubSyncSkillZip(t *testing.T, name, description, body string) []byte {
	t.Helper()
	return hubImportZip(t, map[string]string{
		"SKILL.md":            "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + body + "\n",
		"references/notes.md": "Notes for " + name + ".\n",
	})
}

// hubSyncWorkspace is a fresh workspace whose hub rows are removed on cleanup.
func hubSyncWorkspace(t *testing.T) hubPermWorkspace {
	t.Helper()
	f := newHubPermWorkspace(t)
	dbfx.Cleanup(t, "DELETE FROM hub_item_version_blob WHERE workspace_id = $1", f.ws)
	dbfx.Cleanup(t, "DELETE FROM hub_item WHERE workspace_id = $1", f.ws)
	dbfx.Cleanup(t, "DELETE FROM hub_source WHERE workspace_id = $1", f.ws)
	return f
}

// importHubSkillForSync previews and commits a url_zip skill into the workspace
// and returns the skill id and the hub item id.
func importHubSkillForSync(t *testing.T, ws, owner, sourceURL string) (string, string) {
	t.Helper()
	var preview hubImportPreviewBody
	testutil.Call(t, testHandler.PreviewHubImport,
		hubTestRequest(http.MethodPost, "/api/hub/import/preview", map[string]any{
			"source": map[string]any{"type": "url_zip", "url": sourceURL},
		}, ws, owner)).Want(http.StatusOK).JSON(&preview)
	rec := commitHubImportForTest(t, ws, owner, map[string]any{"preview_id": preview.PreviewID})
	body := decodeHubImportCommit(t, rec)
	if rec.Code != http.StatusOK || body.HubItemID == "" {
		t.Fatalf("import commit = %d %+v", rec.Code, body)
	}
	return body.ID, body.HubItemID
}

// hubSyncSourceID finds the source a url_zip import created for a URL.
func hubSyncSourceID(t *testing.T, ws, sourceURL string) string {
	t.Helper()
	var id string
	dbfx.QueryRow(t, `SELECT id::text FROM hub_source WHERE workspace_id = $1 AND type = 'url_zip' AND url = $2`, ws, sourceURL).Scan(&id)
	return id
}

func syncHubSourceForTest(t *testing.T, ws, owner, sourceID string) *testutil.Response {
	t.Helper()
	return testutil.Call(t, testHandler.SyncHubSource,
		testutil.WithURLParams(hubTestRequest(http.MethodPost, "/api/hub/sources/"+sourceID+"/sync", nil, ws, owner), "id", sourceID))
}

func checkHubItemForTest(t *testing.T, ws, owner, kind, refID string) HubUpdateCheckResponse {
	t.Helper()
	var out HubUpdateCheckResponse
	testutil.Call(t, testHandler.GetHubItemUpdate,
		testutil.WithURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/"+kind+"/"+refID+"/update", nil, ws, owner), "kind", kind, "id", refID)).
		Want(http.StatusOK).JSON(&out)
	return out
}

func applyHubItemForTest(t *testing.T, ws, owner, kind, refID string, body map[string]any) *testutil.Response {
	t.Helper()
	return testutil.Call(t, testHandler.ApplyHubItemUpdate,
		testutil.WithURLParams(hubTestRequest(http.MethodPost, "/api/hub/items/"+kind+"/"+refID+"/update/apply", body, ws, owner), "kind", kind, "id", refID))
}

// editSkillForTest saves a skill's content through the normal skill route.
func editSkillForTest(t *testing.T, ws, owner, skillID, content string) {
	t.Helper()
	testutil.Call(t, testHandler.UpdateSkill,
		testutil.WithURLParams(hubTestRequest(http.MethodPut, "/api/skills/"+skillID, map[string]any{"content": content}, ws, owner), "id", skillID)).
		Want(http.StatusOK)
}

func skillContentForTest(t *testing.T, skillID string) string {
	t.Helper()
	var content string
	dbfx.QueryRow(t, `SELECT content FROM skill WHERE id = $1`, skillID).Scan(&content)
	return content
}

func hubItemStateForTest(t *testing.T, itemID string) (tracking string, updateAvailable, localModified bool, sourceDigest string) {
	t.Helper()
	dbfx.QueryRow(t, `SELECT tracking, update_available, local_modified, source_digest FROM hub_item WHERE id = $1`, itemID).
		Scan(&tracking, &updateAvailable, &localModified, &sourceDigest)
	return
}

// decodeErrorCode returns the machine-readable code of an error response.
func decodeErrorCode(t *testing.T, resp *testutil.Response) string {
	t.Helper()
	code, _ := resp.Map()["code"].(string)
	return code
}

func hubActivityActionsForTest(t *testing.T, ws, owner string) map[string]bool {
	t.Helper()
	var out HubActivityResponse
	testutil.Call(t, testHandler.GetHubActivity,
		hubTestRequest(http.MethodGet, "/api/hub/activity?limit=200", nil, ws, owner)).Want(http.StatusOK).JSON(&out)
	seen := map[string]bool{}
	for _, e := range out.Entries {
		seen[e.Action] = true
	}
	return seen
}

func TestHubSourceIsDue(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { t := now.Add(-d); return &t }
	cases := []struct {
		name     string
		enabled  bool
		interval int32
		last     *time.Time
		want     bool
	}{
		{"never synced", true, 300, nil, true},
		{"interval elapsed exactly", true, 300, ago(5 * time.Minute), true},
		{"interval not elapsed", true, 300, ago(4 * time.Minute), false},
		{"disabled", false, 300, ago(time.Hour), false},
		{"manual only", true, 0, ago(time.Hour), false},
	}
	for _, c := range cases {
		if got := hubSourceIsDue(c.enabled, c.interval, c.last, now); got != c.want {
			t.Errorf("%s: due = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestHubSyncDetectsRemoteChangeAndUnchanged(t *testing.T) {
	setupHubImportTest(t)
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	remote := newHubSyncRemote(t)
	name := "sync-detect-" + hubItemsSuffix()
	path := "/detect.zip"
	remote.set(path, hubSyncSkillZip(t, name, "v1", "# v1"))
	sourceURL := remote.url(path)
	skillID, itemID := importHubSkillForSync(t, f.ws, owner, sourceURL)
	sourceID := hubSyncSourceID(t, f.ws, sourceURL)

	var first HubSourceSyncResponse
	syncHubSourceForTest(t, f.ws, owner, sourceID).Want(http.StatusOK).JSON(&first)
	if first.Status != "ok" || first.Checked != 1 || first.Unchanged != 1 || first.UpdatesAvailable != 0 {
		t.Fatalf("unchanged sync = %+v", first)
	}
	if _, update, _, _ := hubItemStateForTest(t, itemID); update {
		t.Fatal("update_available set after an unchanged sync")
	}

	remote.set(path, hubSyncSkillZip(t, name, "v2", "# v2"))
	var second HubSourceSyncResponse
	syncHubSourceForTest(t, f.ws, owner, sourceID).Want(http.StatusOK).JSON(&second)
	if second.UpdatesAvailable != 1 || second.Items[0].Change != hubChangeUpdate {
		t.Fatalf("changed sync = %+v", second)
	}
	tracking, update, local, _ := hubItemStateForTest(t, itemID)
	if tracking != "tracking" || !update || local {
		t.Fatalf("item state after change: tracking=%s update=%v local=%v", tracking, update, local)
	}
	if got := hubActivityActionsForTest(t, f.ws, owner); !got["source_synced"] || !got["import_committed"] {
		t.Fatalf("activity actions = %v, want source_synced and import_committed", got)
	}
	_ = skillID
}

func TestHubSyncErrorIsolatedPerItem(t *testing.T) {
	setupHubImportTest(t)
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	remote := newHubSyncRemote(t)
	suffix := hubItemsSuffix()
	goodPath, badPath := "/good-"+suffix+".zip", "/bad-"+suffix+".zip"
	remote.set(goodPath, hubSyncSkillZip(t, "sync-good-"+suffix, "good", "# good"))
	remote.set(badPath, hubSyncSkillZip(t, "sync-bad-"+suffix, "bad", "# bad"))
	_, goodItem := importHubSkillForSync(t, f.ws, owner, remote.url(goodPath))
	_, badItem := importHubSkillForSync(t, f.ws, owner, remote.url(badPath))
	// Both skills share one source only if they share a URL, so each has its own.
	remote.drop(badPath)

	var resp HubSourceSyncResponse
	syncHubSourceForTest(t, f.ws, owner, hubSyncSourceID(t, f.ws, remote.url(goodPath))).Want(http.StatusOK).JSON(&resp)
	_ = resp
	var badResp HubSourceSyncResponse
	syncHubSourceForTest(t, f.ws, owner, hubSyncSourceID(t, f.ws, remote.url(badPath))).Want(http.StatusOK).JSON(&badResp)
	if badResp.Status != "error" || badResp.Checked != 1 || len(badResp.Items) != 1 || badResp.Items[0].Change != hubChangeError {
		t.Fatalf("failing source sync = %+v", badResp)
	}
	if badResp.Items[0].Error == "" {
		t.Fatal("failing item has no error text")
	}
	if resp.Status != "ok" || resp.Unchanged != 1 {
		t.Fatalf("healthy source sync = %+v", resp)
	}
	_, _ = goodItem, badItem
}

func TestHubItemUpdateDiffAndApplyOverwrite(t *testing.T) {
	setupHubImportTest(t)
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	remote := newHubSyncRemote(t)
	name := "sync-apply-" + hubItemsSuffix()
	path := "/apply.zip"
	remote.set(path, hubSyncSkillZip(t, name, "v1", "# v1"))
	skillID, itemID := importHubSkillForSync(t, f.ws, owner, remote.url(path))

	// A local edit makes the item locally modified; the remote then moves on.
	editSkillForTest(t, f.ws, owner, skillID, "---\nname: "+name+"\ndescription: v1\n---\n\n# local edit\n")
	if _, _, local, _ := hubItemStateForTest(t, itemID); !local {
		t.Fatal("local edit did not flag the hub item as locally modified")
	}
	remote.set(path, hubSyncSkillZip(t, name, "v2", "# v2"))

	check := checkHubItemForTest(t, f.ws, owner, "skill", skillID)
	if !check.UpdateAvailable || !check.LocalModified {
		t.Fatalf("check = %+v, want update and local changes", check)
	}
	var sawModified bool
	for _, file := range check.Files {
		if file.Path == "SKILL.md" && file.Status == "modified" {
			sawModified = true
		}
	}
	if !sawModified {
		t.Fatalf("file list %+v does not mark SKILL.md modified", check.Files)
	}

	var diff HubUpdateDiffResponse
	testutil.Call(t, testHandler.GetHubItemUpdateDiff,
		testutil.WithURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/skill/"+skillID+"/update/diff?path=SKILL.md", nil, f.ws, owner), "kind", "skill", "id", skillID)).
		Want(http.StatusOK).JSON(&diff)
	if diff.Binary || diff.Old == nil || diff.New == nil || !strings.Contains(*diff.New, "# v2") || !strings.Contains(*diff.Old, "local edit") {
		t.Fatalf("diff = %+v", diff)
	}
	testutil.Call(t, testHandler.GetHubItemUpdateDiff,
		testutil.WithURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/skill/"+skillID+"/update/diff?path=../escape", nil, f.ws, owner), "kind", "skill", "id", skillID)).
		Want(http.StatusBadRequest)

	// Local changes need a choice.
	applyHubItemForTest(t, f.ws, owner, "skill", skillID, map[string]any{}).Want(http.StatusConflict)
	if code := decodeErrorCode(t, applyHubItemForTest(t, f.ws, owner, "skill", skillID, map[string]any{})); code != "local_changes" {
		t.Fatalf("code = %q, want local_changes", code)
	}

	applyHubItemForTest(t, f.ws, owner, "skill", skillID, map[string]any{"on_local_changes": "overwrite"}).Want(http.StatusOK)
	if got := skillContentForTest(t, skillID); !strings.Contains(got, "# v2") {
		t.Fatalf("skill content after overwrite = %q", got)
	}
	_, update, local, digest := hubItemStateForTest(t, itemID)
	if update || local || digest == "" {
		t.Fatalf("item after overwrite: update=%v local=%v digest=%q", update, local, digest)
	}
	if got := hubActivityActionsForTest(t, f.ws, owner); !got["update_applied"] || !got["file_saved"] {
		t.Fatalf("activity actions = %v", got)
	}
}

func TestHubItemApplyForkKeepsLocalContent(t *testing.T) {
	setupHubImportTest(t)
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	remote := newHubSyncRemote(t)
	name := "sync-fork-" + hubItemsSuffix()
	path := "/fork.zip"
	remote.set(path, hubSyncSkillZip(t, name, "v1", "# v1"))
	skillID, itemID := importHubSkillForSync(t, f.ws, owner, remote.url(path))
	editSkillForTest(t, f.ws, owner, skillID, "# keep me\n")
	remote.set(path, hubSyncSkillZip(t, name, "v2", "# v2"))

	applyHubItemForTest(t, f.ws, owner, "skill", skillID, map[string]any{"on_local_changes": "fork"}).Want(http.StatusOK)
	tracking, _, _, _ := hubItemStateForTest(t, itemID)
	if tracking != "forked" {
		t.Fatalf("tracking = %q, want forked", tracking)
	}
	if got := skillContentForTest(t, skillID); !strings.Contains(got, "keep me") {
		t.Fatalf("fork changed the content: %q", got)
	}
	// A forked item is no longer checked against its source.
	testutil.Call(t, testHandler.GetHubItemUpdate,
		testutil.WithURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/skill/"+skillID+"/update", nil, f.ws, owner), "kind", "skill", "id", skillID)).
		Want(http.StatusConflict)
	if got := hubActivityActionsForTest(t, f.ws, owner); !got["item_forked"] {
		t.Fatalf("activity actions = %v, want item_forked", got)
	}
}

func TestHubForkEndpointIsIdempotent(t *testing.T) {
	setupHubImportTest(t)
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	remote := newHubSyncRemote(t)
	path := "/forkep.zip"
	remote.set(path, hubSyncSkillZip(t, "sync-forkep-"+hubItemsSuffix(), "v1", "# v1"))
	skillID, itemID := importHubSkillForSync(t, f.ws, owner, remote.url(path))

	for i := 0; i < 2; i++ {
		var out HubItemVersionResponse
		testutil.Call(t, testHandler.ForkHubItem,
			testutil.WithURLParams(hubTestRequest(http.MethodPost, "/api/hub/items/skill/"+skillID+"/fork", nil, f.ws, owner), "kind", "skill", "id", skillID)).
			Want(http.StatusOK).JSON(&out)
		if out.Tracking != "forked" {
			t.Fatalf("fork %d tracking = %q", i, out.Tracking)
		}
	}
	if tracking, _, _, _ := hubItemStateForTest(t, itemID); tracking != "forked" {
		t.Fatalf("stored tracking = %q", tracking)
	}
}

// hubSyncNoisyText is n bytes of text that does not compress well, so the archive's
// ratio guard (100x) does not refuse a large-file fixture.
func hubSyncNoisyText(n int) string {
	var b strings.Builder
	b.Grow(n)
	x := uint32(2463534242)
	for i := 0; i < n; i++ {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		b.WriteByte(byte('a' + x%26))
	}
	return b.String()
}

func TestHubItemApplyBlockedAndWarningScans(t *testing.T) {
	setupHubImportTest(t)
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	remote := newHubSyncRemote(t)
	name := "sync-scan-" + hubItemsSuffix()
	path := "/scan.zip"
	remote.set(path, hubSyncSkillZip(t, name, "v1", "# v1"))
	skillID, _ := importHubSkillForSync(t, f.ws, owner, remote.url(path))

	// An ELF binary in the new version is blocked, and nothing is written.
	remote.set(path, hubImportZip(t, map[string]string{
		"SKILL.md": "---\nname: " + name + "\ndescription: v2\n---\n\n# v2\n",
		"bin/tool": "\x7fELF\x02\x01\x01 not really an executable",
	}))
	before := skillContentForTest(t, skillID)
	if code := decodeErrorCode(t, applyHubItemForTest(t, f.ws, owner, "skill", skillID, map[string]any{"on_local_changes": "overwrite"})); code != "scan_blocked" {
		t.Fatalf("code = %q, want scan_blocked", code)
	}
	if skillContentForTest(t, skillID) != before {
		t.Fatal("a blocked apply changed the skill")
	}

	// A large file is a warning: refused until confirmed, then applied.
	remote.set(path, hubImportZip(t, map[string]string{
		"SKILL.md":     "---\nname: " + name + "\ndescription: v3\n---\n\n# v3\n",
		"data/big.txt": hubSyncNoisyText(300 << 10),
	}))
	if resp := applyHubItemForTest(t, f.ws, owner, "skill", skillID, map[string]any{"on_local_changes": "overwrite"}); decodeErrorCode(t, resp) != "warnings_unconfirmed" {
		t.Fatalf("status %d body %s, want warnings_unconfirmed", resp.Code, resp.Body.String())
	}
	applyHubItemForTest(t, f.ws, owner, "skill", skillID, map[string]any{"on_local_changes": "overwrite", "confirm_warnings": true}).Want(http.StatusOK)
}

func TestHubItemApplyQuotaExceeded(t *testing.T) {
	setupHubImportTest(t)
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	remote := newHubSyncRemote(t)
	name := "sync-quota-" + hubItemsSuffix()
	path := "/quota.zip"
	remote.set(path, hubSyncSkillZip(t, name, "v1", "# v1"))
	skillID, _ := importHubSkillForSync(t, f.ws, owner, remote.url(path))

	usage, err := testHandler.Queries.CountHubUsage(context.Background(), parseUUID(f.ws))
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	t.Setenv(hubMaxStorageBytesEnv, fmt.Sprint(usage.StorageBytes+10))
	remote.set(path, hubImportZip(t, map[string]string{
		"SKILL.md":     "---\nname: " + name + "\ndescription: v2\n---\n\n# v2\n",
		"data/more.md": strings.Repeat("b", 4<<10),
	}))
	if code := decodeErrorCode(t, applyHubItemForTest(t, f.ws, owner, "skill", skillID, map[string]any{"on_local_changes": "overwrite"})); code != "quota_exceeded" {
		t.Fatalf("code = %q, want quota_exceeded", code)
	}
}

func TestHubSyncLockRefusesConcurrentSync(t *testing.T) {
	setupHubImportTest(t)
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	sourceID := insertHubSourceRowForTest(t, f.ws)

	// Another holder (a scheduled sync, or another instance) has the lock.
	tx, err := testPool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(context.Background())
	var locked bool
	if err := tx.QueryRow(context.Background(),
		`SELECT pg_try_advisory_xact_lock(hashtextextended('hub_source_sync:' || $1::text, 0))`, sourceID).Scan(&locked); err != nil || !locked {
		t.Fatalf("holder lock: locked=%v err=%v", locked, err)
	}
	if code := decodeErrorCode(t, syncHubSourceForTest(t, f.ws, owner, sourceID)); code != "sync_in_progress" {
		t.Fatalf("manual sync during a held lock: code = %q, want sync_in_progress", code)
	}
	// The scheduled path skips quietly while the lock is held.
	if err := testHandler.SyncHubSourceAuto(context.Background(), sourceID); err != nil {
		t.Fatalf("auto sync while locked: %v", err)
	}
	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatalf("release: %v", err)
	}
	syncHubSourceForTest(t, f.ws, owner, sourceID).Want(http.StatusOK)
}

func TestHubSourceDueSelection(t *testing.T) {
	f := hubSyncWorkspace(t)
	insert := func(name string, enabled bool, interval int, lastSyncAgo string) string {
		last := "NULL"
		if lastSyncAgo != "" {
			last = "now() - interval '" + lastSyncAgo + "'"
		}
		id := dbfx.Insert(t, "hub_source", testutil.Cols{
			"workspace_id":           f.ws,
			"name":                   hubTestLabel(t, name),
			"type":                   "url_zip",
			"url":                    "https://example.test/" + hubTestUUID(t),
			"enabled":                enabled,
			"auto_sync_interval_sec": interval,
		})
		dbfx.Exec(t, "UPDATE hub_source SET last_sync_at = "+last+" WHERE id = $1", id)
		return id
	}
	dueNever := insert("never", true, 300, "")
	dueOld := insert("old", true, 300, "10 minutes")
	notYet := insert("recent", true, 300, "1 minute")
	disabled := insert("disabled", false, 300, "")
	manual := insert("manual", true, 0, "")

	rows, err := testHandler.Queries.ListDueHubSources(context.Background(), 10000)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	due := map[string]bool{}
	for _, r := range rows {
		due[uuidToString(r.ID)] = true
	}
	if !due[dueNever] || !due[dueOld] {
		t.Fatalf("due sources missing: never=%v old=%v", due[dueNever], due[dueOld])
	}
	for name, id := range map[string]string{"recent": notYet, "disabled": disabled, "manual": manual} {
		if due[id] {
			t.Fatalf("%s source selected as due", name)
		}
	}
}

func TestHubSyncAutoRunsOnlyWhenDue(t *testing.T) {
	setupHubImportTest(t)
	f := hubSyncWorkspace(t)
	sourceID := dbfx.Insert(t, "hub_source", testutil.Cols{
		"workspace_id":           f.ws,
		"name":                   hubTestLabel(t, "auto"),
		"type":                   "url_zip",
		"url":                    "https://example.test/" + hubTestUUID(t),
		"auto_sync_interval_sec": 60,
	})
	dbfx.Exec(t, "UPDATE hub_source SET last_sync_at = now() - interval '5 minutes' WHERE id = $1", sourceID)

	if err := testHandler.SyncHubSourceAuto(context.Background(), sourceID); err != nil {
		t.Fatalf("due auto sync: %v", err)
	}
	var stamp time.Time
	dbfx.QueryRow(t, `SELECT last_sync_at FROM hub_source WHERE id = $1`, sourceID).Scan(&stamp)
	if time.Since(stamp) > time.Minute {
		t.Fatalf("auto sync did not record last_sync_at: %v", stamp)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM hub_activity WHERE workspace_id = $1 AND action = 'source_synced' AND source_id = $2 AND actor_type = 'system'`, f.ws, sourceID); n != 1 {
		t.Fatalf("system source_synced rows = %d, want 1", n)
	}
	// Synced a moment ago: not due, so nothing runs.
	if err := testHandler.SyncHubSourceAuto(context.Background(), sourceID); err != nil {
		t.Fatalf("not-due auto sync: %v", err)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM hub_activity WHERE workspace_id = $1 AND action = 'source_synced' AND source_id = $2`, f.ws, sourceID); n != 1 {
		t.Fatalf("source_synced rows after a not-due call = %d, want 1", n)
	}
}
