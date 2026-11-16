package console

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/internal/accountrequest"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/platformauth"
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
		Emails: store,
	})
	t.Cleanup(func() {
		f.pool.Exec(context.Background(), `DELETE FROM platform_account_request WHERE email LIKE 'acct-db-%'`)
	})
	return f
}

func TestAccountRequestDBAdminCreatesListsAndCancels(t *testing.T) {
	f := accountsDBFixture(t, 30)
	adminUser, adminEmail := f.staffUser(staff.RoleAdmin)
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

func TestAccountCreateDBRateLimitAnswers429WithRetryAfter(t *testing.T) {
	f := accountsDBFixture(t, 1)
	adminUser, _ := f.staffUser(staff.RoleAdmin)
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
