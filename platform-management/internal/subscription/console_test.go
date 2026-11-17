package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/platformauth"
	"github.com/multica-ai/multica/platform-management/internal/session"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

const (
	secret = "0123456789abcdef0123456789abcdef"
	origin = "https://console.example.com"

	adminUser = "aaaaaaaa-0000-0000-0000-000000000001"
	adminMail = "admin@example.com"
	suppUser  = "aaaaaaaa-0000-0000-0000-000000000002"
	billUser  = staffID // aaaaaaaa-0000-0000-0000-000000000003
	otherWS   = "cccccccc-0000-0000-0000-00000000000f"
)

type fakeDirectory struct {
	roles  map[string]staff.Role
	emails map[string]string
}

func (d *fakeDirectory) UserIDByEmail(context.Context, string) (string, bool, error) {
	return "", false, nil
}

func (d *fakeDirectory) ActiveStaff(_ context.Context, id string) (staff.Role, bool, error) {
	r, ok := d.roles[id]
	return r, ok, nil
}

func (d *fakeDirectory) EmailByUserID(_ context.Context, id string) (string, error) {
	if e, ok := d.emails[id]; ok {
		return e, nil
	}
	return "", errors.New("no such user")
}

type activeGuard struct{}

func (activeGuard) State(context.Context, string) (accountguard.Record, error) {
	return accountguard.Record{Status: accountguard.Active}, nil
}

// fakePlanStore is the plan table with transaction semantics, and the subscription
// counts the delete check reads.
type fakePlanStore struct {
	rows      map[string]plan.Plan
	subs      map[string]int64
	audits    []audit.Entry
	failAudit error
}

func (s *fakePlanStore) InTx(_ context.Context, fn func(plan.Tx) error) error {
	rows := map[string]plan.Plan{}
	for k, v := range s.rows {
		rows[k] = v
	}
	audits := append([]audit.Entry(nil), s.audits...)
	if err := fn(&fakePlanTx{s: s}); err != nil {
		s.rows, s.audits = rows, audits
		return err
	}
	return nil
}

func (s *fakePlanStore) Get(_ context.Context, key string) (plan.Plan, bool, error) {
	p, ok := s.rows[key]
	return p, ok, nil
}

func (s *fakePlanStore) List(context.Context) ([]plan.Plan, error) {
	var out []plan.Plan
	for _, k := range []string{"free", "pro", "business", "enterprise", "team", "legacy"} {
		if p, ok := s.rows[k]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

type fakePlanTx struct{ s *fakePlanStore }

func (t *fakePlanTx) Deleted(context.Context, string) (bool, error) {
	return false, errors.New("not used")
}
func (t *fakePlanTx) InsertIfAbsent(context.Context, plan.Plan) (bool, error) {
	return false, errors.New("not used")
}
func (t *fakePlanTx) Get(_ context.Context, key string) (plan.Plan, bool, error) {
	p, ok := t.s.rows[key]
	return p, ok, nil
}
func (t *fakePlanTx) Upsert(_ context.Context, p plan.Plan) error { t.s.rows[p.Key] = p; return nil }
func (t *fakePlanTx) Delete(_ context.Context, key string) (bool, error) {
	_, ok := t.s.rows[key]
	delete(t.s.rows, key)
	return ok, nil
}
func (t *fakePlanTx) CountSubscriptions(_ context.Context, key string) (int64, error) {
	return t.s.subs[key], nil
}
func (t *fakePlanTx) InsertAuditEvent(_ context.Context, e audit.Entry) error {
	if t.s.failAudit != nil {
		return t.s.failAudit
	}
	t.s.audits = append(t.s.audits, e)
	return nil
}

// consoleEnv is the console routes behind the real staff middleware, on fakes.
type consoleEnv struct {
	t      *testing.T
	h      http.Handler
	plans  *fakePlanStore
	subs   *fakeStore
	profs  *fakeProfiles
	cache  *recordingCache
	policy *recordingCache // the cache the plan routes clear
}

func newConsoleEnv(t *testing.T) *consoleEnv {
	t.Helper()
	subStore := newFakeStore(t)
	planStore := &fakePlanStore{rows: map[string]plan.Plan{}, subs: map[string]int64{}}
	for k, v := range subStore.plans {
		planStore.rows[k] = v
	}
	e := &consoleEnv{t: t, plans: planStore, subs: subStore, profs: newFakeProfiles(t), cache: &recordingCache{}, policy: &recordingCache{}}

	dir := &fakeDirectory{
		roles:  map[string]staff.Role{adminUser: staff.RoleAdmin, suppUser: staff.RoleSupport, billUser: staff.RoleBilling},
		emails: map[string]string{adminUser: adminMail, suppUser: "support@example.com", billUser: staffEm},
	}
	auth := platformauth.New(platformauth.Deps{
		Config:    platformauth.Config{SessionSecret: secret, ConsoleOrigin: origin},
		Directory: dir,
		Guard:     activeGuard{},
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:       func() time.Time { return t0 },
	})
	e.h = NewConsole(ConsoleDeps{
		RequireStaff:  auth.RequirePlatformStaff,
		RequireOrigin: auth.RequireOrigin,
		Plans:         planStore,
		PlanRunner:    planStore,
		Subscriptions: NewService(subStore, e.cache),
		Profiles:      NewProfileService(e.profs),
		Emails:        dir,
		Cache:         e.policy,
		Log:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:           func() time.Time { return t0 },
	})
	return e
}

func (e *consoleEnv) do(user, method, path, body string) *httptest.ResponseRecorder {
	e.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if user != "" {
		tok, err := platformauth.SignMFASession(secret, user, t0.Add(-time.Minute))
		if err != nil {
			e.t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: session.CookieName, Value: tok})
	}
	if method != http.MethodGet {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body %s", rec.Code, status, rec.Body)
	}
}

func wantCode(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	wantStatus(t, rec, status)
	if got := decodeMap(t, rec)["code"]; got != code {
		t.Fatalf("code = %v, want %s; body %s", got, code, rec.Body)
	}
}

const subPath = "/api/platform/tenants/" + wsID + "/subscription"

const proBody = `{"plan_key":"pro","seats":5,"interval":"month","reason":"bank transfer 1234"}`

// --- Who may call what.

func TestRoutesAdmitOnlyTheirRoles(t *testing.T) {
	cases := []struct {
		name, method, path, body string
		allowed                  []string
	}{
		{"list plans", "GET", "/api/platform/plans", "", []string{adminUser}},
		{"put plan", "PUT", "/api/platform/plans/team", teamBody, []string{adminUser}},
		{"delete plan", "DELETE", "/api/platform/plans/legacy", "", []string{adminUser}},
		{"put subscription", "PUT", subPath, proBody, []string{billUser, adminUser}},
	}
	for _, tc := range cases {
		for _, user := range []string{"", suppUser, billUser, adminUser} {
			e := newConsoleEnv(t)
			rec := e.do(user, tc.method, tc.path, tc.body)
			allowed := false
			for _, a := range tc.allowed {
				allowed = allowed || a == user
			}
			if allowed && rec.Code >= 400 {
				t.Errorf("%s as %q: status %d, want success; %s", tc.name, user, rec.Code, rec.Body)
			}
			// Every refusal is the 404 of an unknown route, and writes nothing.
			if !allowed && rec.Code != 404 {
				t.Errorf("%s as %q: status %d, want 404", tc.name, user, rec.Code)
			}
			if !allowed && (len(e.subs.audits) != 0 || len(e.plans.audits) != 0) {
				t.Errorf("%s as %q: a refused call wrote an audit row", tc.name, user)
			}
		}
	}
}

func TestWritesNeedTheConsoleOrigin(t *testing.T) {
	e := newConsoleEnv(t)
	tok, _ := platformauth.SignMFASession(secret, adminUser, t0.Add(-time.Minute))
	req := httptest.NewRequest("PUT", subPath, strings.NewReader(proBody))
	req.AddCookie(&http.Cookie{Name: session.CookieName, Value: tok})
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	wantCode(t, rec, 403, "bad_origin")
	if len(e.subs.subs) != 0 {
		t.Fatal("a cross-origin write changed the subscription")
	}
}

func TestOtherMethodsOnTheseRoutesAre404(t *testing.T) {
	e := newConsoleEnv(t)
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/platform/plans"},
		{"DELETE", "/api/platform/plans"},
		{"GET", "/api/platform/plans/pro"},
		{"POST", "/api/platform/plans/pro"},
		{"GET", subPath},
		{"POST", subPath},
		{"DELETE", subPath},
	} {
		wantStatus(t, e.do(adminUser, tc.method, tc.path, "{}"), 404)
	}
}

func TestPatternsListTheRoutesTheServerMounts(t *testing.T) {
	want := map[string]bool{
		"/api/platform/plans": true, "/api/platform/plans/{key}": true, "/api/platform/tenants/{id}/subscription": true,
		"/api/platform/tenants/{id}/billing-profile/verify":   true,
		"/api/platform/tenants/{id}/billing-profile/unverify": true,
	}
	got := ConsolePatterns()
	if len(got) != len(want) {
		t.Fatalf("patterns = %v", got)
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected pattern %s", p)
		}
	}
}

// --- PUT /tenants/{id}/subscription

func TestPutSubscriptionIncrementsTheVersionByOne(t *testing.T) {
	e := newConsoleEnv(t)
	for want := int64(1); want <= 3; want++ {
		body := `{"plan_key":"pro","seats":` + string(rune('0'+want+4)) + `,"interval":"month","reason":"step"}`
		rec := e.do(billUser, "PUT", subPath, body)
		wantStatus(t, rec, 200)
		m := decodeMap(t, rec)
		sub := obj(t, m["subscription"], "subscription")
		if m["changed"] != true || sub["version"] != float64(want) || e.subs.subs[wsID].Version != want {
			t.Fatalf("step %d: %v (stored version %d)", want, m, e.subs.subs[wsID].Version)
		}
	}
	if len(e.subs.audits) != 3 {
		t.Fatalf("audit rows = %d, want one per change", len(e.subs.audits))
	}
	if got := e.cache.workspaces; len(got) != 3 || got[0] != wsID {
		t.Fatalf("policy cache cleared for %v, want the workspace once per change", got)
	}
}

func TestPutSubscriptionReturnsTheRow(t *testing.T) {
	e := newConsoleEnv(t)
	body := `{"plan_key":"pro","seats":5,"interval":"year","current_period_start":"2030-05-01T00:00:00Z",
	"current_period_end":"2031-05-01T00:00:00Z","trial_end":null,"grace_until":"2031-05-08T00:00:00Z",
	"cancel_at_period_end":true,"limit_overrides":{"issue_count":500,"autopilot_runs":null},"reason":"annual"}`
	rec := e.do(adminUser, "PUT", subPath, body)
	wantStatus(t, rec, 200)
	sub := obj(t, decodeMap(t, rec)["subscription"], "subscription")
	for k, want := range map[string]any{
		"workspace_id": wsID, "plan_key": "pro", "status": "active", "seats": float64(5), "interval": "year",
		"current_period_start": "2030-05-01T00:00:00Z", "current_period_end": "2031-05-01T00:00:00Z",
		"trial_end": nil, "grace_until": "2031-05-08T00:00:00Z", "cancel_at_period_end": true, "version": float64(1),
	} {
		if sub[k] != want {
			t.Errorf("subscription.%s = %v, want %v", k, sub[k], want)
		}
	}
	ov := obj(t, sub["limit_overrides"], "limit_overrides")
	if v, ok := ov["autopilot_runs"]; !ok || v != nil || ov["issue_count"] != float64(500) {
		t.Errorf("limit_overrides = %v, want 500 and an explicit null", ov)
	}
}

func TestPutSubscriptionWithoutAReasonIs400(t *testing.T) {
	for _, body := range []string{
		`{"plan_key":"pro","seats":5,"interval":"month"}`,
		`{"plan_key":"pro","seats":5,"interval":"month","reason":""}`,
		`{"plan_key":"pro","seats":5,"interval":"month","reason":"   "}`,
	} {
		e := newConsoleEnv(t)
		wantCode(t, e.do(billUser, "PUT", subPath, body), 400, "reason_required")
		if len(e.subs.subs) != 0 || len(e.subs.audits) != 0 || len(e.cache.workspaces) != 0 {
			t.Errorf("%s: a refused change wrote something", body)
		}
	}
}

func TestPutSubscriptionAuditsOnlyTheChangedFieldsWithTheStaffActor(t *testing.T) {
	e := newConsoleEnv(t)
	wantStatus(t, e.do(billUser, "PUT", subPath, proBody), 200)
	wantStatus(t, e.do(adminUser, "PUT", subPath, `{"plan_key":"pro","seats":8,"interval":"month","reason":"more seats"}`), 200)

	if len(e.subs.audits) != 2 {
		t.Fatalf("audit rows = %d", len(e.subs.audits))
	}
	second := e.subs.audits[1]
	if second.Action != audit.ActionSubscriptionChanged || second.ActorType != audit.ActorStaff || second.ActorID != adminUser || second.ActorLabel != adminMail {
		t.Errorf("entry = %+v, want the staff actor", second)
	}
	got := changes(t, second)
	if len(got) != 1 || got["seats"]["from"] != 5 || got["seats"]["to"] != 8 {
		t.Errorf("changes = %v, want only seats 5 -> 8", got)
	}
	if second.Detail["reason"] != "more seats" || second.Detail["version"] != int64(2) {
		t.Errorf("detail = %v", second.Detail)
	}
}

func TestPutSubscriptionOnAnUnchangedBodyWritesNothing(t *testing.T) {
	e := newConsoleEnv(t)
	wantStatus(t, e.do(billUser, "PUT", subPath, proBody), 200)
	rec := e.do(billUser, "PUT", subPath, proBody)
	wantStatus(t, rec, 200)
	m := decodeMap(t, rec)
	if m["changed"] != false || obj(t, m["subscription"], "subscription")["version"] != float64(1) {
		t.Fatalf("response = %v", m)
	}
	if len(e.subs.audits) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(e.subs.audits))
	}
}

func TestPutSubscriptionErrors(t *testing.T) {
	cases := []struct {
		name, path, body string
		status           int
		code             string
	}{
		{"unknown plan", subPath, `{"plan_key":"nope","interval":"month","reason":"r"}`, 400, "unknown_plan"},
		{"archived plan", subPath, `{"plan_key":"legacy","interval":"month","reason":"r"}`, 400, "plan_archived"},
		{"below the minimum", subPath, `{"plan_key":"business","seats":2,"interval":"month","reason":"r"}`, 400, "seats_below_minimum"},
		{"bad interval", subPath, `{"plan_key":"pro","seats":1,"interval":"week","reason":"r"}`, 400, "invalid_subscription"},
		{"missing plan key", subPath, `{"seats":1,"interval":"month","reason":"r"}`, 400, "invalid_subscription"},
		{"unknown field", subPath, `{"plan_key":"pro","interval":"month","reason":"r","status":"canceled"}`, 400, "invalid_body"},
		{"not json", subPath, `plan=pro`, 400, "invalid_body"},
		{"two values", subPath, `{"plan_key":"pro","interval":"month","reason":"r"} {}`, 400, "invalid_body"},
		{"bad time", subPath, `{"plan_key":"pro","interval":"month","reason":"r","trial_end":"tomorrow"}`, 400, "invalid_body"},
		{"zero issue override", subPath, `{"plan_key":"pro","interval":"month","reason":"r","limit_overrides":{"issue_count":0}}`, 400, "invalid_subscription"},
		{"unknown workspace", "/api/platform/tenants/" + otherWS + "/subscription", proBody, 404, "not_found"},
		{"bad workspace id", "/api/platform/tenants/acme/subscription", proBody, 404, "not_found"},
	}
	for _, tc := range cases {
		e := newConsoleEnv(t)
		rec := e.do(billUser, "PUT", tc.path, tc.body)
		wantCode(t, rec, tc.status, tc.code)
		if len(e.subs.subs) != 0 || len(e.subs.audits) != 0 {
			t.Errorf("%s: wrote something", tc.name)
		}
	}
}

func TestPutSubscriptionDoesNotLeakInternalErrors(t *testing.T) {
	e := newConsoleEnv(t)
	e.subs.failAudit = errors.New(`pq: relation "audit_event" does not exist`)
	rec := e.do(billUser, "PUT", subPath, proBody)
	wantCode(t, rec, 500, "internal")
	if strings.Contains(rec.Body.String(), "audit_event") {
		t.Fatalf("the body leaks the cause: %s", rec.Body)
	}
	if len(e.subs.subs) != 0 || len(e.cache.workspaces) != 0 {
		t.Fatal("a failed audit write left the change in place or cleared the cache")
	}
}

// --- Plans

const teamBody = `{"name":"Team","limits":{"issue_count":1000,"autopilot_runs":null},"features":{"sso":true},
"currency":"vnd","seat_price_month":99000,"seat_price_year":990000,"seats_fixed":null,"min_seats":2,
"trial_days":7,"is_public":true,"sort":25,"archived":false}`

func TestListPlansIncludesArchivedOnes(t *testing.T) {
	e := newConsoleEnv(t)
	rec := e.do(adminUser, "GET", "/api/platform/plans", "")
	wantStatus(t, rec, 200)
	list, _ := decodeMap(t, rec)["plans"].([]any)
	var keys []string
	for _, p := range list {
		keys = append(keys, obj(t, p, "plan")["key"].(string))
	}
	if strings.Join(keys, ",") != "free,pro,business,enterprise,legacy" {
		t.Fatalf("plans = %v", keys)
	}
	legacy := obj(t, list[4], "legacy")
	if legacy["archived_at"] == nil {
		t.Error("an archived plan lists no archived_at")
	}
	free := obj(t, list[0], "free")
	if free["seats_fixed"] != float64(3) || obj(t, free["limits"], "limits")["issue_count"] != float64(200) {
		t.Errorf("free = %v", free)
	}
}

func TestPutPlanCreatesThenReplaces(t *testing.T) {
	e := newConsoleEnv(t)
	rec := e.do(adminUser, "PUT", "/api/platform/plans/team", teamBody)
	wantStatus(t, rec, 200)
	m := decodeMap(t, rec)
	p := obj(t, m["plan"], "plan")
	if m["changed"] != true || p["key"] != "team" || p["seat_price_month"] != float64(99000) || p["min_seats"] != float64(2) || p["archived_at"] != nil {
		t.Fatalf("response = %v", m)
	}
	if got := e.plans.rows["team"]; got.Name != "Team" || got.Limits["autopilot_runs"] != nil || *got.Limits["issue_count"] != 1000 {
		t.Fatalf("stored = %+v", got)
	}
	if e.policy.all != 1 {
		t.Fatalf("the policy cache was cleared %d times, want once for every workspace", e.policy.all)
	}

	// A second call with a new price replaces the plan.
	changed := strings.Replace(teamBody, `"seat_price_month":99000`, `"seat_price_month":109000`, 1)
	wantStatus(t, e.do(adminUser, "PUT", "/api/platform/plans/team", changed), 200)
	if got := e.plans.rows["team"]; *got.SeatPriceMonth != 109000 {
		t.Fatalf("stored price = %d", *got.SeatPriceMonth)
	}
	if len(e.plans.audits) != 2 || e.policy.all != 2 {
		t.Fatalf("audits %d, cache clears %d", len(e.plans.audits), e.policy.all)
	}

	a := e.plans.audits[1]
	if a.Action != audit.ActionPlanChanged || a.ActorType != audit.ActorStaff || a.ActorID != adminUser || a.ActorLabel != adminMail ||
		a.TargetType != "plan" || a.TargetID != "team" || a.WorkspaceID != "" {
		t.Errorf("audit entry = %+v", a)
	}
	got := changes(t, a)
	if len(got) != 1 || got["seat_price_month"]["from"] != float64(99000) || got["seat_price_month"]["to"] != float64(109000) {
		t.Errorf("changes = %v, want only the price", got)
	}
	if redacted := audit.Redact(a.Detail); len(changes(t, audit.Entry{Detail: redacted})) != 1 {
		t.Errorf("redaction changed the detail: %v", redacted)
	}
}

func TestPutPlanThatChangesNothingWritesNothing(t *testing.T) {
	e := newConsoleEnv(t)
	wantStatus(t, e.do(adminUser, "PUT", "/api/platform/plans/team", teamBody), 200)
	rec := e.do(adminUser, "PUT", "/api/platform/plans/team", teamBody)
	wantStatus(t, rec, 200)
	if decodeMap(t, rec)["changed"] != false || len(e.plans.audits) != 1 || e.policy.all != 1 {
		t.Fatalf("response %s; audits %d, cache clears %d", rec.Body, len(e.plans.audits), e.policy.all)
	}
}

func TestArchivingAPlanKeepsItsTimestampAndRestoringClearsIt(t *testing.T) {
	e := newConsoleEnv(t)
	wantStatus(t, e.do(adminUser, "PUT", "/api/platform/plans/team", teamBody), 200)

	archived := strings.Replace(teamBody, `"archived":false`, `"archived":true`, 1)
	wantStatus(t, e.do(adminUser, "PUT", "/api/platform/plans/team", archived), 200)
	at := e.plans.rows["team"].ArchivedAt
	if want := t0.Truncate(time.Microsecond); at == nil || !at.Equal(want) {
		t.Fatalf("archived_at = %v, want %v", at, want)
	}
	got := changes(t, e.plans.audits[1])
	if len(got) != 1 || got["archived_at"]["to"] == nil {
		t.Errorf("changes = %v", got)
	}

	// Archiving again does not move the timestamp, and is not a change.
	rec := e.do(adminUser, "PUT", "/api/platform/plans/team", archived)
	if decodeMap(t, rec)["changed"] != false {
		t.Errorf("a repeated archive counted as a change: %s", rec.Body)
	}

	wantStatus(t, e.do(adminUser, "PUT", "/api/platform/plans/team", teamBody), 200)
	if e.plans.rows["team"].ArchivedAt != nil {
		t.Fatal("restoring the plan left archived_at set")
	}
}

func TestPutPlanErrors(t *testing.T) {
	cases := []struct {
		name, path, body string
		code             string
	}{
		{"bad key", "/api/platform/plans/Team", teamBody, "invalid_plan"},
		{"dash in key", "/api/platform/plans/a-b", teamBody, "invalid_plan"},
		{"blank name", "/api/platform/plans/team", strings.Replace(teamBody, `"Team"`, `" "`, 1), "invalid_plan"},
		{"unknown limit", "/api/platform/plans/team", strings.Replace(teamBody, `"issue_count"`, `"typo"`, 1), "invalid_plan"},
		{"capital currency", "/api/platform/plans/team", strings.Replace(teamBody, `"vnd"`, `"VND"`, 1), "invalid_plan"},
		{"negative price", "/api/platform/plans/team", strings.Replace(teamBody, `99000`, `-1`, 1), "invalid_plan"},
		{"fractional price", "/api/platform/plans/team", strings.Replace(teamBody, `99000`, `99000.5`, 1), "invalid_body"},
		{"unknown field", "/api/platform/plans/team", strings.Replace(teamBody, `"sort":25`, `"sort":25,"extra":1`, 1), "invalid_body"},
		{"key in the body", "/api/platform/plans/team", strings.Replace(teamBody, `"name"`, `"key":"other","name"`, 1), "invalid_body"},
	}
	for _, tc := range cases {
		e := newConsoleEnv(t)
		wantCode(t, e.do(adminUser, "PUT", tc.path, tc.body), 400, tc.code)
		if _, ok := e.plans.rows["team"]; ok || len(e.plans.audits) != 0 || e.policy.all != 0 {
			t.Errorf("%s: wrote something", tc.name)
		}
	}
}

// --- Archive versus delete.

func TestDeleteOfAPlanWithSubscriptionsIs409PlanInUse(t *testing.T) {
	e := newConsoleEnv(t)
	e.plans.subs["pro"] = 3
	rec := e.do(adminUser, "DELETE", "/api/platform/plans/pro", "")
	wantCode(t, rec, 409, "plan_in_use")
	if _, ok := e.plans.rows["pro"]; !ok {
		t.Fatal("the plan was deleted")
	}
	if len(e.plans.audits) != 0 || e.policy.all != 0 {
		t.Fatal("a refused delete wrote an audit row or cleared the cache")
	}

	// It can be archived instead.
	pro := e.plans.rows["pro"]
	body, _ := json.Marshal(map[string]any{
		"name": pro.Name, "limits": pro.Limits, "features": pro.Features, "currency": pro.Currency,
		"seat_price_month": pro.SeatPriceMonth, "seat_price_year": pro.SeatPriceYear, "seats_fixed": pro.SeatsFixed,
		"min_seats": pro.MinSeats, "trial_days": pro.TrialDays, "is_public": pro.IsPublic, "sort": pro.Sort, "archived": true,
	})
	wantStatus(t, e.do(adminUser, "PUT", "/api/platform/plans/pro", string(body)), 200)
	if e.plans.rows["pro"].ArchivedAt == nil {
		t.Fatal("the plan was not archived")
	}
}

// Workspaces with no subscription row are on the free plan, so it is in use
// whatever the subscription count says.
func TestTheFreePlanCannotBeDeleted(t *testing.T) {
	e := newConsoleEnv(t)
	wantCode(t, e.do(adminUser, "DELETE", "/api/platform/plans/free", ""), 409, "plan_in_use")
	if _, ok := e.plans.rows["free"]; !ok {
		t.Fatal("the free plan was deleted")
	}
}

func TestDeleteOfAnUnusedPlan(t *testing.T) {
	e := newConsoleEnv(t)
	wantStatus(t, e.do(adminUser, "PUT", "/api/platform/plans/team", teamBody), 200)
	rec := e.do(adminUser, "DELETE", "/api/platform/plans/team", "")
	wantStatus(t, rec, 200)
	if decodeMap(t, rec)["deleted"] != true {
		t.Errorf("body = %s", rec.Body)
	}
	if _, ok := e.plans.rows["team"]; ok {
		t.Fatal("the plan is still there")
	}
	last := e.plans.audits[len(e.plans.audits)-1]
	if last.Action != audit.ActionPlanChanged || last.TargetID != "team" || last.Detail["deleted"] != true || last.ActorID != adminUser {
		t.Errorf("audit entry = %+v", last)
	}
	if e.policy.all != 2 {
		t.Errorf("cache clears = %d", e.policy.all)
	}
}

func TestDeleteOfAnUnknownPlanIs404(t *testing.T) {
	e := newConsoleEnv(t)
	wantCode(t, e.do(adminUser, "DELETE", "/api/platform/plans/nope", ""), 404, "not_found")
	wantCode(t, e.do(adminUser, "DELETE", "/api/platform/plans/Not-A-Key", ""), 404, "not_found")
}

func TestFailedPlanAuditWriteRollsBack(t *testing.T) {
	e := newConsoleEnv(t)
	e.plans.failAudit = errors.New("disk full")
	wantCode(t, e.do(adminUser, "PUT", "/api/platform/plans/team", teamBody), 500, "internal")
	if _, ok := e.plans.rows["team"]; ok {
		t.Fatal("the plan was kept without its audit row")
	}
	if e.policy.all != 0 {
		t.Fatal("the cache was cleared for a rolled-back change")
	}
}

// --- Platform billing (P24): the audit rows keep the old and the new values.

func TestPlatformBillingSubscriptionAuditHasOldAndNewValues(t *testing.T) {
	e := newConsoleEnv(t)
	first := `{"plan_key":"pro","seats":5,"interval":"month","trial_end":"2031-05-01T00:00:00Z","reason":"start"}`
	wantStatus(t, e.do(billUser, "PUT", subPath, first), 200)
	second := `{"plan_key":"business","seats":9,"interval":"year","trial_end":"2031-06-01T00:00:00Z","reason":"upgrade"}`
	wantStatus(t, e.do(billUser, "PUT", subPath, second), 200)

	if len(e.subs.audits) != 2 {
		t.Fatalf("audit rows = %d, want 2", len(e.subs.audits))
	}
	got := changes(t, e.subs.audits[1])
	want := map[string][2]any{
		"plan":      {"pro", "business"},
		"seats":     {5, 9},
		"interval":  {"month", "year"},
		"trial_end": {"2031-05-01T00:00:00Z", "2031-06-01T00:00:00Z"},
	}
	for field, ft := range want {
		if got[field]["from"] != ft[0] || got[field]["to"] != ft[1] {
			t.Errorf("%s = %v, want from %v to %v", field, got[field], ft[0], ft[1])
		}
	}
}

func TestPlatformBillingDeletePlanInUseIs409(t *testing.T) {
	e := newConsoleEnv(t)
	e.plans.subs["pro"] = 1
	wantCode(t, e.do(adminUser, "DELETE", "/api/platform/plans/pro", ""), 409, "plan_in_use")
	if len(e.plans.audits) != 0 {
		t.Fatalf("a refused delete wrote %d audit rows", len(e.plans.audits))
	}
}

func TestPlatformBillingDeleteUnusedPlanAuditsTheOldValues(t *testing.T) {
	e := newConsoleEnv(t)
	wantStatus(t, e.do(adminUser, "PUT", "/api/platform/plans/team", teamBody), 200)
	wantStatus(t, e.do(adminUser, "DELETE", "/api/platform/plans/team", ""), 200)

	last := e.plans.audits[len(e.plans.audits)-1]
	if last.Action != audit.ActionPlanChanged || last.TargetID != "team" || last.Detail["deleted"] != true {
		t.Fatalf("audit entry = %+v", last)
	}
	diff, ok := last.Detail["changes"].(map[string]any)
	if !ok {
		t.Fatalf("detail has no changes: %v", last.Detail)
	}
	price, ok := diff["seat_price_month"].(map[string]any)
	if !ok || price["from"] != float64(99000) || price["to"] != nil {
		t.Errorf("seat_price_month = %v, want from 99000 to nil", diff["seat_price_month"])
	}
}

// A plan change keeps the old and the new value of each field it changes.
func TestPlatformBillingPlanChangeAuditHasOldAndNewValues(t *testing.T) {
	e := newConsoleEnv(t)
	wantStatus(t, e.do(adminUser, "PUT", "/api/platform/plans/team", teamBody), 200)
	changed := strings.Replace(teamBody, `"seat_price_month":99000`, `"seat_price_month":120000`, 1)
	wantStatus(t, e.do(adminUser, "PUT", "/api/platform/plans/team", changed), 200)

	last := e.plans.audits[len(e.plans.audits)-1]
	diff := last.Detail["changes"].(map[string]any)
	price, ok := diff["seat_price_month"].(map[string]any)
	if !ok || len(diff) != 1 || price["from"] != float64(99000) || price["to"] != float64(120000) {
		t.Errorf("changes = %v, want only seat_price_month 99000 -> 120000", diff)
	}
}

// A PUT that sends only a name keeps the stored prices and currency; it must
// not write NULL prices or a new currency.
func TestNameOnlyPutKeepsTheStoredPricesAndCurrency(t *testing.T) {
	e := newConsoleEnv(t)
	wantStatus(t, e.do(adminUser, "PUT", "/api/platform/plans/team", teamBody), 200)
	rec := e.do(adminUser, "PUT", "/api/platform/plans/team", `{"name":"Team renamed"}`)
	wantStatus(t, rec, 200)
	got := e.plans.rows["team"]
	if got.Name != "Team renamed" {
		t.Fatalf("name = %q, the rename was not written", got.Name)
	}
	if got.SeatPriceMonth == nil || *got.SeatPriceMonth != 99000 {
		t.Fatalf("seat_price_month = %v, want 99000", got.SeatPriceMonth)
	}
	if got.SeatPriceYear == nil || *got.SeatPriceYear != 990000 {
		t.Fatalf("seat_price_year = %v, want 990000", got.SeatPriceYear)
	}
	if got.Currency != "vnd" {
		t.Fatalf("currency = %q, want vnd", got.Currency)
	}
}

// --- POST /tenants/{id}/billing-profile/verify and /unverify

const (
	verifyPath   = "/api/platform/tenants/" + wsID + "/billing-profile/verify"
	unverifyPath = "/api/platform/tenants/" + wsID + "/billing-profile/unverify"
)

func TestVerifyRouteAllowsBillingAndAdminOnly(t *testing.T) {
	for _, tc := range []struct {
		user string
		want int
	}{{"", 404}, {suppUser, 404}, {billUser, 200}, {adminUser, 200}} {
		e := newConsoleEnv(t)
		e.profs.profiles[wsID] = businessProfile()
		rec := e.do(tc.user, "POST", verifyPath, "")
		wantStatus(t, rec, tc.want)
		if tc.want != 200 {
			if len(e.profs.audits) != 0 || e.profs.profiles[wsID].VerifiedAt != nil {
				t.Errorf("user %q: a refused call changed the profile", tc.user)
			}
			continue
		}
		m := decodeMap(t, rec)
		if m["workspace_id"] != wsID || m["account_kind"] != AccountBusiness || m["verified"] != true || m["verified_at"] != t0.Truncate(time.Microsecond).Format(time.RFC3339Nano) {
			t.Errorf("user %q: body = %v", tc.user, m)
		}
		if got := e.profs.profiles[wsID]; got.VerifiedBy != tc.user || len(e.profs.audits) != 1 || e.profs.audits[0].ActorType != audit.ActorStaff {
			t.Errorf("user %q: verified_by = %q, audits = %+v", tc.user, got.VerifiedBy, e.profs.audits)
		}
	}
}

func TestUnverifyRouteClearsAndReportsNull(t *testing.T) {
	e := newConsoleEnv(t)
	e.profs.profiles[wsID] = businessProfile()
	wantStatus(t, e.do(billUser, "POST", verifyPath, ""), 200)
	m := decodeMap(t, e.do(adminUser, "POST", unverifyPath, ""))
	if m["verified"] != false || m["verified_at"] != nil || m["workspace_id"] != wsID {
		t.Fatalf("body = %v", m)
	}
	if e.profs.profiles[wsID].VerifiedAt != nil || len(e.profs.audits) != 2 {
		t.Fatalf("profile = %+v, audits = %d", e.profs.profiles[wsID], len(e.profs.audits))
	}
	if refused := e.do(suppUser, "POST", unverifyPath, ""); refused.Code != 404 {
		t.Fatalf("support unverify: status %d", refused.Code)
	}
}

func TestVerifyRouteErrors(t *testing.T) {
	e := newConsoleEnv(t)
	rec := e.do(billUser, "POST", verifyPath, "")
	wantCode(t, rec, 409, "billing_profile_incomplete")
	if f, _ := decodeMap(t, rec)["fields"].([]any); len(f) != 4 {
		t.Fatalf("fields = %v", f)
	}
	wantCode(t, e.do(billUser, "POST", "/api/platform/tenants/"+otherWS+"/billing-profile/verify", ""), 404, "not_found")
	wantCode(t, e.do(billUser, "POST", "/api/platform/tenants/acme/billing-profile/verify", ""), 404, "not_found")
}

func TestVerifyRouteRefusesGET(t *testing.T) {
	e := newConsoleEnv(t)
	e.profs.profiles[wsID] = businessProfile()
	for _, path := range []string{verifyPath, unverifyPath} {
		wantStatus(t, e.do(adminUser, "GET", path, ""), 404)
	}
	if e.profs.profiles[wsID].VerifiedAt != nil {
		t.Fatal("a GET verified the profile")
	}
}
