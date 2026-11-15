package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/workspacestate"
	"github.com/multica-ai/multica/server/internal/middleware"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// NOT RUN where this was written: this package's TestMain needs a database.
// The tests use fakes only. They check that the two routes that join a user to
// a workspace refuse a workspace that is not active, before any seat is
// reserved or any row is written.

const (
	joinTestUser      = "33333333-3333-4333-8333-333333333333"
	joinTestWorkspace = "22222222-2222-4222-8222-222222222222"
	joinTestEmail     = "invitee@example.test"
)

func joinUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(s)
	return u
}

// joinDB answers the reads the two handlers make before they reach the
// transaction, and records every statement.
type joinDB struct {
	db.DBTX
	names []string
	execs int
}

func (d *joinDB) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	name := strings.TrimPrefix(sql, "-- name: ")
	if i := strings.Index(name, " "); i >= 0 {
		name = name[:i]
	}
	d.names = append(d.names, name)
	return joinRow{name: name}
}

func (d *joinDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	d.execs++
	return pgconn.CommandTag{}, errors.New("joinDB: unexpected write")
}

type joinRow struct {
	pgx.Row
	name string
}

func (r joinRow) Scan(dest ...any) error {
	switch r.name {
	case "GetInvitation": // id, workspace_id, inviter_id, invitee_email, invitee_user_id, role, status, ...
		*dest[0].(*pgtype.UUID) = joinUUID("44444444-4444-4444-8444-444444444444")
		*dest[1].(*pgtype.UUID) = joinUUID(joinTestWorkspace)
		*dest[3].(*string) = joinTestEmail
		*dest[5].(*string) = "member"
		*dest[6].(*string) = "pending"
	case "GetUser": // id, name, email, ...
		*dest[0].(*pgtype.UUID) = joinUUID(joinTestUser)
		*dest[2].(*string) = joinTestEmail
	case "GetActiveShareLinkByCode": // id, workspace_id, ...
		*dest[0].(*pgtype.UUID) = joinUUID("55555555-5555-4555-8555-555555555555")
		*dest[1].(*pgtype.UUID) = joinUUID(joinTestWorkspace)
	case "GetMemberByUserAndWorkspace":
		return pgx.ErrNoRows
	default:
		return errors.New("joinDB: unexpected query " + r.name)
	}
	return nil
}

// joinTx counts Begin calls: the first thing either handler does after its
// seat reservation, and what an admitted join reaches.
type joinTx struct{ begins int }

func (t *joinTx) Begin(context.Context) (pgx.Tx, error) {
	t.begins++
	return nil, errors.New("joinTx: no transaction in this test")
}

type joinFixture struct {
	router http.Handler
	db     *joinDB
	tx     *joinTx
}

func newJoinFixture(store *wsStateStore, withGuard bool) joinFixture {
	d := &joinDB{}
	tx := &joinTx{}
	h := &Handler{Queries: db.New(d), TxStarter: tx}
	r := chi.NewRouter()
	if withGuard {
		r.Use(middleware.WithWorkspaceState(workspacestate.New(store, wsStateNoCache{})))
	}
	r.Post("/api/invitations/{id}/accept", h.AcceptInvitation)
	r.Post("/api/invitations/{id}/decline", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	r.Post("/api/share-links/join", h.JoinByShareLink)
	return joinFixture{router: r, db: d, tx: tx}
}

func (f joinFixture) call(path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("X-User-ID", joinTestUser)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

var joinRoutes = []struct{ name, path, body string }{
	{"accept invitation", "/api/invitations/44444444-4444-4444-8444-444444444444/accept", ""},
	{"join by share link", "/api/share-links/join", `{"code":"abc"}`},
}

func TestJoinRoutesRefuseAWorkspaceThatIsNotActive(t *testing.T) {
	for _, route := range joinRoutes {
		for status, code := range map[workspacestate.Status]string{
			workspacestate.ReadOnly:        "workspace_read_only",
			workspacestate.Suspended:       "workspace_suspended",
			workspacestate.PendingDeletion: "workspace_pending_deletion",
		} {
			t.Run(route.name+"/"+string(status), func(t *testing.T) {
				f := newJoinFixture(wsStateStoreFor(status), true)
				rec := f.call(route.path, route.body)
				if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"`+code+`"`) {
					t.Fatalf("status %d body %q, want 403 %s", rec.Code, rec.Body.String(), code)
				}
				// Nothing was reserved or written: no transaction, no statement
				// beyond the reads that identify the workspace, and no
				// membership lookup that could reveal the user's membership.
				if f.tx.begins != 0 || f.db.execs != 0 {
					t.Fatalf("a refused join began %d transactions and ran %d writes", f.tx.begins, f.db.execs)
				}
				for _, n := range f.db.names {
					if n != "GetInvitation" && n != "GetUser" && n != "GetActiveShareLinkByCode" {
						t.Fatalf("a refused join ran %s (all statements: %v)", n, f.db.names)
					}
				}
			})
		}
	}
}

func TestJoinRoutesAdmitAnActiveWorkspaceAndOneWithNoRow(t *testing.T) {
	stores := map[string]*wsStateStore{
		"no row":     wsStateStoreFor(workspacestate.Active),
		"active row": {rec: workspacestate.Record{Status: workspacestate.Active}, found: true},
	}
	for _, route := range joinRoutes {
		for name, store := range stores {
			t.Run(route.name+"/"+name, func(t *testing.T) {
				f := newJoinFixture(store, true)
				rec := f.call(route.path, route.body)
				// The join proceeds to its transaction; the test's transaction
				// starter fails there, which is the 500 below. A 403 would mean
				// the gate refused.
				if f.tx.begins != 1 || rec.Code != http.StatusInternalServerError {
					t.Fatalf("status %d, transactions begun %d; want the join to reach its transaction", rec.Code, f.tx.begins)
				}
			})
		}
	}
}

func TestJoinRoutesWithoutAGuardAreUnchanged(t *testing.T) {
	for _, route := range joinRoutes {
		f := newJoinFixture(wsStateStoreFor(workspacestate.Suspended), false)
		if rec := f.call(route.path, route.body); f.tx.begins != 1 || rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status %d, transactions begun %d", route.name, rec.Code, f.tx.begins)
		}
	}
}

func TestJoinRoutesFailClosedWhenTheStateIsUnavailable(t *testing.T) {
	for _, route := range joinRoutes {
		f := newJoinFixture(&wsStateStore{err: errors.New("db down")}, true)
		rec := f.call(route.path, route.body)
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"code":"workspace_state_unavailable"`) {
			t.Errorf("%s: status %d body %q, want 503 workspace_state_unavailable", route.name, rec.Code, rec.Body.String())
		}
		if f.tx.begins != 0 || f.db.execs != 0 {
			t.Errorf("%s: a join with an unreadable state began %d transactions", route.name, f.tx.begins)
		}
	}
}
