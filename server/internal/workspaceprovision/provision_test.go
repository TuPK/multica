package workspaceprovision

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/auditstore"
	"github.com/multica-ai/multica/server/internal/issuestatus"
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
		fmt.Fprintln(os.Stderr, "workspaceprovision tests:", err)
		os.Exit(1)
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "workspaceprovision tests: connect:", err)
		os.Exit(1)
	}
	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "workspaceprovision tests: ping:", err)
		os.Exit(1)
	}
	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func token() string { return strings.ReplaceAll(uuid.NewString(), "-", "")[:12] }

func newUser(t *testing.T) db.User {
	t.Helper()
	u, err := db.New(testPool).CreateUser(context.Background(), db.CreateUserParams{
		Name:  "provision test",
		Email: "provision-" + token() + "@example.test",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = testPool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, u.ID)
	})
	return u
}

func cleanupWorkspaceBySlug(t *testing.T, slug string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = testPool.Exec(ctx, `DELETE FROM audit_event WHERE workspace_id IN (SELECT id FROM workspace WHERE slug = $1)`, slug)
		_, _ = testPool.Exec(ctx, `DELETE FROM workspace WHERE slug = $1`, slug)
	})
}

func countWhere(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// runCreate runs Create in its own transaction and commits when Create succeeds.
func runCreate(t *testing.T, p Params) (db.Workspace, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	qtx := db.New(testPool).WithTx(tx)
	ws, err := Create(ctx, qtx, p)
	if err != nil {
		return db.Workspace{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return ws, nil
}

func auditRecorder(t *testing.T, calls *[]audit.Entry) func(audit.Entry) error {
	return func(e audit.Entry) error {
		*calls = append(*calls, e)
		return nil
	}
}

func TestProvisionCreateWritesWorkspaceOwnerStatusesAndAudit(t *testing.T) {
	owner := newUser(t)
	slug := "prov-" + token()
	cleanupWorkspaceBySlug(t, slug)

	var recorded []audit.Entry
	ws, err := runCreate(t, Params{
		Name:        "Provision Co",
		Slug:        slug,
		IssuePrefix: "PRV",
		Description: pgtype.Text{String: "a description", Valid: true},
		OwnerID:     owner.ID,
		Record:      auditRecorder(t, &recorded),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ws.Name != "Provision Co" || ws.Slug != slug || ws.IssuePrefix != "PRV" {
		t.Fatalf("workspace = %+v", ws)
	}
	if !ws.Description.Valid || ws.Description.String != "a description" {
		t.Fatalf("description = %+v", ws.Description)
	}

	if n := countWhere(t, `SELECT count(*) FROM member WHERE workspace_id = $1 AND user_id = $2 AND role = 'owner'`, ws.ID, owner.ID); n != 1 {
		t.Fatalf("owner member rows = %d, want 1", n)
	}
	if n := countWhere(t, `SELECT count(*) FROM member WHERE workspace_id = $1`, ws.ID); n != 1 {
		t.Fatalf("member rows = %d, want only the owner", n)
	}
	built := countWhere(t, `SELECT count(*) FROM issue_status WHERE workspace_id = $1 AND is_system`, ws.ID)
	if want := len(issuestatus.Canonical()); built != want || want != 7 {
		t.Fatalf("built-in issue statuses = %d (canonical %d), want 7", built, want)
	}
	if len(recorded) != 1 {
		t.Fatalf("Record called %d times, want 1", len(recorded))
	}
	e := recorded[0]
	if e.Action != audit.ActionWorkspaceCreated || e.WorkspaceID != uuid.UUID(ws.ID.Bytes).String() ||
		e.TargetType != "workspace" || e.TargetID != uuid.UUID(ws.ID.Bytes).String() || e.TargetLabel != ws.Name {
		t.Fatalf("audit entry = %+v", e)
	}
}

// Record runs inside the caller's transaction: the entry it writes commits or
// rolls back with the workspace.
func TestProvisionCreateRecordRunsInCallerTransaction(t *testing.T) {
	owner := newUser(t)
	slug := "prov-" + token()
	cleanupWorkspaceBySlug(t, slug)
	ctx := context.Background()

	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	qtx := db.New(testPool).WithTx(tx)
	ws, err := Create(ctx, qtx, Params{
		Name: "Tx Co", Slug: slug, IssuePrefix: "TXC", OwnerID: owner.ID,
		Record: func(e audit.Entry) error {
			e.ActorType = audit.ActorSystem
			return audit.Record(ctx, auditstore.New(qtx), e)
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n := countWhere(t, `SELECT count(*) FROM workspace WHERE slug = $1`, slug); n != 0 {
		t.Fatalf("workspace visible before commit: %d", n)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if n := countWhere(t, `SELECT count(*) FROM workspace WHERE slug = $1`, slug); n != 0 {
		t.Fatalf("workspace survived rollback: %d", n)
	}
	if n := countWhere(t, `SELECT count(*) FROM audit_event WHERE workspace_id = $1`, ws.ID); n != 0 {
		t.Fatalf("audit rows survived rollback: %d", n)
	}
	if n := countWhere(t, `SELECT count(*) FROM issue_status WHERE workspace_id = $1`, ws.ID); n != 0 {
		t.Fatalf("issue statuses survived rollback: %d", n)
	}
}

func TestProvisionCreateStepErrors(t *testing.T) {
	t.Run("create: duplicate slug", func(t *testing.T) {
		owner := newUser(t)
		slug := "prov-" + token()
		cleanupWorkspaceBySlug(t, slug)
		if _, err := runCreate(t, Params{Name: "First", Slug: slug, IssuePrefix: "FST", OwnerID: owner.ID, Record: func(audit.Entry) error { return nil }}); err != nil {
			t.Fatalf("first Create: %v", err)
		}
		_, err := runCreate(t, Params{Name: "Second", Slug: slug, IssuePrefix: "SND", OwnerID: owner.ID, Record: func(audit.Entry) error { return nil }})
		assertStep(t, err, "create")
		if n := countWhere(t, `SELECT count(*) FROM workspace WHERE slug = $1`, slug); n != 1 {
			t.Fatalf("workspaces with slug = %d, want 1", n)
		}
	})

	t.Run("owner: unknown user", func(t *testing.T) {
		slug := "prov-" + token()
		cleanupWorkspaceBySlug(t, slug)
		ghost := pgtype.UUID{Bytes: uuid.New(), Valid: true}
		_, err := runCreate(t, Params{Name: "Ghost", Slug: slug, IssuePrefix: "GHO", OwnerID: ghost, Record: func(audit.Entry) error { return nil }})
		assertStep(t, err, "owner")
		if n := countWhere(t, `SELECT count(*) FROM workspace WHERE slug = $1`, slug); n != 0 {
			t.Fatalf("workspace survived a failed owner step: %d", n)
		}
	})

	t.Run("audit: Record error", func(t *testing.T) {
		owner := newUser(t)
		slug := "prov-" + token()
		cleanupWorkspaceBySlug(t, slug)
		boom := errors.New("audit down")
		_, err := runCreate(t, Params{
			Name: "Audit", Slug: slug, IssuePrefix: "AUD", OwnerID: owner.ID,
			Record: func(audit.Entry) error { return boom },
		})
		assertStep(t, err, "audit")
		var se *StepError
		if !errors.As(err, &se) || !errors.Is(se.Unwrap(), boom) || !errors.Is(err, boom) {
			t.Fatalf("StepError must unwrap to the Record error, got %v", err)
		}
		if n := countWhere(t, `SELECT count(*) FROM workspace WHERE slug = $1`, slug); n != 0 {
			t.Fatalf("workspace survived a failed audit step: %d", n)
		}
	})
}

func assertStep(t *testing.T, err error, step string) {
	t.Helper()
	var se *StepError
	if !errors.As(err, &se) {
		t.Fatalf("error = %v (%T), want *StepError", err, err)
	}
	if se.Step != step {
		t.Fatalf("Step = %q, want %q (err: %v)", se.Step, step, se.Err)
	}
	if se.Err == nil {
		t.Fatalf("StepError.Err is nil")
	}
}
