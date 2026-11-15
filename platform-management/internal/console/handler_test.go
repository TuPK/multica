package console

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/accountcache"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("response is not a JSON object: %v\n%s", err, rec.Body.String())
	}
	return m
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body %s", rec.Code, status, rec.Body.String())
	}
}

func wantCode(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	wantStatus(t, rec, status)
	if got := decode(t, rec)["code"]; got != code {
		t.Fatalf("code = %v, want %s; body %s", got, code, rec.Body.String())
	}
}

// unknownRoute is what the middleware and a wrong method both answer.
func wantUnknownRoute(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	wantStatus(t, rec, http.StatusNotFound)
	if body := rec.Body.String(); body != "404 page not found\n" {
		t.Fatalf("404 body = %q, want the body of an unknown route", body)
	}
}

// --- role matrix ---

type matrixRoute struct {
	method  string
	path    string
	body    string
	allowed map[string]bool // staff user id -> may use the route
	status  int             // the status an allowed caller gets
}

func roleSet(ids ...string) map[string]bool {
	m := map[string]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func matrix() []matrixRoute {
	all := roleSet(adminID, suppID, billID)
	supportAdmin := roleSet(adminID, suppID)
	admin := roleSet(adminID)
	return []matrixRoute{
		{"GET", "/api/platform/tenants", "", all, 200},
		{"GET", "/api/platform/tenants/" + wsID, "", all, 200},
		{"POST", "/api/platform/tenants/" + wsID + "/status", `{"status":"read_only","reason":"r"}`, supportAdmin, 200},
		{"GET", "/api/platform/tenants/" + wsID + "/audit-events", "", all, 200},
		{"GET", "/api/platform/tenants/" + wsID + "/notes", "", all, 200},
		{"POST", "/api/platform/tenants/" + wsID + "/notes", `{"body":"hello"}`, all, 201},
		{"GET", "/api/platform/users", "", all, 200},
		{"GET", "/api/platform/users/" + custID, "", all, 200},
		{"POST", "/api/platform/users/" + custID + "/status", `{"status":"suspended","reason":"r"}`, supportAdmin, 200},
		{"POST", "/api/platform/users/" + custID + "/revoke-sessions", `{"reason":"r"}`, supportAdmin, 200},
		{"GET", "/api/platform/staff", "", admin, 200},
		{"POST", "/api/platform/staff", `{"email":"` + custMail + `","role":"support"}`, admin, 201},
		{"DELETE", "/api/platform/staff/" + suppID, "", admin, 200},
		{"GET", "/api/platform/audit-events", "", all, 200},
		{"GET", "/api/platform/overview", "", all, 200},
	}
}

// Every route answers a role it admits, and answers 404 to the others, to a
// caller with no session and to a user who is not staff. A staff-only route is
// not found by support: it is a 404, not a 403.
func TestRoleMatrix(t *testing.T) {
	users := []string{adminID, suppID, billID}
	for _, rt := range matrix() {
		for _, user := range users {
			// A fresh environment: the writes change what the next call sees.
			e := newEnv(t)
			rec := e.do(user, rt.method, rt.path, rt.body)
			if rt.allowed[user] {
				if rec.Code != rt.status {
					t.Errorf("%s %s as %s = %d, want %d; %s", rt.method, rt.path, e.dir.emails[user], rec.Code, rt.status, rec.Body.String())
				}
				continue
			}
			if rec.Code != http.StatusNotFound || rec.Body.String() != "404 page not found\n" {
				t.Errorf("%s %s as %s = %d %q, want the 404 of an unknown route", rt.method, rt.path, e.dir.emails[user], rec.Code, rec.Body.String())
			}
		}
		e := newEnv(t)
		wantUnknownRoute(t, e.do("", rt.method, rt.path, rt.body))
		wantUnknownRoute(t, e.do(custID, rt.method, rt.path, rt.body)) // a customer, not staff
	}
}

// billing cannot change a workspace state or an account: the middleware answers
// 404, and nothing is written.
func TestBillingCannotWrite(t *testing.T) {
	e := newEnv(t)
	wantUnknownRoute(t, e.do(billID, "POST", "/api/platform/tenants/"+wsID+"/status", `{"status":"suspended","reason":"r"}`))
	wantUnknownRoute(t, e.do(billID, "POST", "/api/platform/users/"+custID+"/status", `{"status":"suspended","reason":"r"}`))
	wantUnknownRoute(t, e.do(billID, "POST", "/api/platform/users/"+custID+"/revoke-sessions", `{"reason":"r"}`))
	if len(e.ws.calls) != 0 || e.store.txs != 0 || len(e.pub.sent) != 0 {
		t.Fatalf("a refused write reached the store: %d %d %d", len(e.ws.calls), e.store.txs, len(e.pub.sent))
	}
}

// support cannot reach /staff.
func TestSupportCannotReachStaff(t *testing.T) {
	e := newEnv(t)
	wantUnknownRoute(t, e.do(suppID, "GET", "/api/platform/staff", ""))
	wantUnknownRoute(t, e.do(suppID, "POST", "/api/platform/staff", `{"email":"`+custMail+`","role":"admin"}`))
	wantUnknownRoute(t, e.do(suppID, "DELETE", "/api/platform/staff/"+adminID, ""))
	if e.store.txs != 0 {
		t.Fatal("a refused request opened a transaction")
	}
}

// A wrong method is a 404, not a 405, for every route, and also for a staff
// member who may use the path with another method.
func TestWrongMethodIsNotFound(t *testing.T) {
	e := newEnv(t)
	for _, c := range []struct{ method, path string }{
		{"PUT", "/api/platform/tenants"},
		{"POST", "/api/platform/tenants"},
		{"DELETE", "/api/platform/tenants/" + wsID},
		{"GET", "/api/platform/tenants/" + wsID + "/status"},
		{"PUT", "/api/platform/tenants/" + wsID + "/notes"},
		{"GET", "/api/platform/users/" + custID + "/status"},
		{"GET", "/api/platform/users/" + custID + "/revoke-sessions"},
		{"PUT", "/api/platform/staff"},
		{"GET", "/api/platform/staff/" + suppID},
		{"POST", "/api/platform/audit-events"},
		{"POST", "/api/platform/overview"},
		{"HEAD", "/api/platform/overview"},
	} {
		wantUnknownRoute(t, e.do(adminID, c.method, c.path, ""))
	}
}

// A path that is not a UUID names nothing.
func TestMalformedPathIDIsNotFound(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{
		"/api/platform/tenants/not-a-uuid",
		"/api/platform/users/1234",
		"/api/platform/tenants/not-a-uuid/notes",
		"/api/platform/tenants/not-a-uuid/audit-events",
	} {
		wantCode(t, e.do(adminID, "GET", p, ""), 404, "not_found")
	}
	wantCode(t, e.do(adminID, "DELETE", "/api/platform/staff/nope", ""), 404, "not_found")
	wantCode(t, e.do(adminID, "POST", "/api/platform/tenants/nope/status", `{"status":"active","reason":"r"}`), 404, "not_found")
}

// A state-changing request must carry the console's Origin.
func TestWritesRequireTheConsoleOrigin(t *testing.T) {
	e := newEnv(t)
	body := `{"status":"suspended","reason":"r"}`
	wantCode(t, e.doWith(adminID, "POST", "/api/platform/users/"+custID+"/status", body, "https://evil.example.com"), 403, "bad_origin")
	// A cookie with no Origin header at all is refused as well.
	wantCode(t, e.doWith(adminID, "POST", "/api/platform/users/"+custID+"/status", body, ""), 403, "bad_origin")
	if e.store.txs != 0 || len(e.pub.sent) != 0 {
		t.Fatal("a cross-origin write reached the store")
	}
	// Reads do not need it, and a bad origin does not reveal the route to a caller
	// the middleware refuses.
	wantStatus(t, e.do(adminID, "GET", "/api/platform/overview", ""), 200)
	wantUnknownRoute(t, e.doWith(billID, "POST", "/api/platform/users/"+custID+"/status", body, "https://evil.example.com"))
}

// --- audit rows ---

func onlyAudit(t *testing.T, e *env) audit.Entry {
	t.Helper()
	if len(e.store.audits) != 1 {
		t.Fatalf("audit rows = %d, want 1: %+v", len(e.store.audits), e.store.audits)
	}
	return e.store.audits[0]
}

func wantStaffActor(t *testing.T, a audit.Entry, id, email string) {
	t.Helper()
	if a.ActorType != "staff" || a.ActorID != id || a.ActorLabel != email {
		t.Fatalf("actor = %q %q %q, want staff %s %s", a.ActorType, a.ActorID, a.ActorLabel, id, email)
	}
}

func TestAccountStatusChangeWritesOneStaffAuditRow(t *testing.T) {
	e := newEnv(t)
	rec := e.do(suppID, "POST", "/api/platform/users/"+custID+"/status", `{"status":"suspended","reason":"  chargeback fraud  "}`)
	wantStatus(t, rec, 200)
	if m := decode(t, rec); m["changed"] != true || m["status"] != "suspended" {
		t.Fatalf("response = %v", m)
	}

	a := onlyAudit(t, e)
	wantStaffActor(t, a, suppID, suppMail)
	if a.Action != audit.ActionAccountStatusChanged || a.WorkspaceID != "" ||
		a.TargetType != "user" || a.TargetID != custID || a.TargetLabel != custMail {
		t.Fatalf("entry = %+v", a)
	}
	if a.Detail["from"] != "active" || a.Detail["to"] != "suspended" || a.Detail["reason"] != "chargeback fraud" {
		t.Fatalf("detail = %v", a.Detail)
	}
	if e.store.accounts[custID] != "suspended" {
		t.Fatalf("account = %q", e.store.accounts[custID])
	}
	if len(e.cache.dropped) != 1 || e.cache.dropped[0] != custID {
		t.Fatalf("cache drops = %v", e.cache.dropped)
	}
	if len(e.pub.sent) != 1 || e.pub.sent[0] != (published{custID, accountcache.KindStatus}) {
		t.Fatalf("published = %v", e.pub.sent)
	}
}

func TestAccountStatusUnchangedWritesNothing(t *testing.T) {
	e := newEnv(t)
	wantStatus(t, e.do(adminID, "POST", "/api/platform/users/"+custID+"/status", `{"status":"active","reason":"r"}`), 200)
	if len(e.store.audits) != 0 || len(e.pub.sent) != 0 || len(e.cache.dropped) != 0 {
		t.Fatalf("an unchanged status wrote %d audit rows, published %d, dropped %d", len(e.store.audits), len(e.pub.sent), len(e.cache.dropped))
	}

	// Re-activating a suspended account is a change.
	e.store.accounts[custID] = "suspended"
	wantStatus(t, e.do(adminID, "POST", "/api/platform/users/"+custID+"/status", `{"status":"active","reason":"appeal upheld"}`), 200)
	a := onlyAudit(t, e)
	if a.Detail["from"] != "suspended" || a.Detail["to"] != "active" {
		t.Fatalf("detail = %v", a.Detail)
	}
}

func TestAccountStatusRefusals(t *testing.T) {
	e := newEnv(t)
	path := "/api/platform/users/" + custID + "/status"

	wantCode(t, e.do(adminID, "POST", "/api/platform/users/"+adminID+"/status", `{"status":"suspended","reason":"r"}`), 400, "self_target")
	wantCode(t, e.do(adminID, "POST", "/api/platform/users/"+noOneID+"/status", `{"status":"suspended","reason":"r"}`), 404, "not_found")

	for _, locked := range []string{"pending_deletion", "deleted"} {
		e.store.accounts[custID] = locked
		wantCode(t, e.do(adminID, "POST", path, `{"status":"active","reason":"r"}`), 409, "account_locked")
		if e.store.accounts[custID] != locked {
			t.Fatalf("the %s account was changed", locked)
		}
	}
	if len(e.store.audits) != 0 || len(e.pub.sent) != 0 {
		t.Fatal("a refused change left an audit row or a notice")
	}
}

func TestRevokeSessionsWritesWholeSecondsAuditsAndPublishes(t *testing.T) {
	e := newEnv(t)
	rec := e.do(suppID, "POST", "/api/platform/users/"+custID+"/revoke-sessions", `{"reason":"lost laptop"}`)
	wantStatus(t, rec, 200)

	at := e.store.sessions[custID]
	if at.Nanosecond() != 0 || !at.Equal(t0.Truncate(time.Second)) {
		t.Fatalf("sessions_valid_after = %v, want %v", at, t0.Truncate(time.Second))
	}
	if got := decode(t, rec)["sessions_valid_after"]; got != "2030-05-06T07:08:09Z" {
		t.Fatalf("response mark = %v", got)
	}
	a := onlyAudit(t, e)
	wantStaffActor(t, a, suppID, suppMail)
	if a.Action != audit.ActionAccountSessionsRevoked || a.TargetID != custID || a.Detail["reason"] != "lost laptop" {
		t.Fatalf("entry = %+v", a)
	}
	if len(e.cache.dropped) != 1 || len(e.pub.sent) != 1 || e.pub.sent[0] != (published{custID, accountcache.KindSessions}) {
		t.Fatalf("cache %v, published %v", e.cache.dropped, e.pub.sent)
	}

	wantCode(t, e.do(adminID, "POST", "/api/platform/users/"+noOneID+"/revoke-sessions", `{"reason":"r"}`), 404, "not_found")
}

// Revoking sessions leaves the account's status alone.
func TestRevokeSessionsKeepsASuspendedStatus(t *testing.T) {
	e := newEnv(t)
	e.store.accounts[custID] = "suspended"
	wantStatus(t, e.do(adminID, "POST", "/api/platform/users/"+custID+"/revoke-sessions", `{"reason":"r"}`), 200)
	if e.store.accounts[custID] != "suspended" {
		t.Fatalf("status = %q", e.store.accounts[custID])
	}
}

// A failed audit write undoes the change and sends no notice.
func TestAuditFailureRollsTheWriteBack(t *testing.T) {
	boom := errors.New("audit insert failed: connection string postgres://pw")
	for name, call := range map[string]func(e *env) *httptest.ResponseRecorder{
		"status": func(e *env) *httptest.ResponseRecorder {
			return e.do(adminID, "POST", "/api/platform/users/"+custID+"/status", `{"status":"suspended","reason":"r"}`)
		},
		"revoke": func(e *env) *httptest.ResponseRecorder {
			return e.do(adminID, "POST", "/api/platform/users/"+custID+"/revoke-sessions", `{"reason":"r"}`)
		},
		"note": func(e *env) *httptest.ResponseRecorder {
			return e.do(adminID, "POST", "/api/platform/tenants/"+wsID+"/notes", `{"body":"n"}`)
		},
		"add staff": func(e *env) *httptest.ResponseRecorder {
			return e.do(adminID, "POST", "/api/platform/staff", `{"email":"`+custMail+`","role":"support"}`)
		},
		"remove staff": func(e *env) *httptest.ResponseRecorder {
			return e.do(adminID, "DELETE", "/api/platform/staff/"+suppID, "")
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			e.store.failAudit = boom
			rec := call(e)
			wantCode(t, rec, 500, "internal")
			if strings.Contains(rec.Body.String(), "postgres") {
				t.Fatalf("the response leaks the cause: %s", rec.Body.String())
			}
			if !strings.Contains(e.logs.String(), "postgres://pw") {
				t.Fatal("the cause is not in the log")
			}
			if len(e.store.accounts) != 0 || len(e.store.sessions) != 0 || len(e.store.notes) != 0 ||
				len(e.store.staffRows) != 3 || len(e.pub.sent) != 0 || len(e.cache.dropped) != 0 {
				t.Fatalf("the failed write left traces: %+v", e.store)
			}
			if _, ok := e.store.activeRole(suppID); !ok {
				t.Fatal("support was removed by a failed removal")
			}
		})
	}
}

// The change is committed before the notice, so a Redis failure is logged and
// the request succeeds.
func TestNoticeFailuresDoNotFailTheRequest(t *testing.T) {
	e := newEnv(t)
	e.cache.err = errors.New("redis del failed")
	e.pub.err = errors.New("redis publish failed")
	wantStatus(t, e.do(adminID, "POST", "/api/platform/users/"+custID+"/status", `{"status":"suspended","reason":"r"}`), 200)
	if e.store.accounts[custID] != "suspended" {
		t.Fatal("the change was not kept")
	}
	if !strings.Contains(e.logs.String(), "redis publish failed") || !strings.Contains(e.logs.String(), "redis del failed") {
		t.Fatalf("the failures are not logged: %s", e.logs.String())
	}
}

func TestTenantStatusCallsWorkspaceStatusSet(t *testing.T) {
	e := newEnv(t)
	e.reader.details[wsID] = TenantDetail{
		TenantRow:      TenantRow{ID: wsID, Name: wsName, Slug: "acme", Status: "read_only", CreatedAt: t0, LastActivityAt: t0},
		StateReason:    "unpaid invoice",
		StateChangedBy: suppID,
		StateChangedAt: &t0,
	}
	rec := e.do(suppID, "POST", "/api/platform/tenants/"+wsID+"/status", `{"status":"read_only","reason":" unpaid invoice "}`)
	wantStatus(t, rec, 200)

	if len(e.ws.calls) != 1 {
		t.Fatalf("Set calls = %d", len(e.ws.calls))
	}
	c := e.ws.calls[0]
	if c.WorkspaceID != wsID || c.Status != workspacestate.ReadOnly || c.Reason != "unpaid invoice" || c.ChangedBy != suppID || c.ActorLabel != suppMail || !c.DeletionScheduledAt.IsZero() {
		t.Fatalf("change = %+v", c)
	}
	m := decode(t, rec)
	state, _ := m["state"].(map[string]any)
	if m["changed"] != true || state["status"] != "read_only" || state["reason"] != "unpaid invoice" || state["changed_by"] != suppID {
		t.Fatalf("response = %v", m)
	}
	// Set writes the workspace's audit row; the console writes none of its own.
	if e.store.txs != 0 {
		t.Fatal("the console opened its own transaction for a workspace state change")
	}
}

func TestTenantStatusRefusals(t *testing.T) {
	e := newEnv(t)
	path := "/api/platform/tenants/" + wsID + "/status"
	wantCode(t, e.do(adminID, "POST", path, `{"status":"suspended"}`), 400, "reason_required")
	wantCode(t, e.do(adminID, "POST", path, `{"status":"suspended","reason":"  "}`), 400, "reason_required")
	wantCode(t, e.do(adminID, "POST", path, `{"status":"deleted","reason":"r"}`), 400, "invalid_status")
	wantCode(t, e.do(adminID, "POST", path, `{"reason":"r"}`), 400, "invalid_status")
	wantCode(t, e.do(adminID, "POST", path, `{"status":"suspended","reason":"r","extra":1}`), 400, "invalid_body")
	wantCode(t, e.do(adminID, "POST", path, `not json`), 400, "invalid_body")
	wantCode(t, e.do(adminID, "POST", path, `{"status":"suspended","reason":"r"} {}`), 400, "invalid_body")
	wantCode(t, e.do(adminID, "POST", "/api/platform/tenants/"+noOneID+"/status", `{"status":"suspended","reason":"r"}`), 404, "not_found")
	if len(e.ws.calls) != 0 {
		t.Fatalf("a refused request called Set %d times", len(e.ws.calls))
	}

	e.ws.err = errors.New("db down")
	wantCode(t, e.do(adminID, "POST", path, `{"status":"suspended","reason":"r"}`), 500, "internal")
}

func TestAccountRefusalsChangeNothing(t *testing.T) {
	e := newEnv(t)
	for _, c := range []struct{ path, body, code string }{
		{"status", `{"status":"suspended"}`, "reason_required"},
		{"status", `{"status":"suspended","reason":""}`, "reason_required"},
		{"status", `{"status":"deleted","reason":"r"}`, "invalid_status"},
		{"status", `{"status":"pending_deletion","reason":"r"}`, "invalid_status"},
		{"status", `{"reason":"r"}`, "invalid_status"},
		{"revoke-sessions", `{}`, "reason_required"},
		{"revoke-sessions", `{"reason":" \n"}`, "reason_required"},
	} {
		wantCode(t, e.do(adminID, "POST", "/api/platform/users/"+custID+"/"+c.path, c.body), 400, c.code)
	}
	if e.store.txs != 0 || len(e.pub.sent) != 0 || len(e.cache.dropped) != 0 {
		t.Fatal("a refused request changed something")
	}
}

// --- notes ---

func TestAddNoteAuditsTheNoteWithoutItsText(t *testing.T) {
	e := newEnv(t)
	rec := e.do(suppID, "POST", "/api/platform/tenants/"+wsID+"/notes", `{"body":"  Owner called; wants a refund.  "}`)
	wantStatus(t, rec, 201)
	m := decode(t, rec)
	if m["body"] != "Owner called; wants a refund." || m["author_id"] != suppID || m["author_email"] != suppMail || m["workspace_id"] != wsID {
		t.Fatalf("note = %v", m)
	}

	a := onlyAudit(t, e)
	wantStaffActor(t, a, suppID, suppMail)
	if a.Action != audit.ActionNoteAdded || a.WorkspaceID != wsID || a.TargetType != "workspace" || a.TargetID != wsID || a.TargetLabel != wsName {
		t.Fatalf("entry = %+v", a)
	}
	// The workspace's owners read this row in the workspace audit log.
	raw, _ := json.Marshal(a)
	if strings.Contains(string(raw), "refund") {
		t.Fatalf("the audit row carries the note's text: %s", raw)
	}
	if len(e.store.notes) != 1 || e.store.notes[0].Body != "Owner called; wants a refund." {
		t.Fatalf("notes = %+v", e.store.notes)
	}
}

func TestAddNoteRefusals(t *testing.T) {
	e := newEnv(t)
	path := "/api/platform/tenants/" + wsID + "/notes"
	wantCode(t, e.do(adminID, "POST", path, `{"body":""}`), 400, "body_required")
	wantCode(t, e.do(adminID, "POST", path, `{"body":"   \n"}`), 400, "body_required")
	wantCode(t, e.do(adminID, "POST", path, `{}`), 400, "body_required")
	wantCode(t, e.do(adminID, "POST", path, `{"body":"`+strings.Repeat("x", 4001)+`"}`), 400, "body_too_long")
	wantCode(t, e.do(adminID, "POST", "/api/platform/tenants/"+noOneID+"/notes", `{"body":"x"}`), 404, "not_found")
	if len(e.store.notes) != 0 || len(e.store.audits) != 0 {
		t.Fatal("a refused note left a trace")
	}
	wantStatus(t, e.do(adminID, "POST", path, `{"body":"`+strings.Repeat("x", 4000)+`"}`), 201)
}

// --- staff ---

func TestAddStaff(t *testing.T) {
	e := newEnv(t)
	rec := e.do(adminID, "POST", "/api/platform/staff", `{"email":"  Customer@Example.COM ","role":"billing"}`)
	wantStatus(t, rec, 201)
	if m := decode(t, rec); m["user_id"] != custID || m["email"] != custMail || m["role"] != "billing" {
		t.Fatalf("response = %v", m)
	}
	if r, ok := e.store.activeRole(custID); !ok || r != "billing" {
		t.Fatalf("role = %q %v", r, ok)
	}
	a := onlyAudit(t, e)
	wantStaffActor(t, a, adminID, adminMail)
	if a.Action != audit.ActionStaffAdded || a.TargetID != custID || a.TargetLabel != custMail || a.WorkspaceID != "" ||
		a.Detail["role"] != "billing" || a.Detail["reactivated"] != false {
		t.Fatalf("entry = %+v", a)
	}
}

func TestAddStaffReactivatesARevokedRow(t *testing.T) {
	e := newEnv(t)
	e.store.staffRows = append(e.store.staffRows, fakeStaffRow{user: custID, role: "support", revoked: true})
	wantStatus(t, e.do(adminID, "POST", "/api/platform/staff", `{"email":"`+custMail+`","role":"admin"}`), 201)
	if n := len(e.store.staffRows); n != 4 {
		t.Fatalf("staff rows = %d, want the revoked row reused", n)
	}
	if r, ok := e.store.activeRole(custID); !ok || r != "admin" {
		t.Fatalf("role = %q %v", r, ok)
	}
	if a := onlyAudit(t, e); a.Detail["reactivated"] != true {
		t.Fatalf("detail = %v", a.Detail)
	}
}

func TestAddStaffRefusals(t *testing.T) {
	e := newEnv(t)
	wantCode(t, e.do(adminID, "POST", "/api/platform/staff", `{"email":"nobody@example.com","role":"support"}`), 404, "no_such_account")
	wantCode(t, e.do(adminID, "POST", "/api/platform/staff", `{"email":"`+suppMail+`","role":"admin"}`), 409, "already_staff")
	wantCode(t, e.do(adminID, "POST", "/api/platform/staff", `{"email":"x","role":"support"}`), 400, "invalid_email")
	wantCode(t, e.do(adminID, "POST", "/api/platform/staff", `{"email":"`+custMail+`","role":"root"}`), 400, "invalid_role")
	wantCode(t, e.do(adminID, "POST", "/api/platform/staff", `{"email":"`+custMail+`"}`), 400, "invalid_role")
	if len(e.store.audits) != 0 || len(e.store.staffRows) != 3 {
		t.Fatal("a refused request changed staff")
	}
}

func TestRemoveStaff(t *testing.T) {
	e := newEnv(t)
	rec := e.do(adminID, "DELETE", "/api/platform/staff/"+suppID, "")
	wantStatus(t, rec, 200)
	if _, ok := e.store.activeRole(suppID); ok {
		t.Fatal("support is still active")
	}
	a := onlyAudit(t, e)
	wantStaffActor(t, a, adminID, adminMail)
	if a.Action != audit.ActionStaffRemoved || a.TargetID != suppID || a.TargetLabel != suppMail || a.Detail["role"] != "support" {
		t.Fatalf("entry = %+v", a)
	}
	wantCode(t, e.do(adminID, "DELETE", "/api/platform/staff/"+suppID, ""), 404, "not_found")
	wantCode(t, e.do(adminID, "DELETE", "/api/platform/staff/"+custID, ""), 404, "not_found")
}

func TestLastAdminCannotBeRemoved(t *testing.T) {
	e := newEnv(t)
	// The only active admin, removing themselves.
	wantCode(t, e.do(adminID, "DELETE", "/api/platform/staff/"+adminID, ""), 400, "last_admin")
	if _, ok := e.store.activeRole(adminID); !ok || len(e.store.audits) != 0 {
		t.Fatal("the last admin was removed")
	}

	// With a second admin, one can go; then the other cannot.
	e.store.staffRows = append(e.store.staffRows, fakeStaffRow{user: custID, role: "admin"})
	wantStatus(t, e.do(adminID, "DELETE", "/api/platform/staff/"+custID, ""), 200)
	wantCode(t, e.do(adminID, "DELETE", "/api/platform/staff/"+adminID, ""), 400, "last_admin")

	// A revoked admin row does not count as an admin.
	e.store.staffRows = append(e.store.staffRows, fakeStaffRow{user: custID, role: "admin", revoked: true})
	wantCode(t, e.do(adminID, "DELETE", "/api/platform/staff/"+adminID, ""), 400, "last_admin")
}

// --- reads ---

func TestListTenantsPassesFiltersAndPages(t *testing.T) {
	e := newEnv(t)
	e.reader.tenants = []TenantRow{
		{ID: "bbbbbbbb-0000-0000-0000-000000000003", Name: "C", Slug: "c", Status: "active", MemberCount: 3, CreatedAt: t0.Add(2 * time.Hour), LastActivityAt: t0},
		{ID: "bbbbbbbb-0000-0000-0000-000000000002", Name: "B", Slug: "b", Status: "suspended", CreatedAt: t0.Add(time.Hour), LastActivityAt: t0},
		{ID: "bbbbbbbb-0000-0000-0000-000000000001", Name: "A", Slug: "a", Status: "active", CreatedAt: t0, LastActivityAt: t0},
	}
	rec := e.do(suppID, "GET", "/api/platform/tenants?q=50%25_off&limit=2", "")
	wantStatus(t, rec, 200)
	if e.reader.lastTenants.Limit != 2 || e.reader.lastTenants.Pattern == nil || *e.reader.lastTenants.Pattern != `%50\%\_off%` {
		t.Fatalf("search = %+v", e.reader.lastTenants)
	}
	m := decode(t, rec)
	rows, _ := m["tenants"].([]any)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want the page of 2", len(rows))
	}
	first, _ := rows[0].(map[string]any)
	if first["name"] != "C" || first["member_count"] != float64(3) || first["status"] != "active" || first["last_activity_at"] == nil {
		t.Fatalf("row = %v", first)
	}
	next, _ := m["next_cursor"].(string)
	cur, err := decodeCursor(next)
	if err != nil || cur.ID != "bbbbbbbb-0000-0000-0000-000000000002" || !cur.CreatedAt.Equal(t0.Add(time.Hour)) {
		t.Fatalf("next cursor = %q %+v %v", next, cur, err)
	}

	// The cursor goes back to the reader; a page that fits has no next cursor.
	e.reader.tenants = e.reader.tenants[2:]
	rec = e.do(adminID, "GET", "/api/platform/tenants?limit=2&cursor="+next, "")
	wantStatus(t, rec, 200)
	if e.reader.lastTenants.Cursor == nil || e.reader.lastTenants.Cursor.ID != cur.ID {
		t.Fatalf("cursor was not passed: %+v", e.reader.lastTenants)
	}
	if decode(t, rec)["next_cursor"] != nil {
		t.Fatal("the last page has a next cursor")
	}

	wantCode(t, e.do(adminID, "GET", "/api/platform/tenants?limit=0", ""), 400, "invalid_filter")
	wantCode(t, e.do(adminID, "GET", "/api/platform/tenants?cursor=zzz", ""), 400, "invalid_cursor")
}

func TestGetTenantReturnsStateNotesCountAndOwner(t *testing.T) {
	e := newEnv(t)
	sched := t0.Add(30 * 24 * time.Hour)
	e.reader.details[wsID] = TenantDetail{
		TenantRow:           TenantRow{ID: wsID, Name: wsName, Slug: "acme", Status: "pending_deletion", MemberCount: 4, CreatedAt: t0, LastActivityAt: t0.Add(time.Hour)},
		StateReason:         "owner request",
		StateChangedBy:      adminID,
		StateChangedAt:      &t0,
		DeletionScheduledAt: &sched,
		NotesCount:          2,
		OwnerID:             custID,
		OwnerEmail:          custMail,
	}
	m := decode(t, e.do(billID, "GET", "/api/platform/tenants/"+wsID, ""))
	state, _ := m["state"].(map[string]any)
	owner, _ := m["owner"].(map[string]any)
	if m["id"] != wsID || m["status"] != "pending_deletion" || m["member_count"] != float64(4) || m["notes_count"] != float64(2) ||
		state["reason"] != "owner request" || state["changed_by"] != adminID || state["deletion_scheduled_at"] == nil ||
		owner["id"] != custID || owner["email"] != custMail {
		t.Fatalf("tenant = %v", m)
	}
	if _, has := m["subscription"]; has {
		t.Fatal("the subscription belongs to a later slice")
	}

	// No platform row and no owner: the state record is empty and the owner null.
	e.reader.details[wsID] = TenantDetail{TenantRow: TenantRow{ID: wsID, Name: wsName, Status: "active", CreatedAt: t0, LastActivityAt: t0}}
	m = decode(t, e.do(billID, "GET", "/api/platform/tenants/"+wsID, ""))
	state, _ = m["state"].(map[string]any)
	if m["owner"] != nil || state["status"] != "active" || state["changed_at"] != nil || state["changed_by"] != nil {
		t.Fatalf("tenant = %v", m)
	}

	wantCode(t, e.do(adminID, "GET", "/api/platform/tenants/"+noOneID, ""), 404, "not_found")
}

func TestUsersListAndDetail(t *testing.T) {
	e := newEnv(t)
	e.reader.users = []UserRow{{ID: custID, Email: custMail, Name: "Cu", Status: "suspended", MembershipsCount: 2, CreatedAt: t0}}
	rec := e.do(adminID, "GET", "/api/platform/users?q=cust", "")
	wantStatus(t, rec, 200)
	if e.reader.lastUsers.Pattern == nil || *e.reader.lastUsers.Pattern != "%cust%" {
		t.Fatalf("search = %+v", e.reader.lastUsers)
	}
	rows, _ := decode(t, rec)["users"].([]any)
	row, _ := rows[0].(map[string]any)
	if len(rows) != 1 || row["email"] != custMail || row["status"] != "suspended" || row["memberships_count"] != float64(2) {
		t.Fatalf("users = %v", rows)
	}

	valid := t0.Add(-time.Hour)
	e.reader.userDets[custID] = UserDetail{
		UserRow:            UserRow{ID: custID, Email: custMail, Name: "Cu", Status: "suspended", MembershipsCount: 1, CreatedAt: t0},
		Memberships:        []Membership{{WorkspaceID: wsID, Name: wsName, Role: "owner"}},
		AccountReason:      "fraud",
		AccountChangedAt:   &t0,
		SessionsValidAfter: &valid,
	}
	m := decode(t, e.do(adminID, "GET", "/api/platform/users/"+custID, ""))
	ms, _ := m["memberships"].([]any)
	m0, _ := ms[0].(map[string]any)
	acct, _ := m["account"].(map[string]any)
	if len(ms) != 1 || m0["workspace_id"] != wsID || m0["name"] != wsName || m0["role"] != "owner" ||
		acct["status"] != "suspended" || acct["reason"] != "fraud" || acct["sessions_valid_after"] == nil || acct["changed_at"] == nil {
		t.Fatalf("user = %v", m)
	}
	wantCode(t, e.do(adminID, "GET", "/api/platform/users/"+noOneID, ""), 404, "not_found")
}

func TestListNotes(t *testing.T) {
	e := newEnv(t)
	e.reader.notes = []Note{{ID: "eeeeeeee-0000-0000-0000-000000000001", WorkspaceID: wsID, AuthorID: suppID, AuthorEmail: suppMail, Body: "hi", CreatedAt: t0}}
	rec := e.do(billID, "GET", "/api/platform/tenants/"+wsID+"/notes", "")
	wantStatus(t, rec, 200)
	notes, _ := decode(t, rec)["notes"].([]any)
	n, _ := notes[0].(map[string]any)
	if e.reader.lastNotes.WorkspaceID != wsID || len(notes) != 1 || n["body"] != "hi" || n["author_email"] != suppMail {
		t.Fatalf("notes = %v, query %+v", notes, e.reader.lastNotes)
	}
}

func TestListStaff(t *testing.T) {
	e := newEnv(t)
	e.reader.staff = []StaffRow{
		{UserID: adminID, Email: adminMail, Role: "admin", CreatedAt: t0},
		{UserID: suppID, Email: suppMail, Role: "support", CreatedBy: adminID, CreatedAt: t0},
	}
	rows, _ := decode(t, e.do(adminID, "GET", "/api/platform/staff", ""))["staff"].([]any)
	first, _ := rows[0].(map[string]any)
	second, _ := rows[1].(map[string]any)
	if len(rows) != 2 || first["created_by"] != nil || second["created_by"] != adminID || second["role"] != "support" || second["email"] != suppMail {
		t.Fatalf("staff = %v", rows)
	}
}

func TestAuditEventsFiltersAndShape(t *testing.T) {
	e := newEnv(t)
	e.reader.events = []AuditRow{
		{ID: "ffffffff-0000-0000-0000-000000000002", WorkspaceID: wsID, ActorType: "staff", ActorID: adminID, ActorLabel: adminMail, Action: "staff.added", Detail: []byte(`{"role":"admin"}`), CreatedAt: t0.Add(time.Minute)},
		{ID: "ffffffff-0000-0000-0000-000000000001", ActorType: "member", Action: "auth.login", CreatedAt: t0},
	}
	rec := e.do(suppID, "GET", "/api/platform/audit-events?limit=1&action=staff.added&actor_id="+adminID+"&from=2030-01-01T00:00:00Z&to=2031-01-01T00:00:00Z", "")
	wantStatus(t, rec, 200)
	q := e.reader.lastAudit
	if q.WorkspaceID != "" || q.Limit != 1 || q.Action == nil || *q.Action != "staff.added" || q.ActorID == nil || *q.ActorID != adminID ||
		q.From == nil || q.To == nil {
		t.Fatalf("query = %+v", q)
	}
	m := decode(t, rec)
	events, _ := m["events"].([]any)
	ev, _ := events[0].(map[string]any)
	detail, _ := ev["detail"].(map[string]any)
	if len(events) != 1 || m["next_cursor"] == nil || ev["actor_type"] != "staff" || ev["workspace_id"] != wsID || ev["actor_label"] != adminMail || detail["role"] != "admin" {
		t.Fatalf("events = %v", m)
	}

	// An account-level row has a null workspace and an empty detail object.
	e.reader.events = e.reader.events[1:]
	m = decode(t, e.do(suppID, "GET", "/api/platform/audit-events", ""))
	events, _ = m["events"].([]any)
	ev, _ = events[0].(map[string]any)
	if ev["workspace_id"] != nil || ev["actor_id"] != nil || m["next_cursor"] != nil {
		t.Fatalf("event = %v", ev)
	}

	wantCode(t, e.do(suppID, "GET", "/api/platform/audit-events?action=nope", ""), 400, "invalid_filter")
	wantCode(t, e.do(suppID, "GET", "/api/platform/audit-events?from=yesterday", ""), 400, "invalid_filter")
}

func TestTenantAuditEventsAreScopedToTheWorkspace(t *testing.T) {
	e := newEnv(t)
	wantStatus(t, e.do(billID, "GET", "/api/platform/tenants/"+wsID+"/audit-events?action=workspace.status_changed", ""), 200)
	if e.reader.lastAudit.WorkspaceID != wsID || e.reader.lastAudit.Action == nil || *e.reader.lastAudit.Action != "workspace.status_changed" {
		t.Fatalf("query = %+v", e.reader.lastAudit)
	}
	// The rows of a workspace that no longer exists are still readable.
	wantStatus(t, e.do(billID, "GET", "/api/platform/tenants/"+noOneID+"/audit-events", ""), 200)
	if e.reader.lastAudit.WorkspaceID != noOneID {
		t.Fatalf("query = %+v", e.reader.lastAudit)
	}
}

func TestOverviewComputesActiveAsTheRest(t *testing.T) {
	e := newEnv(t)
	e.reader.overview = Overview{
		WorkspacesTotal: 100,
		NonActive:       map[string]int64{"read_only": 3, "suspended": 5, "pending_deletion": 2},
		Signups:         17,
		ByPlan:          map[string]int64{"free": 70, "pro": 25, "business": 5},
	}
	m := decode(t, e.do(billID, "GET", "/api/platform/overview", ""))
	by, _ := m["workspaces_by_status"].(map[string]any)
	if by["active"] != float64(90) || by["read_only"] != float64(3) || by["suspended"] != float64(5) || by["pending_deletion"] != float64(2) || m["signups_7d"] != float64(17) {
		t.Fatalf("overview = %v", m)
	}
	if want := t0.Add(-7 * 24 * time.Hour); !e.reader.lastSince.Equal(want) {
		t.Fatalf("signups since %v, want %v", e.reader.lastSince, want)
	}
	byPlan, _ := m["by_plan"].(map[string]any)
	if len(byPlan) != 3 || byPlan["free"] != float64(70) || byPlan["pro"] != float64(25) || byPlan["business"] != float64(5) {
		t.Fatalf("by_plan = %v, want the counts per plan key", m["by_plan"])
	}

	// A state row for a workspace the count does not see never makes it negative.
	e.reader.overview = Overview{WorkspacesTotal: 1, NonActive: map[string]int64{"suspended": 2}}
	by, _ = decode(t, e.do(billID, "GET", "/api/platform/overview", ""))["workspaces_by_status"].(map[string]any)
	if by["active"] != float64(0) {
		t.Fatalf("active = %v", by["active"])
	}
}

// With no count, by_plan is an empty object: a client reads it as a map.
func TestOverviewByPlanIsAnObjectWhenEmpty(t *testing.T) {
	e := newEnv(t)
	m := decode(t, e.do(billID, "GET", "/api/platform/overview", ""))
	if by, ok := m["by_plan"].(map[string]any); !ok || len(by) != 0 {
		t.Fatalf("by_plan = %#v, want {}", m["by_plan"])
	}
}

func TestReadErrorsAreNotLeaked(t *testing.T) {
	e := newEnv(t)
	e.reader.err = errors.New("pq: relation platform_note does not exist")
	for _, p := range []string{"/api/platform/tenants", "/api/platform/users", "/api/platform/staff", "/api/platform/audit-events", "/api/platform/overview",
		"/api/platform/tenants/" + wsID, "/api/platform/users/" + custID, "/api/platform/tenants/" + wsID + "/notes"} {
		rec := e.do(adminID, "GET", p, "")
		wantCode(t, rec, 500, "internal")
		if strings.Contains(rec.Body.String(), "relation") {
			t.Fatalf("%s leaks the cause: %s", p, rec.Body.String())
		}
	}
}

func TestResponsesAreNotCached(t *testing.T) {
	e := newEnv(t)
	if cc := e.do(adminID, "GET", "/api/platform/overview", "").Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q", cc)
	}
}

// Spec 5.2: support locks and unlocks; only admin schedules or cancels a
// deletion. The middleware admits support to the route, so the handler answers
// 403.
func TestPendingDeletionNeedsAdmin(t *testing.T) {
	path := "/api/platform/tenants/" + wsID + "/status"

	e := newEnv(t)
	wantCode(t, e.do(suppID, "POST", path, `{"status":"pending_deletion","reason":"owner request"}`), 403, "forbidden")
	if len(e.ws.calls) != 0 {
		t.Fatalf("a refused request called Set %d times", len(e.ws.calls))
	}
	for _, status := range []string{"suspended", "read_only", "active"} {
		wantStatus(t, e.do(suppID, "POST", path, `{"status":"`+status+`","reason":"r"}`), 200)
	}
	if len(e.ws.calls) != 3 {
		t.Fatalf("Set calls = %d, want 3", len(e.ws.calls))
	}

	e = newEnv(t)
	wantStatus(t, e.do(adminID, "POST", path, `{"status":"pending_deletion","reason":"owner request"}`), 200)
	if len(e.ws.calls) != 1 || e.ws.calls[0].Status != "pending_deletion" {
		t.Fatalf("calls = %+v", e.ws.calls)
	}

	// An invalid body is still a 400 for support, not a 403.
	wantCode(t, newEnv(t).do(suppID, "POST", path, `{"status":"pending_deletion"}`), 400, "reason_required")
}

// Leaving pending_deletion cancels the deletion, which is also admin's.
func TestCancellingADeletionNeedsAdmin(t *testing.T) {
	path := "/api/platform/tenants/" + wsID + "/status"
	pending := func(e *env) {
		e.reader.details[wsID] = TenantDetail{TenantRow: TenantRow{ID: wsID, Name: wsName, Status: "pending_deletion", CreatedAt: t0, LastActivityAt: t0}}
	}

	e := newEnv(t)
	pending(e)
	wantCode(t, e.do(suppID, "POST", path, `{"status":"active","reason":"r"}`), 403, "forbidden")
	wantCode(t, e.do(suppID, "POST", path, `{"status":"suspended","reason":"r"}`), 403, "forbidden")
	if len(e.ws.calls) != 0 {
		t.Fatalf("a refused request called Set %d times", len(e.ws.calls))
	}

	e = newEnv(t)
	pending(e)
	wantStatus(t, e.do(adminID, "POST", path, `{"status":"active","reason":"owner changed their mind"}`), 200)
	if len(e.ws.calls) != 1 {
		t.Fatalf("Set calls = %d", len(e.ws.calls))
	}
}
