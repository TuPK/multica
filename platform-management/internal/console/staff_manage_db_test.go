package console

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// newTenantID makes a tenant for a test. The fixture removes it, and the audit
// rows that carry it, at the end of the test.
func (f *dbFixture) newTenantID() string {
	f.t.Helper()
	var id string
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO tenant (id, name) VALUES (gen_random_uuid(), 'Console Test tenant') RETURNING id::text`).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	f.tenantIDs = append(f.tenantIDs, id)
	return id
}

// staffUserInTenant creates a staff row with a tenant.
func (f *dbFixture) staffUserInTenant(role staff.Role, tenant string) (id, email string) {
	f.t.Helper()
	id, email = f.user("staff")
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO platform_staff (user_id, role, tenant_id) VALUES ($1, $2, $3)`, id, string(role), tenant); err != nil {
		f.t.Fatalf("create staff row: %v", err)
	}
	return id, email
}

func (f *dbFixture) staffRow(id string) (role, tenant string, active bool) {
	f.t.Helper()
	err := f.pool.QueryRow(context.Background(),
		`SELECT role, COALESCE(tenant_id::text, ''), revoked_at IS NULL FROM platform_staff
		 WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`, id).Scan(&role, &tenant, &active)
	if err != nil {
		f.t.Fatalf("read staff row %s: %v", id, err)
	}
	return role, tenant, active
}

func TestAddStaffAdminRequiresTenantID(t *testing.T) {
	f := newDBFixture(t)
	sysID, _ := f.staffUser(staff.RoleSystemAdmin)
	hireID, hireEmail := f.user("hire")
	tenant := f.newTenantID()

	rec := f.do(sysID, "POST", "/api/platform/staff", `{"email":"`+hireEmail+`","role":"admin"}`)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"code":"tenant_required"`) {
		t.Fatalf("admin without tenant = %d %s", rec.Code, rec.Body.String())
	}
	if n := f.count(`SELECT count(*) FROM platform_staff WHERE user_id = $1`, hireID); n != 0 {
		t.Fatalf("a refused request wrote %d staff rows", n)
	}

	rec = f.do(sysID, "POST", "/api/platform/staff", `{"email":"`+hireEmail+`","role":"admin","tenant_id":"`+tenant+`"}`)
	if rec.Code != 201 {
		t.Fatalf("admin with tenant = %d %s", rec.Code, rec.Body.String())
	}
	if role, got, active := f.staffRow(hireID); role != "admin" || got != tenant || !active {
		t.Fatalf("row = %q %q %v", role, got, active)
	}
}

func TestAddStaffOtherRolesRejectTenantID(t *testing.T) {
	f := newDBFixture(t)
	sysID, _ := f.staffUser(staff.RoleSystemAdmin)
	hireID, hireEmail := f.user("hire")
	tenant := f.newTenantID()
	for _, role := range []string{"system_admin", "support", "billing"} {
		rec := f.do(sysID, "POST", "/api/platform/staff", `{"email":"`+hireEmail+`","role":"`+role+`","tenant_id":"`+tenant+`"}`)
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"code":"tenant_not_allowed"`) {
			t.Errorf("%s with tenant = %d %s", role, rec.Code, rec.Body.String())
		}
	}
	if n := f.count(`SELECT count(*) FROM platform_staff WHERE user_id = $1`, hireID); n != 0 {
		t.Fatalf("a refused request wrote %d staff rows", n)
	}
}

// Only system_admin reaches /staff. Every other role gets the 404 of an unknown
// route and nothing is written.
func TestStaffRoutesAreSystemAdminOnly(t *testing.T) {
	f := newDBFixture(t)
	tenant := f.newTenantID()
	targetID, targetEmail := f.staffUser(staff.RoleSupport)
	callers := map[string]string{}
	callers["admin"], _ = f.staffUserInTenant(staff.RoleAdmin, tenant)
	callers["support"], _ = f.staffUser(staff.RoleSupport)
	callers["billing"], _ = f.staffUser(staff.RoleBilling)
	for name, id := range callers {
		for _, c := range []struct{ method, path, body string }{
			{"GET", "/api/platform/staff", ""},
			{"POST", "/api/platform/staff", `{"email":"` + targetEmail + `","role":"billing"}`},
			{"PUT", "/api/platform/staff/" + targetID, `{"role":"system_admin"}`},
			{"DELETE", "/api/platform/staff/" + targetID, ""},
		} {
			rec := f.do(id, c.method, c.path, c.body)
			if rec.Code != 404 || rec.Body.String() != "404 page not found\n" {
				t.Errorf("%s %s %s = %d %q, want the 404 of an unknown route", name, c.method, c.path, rec.Code, rec.Body.String())
			}
		}
	}
	if role, _, active := f.staffRow(targetID); role != "support" || !active {
		t.Fatalf("the target changed: %q %v", role, active)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE target_id = $1`, targetID); n != 0 {
		t.Fatalf("a refused request wrote %d audit rows", n)
	}
}

func TestPutStaffChangesRoleAndTenantAndAuditsInOneTransaction(t *testing.T) {
	f := newDBFixture(t)
	sysID, sysEmail := f.staffUser(staff.RoleSystemAdmin)
	targetID, _ := f.staffUser(staff.RoleSupport)
	t1, t2 := f.newTenantID(), f.newTenantID()

	if rec := f.do(sysID, "PUT", "/api/platform/staff/"+targetID, `{"role":"admin","tenant_id":"`+t1+`"}`); rec.Code != 200 {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body.String())
	}
	if role, tenant, active := f.staffRow(targetID); role != "admin" || tenant != t1 || !active {
		t.Fatalf("row = %q %q %v", role, tenant, active)
	}
	for _, action := range []string{audit.ActionStaffRoleChanged, audit.ActionStaffTenantAssigned} {
		var actorType, actorLabel, tenantOnRow, oldRole, newRole string
		err := f.pool.QueryRow(context.Background(), `
			SELECT actor_type, actor_label, COALESCE(tenant_id::text, ''), detail->>'old_role', detail->>'new_role'
			FROM audit_event WHERE action = $1 AND target_id = $2`, action, targetID).
			Scan(&actorType, &actorLabel, &tenantOnRow, &oldRole, &newRole)
		if err != nil {
			t.Fatalf("%s row: %v", action, err)
		}
		if actorType != "staff" || actorLabel != sysEmail || tenantOnRow != t1 || oldRole != "support" || newRole != "admin" {
			t.Errorf("%s row = %s %s %s %s %s", action, actorType, actorLabel, tenantOnRow, oldRole, newRole)
		}
	}

	if rec := f.do(sysID, "PUT", "/api/platform/staff/"+targetID, `{"role":"admin","tenant_id":"`+t2+`"}`); rec.Code != 200 {
		t.Fatalf("move = %d %s", rec.Code, rec.Body.String())
	}
	if _, tenant, _ := f.staffRow(targetID); tenant != t2 {
		t.Fatalf("tenant = %q, want %q", tenant, t2)
	}

	if rec := f.do(sysID, "PUT", "/api/platform/staff/"+targetID, `{"role":"billing"}`); rec.Code != 200 {
		t.Fatalf("demote = %d %s", rec.Code, rec.Body.String())
	}
	if role, tenant, _ := f.staffRow(targetID); role != "billing" || tenant != "" {
		t.Fatalf("row = %q %q, want billing with no tenant", role, tenant)
	}
}

// failingAuditRunner runs the real transaction with an audit writer that fails,
// to show the change does not outlive its audit row (I7).
type failingAuditRunner struct{ Runner }

func (r failingAuditRunner) InTx(ctx context.Context, fn func(Tx) error) error {
	return r.Runner.InTx(ctx, func(tx Tx) error { return fn(failingAuditTx{tx}) })
}

type failingAuditTx struct{ Tx }

func (failingAuditTx) InsertAuditEvent(context.Context, audit.Entry) error {
	return errors.New("audit insert failed")
}

func TestPutStaffAuditFailureRollsBack(t *testing.T) {
	f := newDBFixture(t)
	sysID, _ := f.staffUser(staff.RoleSystemAdmin)
	targetID, targetEmail := f.staffUser(staff.RoleSupport)
	tenant := f.newTenantID()
	deps := f.deps
	deps.Runner = failingAuditRunner{deps.Runner}
	h := New(deps)

	call := func(method, path, body string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, f.request(sysID, method, path, body))
		return rec.Code
	}
	if code := call("PUT", "/api/platform/staff/"+targetID, `{"role":"admin","tenant_id":"`+tenant+`"}`); code != 500 {
		t.Fatalf("PUT with a failing audit = %d, want 500", code)
	}
	if role, got, active := f.staffRow(targetID); role != "support" || got != "" || !active {
		t.Fatalf("the change survived the audit failure: %q %q %v", role, got, active)
	}
	if code := call("DELETE", "/api/platform/staff/"+targetID, ""); code != 500 {
		t.Fatalf("DELETE with a failing audit = %d, want 500", code)
	}
	if _, _, active := f.staffRow(targetID); !active {
		t.Fatal("the removal survived the audit failure")
	}
	_ = targetEmail
}

// The four staff handlers check the role themselves. A request that reaches them
// without the route's role list, as a wrong table would let it, changes nothing.
func TestStaffHandlersRefuseNonSystemAdminWhenCalledDirectly(t *testing.T) {
	f := newDBFixture(t)
	tenant := f.newTenantID()
	targetID, targetEmail := f.staffUser(staff.RoleSupport)
	callers := map[string]string{}
	callers["admin"], _ = f.staffUserInTenant(staff.RoleAdmin, tenant)
	callers["support"], _ = f.staffUser(staff.RoleSupport)
	callers["billing"], _ = f.staffUser(staff.RoleBilling)

	handlers := map[string]struct {
		fn     func(http.ResponseWriter, *http.Request)
		method string
		path   string
		body   string
	}{
		"list":   {f.console.listStaff, "GET", "/api/platform/staff", ""},
		"add":    {f.console.addStaff, "POST", "/api/platform/staff", `{"email":"` + targetEmail + `","role":"billing"}`},
		"update": {f.console.updateStaff, "PUT", "/api/platform/staff/" + targetID, `{"role":"system_admin"}`},
		"remove": {f.console.removeStaff, "DELETE", "/api/platform/staff/" + targetID, ""},
	}
	for who, id := range callers {
		for name, c := range handlers {
			mux := http.NewServeMux()
			mux.Handle(c.method+" /api/platform/staff", f.auth.RequirePlatformStaff()(http.HandlerFunc(c.fn)))
			mux.Handle(c.method+" /api/platform/staff/{user_id}", f.auth.RequirePlatformStaff()(http.HandlerFunc(c.fn)))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, f.request(id, c.method, c.path, c.body))
			if rec.Code != 404 || rec.Body.String() != "404 page not found\n" {
				t.Errorf("%s calling %s = %d %q, want a flat 404", who, name, rec.Code, rec.Body.String())
			}
		}
	}
	if role, _, active := f.staffRow(targetID); role != "support" || !active {
		t.Fatalf("the target changed: %q %v", role, active)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE target_id = $1`, targetID); n != 0 {
		t.Fatalf("a refused call wrote %d audit rows", n)
	}
}

func TestAdminCannotRaiseOwnOrOthersRole(t *testing.T) {
	f := newDBFixture(t)
	tenant := f.newTenantID()
	adminID, _ := f.staffUserInTenant(staff.RoleAdmin, tenant)
	otherID, _ := f.staffUser(staff.RoleSupport)
	for _, id := range []string{adminID, otherID} {
		rec := f.do(adminID, "PUT", "/api/platform/staff/"+id, `{"role":"system_admin"}`)
		if rec.Code != 404 {
			t.Errorf("admin raising %s = %d, want 404", id, rec.Code)
		}
	}
	if role, got, _ := f.staffRow(adminID); role != "admin" || got != tenant {
		t.Fatalf("admin row = %q %q", role, got)
	}
	if role, _, _ := f.staffRow(otherID); role != "support" {
		t.Fatalf("support row = %q", role)
	}
}

// The role is read from the database at each request (spec R6 section 3.5, rule 3).
func TestRoleChangeTakesEffectOnNextRequest(t *testing.T) {
	f := newDBFixture(t)
	boss, _ := f.staffUser(staff.RoleSystemAdmin)
	second, _ := f.staffUser(staff.RoleSystemAdmin)
	if rec := f.do(second, "GET", "/api/platform/staff", ""); rec.Code != 200 {
		t.Fatalf("system_admin GET /staff = %d", rec.Code)
	}
	if rec := f.do(boss, "PUT", "/api/platform/staff/"+second, `{"role":"support"}`); rec.Code != 200 {
		t.Fatalf("demote = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do(second, "GET", "/api/platform/staff", ""); rec.Code != 404 {
		t.Fatalf("demoted user GET /staff = %d, want 404 on the next request", rec.Code)
	}
	if rec := f.do(boss, "PUT", "/api/platform/staff/"+second, `{"role":"system_admin"}`); rec.Code != 200 {
		t.Fatalf("promote = %d", rec.Code)
	}
	if rec := f.do(second, "GET", "/api/platform/staff", ""); rec.Code != 200 {
		t.Fatalf("promoted user GET /staff = %d, want 200 on the next request", rec.Code)
	}
}

// Two requests that each take one of the last two system admins away must not
// both succeed.
func TestLastSystemAdminCannotBeDemotedOrRevokedConcurrently(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	a, _ := f.staffUser(staff.RoleSystemAdmin)
	b, _ := f.staffUser(staff.RoleSystemAdmin)

	// The test database is shared: park the other system admins.
	rows, err := f.pool.Query(ctx,
		`UPDATE platform_staff SET revoked_at = now()
		 WHERE role = 'system_admin' AND revoked_at IS NULL AND user_id::text <> ALL($1::text[])
		 RETURNING user_id::text, created_at`, []string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	type parked struct {
		id string
		at any
	}
	var others []parked
	for rows.Next() {
		var p parked
		if err := rows.Scan(&p.id, &p.at); err != nil {
			t.Fatal(err)
		}
		others = append(others, p)
	}
	rows.Close()
	t.Cleanup(func() {
		for _, p := range others {
			f.pool.Exec(ctx, `UPDATE platform_staff SET revoked_at = NULL WHERE user_id = $1 AND created_at = $2 AND role = 'system_admin'`, p.id, p.at)
		}
	})

	for round := 0; round < 6; round++ {
		var wg sync.WaitGroup
		codes := make([]int, 2)
		wg.Add(2)
		start := make(chan struct{})
		go func() { defer wg.Done(); <-start; codes[0] = f.do(a, "DELETE", "/api/platform/staff/"+b, "").Code }()
		go func() {
			defer wg.Done()
			<-start
			codes[1] = f.do(b, "PUT", "/api/platform/staff/"+a, `{"role":"support"}`).Code
		}()
		close(start)
		wg.Wait()
		n := f.count(`SELECT count(*) FROM platform_staff WHERE role = 'system_admin' AND revoked_at IS NULL AND user_id::text = ANY($1::text[])`, []string{a, b})
		if n < 1 {
			t.Fatalf("round %d: both system admins were taken away (codes %v)", round, codes)
		}
		// Put both back for the next round.
		f.pool.Exec(ctx, `UPDATE platform_staff SET revoked_at = NULL, role = 'system_admin', tenant_id = NULL WHERE user_id::text = ANY($1::text[])`, []string{a, b})
	}
}

func TestStaffAuditEntriesCarryTenantID(t *testing.T) {
	f := newDBFixture(t)
	sysID, _ := f.staffUser(staff.RoleSystemAdmin)
	hireID, hireEmail := f.user("hire")
	plainID, plainEmail := f.user("plain")
	tenant := f.newTenantID()

	if rec := f.do(sysID, "POST", "/api/platform/staff", `{"email":"`+hireEmail+`","role":"admin","tenant_id":"`+tenant+`"}`); rec.Code != 201 {
		t.Fatalf("add admin = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do(sysID, "POST", "/api/platform/staff", `{"email":"`+plainEmail+`","role":"support"}`); rec.Code != 201 {
		t.Fatalf("add support = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do(sysID, "DELETE", "/api/platform/staff/"+hireID, ""); rec.Code != 200 {
		t.Fatalf("remove admin = %d", rec.Code)
	}
	tenantOf := func(action, target string) string {
		var tenantOnRow string
		if err := f.pool.QueryRow(context.Background(),
			`SELECT COALESCE(tenant_id::text, '') FROM audit_event WHERE action = $1 AND target_id = $2`, action, target).Scan(&tenantOnRow); err != nil {
			t.Fatalf("%s row of %s: %v", action, target, err)
		}
		return tenantOnRow
	}
	if got := tenantOf(audit.ActionStaffAdded, hireID); got != tenant {
		t.Errorf("staff.added tenant_id = %q, want %q", got, tenant)
	}
	if got := tenantOf(audit.ActionStaffRemoved, hireID); got != tenant {
		t.Errorf("staff.removed tenant_id = %q, want %q", got, tenant)
	}
	if got := tenantOf(audit.ActionStaffAdded, plainID); got != "" {
		t.Errorf("staff.added for support has tenant_id %q, want NULL", got)
	}
}
