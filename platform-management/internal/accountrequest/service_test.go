package accountrequest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
)

func wantError(t *testing.T, err error, status int, code string) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error = %v, want *Error %d %s", err, status, code)
	}
	if e.Status != status || e.Code != code {
		t.Fatalf("error = %d %s, want %d %s", e.Status, e.Code, status, code)
	}
	return e
}

func TestAccountRequestCreateStoresPendingRequestAndOneAuditRow(t *testing.T) {
	e := newEnv(30)
	r, created, err := e.svc.Create(context.Background(), admin(adminA), goodIndividual())
	if err != nil || !created {
		t.Fatalf("created = %v, err = %v", created, err)
	}
	if r.State != StatePending || r.Email != "ana@example.com" || r.PlanKey != "free" || r.CreatedBy != adminA || r.ID == "" {
		t.Errorf("request = %+v", r)
	}
	if got := e.auditActions(); len(got) != 1 || got[0] != "account.provision_requested" {
		t.Fatalf("audit actions = %v, want one account.provision_requested", got)
	}
	a := e.store.audits[0]
	if a.ActorType != audit.ActorStaff || a.ActorID != adminA || a.WorkspaceID != "" || a.TargetLabel != "ana@example.com" || a.TargetID != r.ID {
		t.Errorf("audit entry = %+v", a)
	}
	// "plan_key" would be redacted by the audit filter (it contains "key").
	if a.Detail["plan"] != "free" || a.Detail["kind"] != "individual" {
		t.Errorf("audit detail = %v", a.Detail)
	}
}

func TestAccountRequestCreateRefusesBadInputBeforeAnyWrite(t *testing.T) {
	e := newEnv(30)
	in := goodIndividual()
	in.Email = "nope"
	_, _, err := e.svc.Create(context.Background(), admin(adminA), in)
	if e := wantError(t, err, 400, "account_request_invalid"); len(e.Fields) != 1 || e.Fields[0] != "email" {
		t.Errorf("fields = %v", e.Fields)
	}
	if e.count() != 0 || len(e.limiter.calls) != 0 || len(e.store.audits) != 0 {
		t.Errorf("a refused input wrote %d rows, %d audits, counted %d", e.count(), len(e.store.audits), len(e.limiter.calls))
	}
}

func TestAccountRequestUnknownPlanIsInvalid(t *testing.T) {
	e := newEnv(30)
	in := goodIndividual()
	in.PlanKey = "nope"
	_, _, err := e.svc.Create(context.Background(), admin(adminA), in)
	if e := wantError(t, err, 400, "account_request_invalid"); len(e.Fields) != 1 || e.Fields[0] != "plan_key" {
		t.Errorf("fields = %v", e.Fields)
	}
}

func TestAccountRequestPlanChecksRunOnCreate(t *testing.T) {
	e := newEnv(30)
	in := goodIndividual()
	in.PlanKey = "business"
	_, _, err := e.svc.Create(context.Background(), admin(adminA), in)
	wantError(t, err, 409, "plan_audience_mismatch")
	in.PlanKey = "pro"
	_, _, err = e.svc.Create(context.Background(), admin(adminA), in)
	wantError(t, err, 400, "account_activation_required")
	if e.count() != 0 {
		t.Errorf("%d rows after refused creates", e.count())
	}
}

// Rule 4: an email that belongs to a user is refused and nothing changes. The
// check ignores case.
func TestAccountRequestEmailOfAnExistingUserIsRefused(t *testing.T) {
	e := newEnv(30)
	e.store.users["ana@example.com"] = "user-1"
	in := goodIndividual() // "Ana@Example.COM"
	_, _, err := e.svc.Create(context.Background(), admin(adminA), in)
	wantError(t, err, 409, "account_email_exists")
	if e.count() != 0 || len(e.store.audits) != 0 {
		t.Errorf("a refused create wrote %d rows and %d audits", e.count(), len(e.store.audits))
	}
	if len(e.limiter.calls) != 0 {
		t.Errorf("a refused create used %d rate limit slots", len(e.limiter.calls))
	}
}

// Rule 5: the same email while a request is live returns that request and
// creates nothing, whatever the case of the email.
func TestAccountRequestSecondRequestForALiveEmailReturnsTheFirst(t *testing.T) {
	e := newEnv(30)
	first, _, err := e.svc.Create(context.Background(), admin(adminA), goodIndividual())
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{StatePending, StateProvisioned} {
		r := e.store.rows[first.ID]
		r.State = state
		e.store.rows[first.ID] = r
		again := goodIndividual()
		again.Email = "ANA@example.com"
		again.DisplayName = "Someone else"
		got, created, err := e.svc.Create(context.Background(), admin(adminB), again)
		if err != nil || created {
			t.Fatalf("state %s: created = %v, err = %v", state, created, err)
		}
		if got.ID != first.ID || got.DisplayName != "Ana" {
			t.Errorf("state %s: got %+v, want the first request unchanged", state, got)
		}
	}
	if e.count() != 1 || len(e.store.audits) != 1 {
		t.Errorf("%d rows and %d audits, want 1 and 1", e.count(), len(e.store.audits))
	}
	if len(e.limiter.calls) != 1 {
		t.Errorf("limiter calls = %d, want only the first create counted", len(e.limiter.calls))
	}
}

// A closed request does not hold the email.
func TestAccountRequestCanceledRequestFreesTheEmail(t *testing.T) {
	e := newEnv(30)
	first, _, _ := e.svc.Create(context.Background(), admin(adminA), goodIndividual())
	if _, err := e.svc.Cancel(context.Background(), admin(adminA), first.ID); err != nil {
		t.Fatal(err)
	}
	second, created, err := e.svc.Create(context.Background(), admin(adminA), goodIndividual())
	if err != nil || !created || second.ID == first.ID {
		t.Fatalf("second = %+v created = %v err = %v", second, created, err)
	}
}

// The unique index is the backstop: when the insert loses to a request that
// committed first, the service returns the winner and writes no audit row.
func TestAccountRequestInsertThatLosesTheRaceReturnsTheWinner(t *testing.T) {
	e := newEnv(30)
	e.store.racer = &Request{ID: "20000000-0000-4000-8000-000000000001", Email: "ana@example.com", State: StatePending, DisplayName: "Winner"}
	got, created, err := e.svc.Create(context.Background(), admin(adminA), goodIndividual())
	if err != nil || created {
		t.Fatalf("created = %v err = %v", created, err)
	}
	if got.DisplayName != "Winner" {
		t.Errorf("got %+v, want the racing request", got)
	}
	if len(e.store.audits) != 0 {
		t.Errorf("%d audit rows for a create that wrote nothing", len(e.store.audits))
	}
}

// Rule 1: PLATFORM_ACCOUNT_CREATE_PER_HOUR per staff, counted in Redis.
func TestAccountCreateRateLimitRefusesTheRequestBeyondTheLimit(t *testing.T) {
	e := newEnv(2)
	create := func(actor string, n int) error {
		in := goodIndividual()
		in.Email = strings.Repeat("u", n) + "@example.com"
		_, _, err := e.svc.Create(context.Background(), admin(actor), in)
		return err
	}
	for n := 1; n <= 2; n++ {
		if err := create(adminA, n); err != nil {
			t.Fatalf("request %d: %v", n, err)
		}
	}
	err := create(adminA, 3)
	rl := wantError(t, err, 429, "account_create_rate_limited")
	if rl.RetryAfter <= 0 || rl.RetryAfter > time.Hour {
		t.Errorf("RetryAfter = %v, want within the hour", rl.RetryAfter)
	}
	if e.count() != 2 || len(e.store.audits) != 2 {
		t.Errorf("%d rows and %d audits, want 2 and 2: the refused request wrote something", e.count(), len(e.store.audits))
	}
	// Another staff member has their own count.
	if err := create(adminB, 4); err != nil {
		t.Errorf("a second staff member is refused: %v", err)
	}
	for _, c := range e.limiter.calls {
		if c.limit != 2 || c.window != time.Hour {
			t.Errorf("limiter call = %+v, want limit 2 per hour", c)
		}
		if !strings.Contains(c.key, adminA) && !strings.Contains(c.key, adminB) {
			t.Errorf("limiter key %q does not name the staff member", c.key)
		}
	}
	if e.limiter.calls[0].key == e.limiter.calls[len(e.limiter.calls)-1].key {
		t.Error("two staff members share one limiter key")
	}
}

// The limiter key is per hour window of the limiter itself: the count resets
// when the limiter's window expires, which the Redis limiter does.
func TestAccountCreateRateLimitCountsOnlyAfterTheChecksPass(t *testing.T) {
	e := newEnv(1)
	bad := goodIndividual()
	bad.Email = "bad"
	e.svc.Create(context.Background(), admin(adminA), bad)
	if _, _, err := e.svc.Create(context.Background(), admin(adminA), goodIndividual()); err != nil {
		t.Fatalf("a refused input used the staff member's slot: %v", err)
	}
}

func TestAccountCreateRateLimiterFailureCreatesNothing(t *testing.T) {
	e := newEnv(5)
	e.limiter.err = errors.New("redis down")
	_, _, err := e.svc.Create(context.Background(), admin(adminA), goodIndividual())
	if err == nil {
		t.Fatal("no error when the limiter fails")
	}
	var ae *Error
	if errors.As(err, &ae) {
		t.Errorf("a limiter failure became an API error %d %s", ae.Status, ae.Code)
	}
	if e.count() != 0 {
		t.Errorf("%d rows after a limiter failure", e.count())
	}
}

// I7: no audit row, no request.
func TestAccountRequestAuditFailureCreatesNothing(t *testing.T) {
	e := newEnv(30)
	e.store.auditErr = errors.New("audit write failed")
	_, created, err := e.svc.Create(context.Background(), admin(adminA), goodIndividual())
	if err == nil || created {
		t.Fatalf("created = %v, err = %v, want an error", created, err)
	}
	if e.count() != 0 {
		t.Errorf("%d rows after a failed audit write", e.count())
	}
}

// Rule 12.
func TestAccountRequestCancelPendingWritesTheAuditRow(t *testing.T) {
	e := newEnv(30)
	r, _, _ := e.svc.Create(context.Background(), admin(adminA), goodIndividual())
	got, err := e.svc.Cancel(context.Background(), admin(adminB), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateCanceled || e.store.rows[r.ID].State != StateCanceled {
		t.Errorf("state = %s", got.State)
	}
	acts := e.auditActions()
	if len(acts) != 2 || acts[1] != "account.provision_canceled" {
		t.Fatalf("audit actions = %v", acts)
	}
	a := e.store.audits[1]
	if a.ActorType != audit.ActorStaff || a.ActorID != adminB || a.TargetID != r.ID || a.TargetLabel != "ana@example.com" || a.WorkspaceID != "" {
		t.Errorf("audit entry = %+v", a)
	}
}

func TestAccountRequestCancelRefusesEveryStateButPending(t *testing.T) {
	for _, state := range []string{StateProvisioned, StateCompleted, StateFailed, StateCanceled} {
		e := newEnv(30)
		r, _, _ := e.svc.Create(context.Background(), admin(adminA), goodIndividual())
		row := e.store.rows[r.ID]
		row.State = state
		e.store.rows[r.ID] = row
		before := len(e.store.audits)
		_, err := e.svc.Cancel(context.Background(), admin(adminA), r.ID)
		ae := wantError(t, err, 409, "account_request_not_cancelable")
		if ae.State != state {
			t.Errorf("%s: error state = %q", state, ae.State)
		}
		if e.store.rows[r.ID].State != state || len(e.store.audits) != before {
			t.Errorf("%s: a refused cancel changed the request or wrote audit", state)
		}
	}
}

func TestAccountRequestCancelUnknownIsNotFound(t *testing.T) {
	e := newEnv(30)
	if _, err := e.svc.Cancel(context.Background(), admin(adminA), "30000000-0000-4000-8000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestAccountRequestCancelAuditFailureKeepsThePendingState(t *testing.T) {
	e := newEnv(30)
	r, _, _ := e.svc.Create(context.Background(), admin(adminA), goodIndividual())
	e.store.auditErr = errors.New("audit write failed")
	if _, err := e.svc.Cancel(context.Background(), admin(adminA), r.ID); err == nil {
		t.Fatal("no error")
	}
	if e.store.rows[r.ID].State != StatePending {
		t.Errorf("state = %s, want pending", e.store.rows[r.ID].State)
	}
}
