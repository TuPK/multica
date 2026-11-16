package handler

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Database-backed: they run only against the dedicated *_test database named by
// PLATFORM_TEST_DATABASE_URL (see requirePlatformTestDatabase). The pure
// parameter and CSV tests are in audit_events_query_test.go.

func auditTestRouter() http.Handler {
	r := chi.NewRouter()
	r.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWorkspaceRoleFromURL(testHandler.Queries, "id", "owner", "admin"))
			r.Get("/audit-events", testHandler.ListWorkspaceAuditEvents)
		})
	})
	r.With(RequireHumanActor).Get("/api/me/security-events", testHandler.ListMySecurityEvents)
	return r
}

// seedAuditEvents inserts n events in one statement and removes them when the
// test ends. ws and actor may be empty for NULL. step is the gap between
// consecutive created_at values; zero gives them all the same timestamp.
func seedAuditEvents(t *testing.T, ws, actor, action string, n int, start time.Time, step time.Duration) []string {
	t.Helper()
	ids := make([]string, n)
	for i := range ids {
		ids[i] = uuid.NewString()
	}
	for i, id := range ids {
		var wsArg, actorArg any
		if ws != "" {
			wsArg = ws
		}
		if actor != "" {
			actorArg = actor
		}
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO audit_event (id, workspace_id, actor_type, actor_id, action, created_at)
			 VALUES ($1, $2, 'member', $3, $4, $5)`,
			id, wsArg, actorArg, action, start.Add(time.Duration(i)*step))
		if err != nil {
			t.Fatalf("seed audit event: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM audit_event WHERE id = ANY($1::uuid[])`, ids)
	})
	return ids
}

func auditGet(t *testing.T, router http.Handler, userID, path string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-User-ID", userID)
	testutil.WithHeaders(req, headers...)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeAuditPage(t *testing.T, rec *httptest.ResponseRecorder) AuditEventsResponse {
	t.Helper()
	var page AuditEventsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode page: %v: %s", err, rec.Body.String())
	}
	return page
}

// auditWorkspace creates a workspace with an owner, an admin and a member.
func auditWorkspace(t *testing.T) (ws, owner, admin, member string) {
	t.Helper()
	ws = dbfx.Workspace(t, "Audit "+uuid.NewString()[:8], "audit-"+uuid.NewString()[:8])
	owner = dbfx.User(t, "Owner", testutil.UniqueEmail("audit-owner"))
	admin = dbfx.User(t, "Admin", testutil.UniqueEmail("audit-admin"))
	member = dbfx.User(t, "Member", testutil.UniqueEmail("audit-member"))
	dbfx.Member(t, ws, owner, "owner")
	dbfx.Member(t, ws, admin, "admin")
	dbfx.Member(t, ws, member, "member")
	return
}

func TestListWorkspaceAuditEvents_AccessAndIsolation(t *testing.T) {
	requirePlatformTestDatabase(t)
	router := auditTestRouter()
	ws, owner, admin, member := auditWorkspace(t)
	otherWS, otherOwner, _, _ := auditWorkspace(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	mine := seedAuditEvents(t, ws, owner, "member.removed", 2, now, time.Second)
	theirs := seedAuditEvents(t, otherWS, otherOwner, "member.removed", 1, now, 0)
	path := "/api/workspaces/" + ws + "/audit-events"

	if rec := auditGet(t, router, member, path); rec.Code != http.StatusForbidden {
		t.Fatalf("member: status %d, want 403", rec.Code)
	}
	// A non-member gets 404 from the membership gate, not 403: the workspace is
	// not revealed to someone outside it.
	if rec := auditGet(t, router, otherOwner, path); rec.Code != http.StatusNotFound {
		t.Fatalf("owner of another workspace: status %d, want 404", rec.Code)
	}
	for name, uid := range map[string]string{"owner": owner, "admin": admin} {
		rec := auditGet(t, router, uid, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d, want 200: %s", name, rec.Code, rec.Body.String())
		}
		page := decodeAuditPage(t, rec)
		if len(page.Events) != len(mine) {
			t.Fatalf("%s: %d events, want %d", name, len(page.Events), len(mine))
		}
		for _, e := range page.Events {
			if e.ID == theirs[0] || e.WorkspaceID == nil || *e.WorkspaceID != ws {
				t.Fatalf("%s: event from another workspace: %+v", name, e)
			}
		}
		if page.NextCursor != nil {
			t.Fatalf("%s: unexpected next_cursor", name)
		}
	}
}

func TestListWorkspaceAuditEvents_PaginationWithEqualTimestamps(t *testing.T) {
	requirePlatformTestDatabase(t)
	router := auditTestRouter()
	ws, owner, _, _ := auditWorkspace(t)
	same := time.Now().UTC().Truncate(time.Microsecond)
	want := seedAuditEvents(t, ws, owner, "member.removed", 7, same, 0)

	seen := map[string]bool{}
	var order []string
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		path := "/api/workspaces/" + ws + "/audit-events?limit=3"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		rec := auditGet(t, router, owner, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		page := decodeAuditPage(t, rec)
		for _, e := range page.Events {
			if seen[e.ID] {
				t.Fatalf("duplicate row %s", e.ID)
			}
			seen[e.ID] = true
			order = append(order, e.ID)
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if len(seen) != len(want) {
		t.Fatalf("saw %d rows, want %d", len(seen), len(want))
	}
	for _, id := range want {
		if !seen[id] {
			t.Fatalf("row %s skipped", id)
		}
	}
	for i := 1; i < len(order); i++ {
		if order[i-1] < order[i] {
			t.Fatalf("ids not descending within equal timestamps: %v", order)
		}
	}
}

func TestListWorkspaceAuditEvents_Filters(t *testing.T) {
	requirePlatformTestDatabase(t)
	router := auditTestRouter()
	ws, owner, admin, _ := auditWorkspace(t)
	base := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	seedAuditEvents(t, ws, owner, "member.removed", 1, base, 0)
	seedAuditEvents(t, ws, admin, "pat.created", 1, base.Add(24*time.Hour), 0)
	seedAuditEvents(t, ws, admin, "member.removed", 1, base.Add(48*time.Hour), 0)
	path := "/api/workspaces/" + ws + "/audit-events"

	count := func(query string) int {
		rec := auditGet(t, router, owner, path+"?"+query)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d: %s", query, rec.Code, rec.Body.String())
		}
		return len(decodeAuditPage(t, rec).Events)
	}
	if n := count("action=member.removed"); n != 2 {
		t.Errorf("action filter: %d, want 2", n)
	}
	if n := count("actor_id=" + admin); n != 2 {
		t.Errorf("actor filter: %d, want 2", n)
	}
	from := url.QueryEscape(base.Add(24 * time.Hour).Format(time.RFC3339))
	to := url.QueryEscape(base.Add(48 * time.Hour).Format(time.RFC3339))
	if n := count("from=" + from + "&to=" + to); n != 1 {
		t.Errorf("from inclusive, to exclusive: %d, want 1", n)
	}
	for query, code := range map[string]string{
		"action=nope":                "invalid_filter",
		"limit=0":                    "invalid_filter",
		"from=" + to + "&to=" + from: "invalid_filter",
		"cursor=zzz":                 "invalid_cursor",
	} {
		rec := auditGet(t, router, owner, path+"?"+query)
		var body map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusBadRequest || body["code"] != code {
			t.Errorf("%s: %d %v, want 400 %s", query, rec.Code, body, code)
		}
	}
}

func TestListWorkspaceAuditEvents_CSVAndCap(t *testing.T) {
	requirePlatformTestDatabase(t)
	router := auditTestRouter()
	ws, owner, _, _ := auditWorkspace(t)
	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	seedAuditEvents(t, ws, owner, "member.removed", 3, start, time.Second)
	path := "/api/workspaces/" + ws + "/audit-events?format=csv"

	rec := auditGet(t, router, owner, path)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("status %d, content-type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("X-Audit-Truncated") != "" {
		t.Fatal("small export marked truncated")
	}
	rows, err := csv.NewReader(rec.Body).ReadAll()
	if err != nil || len(rows) != 4 {
		t.Fatalf("rows %d err %v, want header + 3", len(rows), err)
	}

	// One row past the cap: the export stops at the cap and says so.
	_, err = testPool.Exec(context.Background(),
		`INSERT INTO audit_event (workspace_id, actor_type, actor_id, action, created_at)
		 SELECT $1, 'member', $2, 'member.removed', $3::timestamptz - (g || ' seconds')::interval
		 FROM generate_series(1, $4) g`,
		ws, owner, start, auditCSVMaxRows-3+1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM audit_event WHERE workspace_id = $1`, ws)
	})
	rec = auditGet(t, router, owner, path)
	if rec.Header().Get("X-Audit-Truncated") != "true" {
		t.Fatal("capped export lacks X-Audit-Truncated")
	}
	rows, err = csv.NewReader(rec.Body).ReadAll()
	if err != nil || len(rows) != auditCSVMaxRows+1 {
		t.Fatalf("rows %d err %v, want header + %d", len(rows), err, auditCSVMaxRows)
	}
}

func TestListMySecurityEvents(t *testing.T) {
	requirePlatformTestDatabase(t)
	router := auditTestRouter()
	ws, owner, _, _ := auditWorkspace(t)
	other := dbfx.User(t, "Other", testutil.UniqueEmail("audit-other"))
	now := time.Now().UTC().Truncate(time.Microsecond)
	mine := seedAuditEvents(t, "", owner, "auth.login", 3, now, time.Second)
	seedAuditEvents(t, "", other, "auth.login", 1, now, 0)
	seedAuditEvents(t, ws, owner, "member.removed", 1, now, 0)

	rec := auditGet(t, router, owner, "/api/me/security-events?limit=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	page := decodeAuditPage(t, rec)
	if len(page.Events) != 2 || page.NextCursor == nil {
		t.Fatalf("first page: %d events, cursor %v", len(page.Events), page.NextCursor)
	}
	rec = auditGet(t, router, owner, "/api/me/security-events?limit=2&cursor="+url.QueryEscape(*page.NextCursor))
	rest := decodeAuditPage(t, rec)
	if len(rest.Events) != 1 || rest.NextCursor != nil {
		t.Fatalf("second page: %d events, cursor %v", len(rest.Events), rest.NextCursor)
	}
	got := map[string]bool{}
	for _, e := range append(page.Events, rest.Events...) {
		got[e.ID] = true
		if e.WorkspaceID != nil || e.ActorID == nil || *e.ActorID != owner {
			t.Fatalf("not an own account-level event: %+v", e)
		}
	}
	for _, id := range mine {
		if !got[id] {
			t.Fatalf("missing own event %s", id)
		}
	}

	for _, src := range []string{"task_token", "cloud_pat"} {
		rec := auditGet(t, router, owner, "/api/me/security-events", "X-Actor-Source", src)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", src, rec.Code)
		}
	}
}
