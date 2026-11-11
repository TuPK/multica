package handler

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util/secretbox"
)

// A private source's token: sealed at rest, returned never, sent only to the
// source's own host, and kept out of responses and logs.

// credentialRemote serves one zip. When token is set, a request must carry it as a
// bearer token, or it gets a 404 like a private repository would. Every request's
// Authorization header is recorded.
type credentialRemote struct {
	srv   *httptest.Server
	mu    sync.Mutex
	token string
	body  []byte
	seen  []string
	// redirects maps a path to the URL it redirects to.
	redirects map[string]string
	// fail makes every request answer 500, to exercise error paths.
	fail bool
}

func newCredentialRemote(t *testing.T, body []byte) *credentialRemote {
	t.Helper()
	remote := &credentialRemote{body: body, redirects: map[string]string{}}
	remote.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remote.mu.Lock()
		remote.seen = append(remote.seen, r.Header.Get("Authorization"))
		token, fail := remote.token, remote.fail
		target, redirected := remote.redirects[r.URL.Path]
		remote.mu.Unlock()
		switch {
		case fail:
			http.Error(w, "upstream exploded", http.StatusInternalServerError)
		case redirected:
			http.Redirect(w, r, target, http.StatusFound)
		case token != "" && r.Header.Get("Authorization") != "Bearer "+token:
			http.NotFound(w, r)
		default:
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(remote.srv.Close)
	return remote
}

func (c *credentialRemote) setToken(token string) {
	c.mu.Lock()
	c.token = token
	c.mu.Unlock()
}

func (c *credentialRemote) setRedirect(path, target string) {
	c.mu.Lock()
	if c.redirects == nil {
		c.redirects = map[string]string{}
	}
	c.redirects[path] = target
	c.mu.Unlock()
}

func (c *credentialRemote) authHeaders() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.seen...)
}

// useHubSecrets installs a fresh sealing key for one test and restores the old one.
func useHubSecrets(t *testing.T) *secretbox.Box {
	t.Helper()
	key := make([]byte, secretbox.KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatalf("secretbox.New: %v", err)
	}
	prev := testHandler.HubSecrets
	testHandler.HubSecrets = box
	t.Cleanup(func() { testHandler.HubSecrets = prev })
	return box
}

// lockedBuffer collects log output from the goroutines a sync starts.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureHubLogs sends the default logger to a buffer for the rest of the test.
func captureHubLogs(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

func createSourceWithCredentialForTest(t *testing.T, ws, owner, sourceType, rawURL, credential string) *testutil.Response {
	t.Helper()
	body := map[string]any{"name": hubTestLabel(t, "private"), "type": sourceType, "url": rawURL}
	if credential != "" {
		body["credential"] = credential
	}
	return testutil.Call(t, testHandler.CreateHubSource, hubTestRequest(http.MethodPost, "/api/hub/sources", body, ws, owner))
}

func patchHubSourceForTest(t *testing.T, ws, owner, sourceID string, body map[string]any) *testutil.Response {
	t.Helper()
	return testutil.Call(t, testHandler.UpdateHubSource,
		testutil.WithURLParams(hubTestRequest(http.MethodPatch, "/api/hub/sources/"+sourceID, body, ws, owner), "id", sourceID))
}

func storedCredentialForTest(t *testing.T, sourceID string) []byte {
	t.Helper()
	var sealed []byte
	dbfx.QueryRow(t, `SELECT credential_ciphertext FROM hub_source WHERE id = $1`, sourceID).Scan(&sealed)
	return sealed
}

func TestHubSourceCredential_CreateSealsAndHidesToken(t *testing.T) {
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	useHubSecrets(t)
	const token = "tok-create-probe-7f3a"

	rec := createSourceWithCredentialForTest(t, f.ws, owner, "url_zip", "https://files.example.test/private/skills.zip", token)
	rec.Want(http.StatusCreated)
	if strings.Contains(rec.Text(), token) {
		t.Fatalf("create response carries the token: %s", rec.Text())
	}
	var out HubSourceResponse
	rec.JSON(&out)
	dbfx.Cleanup(t, "DELETE FROM hub_source WHERE id = $1", out.ID)
	if !out.HasCredential {
		t.Fatalf("has_credential = false after a credential was saved")
	}

	sealed := storedCredentialForTest(t, out.ID)
	if len(sealed) == 0 {
		t.Fatal("no ciphertext stored")
	}
	if bytes.Contains(sealed, []byte(token)) {
		t.Fatal("the token is stored in plaintext")
	}

	var list []HubSourceResponse
	testutil.Call(t, testHandler.ListHubSources, hubTestRequest(http.MethodGet, "/api/hub/sources", nil, f.ws, owner)).
		Want(http.StatusOK).JSON(&list)
	found := false
	for _, s := range list {
		if s.ID == out.ID {
			found = s.HasCredential
		}
	}
	if !found {
		t.Fatal("list does not report has_credential for the source")
	}
}

func TestHubSourceCredential_OnlyNetworkSourcesTakeOne(t *testing.T) {
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	useHubSecrets(t)

	for _, sourceType := range []string{"skills_sh", "clawhub"} {
		rec := createSourceWithCredentialForTest(t, f.ws, owner, sourceType, "https://"+sourceType+".example.test/"+hubTestSlug(t), "tok")
		rec.Want(http.StatusBadRequest)
	}
	rec := createSourceWithCredentialForTest(t, f.ws, owner, "upload", "", "tok")
	rec.Want(http.StatusBadRequest)
}

func TestHubSourceCredential_RefusedWithoutSecretKey(t *testing.T) {
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	prev := testHandler.HubSecrets
	testHandler.HubSecrets = nil
	t.Cleanup(func() { testHandler.HubSecrets = prev })

	const url = "https://refused.example.test/private.zip"
	rec := createSourceWithCredentialForTest(t, f.ws, owner, "url_zip", url, "tok-never-stored")
	rec.Want(http.StatusConflict)
	if code := decodeErrorCode(t, rec); code != "secrets_not_configured" {
		t.Fatalf("code = %q, want secrets_not_configured", code)
	}
	var rows int
	dbfx.QueryRow(t, `SELECT count(*) FROM hub_source WHERE workspace_id = $1 AND url = $2`, f.ws, url).Scan(&rows)
	if rows != 0 {
		t.Fatalf("a refused source was stored (%d rows)", rows)
	}
}

func TestHubSourceCredential_PatchSetClearAndHostChange(t *testing.T) {
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	useHubSecrets(t)
	const token = "tok-patch-probe-91c"

	var src HubSourceResponse
	createSourceWithCredentialForTest(t, f.ws, owner, "url_zip", "https://patch.example.test/a.zip", "").Want(http.StatusCreated).JSON(&src)
	dbfx.Cleanup(t, "DELETE FROM hub_source WHERE id = $1", src.ID)
	if src.HasCredential {
		t.Fatal("a source without a credential reports has_credential")
	}

	patchHubSourceForTest(t, f.ws, owner, src.ID, map[string]any{"credential": ""}).Want(http.StatusBadRequest)
	patchHubSourceForTest(t, f.ws, owner, src.ID, map[string]any{"credential": "x", "clear_credential": true}).Want(http.StatusBadRequest)

	rec := patchHubSourceForTest(t, f.ws, owner, src.ID, map[string]any{"credential": token}).Want(http.StatusOK)
	if strings.Contains(rec.Text(), token) {
		t.Fatalf("patch response carries the token: %s", rec.Text())
	}
	if len(storedCredentialForTest(t, src.ID)) == 0 {
		t.Fatal("credential was not stored by PATCH")
	}

	var cleared HubSourceResponse
	patchHubSourceForTest(t, f.ws, owner, src.ID, map[string]any{"clear_credential": true}).Want(http.StatusOK).JSON(&cleared)
	if cleared.HasCredential || storedCredentialForTest(t, src.ID) != nil {
		t.Fatal("clear_credential did not remove the stored token")
	}

	// A token is pinned to its host: moving the source elsewhere drops it.
	patchHubSourceForTest(t, f.ws, owner, src.ID, map[string]any{"credential": token}).Want(http.StatusOK)
	var moved HubSourceResponse
	patchHubSourceForTest(t, f.ws, owner, src.ID, map[string]any{"url": "https://elsewhere.example.test/a.zip"}).Want(http.StatusOK).JSON(&moved)
	if moved.HasCredential || storedCredentialForTest(t, src.ID) != nil {
		t.Fatal("a token survived a move to another host")
	}
}

// The token reaches the source's host and never a host a redirect moves to. A
// preview with a saved source uses the token only for that source's exact URL.
func TestHubSourceCredential_SyncAndPreviewSendTokenToSourceHostOnly(t *testing.T) {
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	useHubSecrets(t)
	setupHubImportTest(t)
	const token = "tok-source-host-3d"

	body := hubSyncSkillZip(t, "private-skill", "Reads private notes.", "Use the private notes.")
	origin := newCredentialRemote(t, body)
	target := newCredentialRemote(t, body)
	origin.setRedirect("/moved.zip", target.srv.URL+"/skill.zip")

	// Imported while public, through a redirect, so the source and item exist.
	movedURL := origin.srv.URL + "/moved.zip"
	importHubSkillForSync(t, f.ws, owner, movedURL)
	sourceID := hubSyncSourceID(t, f.ws, movedURL)

	origin.setToken(token)
	target.setToken(token)
	patchHubSourceForTest(t, f.ws, owner, sourceID, map[string]any{"credential": token}).Want(http.StatusOK)

	// The origin gets the token, the redirect target must not: the preview fails.
	testutil.Call(t, testHandler.PreviewHubImport, hubTestRequest(http.MethodPost, "/api/hub/import/preview",
		map[string]any{"source": map[string]any{"type": "url_zip", "url": movedURL, "source_id": sourceID}}, f.ws, owner)).
		Want(http.StatusBadGateway)
	if !hasBearer(origin.authHeaders(), token) {
		t.Fatalf("origin never received the bearer token: %q", origin.authHeaders())
	}
	for _, h := range target.authHeaders() {
		if h != "" {
			t.Fatalf("redirect target received Authorization %q", h)
		}
	}

	// On the source's own URL the token is used and the preview succeeds.
	directURL := origin.srv.URL + "/skill.zip"
	patchHubSourceForTest(t, f.ws, owner, sourceID, map[string]any{"url": directURL}).Want(http.StatusOK)
	testutil.Call(t, testHandler.PreviewHubImport, hubTestRequest(http.MethodPost, "/api/hub/import/preview",
		map[string]any{"source": map[string]any{"type": "url_zip", "url": directURL, "source_id": sourceID}}, f.ws, owner)).
		Want(http.StatusOK)

	// A preview naming a different URL cannot borrow the saved source's token.
	testutil.Call(t, testHandler.PreviewHubImport, hubTestRequest(http.MethodPost, "/api/hub/import/preview",
		map[string]any{"source": map[string]any{"type": "url_zip", "url": movedURL, "source_id": sourceID}}, f.ws, owner)).
		Want(http.StatusBadRequest)

	// Sync checks the item against the same source with the token.
	var synced HubSourceSyncResponse
	syncHubSourceForTest(t, f.ws, owner, sourceID).Want(http.StatusOK).JSON(&synced)
	if synced.Checked != 1 || synced.Status != "ok" {
		t.Fatalf("sync with the saved token = %+v, want one item checked and ok", synced)
	}
}

func hasBearer(headers []string, token string) bool {
	for _, h := range headers {
		if h == "Bearer "+token {
			return true
		}
	}
	return false
}

// The token must not appear in a response or a log line, on success or failure,
// and a stored credential the server cannot open is reported without the token.
func TestHubSourceCredential_TokenNeverLeaksToResponsesOrLogs(t *testing.T) {
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	useHubSecrets(t)
	setupHubImportTest(t)
	logs := captureHubLogs(t)
	const token = "tok-leak-probe-q8w2e"

	remote := newCredentialRemote(t, hubSyncSkillZip(t, "leak-skill", "Checks leaks.", "Body."))
	sourceURL := remote.srv.URL + "/skill.zip"
	// Imported while public, so the source and its tracking item exist.
	importHubSkillForSync(t, f.ws, owner, sourceURL)
	sourceID := hubSyncSourceID(t, f.ws, sourceURL)

	remote.setToken(token)
	patchHubSourceForTest(t, f.ws, owner, sourceID, map[string]any{"credential": token}).Want(http.StatusOK)

	// A preview that fails upstream while the token is in use.
	remote.mu.Lock()
	remote.fail = true
	remote.mu.Unlock()
	failed := testutil.Call(t, testHandler.PreviewHubImport, hubTestRequest(http.MethodPost, "/api/hub/import/preview",
		map[string]any{"source": map[string]any{"type": "url_zip", "url": sourceURL, "source_id": sourceID}}, f.ws, owner))
	if failed.Code < 400 {
		t.Fatalf("preview against a failing origin = %d, want an error", failed.Code)
	}

	// A sync whose item check fails upstream: the error is recorded on the source.
	var synced HubSourceSyncResponse
	syncHubSourceForTest(t, f.ws, owner, sourceID).Want(http.StatusOK).JSON(&synced)
	if synced.Status != "error" {
		t.Fatalf("sync against a failing origin = %+v, want an error status", synced)
	}
	var lastErr string
	dbfx.QueryRow(t, `SELECT last_sync_error FROM hub_source WHERE id = $1`, sourceID).Scan(&lastErr)
	remote.mu.Lock()
	remote.fail = false
	remote.mu.Unlock()

	// A server with another key cannot open the stored token: the preview says so
	// and the sync records it, neither with the token.
	useHubSecrets(t)
	unopened := testutil.Call(t, testHandler.PreviewHubImport, hubTestRequest(http.MethodPost, "/api/hub/import/preview",
		map[string]any{"source": map[string]any{"type": "url_zip", "url": sourceURL, "source_id": sourceID}}, f.ws, owner))
	unopened.Want(http.StatusConflict)
	var unopenedSync HubSourceSyncResponse
	syncHubSourceForTest(t, f.ws, owner, sourceID).Want(http.StatusOK).JSON(&unopenedSync)
	if len(unopenedSync.Items) != 1 || !strings.Contains(unopenedSync.Items[0].Error, "credential") {
		t.Fatalf("sync with an unopenable credential = %+v, want a credential error", unopenedSync)
	}

	for name, text := range map[string]string{
		"failed preview response": failed.Text(),
		"unopened preview":        unopened.Text(),
		"unopened sync response":  hubCredJSON(t, unopenedSync),
		"recorded sync error":     lastErr,
		"log output":              logs.String(),
	} {
		if strings.Contains(text, token) {
			t.Errorf("%s contains the token", name)
		}
	}
	if strings.Contains(logs.String(), "tok-") {
		t.Errorf("log output carries a token-shaped value")
	}
}

func hubCredJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func decodeHubSourceID(t *testing.T, rec *testutil.Response) string {
	t.Helper()
	var out HubSourceResponse
	rec.JSON(&out)
	return out.ID
}
