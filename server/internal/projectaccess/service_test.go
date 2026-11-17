package projectaccess

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/platform-management/audit"
	dbfx "github.com/multica-ai/multica/server/internal/testutil"
)

func (e *env) handler() (*Handler, http.Handler) {
	h := NewHandler(queries(e.pool), e.pool)
	return h, routes(h)
}

// snapshot is the content of every row the switch must not touch.
func (e *env) snapshot(projectID string) string {
	e.t.Helper()
	var s string
	e.f.QueryRow(e.t, `SELECT
		(SELECT coalesce(jsonb_agg(to_jsonb(i) ORDER BY i.id), '[]') FROM issue i WHERE i.project_id = $1)::text ||
		(SELECT coalesce(jsonb_agg(to_jsonb(c) ORDER BY c.id), '[]') FROM comment c JOIN issue i ON i.id = c.issue_id WHERE i.project_id = $1)::text ||
		(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY a.id), '[]') FROM attachment a JOIN issue i ON i.id = a.issue_id WHERE i.project_id = $1)::text`,
		projectID).Scan(&s)
	return s
}

func TestSwitchAccessKeepsIssuesAndAudits(t *testing.T) {
	e := newEnv(t)
	_, router := e.handler()
	project := e.project("p", "workspace", dbfx.Cols{"creator_type": "member", "creator_id": e.users["owner"]})
	issue := e.f.Issue(t, "i", dbfx.Cols{"project_id": project})
	e.f.Comment(t, issue, "c")
	e.f.Insert(t, "attachment", dbfx.Cols{
		"workspace_id": e.ws, "issue_id": issue, "uploader_type": "member", "uploader_id": e.users["owner"],
		"filename": "f", "url": "u", "content_type": "text/plain", "size_bytes": 1,
	})
	before := e.snapshot(project)
	path := "/api/projects/" + project + "/access"

	dbfx.Call(t, router.ServeHTTP, e.as("owner", http.MethodPut, path, map[string]any{"access": "restricted"})).Want(http.StatusOK)
	if got := e.accessOf(project); got != "restricted" {
		t.Fatalf("access=%q after switch to restricted", got)
	}
	if e.auditCount("project.access_changed") != 1 {
		t.Fatalf("audit rows after first switch: %d, want 1", e.auditCount("project.access_changed"))
	}
	if e.snapshot(project) != before {
		t.Error("issue, comment or attachment rows changed after switch to restricted")
	}
	granted := e.members(project)
	if granted == 0 {
		t.Fatal("the creator and assignees should have been granted")
	}

	dbfx.Call(t, router.ServeHTTP, e.as("admin", http.MethodPut, path, map[string]any{"access": "workspace"})).Want(http.StatusOK)
	if got := e.accessOf(project); got != "workspace" {
		t.Fatalf("access=%q after switch back", got)
	}
	if e.auditCount("project.access_changed") != 2 {
		t.Errorf("audit rows after second switch: %d, want 2", e.auditCount("project.access_changed"))
	}
	if e.snapshot(project) != before {
		t.Error("issue, comment or attachment rows changed after switch to workspace")
	}
	if got := e.members(project); got != granted {
		t.Errorf("project_member rows %d after switch back, want %d kept", got, granted)
	}
}

func TestSwitchToRestrictedAddsAssignees(t *testing.T) {
	e := newEnv(t)
	_, router := e.handler()
	creator, lead, assignee, extra, idle := e.users["owner"], e.users["admin"], e.users["member"], e.users["viewer"], e.users["guest"]
	agentID := uuid.NewString()
	project := e.project("p", "workspace", dbfx.Cols{
		"creator_type": "member", "creator_id": creator,
		"lead_type": "member", "lead_id": lead,
	})
	e.f.Issue(t, "assigned", dbfx.Cols{"project_id": project, "assignee_type": "member", "assignee_id": assignee})
	e.f.Issue(t, "to an agent", dbfx.Cols{"project_id": project, "assignee_type": "agent", "assignee_id": agentID})
	// An assignee of another project must not be granted.
	other := e.project("other", "workspace")
	e.f.Issue(t, "elsewhere", dbfx.Cols{"project_id": other, "assignee_type": "member", "assignee_id": idle})

	dbfx.Call(t, router.ServeHTTP, e.as("owner", http.MethodPut, "/api/projects/"+project+"/access",
		map[string]any{"access": "restricted", "member_ids": []string{extra}})).Want(http.StatusOK)

	rows, err := e.pool.Query(t.Context(), `SELECT user_id::text FROM project_member WHERE project_id = $1`, project)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			t.Fatal(err)
		}
		got[u] = true
	}
	want := map[string]bool{creator: true, lead: true, assignee: true, extra: true}
	if len(got) != len(want) {
		t.Errorf("granted %v, want %v", got, want)
	}
	for u := range want {
		if !got[u] {
			t.Errorf("user %s was not granted", u)
		}
	}
	if got[idle] || got[agentID] {
		t.Errorf("granted someone who is not on this project: %v", got)
	}
}

func TestSwitchAccessConcurrentNoMixing(t *testing.T) {
	e := newEnv(t)
	_, router := e.handler()
	m1 := e.users["member"]
	for round := 0; round < 8; round++ {
		project := e.project("p", "workspace")
		path := "/api/projects/" + project + "/access"
		var wg sync.WaitGroup
		codes := make([]int, 2)
		bodies := []map[string]any{
			{"access": "restricted", "member_ids": []string{m1}},
			{"access": "workspace"},
		}
		for i := range bodies {
			wg.Add(1)
			go func() {
				defer wg.Done()
				codes[i] = dbfx.Call(t, router.ServeHTTP, e.as("owner", http.MethodPut, path, bodies[i])).Code
			}()
		}
		wg.Wait()
		if codes[0] != http.StatusOK || codes[1] != http.StatusOK {
			t.Fatalf("round %d: statuses %v, want both 200", round, codes)
		}

		type change struct{ From, To string }
		var changes []change
		rows, err := e.pool.Query(t.Context(), `SELECT detail->>'from', detail->>'to' FROM audit_event WHERE workspace_id = $1 AND action = 'project.access_changed' AND target_id = $2`, e.ws, project)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var c change
			if err := rows.Scan(&c.From, &c.To); err != nil {
				t.Fatal(err)
			}
			changes = append(changes, c)
		}
		rows.Close()
		if len(changes) != 2 {
			t.Fatalf("round %d: %d audit rows, want 2", round, len(changes))
		}
		final := e.accessOf(project)
		chained := func(a, b change) bool { return a.From == "workspace" && a.To == b.From && b.To == final }
		if !chained(changes[0], changes[1]) && !chained(changes[1], changes[0]) {
			t.Errorf("round %d: audit rows %v do not form one chain ending at %q", round, changes, final)
		}
		var granted bool
		e.f.QueryRow(t, `SELECT EXISTS (SELECT 1 FROM project_member WHERE project_id = $1 AND user_id = $2)`, project, m1).Scan(&granted)
		if !granted {
			t.Errorf("round %d: the restricted switch's member_ids were lost", round)
		}
	}
}

func TestAuditFailureRollsBackAccessChange(t *testing.T) {
	e := newEnv(t)
	h, router := e.handler()
	h.Service.AuditWriter = func(pgx.Tx) audit.Writer { return failingAudit{} }
	project := e.project("p", "workspace", dbfx.Cols{"creator_type": "member", "creator_id": e.users["owner"]})
	base := "/api/projects/" + project

	dbfx.Call(t, router.ServeHTTP, e.as("owner", http.MethodPut, base+"/access",
		map[string]any{"access": "restricted", "member_ids": []string{e.users["member"]}})).Want(http.StatusInternalServerError)
	if got := e.accessOf(project); got != "workspace" {
		t.Errorf("access=%q after failed audit, want workspace", got)
	}
	if n := e.members(project); n != 0 {
		t.Errorf("%d project_member rows survived a failed audit", n)
	}

	dbfx.Call(t, router.ServeHTTP, e.as("owner", http.MethodPost, base+"/members",
		map[string]any{"user_ids": []string{e.users["member"]}})).Want(http.StatusInternalServerError)
	if n := e.members(project); n != 0 {
		t.Errorf("%d project_member rows survived a failed audit on grant", n)
	}
}

func TestProjectAccessRoutes(t *testing.T) {
	e := newEnv(t)
	_, router := e.handler()
	open := e.project("open", "workspace")
	hidden := e.project("hidden", "restricted")
	grantedGuest := e.project("guest-project", "restricted")
	e.grant(grantedGuest, e.users["guest"])
	missing := uuid.NewString()
	call := func(kind, method, project, suffix string, body any) *dbfx.Response {
		return dbfx.Call(t, router.ServeHTTP, e.as(kind, method, "/api/projects/"+project+suffix, body))
	}

	// Owner and admin manage access.
	call("owner", http.MethodPut, open, "/access", map[string]any{"access": "restricted"}).Want(http.StatusOK)
	call("admin", http.MethodPut, open, "/access", map[string]any{"access": "workspace"}).Want(http.StatusOK)
	call("owner", http.MethodPut, open, "/access", map[string]any{"access": "bogus"}).Want(http.StatusBadRequest)

	// Someone who sees the project but does not manage it gets 403.
	call("member", http.MethodPut, open, "/access", map[string]any{"access": "restricted"}).Want(http.StatusForbidden)
	call("viewer", http.MethodPost, open, "/members", map[string]any{"user_ids": []string{e.users["member"]}}).Want(http.StatusForbidden)
	call("guest", http.MethodPut, grantedGuest, "/access", map[string]any{"access": "workspace"}).Want(http.StatusForbidden)
	call("guest", http.MethodDelete, grantedGuest, "/members/"+e.users["guest"], nil).Want(http.StatusForbidden)
	if e.accessOf(open) != "workspace" || e.accessOf(grantedGuest) != "restricted" {
		t.Error("a forbidden call changed access")
	}

	// A project the caller cannot see answers exactly like a missing one.
	notFound := call("member", http.MethodGet, missing, "/members", nil).Want(http.StatusNotFound).Text()
	var body map[string]string
	if err := json.Unmarshal([]byte(notFound), &body); err != nil || body["code"] != "project_not_found" {
		t.Fatalf("missing project body %q", notFound)
	}
	for _, c := range []struct {
		kind, method, suffix string
		body                 any
	}{
		{"member", http.MethodGet, "/members", nil},
		{"member", http.MethodPut, "/access", map[string]any{"access": "workspace"}},
		{"viewer", http.MethodPost, "/members", map[string]any{"user_ids": []string{}}},
		{"guest", http.MethodDelete, "/members/" + e.users["admin"], nil},
		{"guest", http.MethodGet, "/members", nil},
	} {
		if got := call(c.kind, c.method, hidden, c.suffix, c.body).Want(http.StatusNotFound).Text(); got != notFound {
			t.Errorf("%s %s on a hidden project: body %q differs from missing project body %q", c.method, c.suffix, got, notFound)
		}
	}

	// A user who is not a workspace member is rejected with the field.
	outsider := e.users["outsider"]
	for _, c := range []struct {
		method, suffix, field string
		body                  any
	}{
		{http.MethodPost, "/members", "user_ids", map[string]any{"user_ids": []string{outsider}}},
		{http.MethodPut, "/access", "member_ids", map[string]any{"access": "restricted", "member_ids": []string{outsider}}},
	} {
		var out map[string]string
		call("owner", c.method, open, c.suffix, c.body).Want(http.StatusBadRequest).JSON(&out)
		if out["code"] != "project_member_invalid" || out["field"] != c.field {
			t.Errorf("%s %s: got %v, want code project_member_invalid field %s", c.method, c.suffix, out, c.field)
		}
	}
	if e.accessOf(open) != "workspace" || e.members(open) != 0 {
		t.Error("a rejected request changed data")
	}

	// Granting is idempotent and audited once.
	grant := map[string]any{"user_ids": []string{e.users["member"], e.users["viewer"]}}
	call("owner", http.MethodPost, hidden, "/members", grant).Want(http.StatusOK)
	call("admin", http.MethodPost, hidden, "/members", grant).Want(http.StatusOK)
	if n := e.members(hidden); n != 2 {
		t.Errorf("%d members after a repeated grant, want 2", n)
	}
	if n := e.auditCount("project.member_added"); n != 2 {
		t.Errorf("%d member_added audit rows, want 2", n)
	}

	var list struct {
		Members []struct {
			UserID string `json:"user_id"`
		} `json:"members"`
	}
	call("member", http.MethodGet, hidden, "/members", nil).Want(http.StatusOK).JSON(&list)
	if len(list.Members) != 2 {
		t.Errorf("listed %d members, want 2", len(list.Members))
	}

	call("member", http.MethodDelete, hidden, "/members/"+e.users["viewer"], nil).Want(http.StatusForbidden)
	call("admin", http.MethodDelete, hidden, "/members/"+e.users["viewer"], nil).Want(http.StatusNoContent)
	if n := e.members(hidden); n != 1 {
		t.Errorf("%d members after revoke, want 1", n)
	}
	if n := e.auditCount("project.member_removed"); n != 1 {
		t.Errorf("%d member_removed audit rows, want 1", n)
	}
	call("admin", http.MethodDelete, hidden, "/members/"+e.users["viewer"], nil).Want(http.StatusNoContent)
	if n := e.auditCount("project.member_removed"); n != 1 {
		t.Errorf("repeated revoke wrote another audit row: %d", n)
	}
}
