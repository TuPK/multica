package accountprovision

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/scheduler"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var testPool *pgxpool.Pool

// requireTestDatabaseURL refuses any database whose name does not end in _test
// (same rule as scripts/require-test-database.sh).
func requireTestDatabaseURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("DATABASE_URL is empty; set it to a database whose name ends in _test")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("DATABASE_URL is not a valid URL: %w", err)
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name == "multica" || !strings.HasSuffix(name, "_test") {
		return fmt.Errorf("database %q is not a test database; use one whose name ends in _test", name)
	}
	return nil
}

func TestMain(m *testing.M) {
	ctx := context.Background()
	dbURL := os.Getenv("DATABASE_URL")
	if err := requireTestDatabaseURL(dbURL); err != nil {
		fmt.Fprintln(os.Stderr, "accountprovision tests:", err)
		os.Exit(1)
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "accountprovision tests: connect:", err)
		os.Exit(1)
	}
	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "accountprovision tests: ping:", err)
		os.Exit(1)
	}
	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// ---- fixtures -------------------------------------------------------------

func token() string { return strings.ReplaceAll(uuid.NewString(), "-", "")[:12] }

type reqSpec struct {
	Email       string // stored as given; the job lowercases the user's email
	DisplayName string
	WSName      string
	WSSlug      string // empty means derive from the name
}

func (s reqSpec) withDefaults() reqSpec {
	if s.Email == "" {
		s.Email = "acct-" + token() + "@example.test"
	}
	if s.DisplayName == "" {
		s.DisplayName = "Ada Provisioned"
	}
	if s.WSName == "" {
		s.WSName = "Provision Co " + token()
	}
	return s
}

// insertRequest writes a pending request and registers cleanup of everything
// the job may create for it. Only rows of this fixture are ever touched.
func insertRequest(t *testing.T, s reqSpec) (uuid.UUID, reqSpec) {
	t.Helper()
	s = s.withDefaults()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO platform_account_request
			(id, kind, email, display_name, workspace_name, workspace_slug, plan_key, created_by)
		VALUES ($1, 'individual', $2, $3, $4, $5, 'free', $6)`,
		id, s.Email, s.DisplayName, s.WSName, s.WSSlug, uuid.New())
	if err != nil {
		t.Fatalf("insert request: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		var wsID, userID pgtype.UUID
		_ = testPool.QueryRow(ctx, `SELECT workspace_id, user_id FROM platform_account_request WHERE id = $1`, id).Scan(&wsID, &userID)
		if wsID.Valid {
			_, _ = testPool.Exec(ctx, `DELETE FROM audit_event WHERE workspace_id = $1`, wsID)
			_, _ = testPool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, wsID)
		}
		_, _ = testPool.Exec(ctx, `DELETE FROM "user" WHERE lower(email) = lower($1)`, s.Email)
		_, _ = testPool.Exec(ctx, `DELETE FROM platform_account_request WHERE id = $1`, id)
	})
	return id, s
}

// insertWorkspace creates a workspace that already holds a slug.
func insertWorkspace(t *testing.T, slug string) pgtype.UUID {
	t.Helper()
	ws, err := db.New(testPool).CreateWorkspace(context.Background(), db.CreateWorkspaceParams{
		Name: "Squatter " + slug, Slug: slug, IssuePrefix: "SQT",
	})
	if err != nil {
		t.Fatalf("insert workspace %q: %v", slug, err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, ws.ID)
	})
	return ws.ID
}

type reqRow struct {
	State       string
	ErrorCode   string
	Attempts    int
	UserID      pgtype.UUID
	WorkspaceID pgtype.UUID
	UpdatedAt   time.Time
}

func getRequest(t *testing.T, id uuid.UUID) reqRow {
	t.Helper()
	var r reqRow
	err := testPool.QueryRow(context.Background(),
		`SELECT state, error_code, attempts, user_id, workspace_id, updated_at FROM platform_account_request WHERE id = $1`, id).
		Scan(&r.State, &r.ErrorCode, &r.Attempts, &r.UserID, &r.WorkspaceID, &r.UpdatedAt)
	if err != nil {
		t.Fatalf("read request: %v", err)
	}
	return r
}

func count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func usersWithEmail(t *testing.T, email string) int {
	return count(t, `SELECT count(*) FROM "user" WHERE lower(email) = lower($1)`, email)
}

func workspacesWithSlug(t *testing.T, slug string) int {
	return count(t, `SELECT count(*) FROM workspace WHERE slug = $1`, slug)
}

func membersOfUserEmail(t *testing.T, email string) int {
	return count(t, `SELECT count(*) FROM member m JOIN "user" u ON u.id = m.user_id WHERE lower(u.email) = lower($1)`, email)
}

// backdate makes a request old enough to be retried even when the job spaces
// retries by a minute.
func backdate(t *testing.T, id uuid.UUID) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`UPDATE platform_account_request SET updated_at = now() - interval '10 minutes' WHERE id = $1`, id); err != nil {
		t.Fatalf("backdate: %v", err)
	}
}

// fakeKnowledge records AutoEnable calls and whether the workspace was already
// visible to another connection (that is, committed) when it was called.
type fakeKnowledge struct {
	mu    sync.Mutex
	calls []knowledgeCall
}

type knowledgeCall struct {
	Workspace, Actor pgtype.UUID
	Committed        bool
}

func (f *fakeKnowledge) AutoEnable(workspaceID, actorID pgtype.UUID) {
	var n int
	_ = testPool.QueryRow(context.Background(), `SELECT count(*) FROM workspace WHERE id = $1`, workspaceID).Scan(&n)
	f.mu.Lock()
	f.calls = append(f.calls, knowledgeCall{Workspace: workspaceID, Actor: actorID, Committed: n == 1})
	f.mu.Unlock()
}

func (f *fakeKnowledge) callsFor(ws pgtype.UUID) []knowledgeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []knowledgeCall
	for _, c := range f.calls {
		if c.Workspace == ws {
			out = append(out, c)
		}
	}
	return out
}

// waitForKnowledge waits for the AutoEnable call(s) for ws; the production code
// may call it from a goroutine after commit.
func (f *fakeKnowledge) waitFor(t *testing.T, ws pgtype.UUID, want int) []knowledgeCall {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(f.callsFor(ws)) >= want {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(150 * time.Millisecond) // let a wrong second call show up
	return f.callsFor(ws)
}

type runnerOpts struct {
	blocked   []string
	suffix    func() (string, error)
	inject    func(stage string) error
	knowledge *fakeKnowledge
}

func newRunner(o runnerOpts) *Runner {
	d := Deps{
		Pool:                testPool,
		Queries:             db.New(testPool),
		BlockedEmailDomains: o.blocked,
		Analytics:           analytics.NoopClient{},
		Suffix:              o.suffix,
		Inject:              o.inject,
	}
	if o.knowledge != nil {
		d.Knowledge = o.knowledge
	}
	return New(d)
}

func runOnce(t *testing.T, r *Runner) int {
	t.Helper()
	n, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	return n
}

func uuidStr(u pgtype.UUID) string { return uuid.UUID(u.Bytes).String() }

func assertFailed(t *testing.T, id uuid.UUID, code string) reqRow {
	t.Helper()
	r := getRequest(t, id)
	if r.State != "failed" || r.ErrorCode != code {
		t.Fatalf("request state=%q error_code=%q, want failed/%q", r.State, r.ErrorCode, code)
	}
	if r.WorkspaceID.Valid {
		t.Fatalf("a failed request must not point at a workspace, got %s", uuidStr(r.WorkspaceID))
	}
	return r
}

// seqSuffix returns the given values in order and counts the calls.
func seqSuffix(values ...string) (func() (string, error), *atomic.Int32) {
	var calls atomic.Int32
	return func() (string, error) {
		i := int(calls.Add(1)) - 1
		if i >= len(values) {
			return "", errors.New("suffix values exhausted")
		}
		return values[i], nil
	}, &calls
}

// ---- job spec ---------------------------------------------------------------

func TestAccountProvisionJobSpec(t *testing.T) {
	r := newRunner(runnerOpts{})
	spec := Job(r)
	if spec.Name != "account_provisioning" {
		t.Fatalf("job name = %q, want account_provisioning", spec.Name)
	}
	if spec.Cadence != time.Minute {
		t.Fatalf("cadence = %v, want 1m", spec.Cadence)
	}
	if spec.Handler == nil {
		t.Fatal("job has no handler")
	}

	id, s := insertRequest(t, reqSpec{})
	res, err := spec.Handler(context.Background(), scheduler.HandlerInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.RowsAffected < 1 {
		t.Fatalf("RowsAffected = %d, want >= 1", res.RowsAffected)
	}
	if got := getRequest(t, id); got.State != "provisioned" {
		t.Fatalf("state = %q after the job ran, want provisioned", got.State)
	}
	if usersWithEmail(t, s.Email) != 1 {
		t.Fatal("job did not create the user")
	}
}

// ---- (b) happy path -----------------------------------------------------------

func TestAccountProvisionHappyPath(t *testing.T) {
	r := newRunner(runnerOpts{})
	slug := "acct-" + token()
	id, s := insertRequest(t, reqSpec{
		Email:       "Mixed.Case-" + token() + "@Example.Test",
		DisplayName: "Nguyễn Văn An",
		WSName:      "Công ty An",
		WSSlug:      slug,
	})

	if n := runOnce(t, r); n < 1 {
		t.Fatalf("RunOnce moved %d requests, want >= 1", n)
	}

	req := getRequest(t, id)
	if req.State != "provisioned" || req.ErrorCode != "" {
		t.Fatalf("state=%q error_code=%q, want provisioned with no error", req.State, req.ErrorCode)
	}
	if !req.UserID.Valid || !req.WorkspaceID.Valid {
		t.Fatalf("user_id/workspace_id not written: %+v", req)
	}

	var (
		name, email string
		onboarded   bool
	)
	err := testPool.QueryRow(context.Background(),
		`SELECT name, email, onboarded_at IS NOT NULL FROM "user" WHERE id = $1`, req.UserID).Scan(&name, &email, &onboarded)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if want := strings.ToLower(s.Email); email != want {
		t.Fatalf("user email = %q, want %q", email, want)
	}
	if name != "Nguyễn Văn An" {
		t.Fatalf("user name = %q, want the display_name", name)
	}
	if !onboarded {
		t.Fatal("user.onboarded_at is not set")
	}
	if n := usersWithEmail(t, s.Email); n != 1 {
		t.Fatalf("users with email = %d, want 1", n)
	}

	var wsName, wsSlug string
	err = testPool.QueryRow(context.Background(), `SELECT name, slug FROM workspace WHERE id = $1`, req.WorkspaceID).Scan(&wsName, &wsSlug)
	if err != nil {
		t.Fatalf("read workspace: %v", err)
	}
	if wsName != "Công ty An" || wsSlug != slug {
		t.Fatalf("workspace = %q/%q, want %q/%q", wsName, wsSlug, "Công ty An", slug)
	}

	if n := count(t, `SELECT count(*) FROM member WHERE workspace_id = $1 AND user_id = $2 AND role = 'owner'`, req.WorkspaceID, req.UserID); n != 1 {
		t.Fatalf("owner member rows = %d, want 1", n)
	}
	if n := count(t, `SELECT count(*) FROM member WHERE workspace_id = $1`, req.WorkspaceID); n != 1 {
		t.Fatalf("member rows = %d, want only the owner (no staff member)", n)
	}
	if n := count(t, `SELECT count(*) FROM issue_status WHERE workspace_id = $1 AND is_system`, req.WorkspaceID); n != 7 {
		t.Fatalf("built-in issue statuses = %d, want 7", n)
	}
}

// ---- (c) slug ---------------------------------------------------------------

func TestAccountProvisionSlugGivenIsUsedAsIs(t *testing.T) {
	r := newRunner(runnerOpts{})
	slug := "acct-given-" + token()
	id, _ := insertRequest(t, reqSpec{WSName: "Totally Different Name", WSSlug: slug})
	runOnce(t, r)
	req := getRequest(t, id)
	if req.State != "provisioned" {
		t.Fatalf("state = %q (%s), want provisioned", req.State, req.ErrorCode)
	}
	var got string
	if err := testPool.QueryRow(context.Background(), `SELECT slug FROM workspace WHERE id = $1`, req.WorkspaceID).Scan(&got); err != nil || got != slug {
		t.Fatalf("slug = %q (err %v), want %q", got, err, slug)
	}
}

func TestAccountProvisionSlugGivenTakenFails(t *testing.T) {
	r := newRunner(runnerOpts{})
	slug := "acct-taken-" + token()
	insertWorkspace(t, slug)
	id, s := insertRequest(t, reqSpec{WSSlug: slug})
	runOnce(t, r)

	assertFailed(t, id, "slug_unavailable")
	if n := workspacesWithSlug(t, slug); n != 1 {
		t.Fatalf("workspaces with slug = %d, want only the pre-existing one", n)
	}
	if n := membersOfUserEmail(t, s.Email); n != 0 {
		t.Fatalf("member rows for the requested email = %d, want 0", n)
	}
}

func TestAccountProvisionSlugGivenReservedFails(t *testing.T) {
	for _, slug := range []string{"settings", "login"} {
		t.Run(slug, func(t *testing.T) {
			r := newRunner(runnerOpts{})
			id, s := insertRequest(t, reqSpec{WSSlug: slug})
			runOnce(t, r)
			assertFailed(t, id, "slug_unavailable")
			if n := membersOfUserEmail(t, s.Email); n != 0 {
				t.Fatalf("member rows for the requested email = %d, want 0", n)
			}
		})
	}
}

func TestAccountProvisionSlugDerivedFromNameWithoutDiacritics(t *testing.T) {
	r := newRunner(runnerOpts{})
	tok := token()
	id, _ := insertRequest(t, reqSpec{WSName: "Công ty Số 1 Đà Nẵng " + tok})
	runOnce(t, r)
	req := getRequest(t, id)
	if req.State != "provisioned" {
		t.Fatalf("state = %q (%s), want provisioned", req.State, req.ErrorCode)
	}
	var got string
	if err := testPool.QueryRow(context.Background(), `SELECT slug FROM workspace WHERE id = $1`, req.WorkspaceID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if want := "cong-ty-so-1-da-nang-" + tok; got != want {
		t.Fatalf("derived slug = %q, want %q", got, want)
	}
}

func TestAccountProvisionSlugDerivedCollisionGetsSuffix(t *testing.T) {
	tok := token()
	base := "cong-ty-so-1-" + tok
	insertWorkspace(t, base)
	suffix, _ := seqSuffix("ab12", "cd34")
	r := newRunner(runnerOpts{suffix: suffix})
	id, _ := insertRequest(t, reqSpec{WSName: "Công ty Số 1 " + tok})
	runOnce(t, r)
	req := getRequest(t, id)
	if req.State != "provisioned" {
		t.Fatalf("state = %q (%s), want provisioned", req.State, req.ErrorCode)
	}
	var got string
	if err := testPool.QueryRow(context.Background(), `SELECT slug FROM workspace WHERE id = $1`, req.WorkspaceID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if want := base + "-ab12"; got != want {
		t.Fatalf("slug = %q, want %q", got, want)
	}
}

func TestAccountProvisionSlugDerivedReservedGetsSuffix(t *testing.T) {
	suffix, _ := seqSuffix("ab12", "cd34")
	r := newRunner(runnerOpts{suffix: suffix})
	// "Login" derives "login", which is reserved.
	id, _ := insertRequest(t, reqSpec{WSName: "Login"})
	runOnce(t, r)
	req := getRequest(t, id)
	if req.State != "provisioned" {
		t.Fatalf("state = %q (%s), want provisioned", req.State, req.ErrorCode)
	}
	var got string
	if err := testPool.QueryRow(context.Background(), `SELECT slug FROM workspace WHERE id = $1`, req.WorkspaceID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "login-ab12" {
		t.Fatalf("slug = %q, want login-ab12", got)
	}
}

// Four taken candidates leave the fifth free: the job must still reach it.
func TestAccountProvisionSlugFifthCandidateSucceeds(t *testing.T) {
	tok := token()
	base := "retry-co-" + tok
	for _, s := range []string{base, base + "-a001", base + "-a002", base + "-a003"} {
		insertWorkspace(t, s)
	}
	suffix, _ := seqSuffix("a001", "a002", "a003", "a004", "a005", "a006")
	r := newRunner(runnerOpts{suffix: suffix})
	id, _ := insertRequest(t, reqSpec{WSName: "Retry Co " + tok})
	runOnce(t, r)
	req := getRequest(t, id)
	if req.State != "provisioned" {
		t.Fatalf("state = %q (%s), want provisioned on the fifth candidate", req.State, req.ErrorCode)
	}
	var got string
	if err := testPool.QueryRow(context.Background(), `SELECT slug FROM workspace WHERE id = $1`, req.WorkspaceID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if want := base + "-a004"; got != want {
		t.Fatalf("slug = %q, want %q", got, want)
	}
}

func TestAccountProvisionSlugGivesUpAfterFiveCandidates(t *testing.T) {
	tok := token()
	base := "full-co-" + tok
	for _, s := range []string{base, base + "-a001", base + "-a002", base + "-a003", base + "-a004", base + "-a005"} {
		insertWorkspace(t, s)
	}
	suffix, calls := seqSuffix("a001", "a002", "a003", "a004", "a005", "a006")
	r := newRunner(runnerOpts{suffix: suffix})
	id, s := insertRequest(t, reqSpec{WSName: "Full Co " + tok})
	runOnce(t, r)

	assertFailed(t, id, "slug_unavailable")
	if n := calls.Load(); n > 5 {
		t.Fatalf("Suffix called %d times, want at most 5 candidates", n)
	}
	if n := count(t, `SELECT count(*) FROM workspace WHERE slug LIKE $1`, base+"%"); n != 6 {
		t.Fatalf("workspaces under the base = %d, want the 6 pre-existing ones", n)
	}
	if n := membersOfUserEmail(t, s.Email); n != 0 {
		t.Fatalf("member rows for the requested email = %d, want 0", n)
	}
}

// ---- (d) email ----------------------------------------------------------------

func TestAccountProvisionEmailExistsFails(t *testing.T) {
	r := newRunner(runnerOpts{})
	tok := token()
	lower := "taken-" + tok + "@example.test"
	existing, err := db.New(testPool).CreateUser(context.Background(), db.CreateUserParams{Name: "Already Here", Email: lower})
	if err != nil {
		t.Fatal(err)
	}
	slug := "acct-exists-" + tok
	id, s := insertRequest(t, reqSpec{Email: "Taken-" + tok + "@Example.Test", DisplayName: "Someone Else", WSSlug: slug})
	runOnce(t, r)

	req := assertFailed(t, id, "email_exists")
	if req.UserID.Valid {
		t.Fatal("a request that hit an existing user must not record a user_id")
	}
	if n := usersWithEmail(t, s.Email); n != 1 {
		t.Fatalf("users with email = %d, want only the existing one", n)
	}
	var name string
	var uid pgtype.UUID
	if err := testPool.QueryRow(context.Background(), `SELECT id, name FROM "user" WHERE lower(email) = $1`, lower).Scan(&uid, &name); err != nil {
		t.Fatal(err)
	}
	if uid != existing.ID || name != "Already Here" {
		t.Fatalf("existing user was touched: id %s name %q", uuidStr(uid), name)
	}
	if n := workspacesWithSlug(t, slug); n != 0 {
		t.Fatalf("workspaces created = %d, want 0", n)
	}
	if n := membersOfUserEmail(t, s.Email); n != 0 {
		t.Fatalf("members for the existing user = %d, want 0", n)
	}
}

func TestAccountProvisionBlockedDomainFails(t *testing.T) {
	tok := token()
	blocked := "blocked-" + tok + ".test"
	r := newRunner(runnerOpts{blocked: []string{blocked}})
	for _, email := range []string{"a-" + tok + "@" + blocked, "b-" + tok + "@mail." + blocked} {
		slug := "acct-blocked-" + token()
		id, s := insertRequest(t, reqSpec{Email: email, WSSlug: slug})
		runOnce(t, r)

		req := assertFailed(t, id, "email_domain_blocked")
		if req.UserID.Valid {
			t.Fatalf("%s: user_id written for a blocked request", email)
		}
		if n := usersWithEmail(t, s.Email); n != 0 {
			t.Fatalf("%s: users created = %d, want 0", email, n)
		}
		if n := workspacesWithSlug(t, slug); n != 0 {
			t.Fatalf("%s: workspaces created = %d, want 0", email, n)
		}
	}
}

// ---- (e) concurrency and idempotence ----------------------------------------

func TestAccountProvisionParallelRunsCreateOnce(t *testing.T) {
	knowledge := &fakeKnowledge{}
	type fx struct {
		id   uuid.UUID
		spec reqSpec
		slug string
	}
	var fxs []fx
	for i := 0; i < 3; i++ {
		slug := "acct-par-" + token()
		id, s := insertRequest(t, reqSpec{WSSlug: slug})
		fxs = append(fxs, fx{id, s, slug})
	}
	// Two runners on the same pool stand for two server processes.
	runners := []*Runner{newRunner(runnerOpts{knowledge: knowledge}), newRunner(runnerOpts{knowledge: knowledge})}

	start := make(chan struct{})
	errs := make([]error, len(runners))
	var wg sync.WaitGroup
	for i, r := range runners {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = r.RunOnce(context.Background())
		}()
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("runner %d: %v", i, err)
		}
	}

	for _, f := range fxs {
		req := getRequest(t, f.id)
		if req.State != "provisioned" {
			t.Fatalf("%s: state = %q (%s), want provisioned", f.slug, req.State, req.ErrorCode)
		}
		if n := usersWithEmail(t, f.spec.Email); n != 1 {
			t.Fatalf("%s: users = %d, want exactly 1", f.slug, n)
		}
		if n := workspacesWithSlug(t, f.slug); n != 1 {
			t.Fatalf("%s: workspaces = %d, want exactly 1", f.slug, n)
		}
		if n := membersOfUserEmail(t, f.spec.Email); n != 1 {
			t.Fatalf("%s: members = %d, want exactly 1", f.slug, n)
		}
		if calls := knowledge.waitFor(t, req.WorkspaceID, 1); len(calls) != 1 {
			t.Fatalf("%s: AutoEnable called %d times across both runners, want 1", f.slug, len(calls))
		}
	}
}

func TestAccountProvisionSecondRunChangesNothing(t *testing.T) {
	knowledge := &fakeKnowledge{}
	r := newRunner(runnerOpts{knowledge: knowledge})
	slug := "acct-idem-" + token()
	id, s := insertRequest(t, reqSpec{WSSlug: slug})

	runOnce(t, r)
	first := getRequest(t, id)
	if first.State != "provisioned" {
		t.Fatalf("state = %q (%s), want provisioned", first.State, first.ErrorCode)
	}
	runOnce(t, r)
	second := getRequest(t, id)
	if second != first {
		t.Fatalf("second run changed the request:\n first  %+v\n second %+v", first, second)
	}
	if n := usersWithEmail(t, s.Email); n != 1 {
		t.Fatalf("users = %d, want 1", n)
	}
	if n := workspacesWithSlug(t, slug); n != 1 {
		t.Fatalf("workspaces = %d, want 1", n)
	}
	if calls := knowledge.waitFor(t, first.WorkspaceID, 1); len(calls) != 1 {
		t.Fatalf("AutoEnable called %d times over two runs, want 1", len(calls))
	}
}

// ---- (f) transient failure ------------------------------------------------------

func TestAccountProvisionTransientFailureRetriesThenFails(t *testing.T) {
	var calls atomic.Int32
	var badStage atomic.Value
	inject := func(stage string) error {
		calls.Add(1)
		if stage != "create" {
			badStage.Store(stage)
		}
		return errors.New("injected transient failure")
	}
	r := newRunner(runnerOpts{inject: inject})
	slug := "acct-flaky-" + token()
	id, s := insertRequest(t, reqSpec{WSSlug: slug})

	for attempt := 1; attempt <= 2; attempt++ {
		if attempt > 1 {
			backdate(t, id)
		}
		runOnce(t, r)
		req := getRequest(t, id)
		if req.State != "pending" || req.Attempts != attempt {
			t.Fatalf("after attempt %d: state=%q attempts=%d, want pending/%d", attempt, req.State, req.Attempts, attempt)
		}
		assertNothingSurvives(t, req, s.Email, slug)
	}

	backdate(t, id)
	runOnce(t, r)
	req := getRequest(t, id)
	if req.State != "failed" || req.ErrorCode != "internal" || req.Attempts != 3 {
		t.Fatalf("after attempt 3: state=%q error_code=%q attempts=%d, want failed/internal/3", req.State, req.ErrorCode, req.Attempts)
	}
	assertNothingSurvives(t, req, s.Email, slug)

	if calls.Load() < 3 {
		t.Fatalf("Inject called %d times, want at least once per attempt", calls.Load())
	}
	if v := badStage.Load(); v != nil {
		t.Fatalf("Inject saw stage %q, only %q is defined", v, "create")
	}

	// A failed request is terminal: another pass leaves it alone.
	runOnce(t, r)
	if again := getRequest(t, id); again != req {
		t.Fatalf("failed request changed on a later run:\n before %+v\n after  %+v", req, again)
	}
}

func TestAccountProvisionTransientFailureThenSuccess(t *testing.T) {
	var calls atomic.Int32
	inject := func(string) error {
		if calls.Add(1) <= 2 {
			return errors.New("injected transient failure")
		}
		return nil
	}
	knowledge := &fakeKnowledge{}
	r := newRunner(runnerOpts{inject: inject, knowledge: knowledge})
	slug := "acct-recover-" + token()
	id, s := insertRequest(t, reqSpec{WSSlug: slug})

	runOnce(t, r)
	backdate(t, id)
	runOnce(t, r)
	if req := getRequest(t, id); req.State != "pending" {
		t.Fatalf("state = %q after two failures, want pending", req.State)
	}
	backdate(t, id)
	runOnce(t, r)

	req := getRequest(t, id)
	if req.State != "provisioned" {
		t.Fatalf("state = %q (%s), want provisioned on the third attempt", req.State, req.ErrorCode)
	}
	if n := usersWithEmail(t, s.Email); n != 1 {
		t.Fatalf("users = %d, want 1 (rolled-back attempts must leave nothing)", n)
	}
	if n := workspacesWithSlug(t, slug); n != 1 {
		t.Fatalf("workspaces = %d, want 1", n)
	}
	if calls := knowledge.waitFor(t, req.WorkspaceID, 1); len(calls) != 1 {
		t.Fatalf("AutoEnable called %d times, want 1", len(calls))
	}
}

func assertNothingSurvives(t *testing.T, req reqRow, email, slug string) {
	t.Helper()
	if req.UserID.Valid || req.WorkspaceID.Valid {
		t.Fatalf("request points at rows after a rolled-back attempt: %+v", req)
	}
	if n := usersWithEmail(t, email); n != 0 {
		t.Fatalf("half-created users = %d, want 0", n)
	}
	if n := workspacesWithSlug(t, slug); n != 0 {
		t.Fatalf("half-created workspaces = %d, want 0", n)
	}
	if n := membersOfUserEmail(t, email); n != 0 {
		t.Fatalf("half-created members = %d, want 0", n)
	}
}

// ---- (i) audit and after-commit hook -------------------------------------------

func TestAccountProvisionWritesAuditRows(t *testing.T) {
	r := newRunner(runnerOpts{})
	slug := "acct-audit-" + token()
	id, _ := insertRequest(t, reqSpec{WSSlug: slug})
	runOnce(t, r)
	req := getRequest(t, id)
	if req.State != "provisioned" {
		t.Fatalf("state = %q (%s), want provisioned", req.State, req.ErrorCode)
	}

	// workspace.created: one row, written in the workspace's transaction.
	var targetID string
	if err := testPool.QueryRow(context.Background(),
		`SELECT target_id FROM audit_event WHERE workspace_id = $1 AND action = 'workspace.created'`, req.WorkspaceID).Scan(&targetID); err != nil {
		t.Fatalf("workspace.created audit row: %v", err)
	}
	if targetID != uuidStr(req.WorkspaceID) {
		t.Fatalf("workspace.created target_id = %q, want the workspace id", targetID)
	}
	if n := count(t, `SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = 'workspace.created'`, req.WorkspaceID); n != 1 {
		t.Fatalf("workspace.created rows = %d, want 1", n)
	}

	// account.provisioned: actor system, workspace_id set. The action is not in
	// the platform audit catalog yet, so the test uses the literal string.
	if n := count(t, `SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = 'account.provisioned'`, req.WorkspaceID); n != 1 {
		t.Fatalf("account.provisioned rows = %d, want 1", n)
	}
	var actorType string
	var wsID pgtype.UUID
	var detail string
	if err := testPool.QueryRow(context.Background(),
		`SELECT actor_type, workspace_id, detail::text FROM audit_event WHERE workspace_id = $1 AND action = 'account.provisioned'`, req.WorkspaceID).
		Scan(&actorType, &wsID, &detail); err != nil {
		t.Fatalf("account.provisioned audit row: %v", err)
	}
	if actorType != "system" {
		t.Fatalf("actor_type = %q, want system", actorType)
	}
	if wsID != req.WorkspaceID {
		t.Fatalf("workspace_id = %s, want %s", uuidStr(wsID), uuidStr(req.WorkspaceID))
	}
	// Detail names the staff who created the request, the kind and the plan.
	var createdBy uuid.UUID
	if err := testPool.QueryRow(context.Background(), `SELECT created_by FROM platform_account_request WHERE id = $1`, id).Scan(&createdBy); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{createdBy.String(), "individual", "free"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("account.provisioned detail %s does not mention %q", detail, want)
		}
	}
}

func TestAccountProvisionCallsAutoEnableOncePerWorkspaceAfterCommit(t *testing.T) {
	knowledge := &fakeKnowledge{}
	r := newRunner(runnerOpts{knowledge: knowledge})
	slug := "acct-know-" + token()
	id, _ := insertRequest(t, reqSpec{WSSlug: slug})
	// A failed request must not enable knowledge for anything.
	failedID, _ := insertRequest(t, reqSpec{WSSlug: "settings"})

	runOnce(t, r)
	req := getRequest(t, id)
	if req.State != "provisioned" {
		t.Fatalf("state = %q (%s), want provisioned", req.State, req.ErrorCode)
	}
	calls := knowledge.waitFor(t, req.WorkspaceID, 1)
	if len(calls) != 1 {
		t.Fatalf("AutoEnable calls = %d, want exactly 1", len(calls))
	}
	if calls[0].Actor != req.UserID {
		t.Fatalf("AutoEnable actor = %s, want the new owner %s", uuidStr(calls[0].Actor), uuidStr(req.UserID))
	}
	if !calls[0].Committed {
		t.Fatal("AutoEnable ran before the workspace was committed")
	}
	if failed := getRequest(t, failedID); failed.State != "failed" {
		t.Fatalf("control request state = %q, want failed", failed.State)
	}

	// A nil Knowledge is allowed and skips the call.
	nilRunner := newRunner(runnerOpts{})
	_, _ = insertRequest(t, reqSpec{WSSlug: "acct-nilk-" + token()})
	if _, err := nilRunner.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce with nil Knowledge: %v", err)
	}
	if len(knowledge.callsFor(req.WorkspaceID)) != 1 {
		t.Fatal("unrelated run changed the AutoEnable count")
	}
}
