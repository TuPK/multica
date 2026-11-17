package projectaccess

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	dbfx "github.com/multica-ai/multica/server/internal/testutil"
	gen "github.com/multica-ai/multica/server/pkg/db/generated"
)

// viewerKinds are the people the access table of spec 7.1 rule 2 tells apart.
var viewerKinds = []string{"owner", "admin", "member", "viewer", "guest"}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// env is one workspace with one user of each viewer kind and an outsider.
type env struct {
	t     *testing.T
	pool  *pgxpool.Pool
	f     *dbfx.Fixture
	ws    string
	users map[string]string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool := testPool(t)
	f := dbfx.New(pool, "", "")
	slug := "pa-" + uuid.NewString()[:12]
	e := &env{t: t, pool: pool, f: f, users: map[string]string{}}
	for _, k := range append(append([]string{}, viewerKinds...), "outsider") {
		e.users[k] = f.User(t, k, dbfx.UniqueEmail(k))
	}
	e.ws = f.Workspace(t, slug, slug)
	f.WorkspaceID, f.UserID = e.ws, e.users["owner"]
	f.Cleanup(t, `DELETE FROM project_member WHERE workspace_id = $1`, e.ws)
	f.Cleanup(t, `DELETE FROM audit_event WHERE workspace_id = $1`, e.ws)
	for _, k := range viewerKinds {
		role := k
		cols := dbfx.Cols{}
		if k == "guest" {
			role, cols["is_guest"] = "member", true
		}
		f.Member(t, e.ws, e.users[k], role, cols)
	}
	return e
}

func (e *env) project(title, access string, over ...dbfx.Cols) string {
	e.t.Helper()
	return e.f.Project(e.t, title, append([]dbfx.Cols{{"access": access}}, over...)...)
}

func (e *env) grant(projectID, userID string) {
	e.t.Helper()
	e.f.Exec(e.t, `INSERT INTO project_member (project_id, workspace_id, user_id) VALUES ($1, $2, $3)`, projectID, e.ws, userID)
}

func (e *env) count(sql string, args ...any) int {
	e.t.Helper()
	return e.f.Count(e.t, sql, args...)
}

func (e *env) auditCount(action string) int {
	e.t.Helper()
	return e.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = $2`, e.ws, action)
}

func (e *env) accessOf(projectID string) string {
	e.t.Helper()
	var a string
	e.f.QueryRow(e.t, `SELECT access FROM project WHERE id = $1`, projectID).Scan(&a)
	return a
}

func (e *env) members(projectID string) int {
	e.t.Helper()
	return e.count(`SELECT count(*) FROM project_member WHERE project_id = $1`, projectID)
}

func pgUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	u, err := uuid.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return pgtype.UUID{Bytes: u, Valid: true}
}

func routes(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/projects/{id}", func(r chi.Router) {
		r.Put("/access", h.PutAccess)
		r.Get("/members", h.ListMembers)
		r.Post("/members", h.AddMembers)
		r.Delete("/members/{userId}", h.RemoveMember)
	})
	return r
}

// as builds a request from one of the environment's users.
func (e *env) as(kind, method, path string, body any) *http.Request {
	return dbfx.WithHeaders(dbfx.JSONRequest(method, path, body),
		"X-User-ID", e.users[kind], "X-Workspace-ID", e.ws)
}

func queries(pool *pgxpool.Pool) *gen.Queries { return gen.New(pool) }
