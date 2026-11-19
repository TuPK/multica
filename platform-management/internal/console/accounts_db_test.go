package console

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/internal/accountrequest"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/platformauth"
	"github.com/multica-ai/multica/platform-management/internal/session"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

type countingLimiter struct{ n int }

func (l *countingLimiter) Allow(_ context.Context, _ string, limit int, _ time.Duration) (bool, error) {
	l.n++
	return l.n <= limit, nil
}

// accountsDBFixture is the real Handler, store, plan reader and account
// request service on the test database, with a fake Redis counter.
func accountsDBFixture(t *testing.T, perHour int) *dbFixture {
	t.Helper()
	f := newDBFixture(t)
	q := db.New(f.pool)
	store := staff.NewStore(q)
	auth := platformauth.New(platformauth.Deps{
		Config:    platformauth.Config{SessionSecret: secret, ConsoleOrigin: origin},
		Directory: store,
		Guard:     accountguard.New(store, f.cache),
	})
	f.h = New(Deps{
		RequireStaff:  auth.RequirePlatformStaff,
		RequireOrigin: auth.RequireOrigin,
		Accounts: accountrequest.New(accountrequest.Deps{
			Runner: accountrequest.NewPgxRunner(f.pool), Reader: accountrequest.NewPgxReader(f.pool),
			Plans: plan.NewPgxReader(f.pool), Limiter: &countingLimiter{},
			CreatePerHour: perHour, IndividualMaxSeats: 5,
		}),
		Logins: NewPgxLogins(f.pool),
		Emails: store,
	})
	t.Cleanup(func() {
		f.pool.Exec(context.Background(), `DELETE FROM platform_account_request WHERE email LIKE 'acct-db-%'`)
	})
	return f
}

func TestAccountRequestDBAdminCreatesListsAndCancels(t *testing.T) {
	f := accountsDBFixture(t, 30)
	adminUser, adminEmail := f.staffUser(staff.RoleSystemAdmin)
	supportUser, _ := f.staffUser(staff.RoleSupport)
	billingUser, _ := f.staffUser(staff.RoleBilling)
	email := "acct-db-" + randomHex(t) + "@example.test"
	body := `{"kind":"individual","email":"` + strings.ToUpper(email) + `","display_name":"Ana","workspace_name":"Ana space"}`

	for _, u := range []string{supportUser, billingUser} {
		rec := f.do(u, "POST", "/api/platform/accounts", body)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("non-admin create = %d, want 404", rec.Code)
		}
	}
	if n := f.count(`SELECT count(*) FROM platform_account_request WHERE email = $1`, email); n != 0 {
		t.Fatalf("%d rows after refused creates", n)
	}

	rec := f.do(adminUser, "POST", "/api/platform/accounts", body)
	wantStatus(t, rec, 202)
	id, _ := decode(t, rec)["id"].(string)
	if id == "" {
		t.Fatalf("no id in %s", rec.Body.String())
	}
	rows := f.staffRows("account.provision_requested", adminUser)
	if len(rows) != 1 || rows[0].actorLabel != adminEmail || rows[0].workspaceID != "" || rows[0].targetID != id {
		t.Fatalf("audit rows = %+v", rows)
	}

	again := f.do(adminUser, "POST", "/api/platform/accounts", body)
	wantStatus(t, again, 200)
	if decode(t, again)["id"] != id {
		t.Errorf("repeat answered with %s", again.Body.String())
	}
	if n := f.count(`SELECT count(*) FROM platform_account_request WHERE email = $1`, email); n != 1 {
		t.Errorf("%d rows after a repeat, want 1", n)
	}

	wantStatus(t, f.do(adminUser, "GET", "/api/platform/accounts/"+id, ""), 200)
	wantUnknownRoute(t, f.do(supportUser, "GET", "/api/platform/accounts/"+id, ""))

	wantStatus(t, f.do(adminUser, "POST", "/api/platform/accounts/"+id+"/cancel", ""), 200)
	if rows := f.staffRows("account.provision_canceled", adminUser); len(rows) != 1 || rows[0].targetID != id {
		t.Errorf("cancel audit rows = %+v", rows)
	}
	rec = f.do(adminUser, "POST", "/api/platform/accounts/"+id+"/cancel", "")
	wantCode(t, rec, 409, "account_request_not_cancelable")
	if decode(t, rec)["state"] != "canceled" {
		t.Errorf("body = %s", rec.Body.String())
	}
}

// An admin session that never passed the TOTP step (no mfa in amr) cannot
// create an account request, and the refused request writes no row (spec 7b.5).
func TestAccountCreateDBRefusesAnAdminSessionWithoutMFA(t *testing.T) {
	f := accountsDBFixture(t, 30)
	adminUser, _ := f.staffUser(staff.RoleSystemAdmin)
	email := "acct-db-" + randomHex(t) + "@example.test"
	body := `{"kind":"individual","email":"` + email + `","display_name":"Ana","workspace_name":"Ana space"}`
	post := func(sign func(secret, userID string, authTime time.Time) (string, error)) *httptest.ResponseRecorder {
		tok, err := sign(secret, adminUser, time.Now().Add(-time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/api/platform/accounts", strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: session.CookieName, Value: tok})
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		f.h.ServeHTTP(rec, req)
		return rec
	}

	if rec := post(session.Sign); rec.Code != http.StatusNotFound {
		t.Fatalf("admin session without mfa = %d, want 404", rec.Code)
	}
	if n := f.count(`SELECT count(*) FROM platform_account_request WHERE email = $1`, email); n != 0 {
		t.Fatalf("%d rows after the refused create", n)
	}

	wantStatus(t, post(platformauth.SignMFASession), 202)
	if n := f.count(`SELECT count(*) FROM platform_account_request WHERE email = $1`, email); n != 1 {
		t.Fatalf("%d rows after the mfa create, want 1", n)
	}
}

func TestAccountCreateDBRateLimitAnswers429WithRetryAfter(t *testing.T) {
	f := accountsDBFixture(t, 1)
	adminUser, _ := f.staffUser(staff.RoleSystemAdmin)
	mk := func(n string) string {
		return `{"kind":"individual","email":"acct-db-` + randomHex(t) + n + `@example.test","display_name":"A","workspace_name":"W"}`
	}
	wantStatus(t, f.do(adminUser, "POST", "/api/platform/accounts", mk("a")), 202)
	rec := f.do(adminUser, "POST", "/api/platform/accounts", mk("b"))
	wantCode(t, rec, 429, "account_create_rate_limited")
	if rec.Header().Get("Retry-After") == "" {
		t.Error("no Retry-After header")
	}
}

// A provisioned request whose user has an auth.login row shows logged_in true;
// one whose user has only other audit rows shows false (spec 7b.5).
func TestAccountRequestDBShowsWhetherTheUserEverLoggedIn(t *testing.T) {
	f := accountsDBFixture(t, 30)
	adminUser, _ := f.staffUser(staff.RoleSystemAdmin)
	used, _ := f.user("acct-db-used")
	unused, _ := f.user("acct-db-unused")
	ctx := context.Background()
	ids := map[string]string{}
	for name, user := range map[string]string{"used": used, "unused": unused, "none": ""} {
		var id string
		var uid any
		if user != "" {
			uid = user
		}
		if err := f.pool.QueryRow(ctx, `INSERT INTO platform_account_request
			(id, kind, email, display_name, workspace_name, plan_key, state, user_id, created_by)
			VALUES (gen_random_uuid(), 'individual', $1, 'A', 'W', 'free', $2, $3, $4) RETURNING id::text`,
			"acct-db-"+name+"-"+randomHex(t)+"@example.test", map[bool]string{true: "provisioned", false: "pending"}[user != ""], uid, adminUser,
		).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[name] = id
	}
	for _, u := range []struct{ user, action string }{{used, "auth.login"}, {unused, "account.provision_requested"}, {unused, "auth.logout"}} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO audit_event (actor_type, actor_id, action) VALUES ('member', $1, $2)`, u.user, u.action); err != nil {
			t.Fatal(err)
		}
	}

	rec := f.do(adminUser, "GET", "/api/platform/accounts?limit=200", "")
	wantStatus(t, rec, 200)
	got := map[string]any{}
	reqs, _ := decode(t, rec)["requests"].([]any)
	for _, r := range reqs {
		row := r.(map[string]any)
		for name, id := range ids {
			if row["id"] == id {
				got[name] = row["logged_in"]
			}
		}
	}
	want := map[string]any{"used": true, "unused": false, "none": nil}
	for name, w := range want {
		if v, ok := got[name]; !ok || v != w {
			t.Errorf("%s: logged_in = %v (found %v), want %v", name, v, ok, w)
		}
	}
	one := decode(t, f.do(adminUser, "GET", "/api/platform/accounts/"+ids["used"], ""))
	if one["logged_in"] != true {
		t.Errorf("get: logged_in = %v, want true", one["logged_in"])
	}
}
