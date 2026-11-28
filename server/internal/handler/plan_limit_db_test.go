package handler

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/entitlement/entitlementtest"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/planlimit"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Database tests for the quota gates of platform spec 5.7 and the owner limit.
// They are skipped unless PLATFORM_TEST_DATABASE_URL is set and equals
// DATABASE_URL (the package's TestMain already refuses a database whose name
// does not end in _test). Each test builds its own workspace, so the counts are
// its own.

func requirePlanLimitDB(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" || dsn != os.Getenv("DATABASE_URL") {
		t.Skip("PLATFORM_TEST_DATABASE_URL must be set and equal DATABASE_URL")
	}
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
}

// planLimitWorkspace creates a workspace owned by the suite's user, and removes
// it with everything it holds when the test ends.
func planLimitWorkspace(t *testing.T) string {
	t.Helper()
	ws := dbfx.Workspace(t, "Plan limit", "plan-limit-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "owner")
	// Registered last, so it runs first: the cascade takes the rows the handler
	// created, which the fixture does not know about.
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, ws) })
	return ws
}

// setPlanLimitGates makes the handler read these gates for every workspace. A gate not
// named is off.
func setPlanLimitGates(t *testing.T, ws string, gates map[entitlement.GateName]int) {
	t.Helper()
	stub := entitlementtest.New()
	for gate, limit := range gates {
		l := limit
		stub.Set(uuid.MustParse(ws), gate, entitlement.Decision{
			Gate: entitlement.Gate{Action: entitlement.ActionEnforce, Limit: &l},
		})
	}
	prior := testHandler.Entitlements
	testHandler.Entitlements = stub
	t.Cleanup(func() { testHandler.Entitlements = prior })
}

func planLimitRequest(ws, method, path string, body any) *http.Request {
	req := newRequest(method, path, body)
	req.Header.Set("X-Workspace-ID", ws)
	return req
}

type planLimitBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
	Gate  string `json:"gate"`
	Limit *int64 `json:"limit"`
	Used  *int64 `json:"used"`
}

// wantPlanLimit asserts the 402 of spec 10 with exact numbers.
func wantPlanLimit(t *testing.T, resp *testutil.Response, gate string, limit, used int64) {
	t.Helper()
	resp.Want(http.StatusPaymentRequired)
	var body planLimitBody
	resp.JSON(&body)
	if body.Code != planlimit.Code || body.Gate != gate || body.Error == "" ||
		body.Limit == nil || *body.Limit != limit || body.Used == nil || *body.Used != used {
		t.Fatalf("body = %s, want plan_limit_reached %s limit %d used %d", resp.Text(), gate, limit, used)
	}
}

func planLimitLiveAgents(t *testing.T, ws string) int {
	t.Helper()
	return dbfx.Count(t, `SELECT count(*) FROM agent WHERE workspace_id = $1 AND archived_at IS NULL AND kind = 'user'`, ws)
}

// ---- agents -----------------------------------------------------------------

// Under the limit a creation passes; at the limit it is a 402 with the exact
// numbers. Archived agents and system agents take no slot.
func TestPlanLimitAgentsUnderAndAtTheLimit(t *testing.T) {
	requirePlanLimitDB(t)
	ws := planLimitWorkspace(t)
	runtime := dbfx.Runtime(t, "plan-limit-runtime", testutil.Cols{"workspace_id": ws})
	dbfx.Agent(t, "live", runtime, testutil.Cols{"workspace_id": ws})
	dbfx.Agent(t, "archived", runtime, testutil.Cols{"workspace_id": ws, "archived_at": testutil.Raw("now()")})
	dbfx.Agent(t, "system", runtime, testutil.Cols{"workspace_id": ws, "kind": "system", "system_key": "plan-limit-test"})
	setPlanLimitGates(t, ws, map[entitlement.GateName]int{entitlement.GateAgents: 2})

	create := func(name string) *testutil.Response {
		return testutil.Call(t, testHandler.CreateAgent, planLimitRequest(ws, http.MethodPost, "/api/agents", map[string]any{
			"name": name, "runtime_id": runtime,
		}))
	}
	create("second").Want(http.StatusCreated)
	wantPlanLimit(t, create("third"), "agents", 2, 2)
	if n := planLimitLiveAgents(t, ws); n != 2 {
		t.Fatalf("live user agents = %d, want 2: a refused creation must not leave a row", n)
	}
}

// Restoring an archived agent takes a slot again.
func TestPlanLimitAgentsRestore(t *testing.T) {
	requirePlanLimitDB(t)
	ws := planLimitWorkspace(t)
	runtime := dbfx.Runtime(t, "plan-limit-runtime", testutil.Cols{"workspace_id": ws})
	live := dbfx.Agent(t, "live", runtime, testutil.Cols{"workspace_id": ws})
	archived := dbfx.Agent(t, "archived", runtime, testutil.Cols{"workspace_id": ws, "archived_at": testutil.Raw("now()")})
	setPlanLimitGates(t, ws, map[entitlement.GateName]int{entitlement.GateAgents: 1})

	restore := func(id string) *testutil.Response {
		req := testutil.WithURLParams(planLimitRequest(ws, http.MethodPost, "/api/agents/"+id+"/restore", nil), "id", id)
		return testutil.Call(t, testHandler.RestoreAgent, req)
	}
	wantPlanLimit(t, restore(archived), "agents", 1, 1)
	if n := dbfx.Count(t, `SELECT count(*) FROM agent WHERE id = $1 AND archived_at IS NULL`, archived); n != 0 {
		t.Fatal("a refused restore changed the agent")
	}

	dbfx.Exec(t, `UPDATE agent SET archived_at = now() WHERE id = $1`, live)
	restore(archived).Want(http.StatusOK)
}

// Two creations at one slot: exactly one passes (AC10).
func TestPlanLimitAgentsConcurrentCreationsAtOneSlot(t *testing.T) {
	requirePlanLimitDB(t)
	ws := planLimitWorkspace(t)
	runtime := dbfx.Runtime(t, "plan-limit-runtime", testutil.Cols{"workspace_id": ws})
	setPlanLimitGates(t, ws, map[entitlement.GateName]int{entitlement.GateAgents: 1})

	const callers = 8
	codes := make([]int, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			testHandler.CreateAgent(w, planLimitRequest(ws, http.MethodPost, "/api/agents", map[string]any{
				"name": fmt.Sprintf("racer-%d", i), "runtime_id": runtime,
			}))
			codes[i] = w.Code
		}()
	}
	wg.Wait()

	passed, refused := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusCreated:
			passed++
		case http.StatusPaymentRequired:
			refused++
		}
	}
	if passed != 1 || refused != callers-1 {
		t.Fatalf("statuses = %v, want one 201 and %d 402", codes, callers-1)
	}
	if n := planLimitLiveAgents(t, ws); n != 1 {
		t.Fatalf("live user agents = %d, want 1", n)
	}
}

// ---- runtimes ---------------------------------------------------------------

func planLimitRegisterRequest(ws, daemonID string, runtimes ...string) *http.Request {
	list := make([]map[string]any, len(runtimes))
	for i, provider := range runtimes {
		list[i] = map[string]any{"name": provider, "type": provider, "version": "test", "status": "online"}
	}
	return planLimitRequest(ws, http.MethodPost, "/api/daemon/register", map[string]any{
		"workspace_id": ws, "daemon_id": daemonID, "device_name": "plan-limit", "runtimes": list,
	})
}

func planLimitRuntimes(t *testing.T, ws string) int {
	t.Helper()
	return dbfx.Count(t, `SELECT count(*) FROM agent_runtime WHERE workspace_id = $1`, ws)
}

// A new runtime past the limit is refused and rolled back; a reconnect of an
// existing one is not a new runtime and passes at the limit.
func TestPlanLimitRuntimes(t *testing.T) {
	requirePlanLimitDB(t)
	ws := planLimitWorkspace(t)
	setPlanLimitGates(t, ws, map[entitlement.GateName]int{entitlement.GateRuntimes: 1})

	testutil.Call(t, testHandler.DaemonRegister, planLimitRegisterRequest(ws, "plan-limit-a", "claude")).Want(http.StatusOK)
	testutil.Call(t, testHandler.DaemonRegister, planLimitRegisterRequest(ws, "plan-limit-a", "claude")).Want(http.StatusOK)
	wantPlanLimit(t, testutil.Call(t, testHandler.DaemonRegister, planLimitRegisterRequest(ws, "plan-limit-b", "claude")), "runtimes", 1, 1)
	if n := planLimitRuntimes(t, ws); n != 1 {
		t.Fatalf("runtimes = %d, want 1: the refused registration must roll back", n)
	}
}

// One registration that would add two runtimes to a workspace with one slot is
// refused whole: used is what the workspace held before the registration.
func TestPlanLimitRuntimesRefusesTheWholeRegistration(t *testing.T) {
	requirePlanLimitDB(t)
	ws := planLimitWorkspace(t)
	setPlanLimitGates(t, ws, map[entitlement.GateName]int{entitlement.GateRuntimes: 1})

	resp := testutil.Call(t, testHandler.DaemonRegister, planLimitRegisterRequest(ws, "plan-limit-c", "claude", "codex"))
	wantPlanLimit(t, resp, "runtimes", 1, 1)
	if n := planLimitRuntimes(t, ws); n != 0 {
		t.Fatalf("runtimes = %d, want 0", n)
	}
}

func TestPlanLimitRuntimesConcurrentRegistrationsAtOneSlot(t *testing.T) {
	requirePlanLimitDB(t)
	ws := planLimitWorkspace(t)
	setPlanLimitGates(t, ws, map[entitlement.GateName]int{entitlement.GateRuntimes: 1})

	const callers = 6
	codes := make([]int, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			testHandler.DaemonRegister(w, planLimitRegisterRequest(ws, fmt.Sprintf("plan-limit-race-%d", i), "claude"))
			codes[i] = w.Code
		}()
	}
	wg.Wait()

	passed, refused := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusOK:
			passed++
		case http.StatusPaymentRequired:
			refused++
		}
	}
	if passed != 1 || refused != callers-1 {
		t.Fatalf("statuses = %v, want one 200 and %d 402", codes, callers-1)
	}
	if n := planLimitRuntimes(t, ws); n != 1 {
		t.Fatalf("runtimes = %d, want 1", n)
	}
}

// ---- storage ----------------------------------------------------------------

// deletedKeysStorage records the objects a refused upload takes back.
type deletedKeysStorage struct {
	mockStorage
	mu      sync.Mutex
	deleted []string
}

func (s *deletedKeysStorage) DeleteKeys(_ context.Context, keys []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted = append(s.deleted, keys...)
}

func planLimitUploadRequest(t *testing.T, ws string, size int) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "plan-limit.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(bytes.Repeat([]byte("a"), size)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/upload-file", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-User-ID", testUserID)
	req.Header.Set("X-Workspace-ID", ws)
	return req
}

func planLimitAttachmentBytes(t *testing.T, ws string) int {
	t.Helper()
	return dbfx.Count(t, `SELECT COALESCE(sum(size_bytes), 0)::int FROM attachment WHERE workspace_id = $1`, ws)
}

// The requested amount is the upload's size: 60 + 40 fits 100, one byte more
// does not, and the refused object is taken back from storage.
func TestPlanLimitStorage(t *testing.T) {
	requirePlanLimitDB(t)
	ws := planLimitWorkspace(t)
	dbfx.Insert(t, "attachment", testutil.Cols{
		"workspace_id": ws, "uploader_type": "member", "uploader_id": testUserID,
		"filename": "seed.bin", "url": "https://cdn.example.com/seed.bin", "content_type": "application/octet-stream",
		"size_bytes": 60,
	})
	store := &deletedKeysStorage{}
	priorStorage, priorSigner := testHandler.Storage, testHandler.CFSigner
	testHandler.Storage, testHandler.CFSigner = store, nil
	t.Cleanup(func() { testHandler.Storage, testHandler.CFSigner = priorStorage, priorSigner })
	setPlanLimitGates(t, ws, map[entitlement.GateName]int{entitlement.GateStorageBytes: 100})

	testutil.Call(t, testHandler.UploadFile, planLimitUploadRequest(t, ws, 40)).Want(http.StatusOK)
	if got := planLimitAttachmentBytes(t, ws); got != 100 {
		t.Fatalf("stored bytes = %d, want 100", got)
	}

	wantPlanLimit(t, testutil.Call(t, testHandler.UploadFile, planLimitUploadRequest(t, ws, 1)), "storage_bytes", 100, 100)
	if got := planLimitAttachmentBytes(t, ws); got != 100 {
		t.Fatalf("stored bytes = %d after a refused upload, want 100", got)
	}
	// The size is known before the store, so the refusal stores nothing: only
	// the first upload's object exists, and nothing had to be taken back.
	if len(store.files) != 1 || len(store.deleted) != 0 {
		t.Fatalf("stored objects = %d, deleted keys = %v; want 1 and none", len(store.files), store.deleted)
	}
}

// An upload larger than the room left is refused, with used before the upload.
func TestPlanLimitStorageRefusesAnUploadThatDoesNotFit(t *testing.T) {
	requirePlanLimitDB(t)
	ws := planLimitWorkspace(t)
	store := &deletedKeysStorage{}
	priorStorage, priorSigner := testHandler.Storage, testHandler.CFSigner
	testHandler.Storage, testHandler.CFSigner = store, nil
	t.Cleanup(func() { testHandler.Storage, testHandler.CFSigner = priorStorage, priorSigner })
	setPlanLimitGates(t, ws, map[entitlement.GateName]int{entitlement.GateStorageBytes: 10})

	wantPlanLimit(t, testutil.Call(t, testHandler.UploadFile, planLimitUploadRequest(t, ws, 11)), "storage_bytes", 10, 0)
}

func TestPlanLimitStorageConcurrentUploadsAtOneSlot(t *testing.T) {
	requirePlanLimitDB(t)
	ws := planLimitWorkspace(t)
	store := &deletedKeysStorage{}
	priorStorage, priorSigner := testHandler.Storage, testHandler.CFSigner
	testHandler.Storage, testHandler.CFSigner = store, nil
	t.Cleanup(func() { testHandler.Storage, testHandler.CFSigner = priorStorage, priorSigner })
	// Room for exactly one of the 8-byte uploads.
	setPlanLimitGates(t, ws, map[entitlement.GateName]int{entitlement.GateStorageBytes: 10})

	const callers = 6
	reqs := make([]*http.Request, callers)
	for i := range reqs {
		reqs[i] = planLimitUploadRequest(t, ws, 8)
	}
	codes := make([]int, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			testHandler.UploadFile(w, reqs[i])
			codes[i] = w.Code
		}()
	}
	wg.Wait()

	passed, refused := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusOK:
			passed++
		case http.StatusPaymentRequired:
			refused++
		}
	}
	if passed != 1 || refused != callers-1 {
		t.Fatalf("statuses = %v, want one 200 and %d 402", codes, callers-1)
	}
	if got := planLimitAttachmentBytes(t, ws); got != 8 {
		t.Fatalf("stored bytes = %d, want 8", got)
	}
}

// ---- no cloud ---------------------------------------------------------------

// MULTICA_CLOUD_URL empty: the client is disabled, every gate is off, and
// creation is as it was.
func TestPlanLimitGatesAreOffWithoutACloudURL(t *testing.T) {
	requirePlanLimitDB(t)
	ws := planLimitWorkspace(t)
	runtime := dbfx.Runtime(t, "plan-limit-runtime", testutil.Cols{"workspace_id": ws})
	disabled, err := entitlement.New(entitlement.Config{})
	if err != nil {
		t.Fatal(err)
	}
	prior := testHandler.Entitlements
	testHandler.Entitlements = disabled
	t.Cleanup(func() { testHandler.Entitlements = prior })
	store := &deletedKeysStorage{}
	priorStorage, priorSigner := testHandler.Storage, testHandler.CFSigner
	testHandler.Storage, testHandler.CFSigner = store, nil
	t.Cleanup(func() { testHandler.Storage, testHandler.CFSigner = priorStorage, priorSigner })

	for i := range 4 {
		testutil.Call(t, testHandler.CreateAgent, planLimitRequest(ws, http.MethodPost, "/api/agents", map[string]any{
			"name": fmt.Sprintf("free-for-all-%d", i), "runtime_id": runtime,
		})).Want(http.StatusCreated)
	}
	for i := range 3 {
		testutil.Call(t, testHandler.DaemonRegister, planLimitRegisterRequest(ws, fmt.Sprintf("plan-limit-off-%d", i), "claude")).Want(http.StatusOK)
	}
	testutil.Call(t, testHandler.UploadFile, planLimitUploadRequest(t, ws, 64)).Want(http.StatusOK)

	// A nil provider is the same.
	testHandler.Entitlements = nil
	testutil.Call(t, testHandler.CreateAgent, planLimitRequest(ws, http.MethodPost, "/api/agents", map[string]any{
		"name": "nil-provider", "runtime_id": runtime,
	})).Want(http.StatusCreated)
}

// ---- owner limit ------------------------------------------------------------

func subscribeWorkspace(t *testing.T, ws, planKey string) {
	t.Helper()
	dbfx.Exec(t, `INSERT INTO workspace_subscription (workspace_id, plan_key, status) VALUES ($1, $2, 'active')`, ws, planKey)
	dbfx.Cleanup(t, `DELETE FROM workspace_subscription WHERE workspace_id = $1`, ws)
}

// The limit counts the free workspaces a user owns: no row, or a row on the
// free plan. A paid workspace is not counted, and neither is one the user is a
// plain member of.
func TestFreeOwnerLimitCountsOnlyFreeWorkspacesTheUserOwns(t *testing.T) {
	requirePlanLimitDB(t)
	t.Setenv(planlimit.OwnerLimitEnv, "2")
	user := dbfx.User(t, "Owner", testutil.UniqueEmail("owner-limit"))
	create := func(slug string) *testutil.Response {
		return testutil.Call(t, testHandler.CreateWorkspace, newRequestAs(user, http.MethodPost, "/api/workspaces", map[string]any{
			"name": slug, "slug": slug,
		}))
	}
	owned := func(role string) string {
		ws := dbfx.Workspace(t, "owned", "owner-limit-"+uuid.NewString()[:8])
		dbfx.Member(t, ws, user, role)
		return ws
	}

	free := owned("owner") // no subscription row
	paid := owned("owner") // a paid plan
	subscribeWorkspace(t, paid, "pro")
	owned("member") // not owned
	owned("admin")  // not owned
	slug1 := "owner-limit-1-" + uuid.NewString()[:6]
	if got := create(slug1); got.Code != http.StatusCreated {
		t.Fatalf("one free owned workspace under a limit of 2: status %d: %s", got.Code, got.Text())
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug1) })

	// The user now owns two free workspaces (one fixture, one just created).
	resp := create("owner-limit-2-" + uuid.NewString()[:6])
	resp.Want(http.StatusForbidden)
	if got := resp.Map()["code"]; got != freeOwnedWorkspaceLimitCode {
		t.Fatalf("code = %v, want %s: %s", got, freeOwnedWorkspaceLimitCode, resp.Text())
	}

	// Moving one of them to a paid plan frees a slot.
	subscribeWorkspace(t, free, "business")
	slug := "owner-limit-3-" + uuid.NewString()[:6]
	create(slug).Want(http.StatusCreated)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug) })
}

// A workspace on the free plan by an explicit row counts like one with no row.
func TestFreeOwnerLimitCountsAWorkspaceOnTheFreePlanRow(t *testing.T) {
	requirePlanLimitDB(t)
	t.Setenv(planlimit.OwnerLimitEnv, "1")
	user := dbfx.User(t, "Owner", testutil.UniqueEmail("owner-free-row"))
	ws := dbfx.Workspace(t, "owned", "owner-free-row-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, user, "owner")
	subscribeWorkspace(t, ws, "free")

	testutil.Call(t, testHandler.CreateWorkspace, newRequestAs(user, http.MethodPost, "/api/workspaces", map[string]any{
		"name": "another", "slug": "owner-free-row-new-" + uuid.NewString()[:6],
	})).Want(http.StatusForbidden)
}

// A usage invoice that is still pending is a debt tied to its workspace. The
// owner must not escape it by opening a new free workspace (spec 7b.3).
func TestFreeOwnerLimitRefusesAnOwnerWithAnUnpaidUsageInvoice(t *testing.T) {
	requirePlanLimitDB(t)
	t.Setenv(planlimit.OwnerLimitEnv, "5")
	user := dbfx.User(t, "Owner", testutil.UniqueEmail("owner-debt"))
	ws := dbfx.Workspace(t, "owned", "owner-debt-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, user, "owner")
	subscribeWorkspace(t, ws, "usage")
	dbfx.Exec(t, `INSERT INTO billing_order (id, workspace_id, kind, plan_key, interval, amount, currency, status, provider, transfer_code)
		VALUES ($1, $2, 'usage', 'usage', 'month', 100000, 'VND', 'pending', 'bank_transfer', $3)`,
		uuid.New(), ws, "DEBT"+uuid.NewString()[:8])
	dbfx.Cleanup(t, `DELETE FROM billing_order WHERE workspace_id = $1`, ws)

	slug := "owner-debt-new-" + uuid.NewString()[:6]
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug) })
	resp := testutil.Call(t, testHandler.CreateWorkspace, newRequestAs(user, http.MethodPost, "/api/workspaces", map[string]any{
		"name": slug, "slug": slug,
	}))
	resp.Want(http.StatusForbidden)
	if got := resp.Map()["code"]; got != ownerUsageDebtCode {
		t.Fatalf("code = %v, want %s: %s", got, ownerUsageDebtCode, resp.Text())
	}
}

// Unset and 0 mean no limit.
func TestFreeOwnerLimitUnsetAndZeroMeanNoLimit(t *testing.T) {
	requirePlanLimitDB(t)
	user := dbfx.User(t, "Owner", testutil.UniqueEmail("owner-none"))
	for i := range 3 {
		ws := dbfx.Workspace(t, "owned", fmt.Sprintf("owner-none-%d-%s", i, uuid.NewString()[:6]))
		dbfx.Member(t, ws, user, "owner")
	}
	for name, value := range map[string]string{"unset": "", "zero": "0"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(planlimit.OwnerLimitEnv, value)
			slug := "owner-none-new-" + uuid.NewString()[:6]
			testutil.Call(t, testHandler.CreateWorkspace, newRequestAs(user, http.MethodPost, "/api/workspaces", map[string]any{
				"name": slug, "slug": slug,
			})).Want(http.StatusCreated)
			t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug) })
		})
	}
}

// ---- failed-profile runtimes --------------------------------------------------

func planLimitProfile(t *testing.T, ws, name string) string {
	t.Helper()
	var id string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO runtime_profile (workspace_id, display_name, protocol_family, command_name, created_by)
		VALUES ($1, $2, 'codex', $3, $4) RETURNING id`, ws, name, "missing-"+name, testUserID).Scan(&id); err != nil {
		t.Fatalf("insert runtime_profile: %v", err)
	}
	return id
}

func planLimitFailedProfileRequest(ws, daemonID string, profiles ...string) *http.Request {
	failed := make([]map[string]any, len(profiles))
	for i, id := range profiles {
		failed[i] = map[string]any{"profile_id": id, "command_name": "missing", "reason": "command not found"}
	}
	return planLimitRequest(ws, http.MethodPost, "/api/daemon/register", map[string]any{
		"workspace_id": ws, "daemon_id": daemonID, "device_name": "plan-limit", "failed_profiles": failed,
	})
}

// The placeholder a failed profile leaves is a runtime row and counts, so it
// takes a slot. When there is none the record is skipped and the registration
// still succeeds: runtimes that do work must not fail on a placeholder.
func TestPlanLimitRuntimesFailedProfilePlaceholders(t *testing.T) {
	requirePlanLimitDB(t)
	ws := planLimitWorkspace(t)
	first, second := planLimitProfile(t, ws, "first"), planLimitProfile(t, ws, "second")
	setPlanLimitGates(t, ws, map[entitlement.GateName]int{entitlement.GateRuntimes: 2})

	testutil.Call(t, testHandler.DaemonRegister, planLimitRegisterRequest(ws, "plan-limit-fp", "claude")).Want(http.StatusOK)
	testutil.Call(t, testHandler.DaemonRegister, planLimitFailedProfileRequest(ws, "plan-limit-fp", first)).Want(http.StatusOK)
	if n := planLimitRuntimes(t, ws); n != 2 {
		t.Fatalf("runtimes = %d, want 2: the placeholder takes the second slot", n)
	}

	// No room for another placeholder: skipped, registration still 200.
	testutil.Call(t, testHandler.DaemonRegister, planLimitFailedProfileRequest(ws, "plan-limit-fp", second)).Want(http.StatusOK)
	if n := dbfx.Count(t, `SELECT count(*) FROM agent_runtime WHERE workspace_id = $1 AND profile_id = $2`, ws, second); n != 0 {
		t.Fatalf("the refused placeholder was recorded")
	}
	// Re-reporting the first failure updates its row, which is not a new runtime.
	testutil.Call(t, testHandler.DaemonRegister, planLimitFailedProfileRequest(ws, "plan-limit-fp", first)).Want(http.StatusOK)

	// A runtime that works is refused: the placeholder counts against the quota.
	wantPlanLimit(t, testutil.Call(t, testHandler.DaemonRegister, planLimitRegisterRequest(ws, "plan-limit-other", "claude")), "runtimes", 2, 2)
}

// ---- owner limit race ----------------------------------------------------------

// Two simultaneous CreateWorkspace calls by one user with a limit of 1 and no
// workspace yet: the user's advisory lock makes exactly one of them pass.
func TestFreeOwnerLimitConcurrentCreationsPassOnce(t *testing.T) {
	requirePlanLimitDB(t)
	t.Setenv(planlimit.OwnerLimitEnv, "1")
	user := dbfx.User(t, "Owner", testutil.UniqueEmail("owner-race"))

	const callers = 6
	prefix := "owner-race-" + uuid.NewString()[:6]
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug LIKE $1`, prefix+"-%") })
	codes := make([]int, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slug := fmt.Sprintf("%s-%d", prefix, i)
			w := httptest.NewRecorder()
			testHandler.CreateWorkspace(w, newRequestAs(user, http.MethodPost, "/api/workspaces", map[string]any{"name": slug, "slug": slug}))
			codes[i] = w.Code
		}()
	}
	wg.Wait()

	passed, refused := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusCreated:
			passed++
		case http.StatusForbidden:
			refused++
		}
	}
	if passed != 1 || refused != callers-1 {
		t.Fatalf("statuses = %v, want one 201 and %d 403", codes, callers-1)
	}
}

// ---- other agent creation paths --------------------------------------------------

// Mika is a user agent: creating it takes an agents slot like any other.
func TestPlanLimitAgentsMikaCreation(t *testing.T) {
	requirePlanLimitDB(t)
	ws := planLimitWorkspace(t)
	runtime := dbfx.Runtime(t, "plan-limit-runtime", testutil.Cols{"workspace_id": ws})
	dbfx.Agent(t, "live", runtime, testutil.Cols{"workspace_id": ws})
	setPlanLimitGates(t, ws, map[entitlement.GateName]int{entitlement.GateAgents: 1})

	// The handler reads the workspace from the request context, as the router's
	// membership middleware sets it; the header alone is not enough.
	memberRow, err := testHandler.Queries.GetMemberByUserAndWorkspace(context.Background(), db.GetMemberByUserAndWorkspaceParams{
		UserID:      util.MustParseUUID(testUserID),
		WorkspaceID: util.MustParseUUID(ws),
	})
	if err != nil {
		t.Fatalf("load owner member row: %v", err)
	}
	mikaReq := planLimitRequest(ws, http.MethodPost, "/api/agents/mika", map[string]any{
		"runtime_id": runtime, "language": "en",
	})
	mikaReq = mikaReq.WithContext(middleware.SetMemberContext(mikaReq.Context(), ws, memberRow))
	resp := testutil.Call(t, testHandler.CreateMikaAgent, mikaReq)
	wantPlanLimit(t, resp, "agents", 1, 1)
	if n := dbfx.Count(t, `SELECT count(*) FROM agent WHERE workspace_id = $1 AND system_key IS NOT NULL AND kind = 'user'`, ws); n != 0 {
		t.Fatal("a refused creation left Mika behind")
	}
}

// A company import that creates more agents than the plan has room for is
// refused whole, in its one transaction: it writes nothing.
func TestPlanLimitAgentsCompanyImport(t *testing.T) {
	requirePlanLimitDB(t)
	ws := planLimitWorkspace(t)
	runtime := dbfx.Runtime(t, "plan-limit-runtime", testutil.Cols{"workspace_id": ws})
	setPlanLimitGates(t, ws, map[entitlement.GateName]int{entitlement.GateAgents: 1})

	resp := testutil.Call(t, testHandler.ImportCompany, planLimitRequest(ws, http.MethodPost, "/api/company/import", map[string]any{
		"runtime_id": runtime,
		"document": map[string]any{
			"schema_version": service.CompanyExportVersion,
			"agents":         []map[string]any{{"name": "Imported One"}, {"name": "Imported Two"}},
		},
	}))
	wantPlanLimit(t, resp, "agents", 1, 1)
	if n := planLimitLiveAgents(t, ws); n != 0 {
		t.Fatalf("live user agents = %d, want 0: the import must write nothing", n)
	}
}
