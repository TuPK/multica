package projectaccess

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/platform-management/audit"
	dbfx "github.com/multica-ai/multica/server/internal/testutil"
	gen "github.com/multica-ai/multica/server/pkg/db/generated"
)

// wantVisible is the table of spec 7.1 rule 2. isRestricted is true for every
// access value other than the exact string "workspace".
func wantVisible(kind, access string, granted bool) bool {
	switch kind {
	case "owner", "admin":
		return true
	case "guest":
		return granted
	}
	return access == "workspace" || granted
}

// sqlAllows runs Fragment against column and reports whether the row passes.
func sqlAllows(t *testing.T, e *env, s Scope, column string) bool {
	t.Helper()
	sql := `SELECT count(*) FROM (SELECT 1 AS x) t WHERE true ` + Fragment(column)
	if !strings.Contains(sql, ScopeMarker) {
		t.Fatalf("fragment lacks %q", ScopeMarker)
	}
	sql = strings.NewReplacer("sqlc.arg(scope_mode)", "$1", "sqlc.arg(scope_user_id)", "$2").Replace(sql)
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, s.Mode(), s.UserID()).Scan(&n); err != nil {
		t.Fatalf("run fragment: %v\n%s", err, sql)
	}
	return n == 1
}

func scopeFor(t *testing.T, e *env, kind string) Scope {
	t.Helper()
	s, err := ForViewer(context.Background(), queries(e.pool), pgUUID(t, e.ws), pgUUID(t, e.users[kind]))
	if err != nil {
		t.Fatalf("ForViewer(%s): %v", kind, err)
	}
	return s
}

// matrix creates, per access value, one project granted to every viewer and
// one granted to nobody.
func matrix(t *testing.T, e *env, accesses []string) map[string][2]string {
	out := map[string][2]string{}
	for _, a := range accesses {
		granted, open := e.project("granted-"+a, a), e.project("ungranted-"+a, a)
		for _, k := range viewerKinds {
			e.grant(granted, e.users[k])
		}
		out[a] = [2]string{granted, open}
	}
	return out
}

func TestScopeMatrix(t *testing.T) {
	e := newEnv(t)
	accesses := []string{"workspace", "restricted", "weird"}
	projects := matrix(t, e, accesses)
	for _, kind := range viewerKinds {
		s := scopeFor(t, e, kind)
		for _, a := range accesses {
			for i, granted := range []bool{true, false} {
				id := projects[a][i]
				want := wantVisible(kind, a, granted)
				if got := s.Allows(pgUUID(t, id)); got != want {
					t.Errorf("%s access=%s granted=%v: Allows=%v, want %v", kind, a, granted, got, want)
				}
				sqlID := "'" + id + "'::uuid"
				if got := sqlAllows(t, e, s, sqlID); got != want {
					t.Errorf("%s access=%s granted=%v: fragment=%v, want %v", kind, a, granted, got, want)
				}
			}
		}
		// An issue that belongs to no project.
		wantNoProject := kind != "guest"
		if got := s.Allows(pgtype.UUID{}); got != wantNoProject {
			t.Errorf("%s: Allows(no project)=%v, want %v", kind, got, wantNoProject)
		}
		if got := sqlAllows(t, e, s, "NULL::uuid"); got != wantNoProject {
			t.Errorf("%s: fragment(no project)=%v, want %v", kind, got, wantNoProject)
		}
	}

	if _, err := ForViewer(context.Background(), queries(e.pool), pgUUID(t, e.ws), pgUUID(t, e.users["outsider"])); !errors.Is(err, ErrNotMember) {
		t.Errorf("outsider: err=%v, want ErrNotMember", err)
	}
}

func TestUnknownAccessValueIsRestricted(t *testing.T) {
	e := newEnv(t)
	restricted := matrix(t, e, []string{"restricted"})["restricted"]
	weird := matrix(t, e, []string{"weird"})["weird"]
	for _, kind := range viewerKinds {
		s := scopeFor(t, e, kind)
		for i := range restricted {
			r, w := s.Allows(pgUUID(t, restricted[i])), s.Allows(pgUUID(t, weird[i]))
			if r != w {
				t.Errorf("%s project %d: restricted=%v but unknown value=%v", kind, i, r, w)
			}
			rs := sqlAllows(t, e, s, "'"+restricted[i]+"'::uuid")
			ws := sqlAllows(t, e, s, "'"+weird[i]+"'::uuid")
			if rs != ws {
				t.Errorf("%s project %d: fragment restricted=%v but unknown value=%v", kind, i, rs, ws)
			}
		}
	}
}

// failingScopes fails the query named by the field that is set.
type failingScopes struct{ member, access bool }

func (f failingScopes) GetViewerMember(ctx context.Context, arg gen.GetViewerMemberParams) (gen.GetViewerMemberRow, error) {
	if f.member {
		return gen.GetViewerMemberRow{}, errors.New("db down")
	}
	return gen.GetViewerMemberRow{Role: "member"}, nil
}

func (f failingScopes) ListViewerProjectAccess(ctx context.Context, arg gen.ListViewerProjectAccessParams) ([]gen.ListViewerProjectAccessRow, error) {
	return nil, errors.New("db down")
}

// neverTx fails the test if a request reaches the business step.
type neverTx struct{ t *testing.T }

func (n neverTx) Begin(ctx context.Context) (pgx.Tx, error) {
	n.t.Error("the business step ran without a Scope")
	return nil, errors.New("must not run")
}

func TestScopeErrorFailsRequest(t *testing.T) {
	e := newEnv(t)
	project := e.project("p", "workspace")
	for name, fake := range map[string]failingScopes{
		"viewer lookup fails": {member: true},
		"project list fails":  {access: true},
	} {
		h := NewHandler(queries(e.pool), neverTx{t})
		h.Scopes = fake
		router := routes(h)
		base := "/api/projects/" + project
		reqs := []*http.Request{
			e.as("member", http.MethodPut, base+"/access", map[string]any{"access": "restricted"}),
			e.as("member", http.MethodGet, base+"/members", nil),
			e.as("member", http.MethodPost, base+"/members", map[string]any{"user_ids": []string{e.users["admin"]}}),
			e.as("member", http.MethodDelete, base+"/members/"+e.users["admin"], nil),
		}
		for _, req := range reqs {
			dbfx.Call(t, router.ServeHTTP, req).Want(http.StatusServiceUnavailable)
		}
		if got := e.accessOf(project); got != "workspace" {
			t.Errorf("%s: access changed to %q", name, got)
		}
	}
}

// failingAudit makes every audit write fail.
type failingAudit struct{}

func (failingAudit) InsertAuditEvent(ctx context.Context, e audit.Entry) error {
	return errors.New("audit down")
}
