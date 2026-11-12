package handler

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/publish"
	proposalkinds "github.com/multica-ai/multica/server/internal/service/proposal_kinds"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/internal/util/secretbox"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// These tests use the handler package's database. Ayrshare is always a fake
// httptest server: nothing in this file reaches the real API.

// fakeAyrshareAPI serves each path from its own handler and records every call.
type fakeAyrshareAPI struct {
	mu     sync.Mutex
	url    string
	calls  map[string]int
	seen   []*http.Request
	bodies []map[string]any
}

func newFakeAyrshareAPI(t *testing.T, routes map[string]http.HandlerFunc) *fakeAyrshareAPI {
	t.Helper()
	f := &fakeAyrshareAPI{calls: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.calls[r.URL.Path]++
		f.seen = append(f.seen, r)
		if r.Body != nil && r.ContentLength != 0 {
			raw, _ := io.ReadAll(r.Body)
			var b map[string]any
			_ = json.Unmarshal(raw, &b)
			f.bodies = append(f.bodies, b)
		}
		f.mu.Unlock()
		if h, ok := routes[r.URL.Path]; ok {
			h(w, r)
			return
		}
		http.Error(w, `{"status":"error","message":"no fake route"}`, http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f
}

func (f *fakeAyrshareAPI) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[path]
}

func (f *fakeAyrshareAPI) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		n += c
	}
	return n
}

func (f *fakeAyrshareAPI) lastRequest() *http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seen[len(f.seen)-1]
}

func (f *fakeAyrshareAPI) lastBody() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[len(f.bodies)-1]
}

// setPublishService points the handler at a publishing service for one test and
// restores the previous one after it. box may be nil, which is a server without a
// publish secret key. baseURL "" keeps the production Ayrshare URL, which no
// test calls.
func setPublishService(t *testing.T, box *secretbox.Box, baseURL string) *publish.Service {
	t.Helper()
	prev := testHandler.Publish
	svc := publish.NewService(testHandler.Queries, box)
	if baseURL != "" {
		svc.BaseURL = baseURL
	}
	svc.Backoff = []time.Duration{0, 0}
	testHandler.Publish = svc
	t.Cleanup(func() { testHandler.Publish = prev })
	return svc
}

// configureTestKeys stores the workspace's Ayrshare key through the service, as
// the settings endpoint does.
func configureTestKeys(t *testing.T, svc *publish.Service, ws string, ayrshareKey string) {
	t.Helper()
	if _, err := svc.SaveSettings(context.Background(), mustParseUUID(t, ws), publish.SettingsPatch{AyrshareKey: &ayrshareKey}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
}

func newPublishBox(t *testing.T) *secretbox.Box {
	t.Helper()
	key := make([]byte, secretbox.KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatalf("secretbox: %v", err)
	}
	return box
}

// resetPublishRows removes this workspace's publishing rows before and after the
// test, so a leftover row from an earlier run cannot be claimed by the publisher.
func resetPublishRows(t *testing.T) {
	t.Helper()
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	clean := func() {
		ctx := context.Background()
		testPool.Exec(ctx, `DELETE FROM publish_attempt WHERE workspace_id = $1`, testWorkspaceID)
		testPool.Exec(ctx, `DELETE FROM publish_channel WHERE workspace_id = $1`, testWorkspaceID)
		testPool.Exec(ctx, `DELETE FROM publish_profile WHERE workspace_id = $1`, testWorkspaceID)
		testPool.Exec(ctx, `DELETE FROM publish_settings WHERE workspace_id = $1`, testWorkspaceID)
	}
	clean()
	t.Cleanup(clean)
}

func publishChannelsPath() string { return "/api/publish-channels" }

// mustParseUUID parses a UUID the test created, failing the test if it is malformed.
func mustParseUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	id, err := util.ParseUUID(s)
	if err != nil {
		t.Fatalf("uuid %q: %v", s, err)
	}
	return id
}

// channelRequest builds a request for one channel. suffix is the action path
// after the id: "", "/connect", "/check" or "/dry-run".
func channelRequest(method, suffix, id string, body any) *http.Request {
	req := newRequest(method, publishChannelsPath()+"/"+id+suffix, body)
	return withURLParam(req, "id", id)
}

// serve runs one handler against a request and returns what it wrote.
func serve(fn func(http.ResponseWriter, *http.Request), req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	fn(w, req)
	return w
}

func createTestChannel(t *testing.T, platform, market, account string) string {
	t.Helper()
	var out PublishChannelResponse
	req := newRequest("POST", publishChannelsPath(), CreatePublishChannelRequest{Platform: platform, Market: market, AccountName: account})
	testutil.Call(t, testHandler.CreatePublishChannel, req).Want(http.StatusCreated).JSON(&out)
	return out.ID
}

func TestPublishChannelsReportNotConfiguredWithoutCallingOut(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	setPublishService(t, newPublishBox(t), "")

	id := createTestChannel(t, "x", "intl", "@founder")

	var list struct {
		Channels []PublishChannelResponse `json:"channels"`
	}
	testutil.Call(t, testHandler.ListPublishChannels, newRequest("GET", publishChannelsPath(), nil)).Want(http.StatusOK).JSON(&list)
	if len(list.Channels) != 1 || list.Channels[0].Status != "error" || list.Channels[0].StatusDetail != "Ayrshare not configured" {
		t.Fatalf("list = %+v, want one channel in error with the not-configured detail", list.Channels)
	}

	w := serve(testHandler.ConnectPublishChannel, channelRequest("POST", "/connect", id, nil))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "ayrshare_not_configured") {
		t.Fatalf("connect without a key = %d %s, want 503 ayrshare_not_configured", w.Code, w.Body.String())
	}

	var checked PublishChannelResponse
	testutil.Call(t, testHandler.CheckPublishChannel, channelRequest("POST", "/check", id, nil)).Want(http.StatusOK).JSON(&checked)
	if checked.Status != "error" || checked.StatusDetail != "Ayrshare not configured" {
		t.Fatalf("check = %+v", checked)
	}
}

func TestPublishChannelCreateValidation(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	setPublishService(t, newPublishBox(t), "")

	bad := func(body CreatePublishChannelRequest) {
		t.Helper()
		testutil.Call(t, testHandler.CreatePublishChannel, newRequest("POST", publishChannelsPath(), body)).Want(http.StatusBadRequest)
	}
	bad(CreatePublishChannelRequest{Platform: "myspace", Market: "vn", AccountName: "a"})
	bad(CreatePublishChannelRequest{Platform: "x", Market: "mars", AccountName: "a"})
	bad(CreatePublishChannelRequest{Platform: "x", Market: "intl", AccountName: "   "})

	createTestChannel(t, "linkedin", "intl", "Founder")
	testutil.Call(t, testHandler.CreatePublishChannel,
		newRequest("POST", publishChannelsPath(), CreatePublishChannelRequest{Platform: "linkedin", Market: "intl", AccountName: "Other"})).
		Want(http.StatusConflict)
}

func TestPublishChannelDryRunReturnsTheExactBodyWithoutSending(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	fake := newFakeAyrshareAPI(t, nil)
	setPublishService(t, newPublishBox(t), fake.url)
	id := createTestChannel(t, "x", "intl", "@founder")

	var out struct {
		Body map[string]any `json:"body"`
	}
	req := channelRequest("POST", "/dry-run", id, DryRunPublishChannelRequest{
		Text: "Shipped it", Hashtags: []string{"golang"}, MediaURL: "https://cdn.example.com/a.png",
	})
	testutil.Call(t, testHandler.DryRunPublishChannel, req).Want(http.StatusOK).JSON(&out)

	if out.Body["post"] != "Shipped it\n\n#golang" {
		t.Fatalf("post = %v", out.Body["post"])
	}
	if platforms, _ := out.Body["platforms"].([]any); len(platforms) != 1 || platforms[0] != "twitter" {
		t.Fatalf("platforms = %v", out.Body["platforms"])
	}
	if _, ok := out.Body["idempotencyKey"]; ok {
		t.Fatal("a dry run has no attempt, so no idempotency key")
	}
	if fake.total() != 0 {
		t.Fatalf("dry run reached Ayrshare %d times", fake.total())
	}
}

func TestPublishChannelCheckReportsLinkedAccount(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	box := newPublishBox(t)
	linked := []string{"twitter"}
	fake := newFakeAyrshareAPI(t, map[string]http.HandlerFunc{
		"/api/user": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "activeSocialAccounts": linked})
		},
	})
	svc := setPublishService(t, box, fake.url)
	configureTestKeys(t, svc, testWorkspaceID, "ayr-check-key")
	id := createTestChannel(t, "x", "intl", "@founder")
	sealed, err := box.Seal([]byte("profile-key-123"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	dbfx.Exec(t, `INSERT INTO publish_profile (workspace_id, profile_key_sealed) VALUES ($1, $2)`, testWorkspaceID, sealed)

	var got PublishChannelResponse
	testutil.Call(t, testHandler.CheckPublishChannel, channelRequest("POST", "/check", id, nil)).Want(http.StatusOK).JSON(&got)
	if got.Status != "ready" || got.LastCheckedAt == nil {
		t.Fatalf("linked channel = %+v, want ready with a check time", got)
	}
	if h := fake.lastRequest().Header.Get("Profile-Key"); h != "profile-key-123" {
		t.Fatalf("Profile-Key header = %q, want the workspace's key", h)
	}
	if h := fake.lastRequest().Header.Get("Authorization"); h != "Bearer ayr-check-key" {
		t.Fatalf("Authorization header = %q, want the workspace's Ayrshare key", h)
	}

	linked = []string{"linkedin"}
	testutil.Call(t, testHandler.CheckPublishChannel, channelRequest("POST", "/check", id, nil)).Want(http.StatusOK).JSON(&got)
	if got.Status != "not_connected" {
		t.Fatalf("unlinked network status = %q, want not_connected", got.Status)
	}
}

func TestPublishConnectCreatesTheProfileOnce(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	box := newPublishBox(t)
	fake := newFakeAyrshareAPI(t, map[string]http.HandlerFunc{
		"/api/profiles": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "profileKey": "pk-new", "refId": "ref-1"})
		},
		"/api/profiles/link-sessions": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "url": "https://profile.ayrshare.com?session=s1"})
		},
	})
	svc := setPublishService(t, box, fake.url)
	configureTestKeys(t, svc, testWorkspaceID, "ayr-connect-key")
	id := createTestChannel(t, "facebook", "vn", "Trang chính")

	var out map[string]string
	testutil.Call(t, testHandler.ConnectPublishChannel, channelRequest("POST", "/connect", id, nil)).Want(http.StatusOK).JSON(&out)
	if out["url"] != "https://profile.ayrshare.com?session=s1" {
		t.Fatalf("url = %q", out["url"])
	}
	testutil.Call(t, testHandler.ConnectPublishChannel, channelRequest("POST", "/connect", id, nil)).Want(http.StatusOK)

	if fake.count("/api/profiles") != 1 {
		t.Fatalf("profile created %d times, want once", fake.count("/api/profiles"))
	}
	if fake.count("/api/profiles/link-sessions") != 2 {
		t.Fatalf("link sessions = %d, want 2", fake.count("/api/profiles/link-sessions"))
	}
	var sealed []byte
	if err := testPool.QueryRow(context.Background(), `SELECT profile_key_sealed FROM publish_profile WHERE workspace_id = $1`, testWorkspaceID).Scan(&sealed); err != nil {
		t.Fatalf("read profile: %v", err)
	}
	key, err := box.Open(sealed)
	if err != nil || string(key) != "pk-new" {
		t.Fatalf("stored key opens to %q (%v); the key must be stored sealed", key, err)
	}
}

// --- Settings: per-workspace Ayrshare keys ---

func settingsPath() string { return publishChannelsPath() + "/settings" }

// TestPublishSettingsStoreKeysSealedAndNeverReturnThem checks the round trip
// through secretbox: the stored bytes are not the key, the box opens them back,
// and the API answers only with booleans.
func TestPublishSettingsStoreKeysSealedAndNeverReturnThem(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	box := newPublishBox(t)
	setPublishService(t, box, "")

	put := newRequest("PUT", settingsPath(), map[string]any{
		"ayrshare_api_key": "ayr-secret-key-1",
		"x_api_key":        "x-developer-key-1",
		"x_api_secret":     "x-developer-secret-1",
	})
	w := serve(testHandler.PutPublishSettings, put)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", w.Code, w.Body.String())
	}
	for _, secret := range []string{"ayr-secret-key-1", "x-developer-key-1", "x-developer-secret-1"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatalf("PUT response leaks a key: %s", w.Body.String())
		}
	}
	var putOut PublishSettingsResponse
	_ = json.Unmarshal(w.Body.Bytes(), &putOut)
	if !putOut.AyrshareConfigured || !putOut.XConfigured {
		t.Fatalf("PUT answer = %+v, want both configured", putOut)
	}

	var ayr, x, xs []byte
	if err := testPool.QueryRow(context.Background(),
		`SELECT ayrshare_key_sealed, x_key_sealed, x_secret_sealed FROM publish_settings WHERE workspace_id = $1`,
		testWorkspaceID).Scan(&ayr, &x, &xs); err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if strings.Contains(string(ayr), "ayr-secret-key-1") || strings.Contains(string(x), "x-developer-key-1") {
		t.Fatal("keys are stored in plaintext")
	}
	for sealed, want := range map[string]string{string(ayr): "ayr-secret-key-1", string(x): "x-developer-key-1", string(xs): "x-developer-secret-1"} {
		got, err := box.Open([]byte(sealed))
		if err != nil || string(got) != want {
			t.Fatalf("opened %q (%v), want %q", got, err, want)
		}
	}

	get := serve(testHandler.GetPublishSettings, newRequest("GET", settingsPath(), nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", get.Code, get.Body.String())
	}
	for _, secret := range []string{"ayr-secret-key-1", "x-developer-key-1", "x-developer-secret-1"} {
		if strings.Contains(get.Body.String(), secret) {
			t.Fatalf("GET response leaks a key: %s", get.Body.String())
		}
	}
	var getOut PublishSettingsResponse
	_ = json.Unmarshal(get.Body.Bytes(), &getOut)
	if !getOut.AyrshareConfigured || !getOut.XConfigured {
		t.Fatalf("GET = %+v, want both configured", getOut)
	}
}

// TestPublishSettingsPartialWriteKeepsTheOtherKeys: a PUT that omits a field
// leaves that key as it was; an empty string clears it.
func TestPublishSettingsPartialWriteKeepsTheOtherKeys(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	box := newPublishBox(t)
	setPublishService(t, box, "")

	serve(testHandler.PutPublishSettings, newRequest("PUT", settingsPath(), map[string]any{"ayrshare_api_key": "ayr-keep-me"}))
	w := serve(testHandler.PutPublishSettings, newRequest("PUT", settingsPath(), map[string]any{"x_api_key": "x-only"}))
	if w.Code != http.StatusOK {
		t.Fatalf("second PUT = %d %s", w.Code, w.Body.String())
	}
	var ayr []byte
	if err := testPool.QueryRow(context.Background(),
		`SELECT ayrshare_key_sealed FROM publish_settings WHERE workspace_id = $1`, testWorkspaceID).Scan(&ayr); err != nil {
		t.Fatalf("read: %v", err)
	}
	if got, err := box.Open(ayr); err != nil || string(got) != "ayr-keep-me" {
		t.Fatalf("a PUT without the Ayrshare key changed it: %q (%v)", got, err)
	}

	var out PublishSettingsResponse
	cleared := serve(testHandler.PutPublishSettings, newRequest("PUT", settingsPath(), map[string]any{"ayrshare_api_key": ""}))
	_ = json.Unmarshal(cleared.Body.Bytes(), &out)
	if out.AyrshareConfigured {
		t.Fatal("an empty string must clear the Ayrshare key")
	}
}

func TestPublishSettingsRefusesAMemberAndABadBody(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	setPublishService(t, newPublishBox(t), "")

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	memberID := dbfx.User(t, "Publish Member", "publish-member-"+suffix+"@example.test")
	dbfx.Member(t, testWorkspaceID, memberID, "member")
	member := newRequest("PUT", settingsPath(), map[string]any{"ayrshare_api_key": "ayr-by-member"})
	member.Header.Set("X-User-ID", memberID)
	testutil.Call(t, testHandler.PutPublishSettings, member).Want(http.StatusForbidden)

	bad := newRequest("PUT", settingsPath(), map[string]any{"ayrshare_api_key": "k", "surprise": true})
	testutil.Call(t, testHandler.PutPublishSettings, bad).Want(http.StatusBadRequest)

	spaced := newRequest("PUT", settingsPath(), map[string]any{"ayrshare_api_key": "has a space"})
	testutil.Call(t, testHandler.PutPublishSettings, spaced).Want(http.StatusBadRequest)
}

func TestPublishSettingsWithoutSecretKeyAnswers503(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	setPublishService(t, nil, "")
	w := serve(testHandler.PutPublishSettings, newRequest("PUT", settingsPath(), map[string]any{"ayrshare_api_key": "k"}))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("PUT without MULTICA_PUBLISH_SECRET_KEY = %d %s, want 503", w.Code, w.Body.String())
	}
}

// TestPublishSettingsRouteIsNotAChannelID checks the router: GET .../settings
// reaches the settings handler, not the {id} routes below it.
func TestPublishSettingsRouteIsNotAChannelID(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/api/publish-channels", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("list")) })
		r.Get("/settings", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("settings")) })
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("channel")) })
		})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/publish-channels/settings", nil))
	if w.Body.String() != "settings" {
		t.Fatalf("GET /settings reached %q, want the settings handler", w.Body.String())
	}
}

// --- Dry run, approval and send ---

// publishProposalFixture files an approved publish proposal whose payload
// carries the given channels, and returns the proposal row.
func publishProposalFixture(t *testing.T, fx proposalTestFixture, payload string) db.Proposal {
	t.Helper()
	title := fx.title(fmt.Sprintf("publish %d", time.Now().UnixNano()))
	id := dbfx.Insert(t, "proposal", testutil.Cols{
		"workspace_id":    testWorkspaceID,
		"squad_id":        fx.SquadID,
		"issue_id":        dbfx.Issue(t, title),
		"urgent":          false,
		"state":           "pending",
		"kind":            "publish",
		"payload":         payload,
		"created_by_type": "agent",
		"created_by_id":   testutil.Raw("gen_random_uuid()"),
	})
	p, err := testHandler.Queries.GetProposalInWorkspace(context.Background(), db.GetProposalInWorkspaceParams{
		ID: mustParseUUID(t, id), WorkspaceID: mustParseUUID(t, testWorkspaceID),
	})
	if err != nil {
		t.Fatalf("load proposal: %v", err)
	}
	return p
}

// payloadChannel is one channel of a test payload. The filing path fills the
// platform from the channel row; the publisher reads it from the payload, so
// fixtures that skip the filing path state it.
type payloadChannel struct{ ID, Platform string }

func publishPayloadJSON(itemID string, version int, market string, channels ...payloadChannel) string {
	entries := make([]string, 0, len(channels))
	for _, c := range channels {
		entries = append(entries, fmt.Sprintf(`{"channel_id":%q,"platform":%q,"account_name":"","text":"Hello from the squad","media_url":null,"hashtags":["launch"]}`, c.ID, c.Platform))
	}
	return fmt.Sprintf(`{"item_id":%q,"version":%d,"market":%q,"scheduled_at":null,"channels":[%s]}`,
		itemID, version, market, strings.Join(entries, ","))
}

func TestPublishKindValidateChecksChannelAndLimit(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	setPublishService(t, newPublishBox(t), "")
	xID := createTestChannel(t, "x", "intl", "@founder")
	kind, err := proposalkinds.NewRegistry(nil).Lookup(proposalkinds.KeyPublish)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	env := proposalkinds.Env{Q: testHandler.Queries, WorkspaceID: mustParseUUID(t, testWorkspaceID)}

	canonical, err := kind.Validate(context.Background(), env, []byte(publishPayloadJSON("post-1", 1, "intl", payloadChannel{xID, "x"})))
	if err != nil {
		t.Fatalf("valid payload refused: %v", err)
	}
	var got publish.Payload
	if err := json.Unmarshal(canonical, &got); err != nil {
		t.Fatalf("canonical: %v", err)
	}
	if got.Channels[0].Platform != "x" || got.Channels[0].AccountName != "@founder" {
		t.Fatalf("platform and account must come from the channel row: %+v", got.Channels[0])
	}

	tooLong := strings.Repeat("a", 281)
	long := fmt.Sprintf(`{"item_id":"post-2","version":1,"market":"intl","channels":[{"channel_id":%q,"text":%q}]}`, xID, tooLong)
	if _, err := kind.Validate(context.Background(), env, []byte(long)); err == nil || !strings.Contains(err.Error(), "limit is 280") {
		t.Fatalf("over-limit text must be refused at filing, got %v", err)
	}

	wrongMarket := publishPayloadJSON("post-3", 1, "vn", payloadChannel{xID, "x"})
	if _, err := kind.Validate(context.Background(), env, []byte(wrongMarket)); err == nil {
		t.Fatal("a channel on another market must be refused")
	}
	if _, err := kind.Validate(context.Background(), env, []byte(`{"item_id":"x","version":1,"market":"intl","channels":[],"extra":1}`)); err == nil {
		t.Fatal("an unknown field must be refused")
	}
}

func TestPublishApprovalEnqueuesEachChannelOnceForAVersion(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	setPublishService(t, newPublishBox(t), "")
	fx := newProposalTestFixture(t)
	xID := createTestChannel(t, "x", "intl", "@founder")
	liID := createTestChannel(t, "linkedin", "intl", "Founder")
	proposal := publishProposalFixture(t, fx, publishPayloadJSON("launch-post", 1, "intl",
		payloadChannel{xID, "x"}, payloadChannel{liID, "linkedin"}))

	kind, _ := proposalkinds.NewRegistry(nil).Lookup(proposalkinds.KeyPublish)
	decision := proposalkinds.Decision{WorkspaceID: proposal.WorkspaceID, Proposal: proposal, DeciderID: mustParseUUID(t, testUserID)}
	for i := 0; i < 2; i++ {
		if _, err := kind.Apply(context.Background(), testHandler.Queries, decision); err != nil {
			t.Fatalf("apply #%d: %v", i+1, err)
		}
	}
	n := dbfx.Count(t, `SELECT count(*) FROM publish_attempt WHERE workspace_id = $1 AND item_id = 'launch-post' AND version = 1`, testWorkspaceID)
	if n != 2 {
		t.Fatalf("attempts = %d, want one per channel (2), not one per approval", n)
	}
	var s string
	if err := testPool.QueryRow(context.Background(),
		`SELECT status FROM publish_attempt WHERE workspace_id = $1 AND item_id = 'launch-post' LIMIT 1`, testWorkspaceID).Scan(&s); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if s != "queued" {
		t.Fatalf("new attempts must start queued, got %q", s)
	}
}

// queuedAttempt enqueues one attempt for a one-channel approved post and returns
// its id.
func queuedAttempt(t *testing.T, fx proposalTestFixture, item string) string {
	t.Helper()
	channelID := createTestChannel(t, "x", "intl", "@founder")
	proposal := publishProposalFixture(t, fx, publishPayloadJSON(item, 1, "intl", payloadChannel{channelID, "x"}))
	kind, _ := proposalkinds.NewRegistry(nil).Lookup(proposalkinds.KeyPublish)
	if _, err := kind.Apply(context.Background(), testHandler.Queries, proposalkinds.Decision{
		WorkspaceID: proposal.WorkspaceID, Proposal: proposal, DeciderID: mustParseUUID(t, testUserID),
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	var id string
	if err := testPool.QueryRow(context.Background(), `SELECT id::text FROM publish_attempt WHERE workspace_id = $1 AND item_id = $2`, testWorkspaceID, item).Scan(&id); err != nil {
		t.Fatalf("attempt id: %v", err)
	}
	return id
}

// linkedProfile stores a workspace Profile-Key so the publisher can send.
func linkedProfile(t *testing.T, box *secretbox.Box) {
	t.Helper()
	sealed, err := box.Seal([]byte("pk-send"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	dbfx.Exec(t, `INSERT INTO publish_profile (workspace_id, profile_key_sealed) VALUES ($1, $2)`, testWorkspaceID, sealed)
}

func attemptState(t *testing.T, id string) (status, externalID, externalURL, errText string) {
	t.Helper()
	if err := testPool.QueryRow(context.Background(),
		`SELECT status, external_post_id, external_url, error FROM publish_attempt WHERE id = $1`, id).
		Scan(&status, &externalID, &externalURL, &errText); err != nil {
		t.Fatalf("read attempt: %v", err)
	}
	return
}

// sendingFixture returns a publisher whose workspace has a key and a linked
// profile, and one queued attempt for it.
func sendingFixture(t *testing.T, fake *fakeAyrshareAPI) (*publish.Service, string) {
	t.Helper()
	box := newPublishBox(t)
	svc := setPublishService(t, box, fake.url)
	configureTestKeys(t, svc, testWorkspaceID, "ayr-send-key")
	linkedProfile(t, box)
	attempt := queuedAttempt(t, newProposalTestFixture(t), fmt.Sprintf("item-%d", time.Now().UnixNano()))
	return svc, attempt
}

func TestPublisherPostsOnceAndRecordsTheResult(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	fake := newFakeAyrshareAPI(t, map[string]http.HandlerFunc{
		"/api/post": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success", "id": "ayr-1",
				"postIds": []map[string]any{{"status": "success", "id": "tw-99", "postUrl": "https://x.com/i/status/99", "platform": "twitter"}},
			})
		},
	})
	svc, attempt := sendingFixture(t, fake)

	processed, err := svc.ProcessNext(context.Background())
	if err != nil || !processed {
		t.Fatalf("ProcessNext = %v, %v; want one attempt processed", processed, err)
	}
	status, extID, extURL, errText := attemptState(t, attempt)
	if status != "posted" || extID != "tw-99" || extURL != "https://x.com/i/status/99" || errText != "" {
		t.Fatalf("attempt = %s %s %s %q, want posted with the platform post", status, extID, extURL, errText)
	}
	if body := fake.lastBody(); body["idempotencyKey"] != attempt {
		t.Fatalf("idempotencyKey = %v, want the attempt id %s", body["idempotencyKey"], attempt)
	} else if body["post"] != "Hello from the squad\n\n#launch" {
		t.Fatalf("post = %v", body["post"])
	}
	if h := fake.lastRequest().Header.Get("Authorization"); h != "Bearer ayr-send-key" {
		t.Fatalf("Authorization = %q, want the workspace's key", h)
	}

	again, err := svc.ProcessNext(context.Background())
	if err != nil || again {
		t.Fatalf("second pass = %v, %v; a posted attempt must not be sent again", again, err)
	}
	if fake.count("/api/post") != 1 {
		t.Fatalf("Ayrshare was called %d times, want exactly one", fake.count("/api/post"))
	}
}

func TestPublisherDoesNotRetryAClientError(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	fake := newFakeAyrshareAPI(t, map[string]http.HandlerFunc{
		"/api/post": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": "Facebook rejected the post."})
		},
	})
	svc, attempt := sendingFixture(t, fake)

	if _, err := svc.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext: %v", err)
	}
	status, _, _, errText := attemptState(t, attempt)
	if status != "failed" || !strings.Contains(errText, "Facebook rejected the post.") {
		t.Fatalf("attempt = %s %q, want failed with Ayrshare's message", status, errText)
	}
	if fake.count("/api/post") != 1 {
		t.Fatalf("a 4xx was sent %d times, want 1", fake.count("/api/post"))
	}
}

func TestPublisherRetriesServerErrorsTwice(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	fake := newFakeAyrshareAPI(t, map[string]http.HandlerFunc{
		"/api/post": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": "upstream down"})
		},
	})
	svc, attempt := sendingFixture(t, fake)

	if _, err := svc.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext: %v", err)
	}
	status, _, _, errText := attemptState(t, attempt)
	if status != "failed" || !strings.Contains(errText, "upstream down") {
		t.Fatalf("attempt = %s %q, want failed after retries", status, errText)
	}
	if fake.count("/api/post") != 3 {
		t.Fatalf("a 5xx was sent %d times, want 3 (one try and two retries)", fake.count("/api/post"))
	}
}

// TestPublisherResendsACrashedSendWithTheSameKey: an attempt left sending by a crash
// is queued again after the stale timeout and sent with its attempt id. Ayrshare's
// duplicate refusal (code 110) fails the attempt with a message to check the channel;
// it is never recorded as posted.
func TestPublisherResendsACrashedSendWithTheSameKey(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	fake := newFakeAyrshareAPI(t, map[string]http.HandlerFunc{
		"/api/post": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "error", "postIds": []any{}, "id": "ayr-dup",
				"errors": []map[string]any{{"action": "post", "status": "error", "code": 110, "message": "Status is a duplicate."}},
			})
		},
	})
	svc, attempt := sendingFixture(t, fake)
	// The crash: the attempt was marked sending by a run that never finished.
	if _, err := testPool.Exec(context.Background(),
		`UPDATE publish_attempt SET status = 'sending', updated_at = now() - interval '1 hour' WHERE id = $1`, attempt); err != nil {
		t.Fatalf("simulate crash: %v", err)
	}

	processed, err := svc.ProcessNext(context.Background())
	if err != nil || !processed {
		t.Fatalf("ProcessNext = %v, %v; a stale sending attempt must be sent again", processed, err)
	}
	if body := fake.lastBody(); body["idempotencyKey"] != attempt {
		t.Fatalf("resend used key %v, want the attempt id %s", body["idempotencyKey"], attempt)
	}
	status, _, _, errText := attemptState(t, attempt)
	if status != "failed" || !strings.Contains(errText, "may already be on the network") {
		t.Fatalf("attempt = %s %q, want failed with the check-the-channel message", status, errText)
	}
	if fake.count("/api/post") != 1 {
		t.Fatalf("resent %d times, want 1", fake.count("/api/post"))
	}
}

// TestPublisherLeavesAFreshSendingAttemptAlone: a sending attempt inside the stale
// timeout belongs to a live run, so no other run takes it.
func TestPublisherLeavesAFreshSendingAttemptAlone(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	fake := newFakeAyrshareAPI(t, nil)
	svc, attempt := sendingFixture(t, fake)
	if _, err := testPool.Exec(context.Background(),
		`UPDATE publish_attempt SET status = 'sending', updated_at = now() WHERE id = $1`, attempt); err != nil {
		t.Fatalf("mark sending: %v", err)
	}
	if processed, err := svc.ProcessNext(context.Background()); err != nil || processed {
		t.Fatalf("ProcessNext = %v, %v; a live sending attempt must not be claimed", processed, err)
	}
	if status, _, _, _ := attemptState(t, attempt); status != "sending" {
		t.Fatalf("status = %s, want sending", status)
	}
	if fake.total() != 0 {
		t.Fatalf("a live attempt reached Ayrshare %d times", fake.total())
	}
}

// TestPublisherSkipsAWorkspaceWithoutAKeyAndSendsTheOthers: an attempt in a
// workspace with no key stays queued, and it does not hold up another workspace.
func TestPublisherSkipsAWorkspaceWithoutAKeyAndSendsTheOthers(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resetPublishRows(t)
	fake := newFakeAyrshareAPI(t, map[string]http.HandlerFunc{
		"/api/post": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "id": "ayr-iso", "postIds": []any{}})
		},
	})
	svc, sentAttempt := sendingFixture(t, fake)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	other := dbfx.Workspace(t, "Publish isolation "+suffix, "publish-iso-"+suffix)
	otherChannel := dbfx.Insert(t, "publish_channel", testutil.Cols{
		"workspace_id": other, "platform": "x", "market": "intl", "account_name": "@nokey",
	})
	heldAttempt := dbfx.Insert(t, "publish_attempt", testutil.Cols{
		"workspace_id": other, "proposal_id": testutil.Raw("gen_random_uuid()"), "item_id": "iso-item",
		"channel_id": otherChannel, "version": 1, "approved_by": "tester",
	})

	if processed, err := svc.ProcessNext(context.Background()); err != nil || !processed {
		t.Fatalf("first ProcessNext = %v, %v; the keyed workspace must be sent", processed, err)
	}
	if status, _, _, _ := attemptState(t, sentAttempt); status != "posted" {
		t.Fatalf("keyed workspace attempt = %s, want posted", status)
	}
	if processed, err := svc.ProcessNext(context.Background()); err != nil || processed {
		t.Fatalf("second ProcessNext = %v, %v; the keyless attempt must be skipped", processed, err)
	}
	if status, _, _, _ := attemptState(t, heldAttempt); status != "queued" {
		t.Fatalf("keyless workspace attempt = %s, want queued", status)
	}
	channels, err := svc.List(context.Background(), mustParseUUID(t, other))
	if err != nil || len(channels) != 1 || channels[0].Status != "error" || channels[0].StatusDetail != "Ayrshare not configured" {
		t.Fatalf("keyless workspace list = %+v, %v; want not configured", channels, err)
	}
	if fake.count("/api/post") != 1 {
		t.Fatalf("Ayrshare was called %d times, want 1 (only the keyed workspace)", fake.count("/api/post"))
	}
}
