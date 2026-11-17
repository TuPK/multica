package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/multica-ai/multica/server/internal/membergroup"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// permissionDB answers the member lookup with a member of role and the
// permission lookup with the given group permission arrays, and counts the
// latter.
type permissionDB struct {
	db.DBTX
	role        string
	groups      [][]byte
	groupLookup *int
}

func (d permissionDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return permissionRow{d: d}
}

type permissionRow struct {
	pgx.Row
	d permissionDB
}

func (r permissionRow) Scan(dest ...any) error {
	switch len(dest) {
	case 7: // GetMemberByUserAndWorkspace: id, workspace_id, user_id, role, created_at, is_guest, pinned_at
		*(dest[3].(*string)) = r.d.role
		*(dest[5].(*bool)) = false
	case 2: // GetMemberPermissionInputs: role, group_permissions
		*r.d.groupLookup++
		*(dest[0].(*string)) = r.d.role
		*(dest[1].(*[][]byte)) = r.d.groups
	default:
		return errors.New("unexpected column layout")
	}
	return nil
}

func groupOf(keys ...string) []byte {
	raw, _ := json.Marshal(keys)
	return raw
}

func permissionRouter(role string, lookups *int, groups ...[]byte) http.Handler {
	queries := db.New(permissionDB{role: role, groups: groups, groupLookup: lookups})
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	r := chi.NewRouter()
	r.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.Use(RequireWorkspacePermissionFromURL(queries, "id", membergroup.KeyAuditRead))
		r.Get("/audit-events", ok)
		r.Post("/audit-events", ok)
	})
	r.Group(func(r chi.Router) {
		r.Use(RequireWorkspacePermission(queries, membergroup.KeyAuditRead))
		r.Get("/api/cloud-subscriptions/usage", ok)
	})
	return r
}

func servePermission(h http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("X-User-ID", stateTestUser)
	req.Header.Set("X-Workspace-ID", stateTestWorkspace)
	req = req.WithContext(WithSession(req.Context(), []string{"otp"}, 0, false))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

const permissionPath = "/api/workspaces/" + stateTestWorkspace + "/audit-events"

func TestOwnerAndAdminPassPermissionWithoutAGroupQuery(t *testing.T) {
	for _, role := range []string{"owner", "admin"} {
		lookups := 0
		rec := servePermission(permissionRouter(role, &lookups), http.MethodGet, permissionPath)
		if rec.Code != http.StatusNoContent {
			t.Errorf("%s: status %d, want 204", role, rec.Code)
		}
		if lookups != 0 {
			t.Errorf("%s: %d group queries, want 0", role, lookups)
		}
	}
}

func TestMemberPermissionNeedsTheKeyFromAGroup(t *testing.T) {
	lookups := 0
	rec := servePermission(permissionRouter("member", &lookups, groupOf(membergroup.KeyAuditRead)), http.MethodGet, permissionPath)
	if rec.Code != http.StatusNoContent {
		t.Errorf("member with the key: status %d, want 204 (body %q)", rec.Code, rec.Body.String())
	}
	if lookups != 1 {
		t.Errorf("%d group queries, want 1", lookups)
	}

	for name, groups := range map[string][][]byte{
		"no group":    nil,
		"other key":   {groupOf(membergroup.KeyBillingView)},
		"unknown key": {groupOf("audit.write")},
	} {
		lookups = 0
		rec := servePermission(permissionRouter("member", &lookups, groups...), http.MethodGet, permissionPath)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, rec.Code)
			continue
		}
		var body struct {
			Code       string `json:"code"`
			Permission string `json:"permission"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: body %q is not JSON: %v", name, rec.Body.String(), err)
		}
		if body.Code != "permission_denied" || body.Permission != membergroup.KeyAuditRead {
			t.Errorf("%s: body %q, want permission_denied for %s", name, rec.Body.String(), membergroup.KeyAuditRead)
		}
	}
}

// The viewer rule stays in force: a viewer holding the key reads but cannot
// write (spec 7b.2, rule 7).
func TestViewerWithTheKeyStillRefusedOnAWrite(t *testing.T) {
	lookups := 0
	h := permissionRouter("viewer", &lookups, groupOf(membergroup.KeyAuditRead))
	if rec := servePermission(h, http.MethodGet, permissionPath); rec.Code != http.StatusNoContent {
		t.Errorf("viewer read: status %d, want 204 (body %q)", rec.Code, rec.Body.String())
	}
	rec := servePermission(h, http.MethodPost, permissionPath)
	if rec.Code != http.StatusForbidden || codeOf(t, rec) != "viewer_read_only" {
		t.Errorf("viewer write: status %d body %q, want 403 viewer_read_only", rec.Code, rec.Body.String())
	}
}

// The variant without a URL parameter reads the workspace from X-Workspace-ID.
func TestRequireWorkspacePermissionReadsTheWorkspaceHeader(t *testing.T) {
	lookups := 0
	h := permissionRouter("member", &lookups, groupOf(membergroup.KeyAuditRead))
	if rec := servePermission(h, http.MethodGet, "/api/cloud-subscriptions/usage"); rec.Code != http.StatusNoContent {
		t.Errorf("with the key: status %d, want 204 (body %q)", rec.Code, rec.Body.String())
	}

	h = permissionRouter("member", &lookups)
	if rec := servePermission(h, http.MethodGet, "/api/cloud-subscriptions/usage"); rec.Code != http.StatusForbidden || codeOf(t, rec) != "permission_denied" {
		t.Errorf("without the key: status %d body %q, want 403 permission_denied", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/cloud-subscriptions/usage", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("no workspace identifier: status %d, want 400", rec.Code)
	}
}
