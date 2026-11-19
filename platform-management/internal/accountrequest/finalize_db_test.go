package accountrequest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// Database tests of step 3 and of the welcome (spec 7b.5, rules 7 and 9). Step 2
// is Multica's job; provisioned writes the rows it writes: the user, the
// workspace, its owner, and the request moved to provisioned.

const finalizeAppURL = "https://app.example.test"

var finalizeNow = time.Date(2026, 5, 4, 9, 30, 0, 0, time.UTC)

type finEnv struct {
	*dbEnv
	staffEmail string
	store      *PgxFinalizeStore
	mail       *fakeWelcomeMailer
	cache      *fakeInvalidator
	fin        *Finalizer
	svc        *Service
	workspaces []string
}

func newFinEnv(t *testing.T) *finEnv {
	t.Helper()
	e := newDBEnv(t)
	fe := &finEnv{dbEnv: e, staffEmail: e.email("staff"), mail: &fakeWelcomeMailer{}, cache: &fakeInvalidator{}}
	ctx := context.Background()
	if err := e.pool.QueryRow(ctx, `INSERT INTO "user" (name, email) VALUES ('Staff', $1) RETURNING id::text`, fe.staffEmail).Scan(&e.staff); err != nil {
		t.Fatal(err)
	}
	fe.store = NewPgxFinalizeStore(e.pool)
	fe.fin = fe.finalizer(fe.store, fe.mail)
	fe.svc = New(Deps{
		Runner: NewPgxRunner(e.pool), Reader: NewPgxReader(e.pool), Plans: plan.NewPgxReader(e.pool), Limiter: e.limiter,
		CreatePerHour: 100, IndividualMaxSeats: 5, Welcome: fe.fin, Now: func() time.Time { return finalizeNow },
	})
	t.Cleanup(func() {
		for _, ws := range fe.workspaces {
			e.pool.Exec(ctx, `DELETE FROM audit_event WHERE workspace_id = $1`, ws)
			e.pool.Exec(ctx, `DELETE FROM billing_order WHERE workspace_id = $1`, ws)
			e.pool.Exec(ctx, `DELETE FROM billing_profile WHERE workspace_id = $1`, ws)
			e.pool.Exec(ctx, `DELETE FROM workspace_subscription WHERE workspace_id = $1`, ws)
			e.pool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, ws)
		}
		e.pool.Exec(ctx, `DELETE FROM audit_event WHERE target_id IN
			(SELECT id::text FROM platform_account_request WHERE email LIKE $1)`, e.tag+"%")
		e.pool.Exec(ctx, `DELETE FROM "user" WHERE email LIKE $1`, e.tag+"%")
	})
	return fe
}

func (e *finEnv) finalizer(store FinalizeStore, m WelcomeMailer) *Finalizer {
	f, err := NewFinalizer(FinalizerDeps{
		Store: store, Plans: plan.NewPgxReader(e.pool), Cache: e.cache, Mailer: m, AppURL: finalizeAppURL,
		Now: func() time.Time { return finalizeNow },
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return f
}

func (e *finEnv) actor() Actor { return Actor{ID: e.staff, Label: e.staffEmail} }

// bizPlan stores the business seat plan with a trial of 14 days.
func (e *finEnv) bizPlan() string { return e.testPlan() }

// usagePlan stores a pay as you go plan under a key of its own.
func (e *finEnv) usagePlan() string {
	e.t.Helper()
	key := "u" + e.tag[3:]
	p := plan.Plan{Key: key, Name: "Metered", Currency: "vnd", Audience: plan.AudienceAny, BillingModel: plan.BillingUsage}
	err := plan.NewPgxRunner(e.pool).InTx(context.Background(), func(tx plan.Tx) error { return tx.Upsert(context.Background(), p) })
	if err != nil {
		e.t.Fatalf("store usage plan: %v", err)
	}
	e.t.Cleanup(func() { e.pool.Exec(context.Background(), `DELETE FROM plan WHERE key = $1`, key) })
	return key
}

func (e *finEnv) businessInput(n, planKey string) Input {
	in := goodBusiness()
	in.Email = e.email(n)
	in.PlanKey = planKey
	in.Seats = 6
	return in
}

// provisioned creates the request through the service, then does what Multica's
// step 2 does: a user, a workspace with the user as owner, and the state.
func (e *finEnv) provisioned(in Input) Request {
	e.t.Helper()
	r, _, err := e.svc.Create(context.Background(), e.actor(), in)
	if err != nil {
		e.t.Fatalf("create: %+v", err)
	}
	return e.step2(r)
}

// step2 does what Multica's account_provisioning job does to a pending request.
func (e *finEnv) step2(r Request) Request {
	e.t.Helper()
	ctx := context.Background()
	var userID, wsID string
	if err := e.pool.QueryRow(ctx, `INSERT INTO "user" (name, email) VALUES ($1, $2) RETURNING id::text`, r.DisplayName, r.Email).Scan(&userID); err != nil {
		e.t.Fatal(err)
	}
	if err := e.pool.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ($1, $2) RETURNING id::text`, r.WorkspaceName, e.tag+"-"+r.ID[len(r.ID)-6:]).Scan(&wsID); err != nil {
		e.t.Fatal(err)
	}
	e.workspaces = append(e.workspaces, wsID)
	if _, err := e.pool.Exec(ctx, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, wsID, userID); err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE platform_account_request SET state = 'provisioned', user_id = $2, workspace_id = $3 WHERE id = $1`, r.ID, userID, wsID); err != nil {
		e.t.Fatal(err)
	}
	return e.get(r.ID)
}

func (e *finEnv) get(id string) Request {
	e.t.Helper()
	r, found, err := NewPgxReader(e.pool).Get(context.Background(), id)
	if err != nil || !found {
		e.t.Fatalf("get %s: %v %v", id, found, err)
	}
	return r
}

func (e *finEnv) run() {
	e.t.Helper()
	if err := e.fin.RunOnce(context.Background()); err != nil {
		e.t.Fatalf("RunOnce: %v", err)
	}
}

func (e *finEnv) sub(ws string) (subscription.Subscription, bool) {
	e.t.Helper()
	s, found, err := subscription.NewPgxReader(e.pool).Get(context.Background(), ws)
	if err != nil {
		e.t.Fatal(err)
	}
	return s, found
}

func (e *finEnv) auditCount(ws, action string) int {
	e.t.Helper()
	return e.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = $2`, ws, action)
}

func (e *finEnv) requestAuditCount(id, action string) int {
	e.t.Helper()
	return e.count(`SELECT count(*) FROM audit_event WHERE target_id = $1 AND action = $2`, id, action)
}

type orderRow struct {
	Kind, Plan, Interval, Currency, Status, Provider, ConfirmedBy, Transfer string
	Seats                                                                   int
	Amount, VAT                                                             int64
	PaidAt                                                                  *time.Time
}

func (e *finEnv) orders(ws string) []orderRow {
	e.t.Helper()
	rows, err := e.pool.Query(context.Background(),
		`SELECT kind, plan_key, interval, currency, status, provider, COALESCE(confirmed_by::text, ''), transfer_code,
		        seats, amount, vat_amount, paid_at
		 FROM billing_order WHERE workspace_id = $1 ORDER BY created_at`, ws)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []orderRow
	for rows.Next() {
		var o orderRow
		if err := rows.Scan(&o.Kind, &o.Plan, &o.Interval, &o.Currency, &o.Status, &o.Provider, &o.ConfirmedBy, &o.Transfer, &o.Seats, &o.Amount, &o.VAT, &o.PaidAt); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, o)
	}
	return out
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func at(t time.Time) *time.Time { return &t }

func TestFinalizeWritesBillingProfileSubscriptionAndGrant(t *testing.T) {
	trialEnd := finalizeNow.Add(14 * 24 * time.Hour)
	compEnd, _ := billing.PeriodEnd(finalizeNow, "year")
	usageStart, usageEnd := subscription.UsagePeriod(finalizeNow)

	cases := []struct {
		name  string
		input func(e *finEnv) Input
		// want is the subscription row; nil means the request writes none.
		want *subscription.Subscription
		kind string
	}{
		{"free plan writes no subscription", func(e *finEnv) Input { return e.input("free") }, nil, KindIndividual},
		{"trial", func(e *finEnv) Input {
			in := e.businessInput("trial", e.bizPlan())
			in.Seats, in.Trial = 5, true
			return in
		}, &subscription.Subscription{
			Status: subscription.StatusTrialing, Seats: 5, Interval: "month",
			CurrentPeriodStart: at(finalizeNow), CurrentPeriodEnd: at(trialEnd), TrialEnd: at(trialEnd),
		}, KindBusiness},
		{"comp reason is one free interval", func(e *finEnv) Input {
			in := e.businessInput("comp", e.bizPlan())
			in.CompReason, in.Interval = "signed contract 42", "year"
			return in
		}, &subscription.Subscription{
			Status: subscription.StatusActive, Seats: 6, Interval: "year",
			CurrentPeriodStart: at(finalizeNow), CurrentPeriodEnd: at(compEnd),
		}, KindBusiness},
		{"usage plan activates at once", func(e *finEnv) Input {
			in := e.input("usage")
			in.PlanKey = e.usagePlan()
			return in
		}, &subscription.Subscription{
			Status: subscription.StatusActive, Seats: 0, Interval: "month",
			CurrentPeriodStart: at(usageStart), CurrentPeriodEnd: at(usageEnd),
		}, KindIndividual},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newFinEnv(t)
			e.fin.Mailer = nil
			in := c.input(e)
			r := e.provisioned(in)
			e.run()

			got := e.get(r.ID)
			if got.State != StateCompleted || got.ErrorCode != "" || got.BillingProfile != nil ||
				got.CompletedAt == nil || !got.CompletedAt.Equal(finalizeNow) {
				t.Errorf("request = state %s error %q profile %v completed %v", got.State, got.ErrorCode, got.BillingProfile, got.CompletedAt)
			}
			if got.UserID != r.UserID || got.WorkspaceID != r.WorkspaceID {
				t.Errorf("the user or the workspace of the request changed: %s %s", got.UserID, got.WorkspaceID)
			}
			ws := r.WorkspaceID

			sub, found := e.sub(ws)
			if c.want == nil {
				if found {
					t.Errorf("a subscription was written: %+v", sub)
				}
				if n := len(e.orders(ws)); n != 0 {
					t.Errorf("%d orders for a plan with nothing to grant", n)
				}
			} else {
				if !found {
					t.Fatal("no subscription row")
				}
				w := c.want
				if sub.PlanKey != in.PlanKey || sub.Status != w.Status || sub.Seats != w.Seats || sub.Interval != w.Interval ||
					!sameTime(sub.CurrentPeriodStart, w.CurrentPeriodStart) || !sameTime(sub.CurrentPeriodEnd, w.CurrentPeriodEnd) ||
					!sameTime(sub.TrialEnd, w.TrialEnd) || sub.GraceUntil != nil || sub.CancelAtPeriodEnd {
					t.Errorf("subscription = %+v, want %+v on %s", sub, *w, in.PlanKey)
				}
				if sub.Version != 1 {
					t.Errorf("version = %d, want 1 for the first write", sub.Version)
				}
				orders := e.orders(ws)
				if len(orders) != 1 {
					t.Fatalf("%d orders, want 1", len(orders))
				}
				o := orders[0]
				if o.Provider != "admin_grant" || o.Status != "paid" || o.Amount != 0 || o.VAT != 0 || o.ConfirmedBy != e.staff ||
					o.PaidAt == nil || !o.PaidAt.Equal(finalizeNow) || o.Kind != "subscribe" || o.Plan != in.PlanKey ||
					o.Interval != sub.Interval || o.Seats != sub.Seats || !billing.ValidTransferCode(o.Transfer) {
					t.Errorf("order = %+v", o)
				}
				for _, action := range []string{"subscription.changed", "billing.order_created", "billing.order_paid"} {
					if n := e.auditCount(ws, action); n != 1 {
						t.Errorf("%d %s audit rows, want 1", n, action)
					}
				}
			}

			profile, found, err := subscription.NewPgxProfiles(e.pool).Profile(context.Background(), ws)
			if err != nil {
				t.Fatal(err)
			}
			if c.kind == KindBusiness {
				if !found || profile.AccountKind != "business" || profile.LegalName != in.BillingProfile.LegalName ||
					profile.TaxCode != in.BillingProfile.TaxCode || profile.Address != in.BillingProfile.Address ||
					profile.InvoiceEmail != in.BillingProfile.InvoiceEmail ||
					profile.VerifiedAt == nil || !profile.VerifiedAt.Equal(finalizeNow) || profile.VerifiedBy != e.staff {
					t.Errorf("profile = %+v found %v", profile, found)
				}
			} else if found {
				t.Errorf("an individual account got a profile row: %+v", profile)
			}

			// Rule 13: the owner reads who made the account, in the workspace log.
			var actorType, actorID, label string
			err = e.pool.QueryRow(context.Background(),
				`SELECT actor_type, COALESCE(actor_id, ''), actor_label FROM audit_event WHERE workspace_id = $1 AND action = 'account.provisioned'`, ws).
				Scan(&actorType, &actorID, &label)
			if err != nil {
				t.Fatalf("account.provisioned row: %v", err)
			}
			if actorType != "staff" || actorID != e.staff || label != e.staffEmail {
				t.Errorf("account.provisioned actor = %s %s %s", actorType, actorID, label)
			}
			if !e.cache.has(ws) {
				t.Error("the workspace's cached policy was not dropped")
			}
		})
	}
}

// Spec 7b.5: the copy of a third party's billing profile in the request is
// removed once the request completes; the workspace keeps the real profile.
func TestFinalizeRemovesTheProfileCopyFromTheRequest(t *testing.T) {
	e := newFinEnv(t)
	e.fin.Mailer = nil
	in := e.businessInput("copy", e.bizPlan())
	in.Seats, in.Trial = 5, true
	r := e.provisioned(in)
	var before bool
	if err := e.pool.QueryRow(context.Background(),
		`SELECT billing_profile IS NOT NULL FROM platform_account_request WHERE id = $1`, r.ID).Scan(&before); err != nil || !before {
		t.Fatalf("the request has no profile copy before finalize: %v %v", before, err)
	}
	e.run()

	var state string
	var copyIsNull bool
	if err := e.pool.QueryRow(context.Background(),
		`SELECT state, billing_profile IS NULL FROM platform_account_request WHERE id = $1`, r.ID).Scan(&state, &copyIsNull); err != nil {
		t.Fatal(err)
	}
	if state != StateCompleted || !copyIsNull {
		t.Errorf("request state %s, billing_profile NULL %v; want completed and NULL", state, copyIsNull)
	}
	if n := e.count(`SELECT count(*) FROM billing_profile WHERE workspace_id = $1`, r.WorkspaceID); n != 1 {
		t.Errorf("%d billing_profile rows for the workspace, want 1", n)
	}
}

// A workspace that already has a row (the backfill gives each workspace the
// free plan) is moved on, and its version goes up by one.
func TestFinalizeIncrementsTheVersionOfAnExistingSubscription(t *testing.T) {
	e := newFinEnv(t)
	e.fin.Mailer = nil
	in := e.businessInput("exists", e.bizPlan())
	in.CompReason = "contract"
	r := e.provisioned(in)
	if _, err := e.pool.Exec(context.Background(),
		`INSERT INTO workspace_subscription (workspace_id, plan_key, status, seats, interval, version) VALUES ($1, 'free', 'active', 0, 'month', 4)`, r.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	e.run()
	sub, found := e.sub(r.WorkspaceID)
	if !found || sub.Version != 5 || sub.PlanKey != in.PlanKey || sub.Status != "active" {
		t.Errorf("subscription = %+v, want version 5 on %s", sub, in.PlanKey)
	}
	if got := e.get(r.ID); got.State != StateCompleted {
		t.Errorf("state = %s", got.State)
	}
}

func TestFinalizeIsIdempotentAndParallelSafe(t *testing.T) {
	e := newFinEnv(t)
	e.fin.Mailer = nil
	in := e.businessInput("par", e.bizPlan())
	in.CompReason = "contract"
	r := e.provisioned(in)

	const n = 6
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = e.fin.RunOnce(context.Background())
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("run %d: %v", i, err)
		}
	}
	e.run() // a later run finds nothing to do

	ws := r.WorkspaceID
	if c := e.count(`SELECT count(*) FROM workspace_subscription WHERE workspace_id = $1`, ws); c != 1 {
		t.Errorf("%d subscriptions, want 1", c)
	}
	if sub, _ := e.sub(ws); sub.Version != 1 {
		t.Errorf("version = %d, want 1", sub.Version)
	}
	if c := len(e.orders(ws)); c != 1 {
		t.Errorf("%d orders, want 1", c)
	}
	for _, action := range []string{"account.provisioned", "subscription.changed", "billing.order_created", "billing.order_paid"} {
		if c := e.auditCount(ws, action); c != 1 {
			t.Errorf("%d %s audit rows, want 1", c, action)
		}
	}
	if got := e.get(r.ID); got.State != StateCompleted || got.Attempts != 0 {
		t.Errorf("request = %s attempts %d", got.State, got.Attempts)
	}
}

// failingFinalizeStore makes Complete, the last write of step 3, fail, so the
// whole transaction rolls back. Recording the failure is not affected.
type failingFinalizeStore struct {
	FinalizeStore
	err error
}

func (s failingFinalizeStore) InTx(ctx context.Context, fn func(FinalizeTx) error) error {
	return s.FinalizeStore.InTx(ctx, func(tx FinalizeTx) error { return fn(failingComplete{tx, s.err}) })
}

type failingComplete struct {
	FinalizeTx
	err error
}

func (f failingComplete) Complete(context.Context, string, time.Time) (Request, error) {
	return Request{}, f.err
}

func (e *finEnv) assertAccountKept(r Request) {
	e.t.Helper()
	got := e.get(r.ID)
	if got.UserID != r.UserID || got.WorkspaceID != r.WorkspaceID {
		e.t.Errorf("user_id and workspace_id = %s %s, want them kept", got.UserID, got.WorkspaceID)
	}
	if c := e.count(`SELECT count(*) FROM "user" WHERE id = $1`, r.UserID); c != 1 {
		e.t.Error("the user is gone")
	}
	if c := e.count(`SELECT count(*) FROM member WHERE workspace_id = $1 AND user_id = $2 AND role = 'owner'`, r.WorkspaceID, r.UserID); c != 1 {
		e.t.Error("the workspace or its owner is gone")
	}
}

func TestFinalizeRetriesTemporaryErrorThreeTimes(t *testing.T) {
	e := newFinEnv(t)
	e.fin = e.finalizer(failingFinalizeStore{e.store, errors.New("connection reset")}, nil)
	in := e.businessInput("retry", e.bizPlan())
	in.CompReason = "contract"
	r := e.provisioned(in)
	ws := r.WorkspaceID

	for attempt := 1; attempt <= 2; attempt++ {
		e.run()
		got := e.get(r.ID)
		if got.State != StateProvisioned || got.Attempts != attempt || got.ErrorCode != "" {
			t.Fatalf("after run %d: state %s attempts %d error %q, want provisioned and %d", attempt, got.State, got.Attempts, got.ErrorCode, attempt)
		}
		// The failed attempt left nothing behind.
		if _, found := e.sub(ws); found || len(e.orders(ws)) != 0 || e.auditCount(ws, "account.provisioned") != 0 {
			t.Fatalf("run %d left a subscription, an order or an audit row", attempt)
		}
		if got.BillingProfile == nil {
			t.Fatal("the copy of the billing profile was cleared before the profile was written")
		}
	}
	e.run()
	got := e.get(r.ID)
	if got.State != StateFailed || got.Attempts != 3 || got.ErrorCode != "finalize_failed" {
		t.Fatalf("after run 3: state %s attempts %d error %q, want failed, 3, finalize_failed", got.State, got.Attempts, got.ErrorCode)
	}
	e.assertAccountKept(r)
	if c := e.requestAuditCount(r.ID, "account.provision_failed"); c != 1 {
		t.Errorf("%d account.provision_failed audit rows, want 1", c)
	}

	e.run() // a failed request is not picked up again
	if got := e.get(r.ID); got.Attempts != 3 {
		t.Errorf("attempts = %d after a later run, want 3", got.Attempts)
	}
}

func TestFinalizePermanentErrorFailsAtOnce(t *testing.T) {
	e := newFinEnv(t)
	e.fin = e.finalizer(failingFinalizeStore{e.store, Permanent(errors.New("the plan cannot be granted"))}, nil)
	r := e.provisioned(e.input("perm"))
	e.run()
	got := e.get(r.ID)
	if got.State != StateFailed || got.Attempts != 1 || got.ErrorCode != "finalize_failed" {
		t.Errorf("state %s attempts %d error %q, want failed, 1, finalize_failed", got.State, got.Attempts, got.ErrorCode)
	}
	e.assertAccountKept(r)
}

func TestFinalizePermanentWrapper(t *testing.T) {
	base := errors.New("boom")
	p := Permanent(base)
	if !IsPermanent(p) || !errors.Is(p, base) || p.Error() != "boom" {
		t.Errorf("Permanent(base) = %v, permanent %v", p, IsPermanent(p))
	}
	if !IsPermanent(errors.Join(errors.New("x"), p)) {
		t.Error("a wrapped permanent error is not permanent")
	}
	if IsPermanent(base) || IsPermanent(nil) || IsPermanent(context.DeadlineExceeded) {
		t.Error("an ordinary error is permanent")
	}
	if Permanent(nil) != nil {
		t.Error("Permanent(nil) is not nil")
	}
}

// The plan of the request was deleted after the request was made: step 2 has
// created the user and the workspace, and step 3 cannot go on. The account
// stays for staff to finish with the ordinary subscription change (spec 5.4).
func TestFinalizeFailsAfterUserCreatedKeepsAccount(t *testing.T) {
	e := newFinEnv(t)
	in := e.businessInput("gone", e.bizPlan())
	in.CompReason = "contract"
	r := e.provisioned(in)
	if _, err := e.pool.Exec(context.Background(), `UPDATE platform_account_request SET plan_key = $2 WHERE id = $1`, r.ID, "gone-"+e.tag); err != nil {
		t.Fatal(err)
	}
	e.run()

	got := e.get(r.ID)
	if got.State != StateFailed || got.ErrorCode != "finalize_failed" || got.Attempts != 1 {
		t.Fatalf("state %s error %q attempts %d, want failed, finalize_failed, 1", got.State, got.ErrorCode, got.Attempts)
	}
	e.assertAccountKept(r)
	if _, found := e.sub(r.WorkspaceID); found || len(e.orders(r.WorkspaceID)) != 0 {
		t.Error("a subscription or an order was written")
	}
	if e.count(`SELECT count(*) FROM billing_profile WHERE workspace_id = $1`, r.WorkspaceID) != 0 {
		t.Error("the billing profile was written in a transaction that failed")
	}
	if got.BillingProfile == nil {
		t.Error("the billing profile was cleared from a request that is not completed")
	}
	if e.mail.count() != 0 {
		t.Error("a welcome was sent for a failed request")
	}
	var actorType, label string
	var ws *string
	err := e.pool.QueryRow(context.Background(),
		`SELECT actor_type, target_label, workspace_id::text FROM audit_event WHERE target_id = $1 AND action = 'account.provision_failed'`, r.ID).
		Scan(&actorType, &label, &ws)
	if err != nil {
		t.Fatalf("account.provision_failed row: %v", err)
	}
	if actorType != "system" || label != r.Email || ws != nil {
		t.Errorf("audit row = %s %s %v", actorType, label, ws)
	}
}

// A request in any state but provisioned is left alone, and so is a provisioned
// one with no workspace.
func TestFinalizeLeavesOtherRequestsAlone(t *testing.T) {
	e := newFinEnv(t)
	e.fin.Mailer = nil
	pending, _, err := e.svc.Create(context.Background(), e.actor(), e.input("pend"))
	if err != nil {
		t.Fatal(err)
	}
	half, _, err := e.svc.Create(context.Background(), e.actor(), e.input("half"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(context.Background(), `UPDATE platform_account_request SET state = 'provisioned' WHERE id = $1`, half.ID); err != nil {
		t.Fatal(err)
	}
	e.run()
	for _, r := range []Request{pending, half} {
		if got := e.get(r.ID); got.State == StateCompleted || got.State == StateFailed || got.Attempts != 0 {
			t.Errorf("request %s moved to %s with %d attempts", r.ID, got.State, got.Attempts)
		}
	}
}

func TestFinalizeNeedsItsDependencies(t *testing.T) {
	e := newFinEnv(t)
	ok := FinalizerDeps{Store: e.store, Plans: plan.NewPgxReader(e.pool), Cache: e.cache}
	if _, err := NewFinalizer(ok); err != nil {
		t.Fatalf("a finalizer with no mailer: %v", err)
	}
	for name, mod := range map[string]func(*FinalizerDeps){
		"store": func(d *FinalizerDeps) { d.Store = nil },
		"plans": func(d *FinalizerDeps) { d.Plans = nil },
		"cache": func(d *FinalizerDeps) { d.Cache = nil },
	} {
		d := ok
		mod(&d)
		if _, err := NewFinalizer(d); err == nil {
			t.Errorf("a finalizer with no %s", name)
		}
	}
}

func TestWelcomeSentExactlyOnce(t *testing.T) {
	e := newFinEnv(t)
	in := e.businessInput("once", e.bizPlan())
	in.Trial = true
	r := e.provisioned(in)
	e.run()

	if e.mail.count() != 1 {
		t.Fatalf("%d welcomes sent, want 1", e.mail.count())
	}
	call := e.mail.calls[0]
	if call.to != r.Email || call.workspace != r.WorkspaceName || call.plan != "Test business" || call.appURL != finalizeAppURL {
		t.Errorf("welcome = %+v", call)
	}
	got := e.get(r.ID)
	if got.State != StateCompleted || got.WelcomeSentAt == nil || !got.WelcomeSentAt.Equal(finalizeNow) || got.WelcomeAttempts != 0 {
		t.Errorf("request = %s sent %v attempts %d", got.State, got.WelcomeSentAt, got.WelcomeAttempts)
	}
	if c := e.requestAuditCount(r.ID, "account.welcome_sent"); c != 1 {
		t.Errorf("%d account.welcome_sent audit rows, want 1", c)
	}
	e.run()
	e.run()
	if e.mail.count() != 1 {
		t.Errorf("%d welcomes after later runs, want 1", e.mail.count())
	}
}

// The welcome rows of a completed request whose email did not go out are found
// by a later run; two runs at once send it once.
func TestWelcomeParallelRunsSendOnce(t *testing.T) {
	e := newFinEnv(t)
	e.fin.Mailer = nil
	r := e.provisioned(e.input("par"))
	e.run() // completes the request; no mailer, so the welcome stays unsent
	if got := e.get(r.ID); got.State != StateCompleted || got.WelcomeSentAt != nil {
		t.Fatalf("request = %s sent %v", got.State, got.WelcomeSentAt)
	}

	e.mail.delay = 200 * time.Millisecond
	e.fin.Mailer = e.mail
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := e.fin.RunOnce(context.Background()); err != nil {
				t.Errorf("RunOnce: %v", err)
			}
		}()
	}
	wg.Wait()
	if e.mail.count() != 1 {
		t.Errorf("%d welcomes sent by parallel runs, want 1", e.mail.count())
	}
	if got := e.get(r.ID); got.WelcomeSentAt == nil {
		t.Error("welcome_sent_at is not set")
	}
	if c := e.requestAuditCount(r.ID, "account.welcome_sent"); c != 1 {
		t.Errorf("%d account.welcome_sent audit rows, want 1", c)
	}
}

func TestWelcomeRetriesSendErrorsThreeTimes(t *testing.T) {
	e := newFinEnv(t)
	e.mail.setErr(errors.New("smtp: 451 try later"))
	r := e.provisioned(e.input("retry"))

	for attempt := 1; attempt <= 3; attempt++ {
		e.run()
		got := e.get(r.ID)
		if got.State != StateCompleted || got.WelcomeSentAt != nil || got.WelcomeAttempts != attempt || e.mail.count() != attempt {
			t.Fatalf("after run %d: state %s sent %v welcome_attempts %d, sends %d", attempt, got.State, got.WelcomeSentAt, got.WelcomeAttempts, e.mail.count())
		}
		if c := e.requestAuditCount(r.ID, "account.welcome_failed"); c != attempt {
			t.Errorf("after run %d: %d account.welcome_failed audit rows", attempt, c)
		}
	}
	e.mail.setErr(nil)
	e.run()
	if e.mail.count() != 3 || e.get(r.ID).WelcomeSentAt != nil {
		t.Errorf("a fourth send happened: %d sends", e.mail.count())
	}
	if c := e.requestAuditCount(r.ID, "account.welcome_sent"); c != 0 {
		t.Errorf("%d account.welcome_sent rows for a welcome that never went out", c)
	}
	// The finalize itself was not undone by the failed sends.
	if _, found := e.sub(r.WorkspaceID); found {
		t.Error("a free plan wrote a subscription")
	}
}

func (e *finEnv) completed(n string) Request {
	e.t.Helper()
	in := e.businessInput(n, e.bizPlan())
	in.CompReason = "contract"
	r := e.provisioned(in)
	e.run()
	if got := e.get(r.ID); got.State != StateCompleted || got.WelcomeSentAt == nil {
		e.t.Fatalf("request = %s sent %v", got.State, got.WelcomeSentAt)
	}
	return e.get(r.ID)
}

func TestWelcomeResendLimitPerDay(t *testing.T) {
	e := newFinEnv(t)
	r := e.completed("resend")
	first := e.mail.count()
	ctx := context.Background()

	var last Request
	for i := 1; i <= 3; i++ {
		got, err := e.svc.ResendWelcome(ctx, e.actor(), r.ID)
		if err != nil {
			t.Fatalf("resend %d: %v", i, err)
		}
		last = got
		if got.ID != r.ID || got.State != StateCompleted || got.WelcomeSentAt == nil {
			t.Errorf("resend %d answered %+v", i, got)
		}
	}
	if e.mail.count() != first+3 {
		t.Errorf("%d sends, want %d", e.mail.count(), first+3)
	}
	_, err := e.svc.ResendWelcome(ctx, e.actor(), r.ID)
	if ae := wantError(t, err, 429, "welcome_resend_rate_limited"); ae.RetryAfter != 24*time.Hour {
		t.Errorf("RetryAfter = %v, want 24h", ae.RetryAfter)
	}
	if e.mail.count() != first+3 {
		t.Error("the fourth resend sent an email")
	}
	if last.WelcomeSentAt == nil || !last.WelcomeSentAt.Equal(finalizeNow) {
		t.Errorf("welcome_sent_at = %v", last.WelcomeSentAt)
	}

	// The count is per request and per day, under a key that names the request.
	var keys []string
	for _, c := range e.limiter.calls {
		if c.limit == 3 && c.window == 24*time.Hour {
			keys = append(keys, c.key)
		}
	}
	if len(keys) != 4 {
		t.Fatalf("limiter keys = %v, want 4 under one key", keys)
	}
	for _, k := range keys {
		if k != keys[0] || !strings.Contains(k, r.ID) {
			t.Errorf("limiter keys = %v, want one key that names the request", keys)
			break
		}
	}
	if c := e.requestAuditCount(r.ID, "account.welcome_sent"); c != 1 {
		t.Errorf("%d account.welcome_sent rows, want 1 automatic", c)
	}
	if c := e.requestAuditCount(r.ID, "account.welcome_resent"); c != 3 {
		t.Errorf("%d account.welcome_resent rows, want 3", c)
	}
	var actorType string
	if err := e.pool.QueryRow(ctx, `SELECT actor_type FROM audit_event WHERE target_id = $1 AND action = 'account.welcome_resent' ORDER BY created_at DESC, id DESC LIMIT 1`, r.ID).Scan(&actorType); err != nil || actorType != "staff" {
		t.Errorf("the resend was audited as %q (%v), want staff", actorType, err)
	}

	other := e.completed("other")
	if _, err := e.svc.ResendWelcome(ctx, e.actor(), other.ID); err != nil {
		t.Errorf("another request shares the count: %v", err)
	}
}

func TestResendWelcomeRecordsWelcomeResent(t *testing.T) {
	e := newFinEnv(t)
	r := e.completed("resent")
	sent := e.requestAuditCount(r.ID, "account.welcome_sent")
	if _, err := e.svc.ResendWelcome(context.Background(), e.actor(), r.ID); err != nil {
		t.Fatal(err)
	}
	if c := e.requestAuditCount(r.ID, "account.welcome_resent"); c != 1 {
		t.Errorf("%d account.welcome_resent rows, want 1", c)
	}
	if c := e.requestAuditCount(r.ID, "account.welcome_sent"); c != sent {
		t.Errorf("account.welcome_sent rows went from %d to %d, want no new row", sent, c)
	}
}

func TestWelcomeResendRefusesARequestThatCannotBeResent(t *testing.T) {
	e := newFinEnv(t)
	ctx := context.Background()
	pending, _, err := e.svc.Create(ctx, e.actor(), e.input("pend"))
	if err != nil {
		t.Fatal(err)
	}
	failedNoUser, _, _ := e.svc.Create(ctx, e.actor(), e.input("nouser"))
	if _, err := e.pool.Exec(ctx, `UPDATE platform_account_request SET state = 'failed', error_code = 'slug_unavailable' WHERE id = $1`, failedNoUser.ID); err != nil {
		t.Fatal(err)
	}
	failedOther := e.provisioned(e.input("other"))
	if _, err := e.pool.Exec(ctx, `UPDATE platform_account_request SET state = 'failed', error_code = 'internal' WHERE id = $1`, failedOther.ID); err != nil {
		t.Fatal(err)
	}
	provisioned := e.provisioned(e.input("prov"))

	for name, r := range map[string]Request{"pending": pending, "failed, no user": failedNoUser, "failed, not finalize_failed": failedOther, "provisioned": provisioned} {
		_, err := e.svc.ResendWelcome(ctx, e.actor(), r.ID)
		ae := wantError(t, err, 409, "account_request_not_resendable")
		if ae.State == "" {
			t.Errorf("%s: the error names no state", name)
		}
	}
	for _, c := range e.limiter.calls {
		if c.window == 24*time.Hour {
			t.Errorf("a refused resend used the day's count: %+v", c)
		}
	}
	if _, err := e.svc.ResendWelcome(ctx, e.actor(), "30000000-0000-4000-8000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}

	// A request whose step 3 failed has a user and can be told its account is there.
	finalizeFailed := e.provisioned(e.input("ff"))
	if _, err := e.pool.Exec(ctx, `UPDATE platform_account_request SET state = 'failed', error_code = 'finalize_failed' WHERE id = $1`, finalizeFailed.ID); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.ResendWelcome(ctx, e.actor(), finalizeFailed.ID)
	if err != nil || got.WelcomeSentAt == nil || got.State != StateFailed {
		t.Errorf("resend of a finalize_failed request = %+v %v", got, err)
	}
}

func TestWelcomeResendSendFailureIs502(t *testing.T) {
	e := newFinEnv(t)
	r := e.completed("fail")
	before := e.get(r.ID).WelcomeSentAt
	e.mail.setErr(errors.New("smtp: relay down at 10.0.0.9"))

	_, err := e.svc.ResendWelcome(context.Background(), e.actor(), r.ID)
	ae := wantError(t, err, 502, "welcome_send_failed")
	if strings.Contains(ae.Message, "10.0.0.9") {
		t.Errorf("the error repeats the cause: %q", ae.Message)
	}
	if got := e.get(r.ID); !sameTime(got.WelcomeSentAt, before) {
		t.Errorf("welcome_sent_at moved to %v after a failed send", got.WelcomeSentAt)
	}
	if c := e.requestAuditCount(r.ID, "account.welcome_failed"); c != 1 {
		t.Errorf("%d account.welcome_failed rows, want 1", c)
	}
}

// Spec 13, group 14, in one run: the request is made, Multica's step 2 creates
// the user and the workspace, the platform's job finishes the account and sends
// the welcome, and the person has everything they need to sign in by email code.
func TestAccountEndToEnd(t *testing.T) {
	clock := finalizeNow
	e := newFinEnv(t)
	e.svc.Now = func() time.Time { return clock }
	e.fin.Now = func() time.Time { return clock }
	ctx := context.Background()

	// Step 1, the console.
	in := e.businessInput("e2e", e.bizPlan())
	in.Seats, in.Trial = 5, true
	req, created, err := e.svc.Create(ctx, e.actor(), in)
	if err != nil || !created || req.State != StatePending {
		t.Fatalf("create = %+v %v %v", req, created, err)
	}
	e.run() // nothing is provisioned yet
	if got := e.get(req.ID); got.State != StatePending || e.mail.count() != 0 {
		t.Fatalf("the job moved a pending request: %s", got.State)
	}

	// Step 2, Multica, a minute later.
	clock = clock.Add(time.Minute)
	prov := e.step2(req)
	if prov.UserID == "" || prov.WorkspaceID == "" {
		t.Fatalf("step 2 left %+v", prov)
	}

	// Step 3, the platform, a minute after that.
	clock = clock.Add(time.Minute)
	e.run()

	got := e.get(prov.ID)
	if got.State != StateCompleted || got.CompletedAt == nil || !got.CompletedAt.Equal(clock) || got.WelcomeSentAt == nil || !got.WelcomeSentAt.Equal(clock) {
		t.Fatalf("request = %s completed %v welcome %v", got.State, got.CompletedAt, got.WelcomeSentAt)
	}

	// The person: a user with the email, owner of a workspace on the plan they were given.
	if c := e.count(`SELECT count(*) FROM "user" WHERE lower(email) = $1`, in.Email); c != 1 {
		t.Errorf("%d users with the email, want 1", c)
	}
	if c := e.count(`SELECT count(*) FROM member WHERE workspace_id = $1 AND user_id = $2 AND role = 'owner'`, prov.WorkspaceID, prov.UserID); c != 1 {
		t.Error("the person is not the owner of the workspace")
	}
	sub, found := e.sub(prov.WorkspaceID)
	wantTrialEnd := clock.Add(14 * 24 * time.Hour)
	if !found || sub.PlanKey != in.PlanKey || sub.Status != "trialing" || sub.Seats != 5 || sub.TrialEnd == nil || !sub.TrialEnd.Equal(wantTrialEnd) {
		t.Errorf("subscription = %+v found %v", sub, found)
	}

	// Rule 8: no credential exists and the staff member is no member.
	if c := e.count(`SELECT count(*) FROM verification_code WHERE email = $1`, in.Email); c != 0 {
		t.Errorf("%d sign-in codes were made", c)
	}
	if c := e.count(`SELECT count(*) FROM member WHERE user_id = $1`, e.staff); c != 0 {
		t.Error("the staff member is a member of a workspace")
	}
	if c := e.count(`SELECT count(*) FROM platform_account WHERE user_id = $1`, prov.UserID); c != 0 {
		t.Error("the account has a state row, so it is not an ordinary active account")
	}

	// The welcome: addressed to the person, no secret, one link.
	if e.mail.count() != 1 {
		t.Fatalf("%d welcomes", e.mail.count())
	}
	if w := e.mail.calls[0]; w.to != in.Email || w.workspace != in.WorkspaceName || w.plan != "Test business" || w.appURL != finalizeAppURL {
		t.Errorf("welcome = %+v", w)
	}

	// Rule 13: the owner reads the line in the workspace log, with the staff actor.
	rows, err := e.pool.Query(ctx, `SELECT action, actor_type FROM audit_event WHERE workspace_id = $1 ORDER BY created_at, id`, prov.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]string{}
	for rows.Next() {
		var action, actor string
		if err := rows.Scan(&action, &actor); err != nil {
			t.Fatal(err)
		}
		seen[action] = actor
	}
	if seen["account.provisioned"] != "staff" {
		t.Errorf("workspace log = %v, want account.provisioned by staff", seen)
	}
}
