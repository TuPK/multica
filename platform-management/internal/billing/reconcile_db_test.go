package billing

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/paymentnotice"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// Database tests of the reconciler: skipped unless PLATFORM_TEST_DATABASE_URL
// names a *_test database and DATABASE_URL equals it. Each test writes notices
// under its own random provider name and orders under a fresh workspace id,
// and deletes what it wrote.

type reconFixture struct {
	*dbFixture
	provider string
	svc      *Service
	rec      *Reconciler
	notices  *paymentnotice.PgxStore
	seq      int
}

func newReconFixture(t *testing.T) *reconFixture {
	t.Helper()
	f := &reconFixture{dbFixture: newDBFixture(t)}
	ctx := context.Background()
	if err := f.pool.QueryRow(ctx, `SELECT 'test-' || gen_random_uuid()::text`).Scan(&f.provider); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.pool.Exec(ctx, `DELETE FROM audit_event WHERE detail->>'provider' = $1`, f.provider)
		f.pool.Exec(ctx, `DELETE FROM payment_notice WHERE provider = $1`, f.provider)
	})
	f.svc = f.service(nil, nil)
	f.rec = NewReconciler(f.svc, NewPgxReconcileStore(f.pool))
	f.notices = paymentnotice.NewPgxStore(f.pool)
	return f
}

func (f *reconFixture) order() Order {
	f.t.Helper()
	f.seq++
	o, _, err := f.svc.CreateOrder(context.Background(), f.draft(fmt.Sprintf("k%d", f.seq)))
	if err != nil {
		f.t.Fatal(err)
	}
	return o
}

func (f *reconFixture) notice(event, code string, amount int64, currency string, receivedAt time.Time) paymentnotice.Notice {
	f.t.Helper()
	n, _, err := f.notices.Insert(context.Background(), paymentnotice.NewNotice{
		Provider: f.provider, ProviderEventID: event, TransferCode: code, Amount: amount, Currency: currency,
		BodySHA256: "aa", ReceivedAt: receivedAt,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *reconFixture) noticeFor(o Order, event string) paymentnotice.Notice {
	return f.notice(event, o.TransferCode, o.Total(), "VND", time.Time{})
}

func (f *reconFixture) get(id string) paymentnotice.Notice {
	f.t.Helper()
	n, err := f.notices.Get(context.Background(), id)
	if err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *reconFixture) orderStatus(id string) Status {
	f.t.Helper()
	var s Status
	if err := f.pool.QueryRow(context.Background(), `SELECT status FROM billing_order WHERE id = $1`, id).Scan(&s); err != nil {
		f.t.Fatal(err)
	}
	return s
}

func (f *reconFixture) orderJSON(id string) string {
	f.t.Helper()
	var s string
	if err := f.pool.QueryRow(context.Background(), `SELECT row_to_json(o)::text FROM billing_order o WHERE id = $1`, id).Scan(&s); err != nil {
		f.t.Fatal(err)
	}
	return s
}

func (f *reconFixture) auditCount(action, targetID string) int {
	f.t.Helper()
	return f.count(`SELECT count(*) FROM audit_event WHERE action = $1 AND target_id = $2`, action, targetID)
}

func (f *reconFixture) reconcile(n paymentnotice.Notice) paymentnotice.Notice {
	f.t.Helper()
	got, err := f.rec.Reconcile(context.Background(), n.ID)
	if err != nil {
		f.t.Fatalf("Reconcile: %v", err)
	}
	return got
}

func (f *reconFixture) wantReview(n paymentnotice.Notice, reason string) {
	f.t.Helper()
	got := f.reconcile(n)
	if got.State != paymentnotice.StateNeedsReview || got.ReviewReason != reason || got.OrderID != "" {
		f.t.Fatalf("notice = %s/%s/order %q, want needs_review/%s with no order", got.State, got.ReviewReason, got.OrderID, reason)
	}
	if f.auditCount(audit.ActionPaymentNoticeNeedsReview, n.ID) != 1 {
		f.t.Fatalf("payment.notice_needs_review rows = %d, want 1", f.auditCount(audit.ActionPaymentNoticeNeedsReview, n.ID))
	}
}

func billingActor(f *reconFixture) Actor {
	return Actor{ID: f.staff, Label: "billing@example.com", Role: staff.RoleBilling}
}

// AC56
func TestSameEventTwiceAppliesOnce(t *testing.T) {
	f := newReconFixture(t)
	o := f.order()
	first := f.noticeFor(o, "evt-1")
	again, inserted, err := f.notices.Insert(context.Background(), paymentnotice.NewNotice{
		Provider: f.provider, ProviderEventID: "evt-1", TransferCode: o.TransferCode, Amount: o.Total(), Currency: "VND",
	})
	if err != nil || inserted || again.ID != first.ID {
		t.Fatalf("second Insert = %+v, %v, %v; want the first row", again, inserted, err)
	}
	for range 2 {
		if got := f.reconcile(first); got.State != paymentnotice.StateApplied {
			t.Fatalf("notice state = %s, want applied", got.State)
		}
	}
	if f.orderStatus(o.ID) != StatusPaid {
		t.Fatal("order is not paid")
	}
	if n := f.auditCount(audit.ActionBillingOrderPaid, o.ID); n != 1 {
		t.Fatalf("billing.order_paid rows = %d, want 1", n)
	}
	if n := f.auditCount(audit.ActionPaymentNoticeApplied, first.ID); n != 1 {
		t.Fatalf("payment.notice_applied rows = %d, want 1", n)
	}
	if n := f.count(`SELECT count(*) FROM payment_notice WHERE provider = $1`, f.provider); n != 1 {
		t.Fatalf("payment_notice rows = %d, want 1", n)
	}
}

func TestSameEventConcurrentAppliesOnce(t *testing.T) {
	f := newReconFixture(t)
	f.seedSubscription(0)
	o := f.order()
	const workers = 10
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			n, _, err := f.notices.Insert(ctx, paymentnotice.NewNotice{
				Provider: f.provider, ProviderEventID: "evt-race", TransferCode: o.TransferCode, Amount: o.Total(), Currency: "VND",
			})
			if err == nil {
				_, err = f.rec.Reconcile(ctx, n.ID)
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("a worker failed: %v", err)
		}
	}
	var id string
	if err := f.pool.QueryRow(context.Background(), `SELECT id::text FROM payment_notice WHERE provider = $1`, f.provider).Scan(&id); err != nil {
		t.Fatalf("want exactly one payment_notice row: %v", err)
	}
	if f.get(id).State != paymentnotice.StateApplied || f.orderStatus(o.ID) != StatusPaid {
		t.Fatalf("notice %s, order %s; want applied and paid", f.get(id).State, f.orderStatus(o.ID))
	}
	if n := f.auditCount(audit.ActionBillingOrderPaid, o.ID); n != 1 {
		t.Fatalf("billing.order_paid rows = %d, want 1", n)
	}
	if v, _ := f.subscriptionVersion(); v != 1 {
		t.Fatalf("subscription version = %d, want 1 (one change)", v)
	}
	if n := f.auditCount(audit.ActionPaymentNoticeNeedsReview, id); n != 0 {
		t.Fatalf("payment.notice_needs_review rows = %d, want 0", n)
	}
}

func TestCrashBetweenRecordAndApplyIsRedriven(t *testing.T) {
	f := newReconFixture(t)
	o, fresh := f.order(), f.order()
	stale := f.notice("evt-stale", o.TransferCode, o.Total(), "VND", time.Now().Add(-10*time.Minute))
	young := f.noticeFor(fresh, "evt-young")
	ctx := context.Background()

	n, err := f.rec.Redrive(ctx, 2*time.Minute, 100)
	if err != nil || n < 1 {
		t.Fatalf("Redrive = %d, %v; want at least the stale notice", n, err)
	}
	if got := f.get(stale.ID); got.State != paymentnotice.StateApplied || got.OrderID != o.ID {
		t.Fatalf("stale notice = %s, order %q; want applied to %s", got.State, got.OrderID, o.ID)
	}
	if got := f.get(young.ID); got.State != paymentnotice.StateReceived {
		t.Fatalf("young notice = %s, want received (inside the cutoff)", got.State)
	}
	if _, err := f.rec.Redrive(ctx, 2*time.Minute, 100); err != nil {
		t.Fatal(err)
	}
	if n := f.auditCount(audit.ActionBillingOrderPaid, o.ID); n != 1 {
		t.Fatalf("billing.order_paid rows = %d, want 1 after two runs", n)
	}
	if f.orderStatus(fresh.ID) != StatusPending {
		t.Fatal("the young notice's order changed")
	}
}

// AC57
func TestExactMatchAppliesThroughApplyPayment(t *testing.T) {
	f := newReconFixture(t)
	o := f.order()
	n := f.noticeFor(o, "evt-exact")
	got := f.reconcile(n)
	if got.State != paymentnotice.StateApplied || got.OrderID != o.ID {
		t.Fatalf("notice = %s, order %q; want applied to %s", got.State, got.OrderID, o.ID)
	}
	if f.orderStatus(o.ID) != StatusPaid {
		t.Fatal("order is not paid")
	}
	if f.auditCount(audit.ActionBillingOrderPaid, o.ID) != 1 || f.auditCount(audit.ActionPaymentNoticeApplied, n.ID) != 1 {
		t.Fatal("want one billing.order_paid and one payment.notice_applied row")
	}
}

func TestAmountMismatchNeedsReview(t *testing.T) {
	f := newReconFixture(t)
	for name, delta := range map[string]int64{"short": -1, "over": 1} {
		o := f.order()
		n := f.notice("evt-"+name, o.TransferCode, o.Total()+delta, "VND", time.Time{})
		f.wantReview(n, ReasonAmountMismatch)
		if f.orderStatus(o.ID) != StatusPending {
			t.Fatalf("%s: order status = %s, want pending", name, f.orderStatus(o.ID))
		}
	}
}

func TestUnknownCodeNeedsReview(t *testing.T) {
	f := newReconFixture(t)
	f.wantReview(f.notice("evt-unknown", "NOSUCHCODE999", 1000, "VND", time.Time{}), ReasonUnknownCode)
	f.wantReview(f.notice("evt-nocode", "", 1000, "VND", time.Time{}), ReasonUnknownCode)
}

func TestAmbiguousCodeNeedsReview(t *testing.T) {
	f := newReconFixture(t)
	o := f.order()
	n := f.noticeFor(o, "evt-ambiguous")
	// The listener (P42.4) records a notice with several codes as needs_review.
	if err := f.notices.Transition(context.Background(), paymentnotice.Change{
		ID: n.ID, From: paymentnotice.StateReceived, To: paymentnotice.StateNeedsReview, ReviewReason: "ambiguous_code",
	}); err != nil {
		t.Fatal(err)
	}
	queue, err := f.notices.ListNeedsReview(context.Background(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	listed := false
	for _, q := range queue {
		listed = listed || q.ID == n.ID
	}
	if !listed {
		t.Fatal("the ambiguous notice is not in the queue")
	}
	got := f.reconcile(n)
	if got.State != paymentnotice.StateNeedsReview || got.ReviewReason != "ambiguous_code" {
		t.Fatalf("notice = %s/%s, want it left as needs_review/ambiguous_code", got.State, got.ReviewReason)
	}
	if f.orderStatus(o.ID) != StatusPending {
		t.Fatal("the order changed")
	}
}

func TestCurrencyMismatchNeedsReview(t *testing.T) {
	f := newReconFixture(t)
	o := f.order()
	f.wantReview(f.notice("evt-usd", o.TransferCode, o.Total(), "USD", time.Time{}), ReasonCurrencyMismatch)
}

func TestPaidOrderNoticeNeedsReview(t *testing.T) {
	f := newReconFixture(t)
	o := f.order()
	if _, err := f.svc.ApplyPayment(context.Background(), Confirmation{OrderID: o.ID, ConfirmedBy: f.staff}); err != nil {
		t.Fatal(err)
	}
	f.wantReview(f.noticeFor(o, "evt-paid"), ReasonOrderNotPending)
	if n := f.auditCount(audit.ActionBillingOrderPaid, o.ID); n != 1 {
		t.Fatalf("billing.order_paid rows = %d, want 1", n)
	}
}

func TestExpiredOrderNoticeNeedsReview(t *testing.T) {
	f := newReconFixture(t)
	old := f.service(nil, func() time.Time { return time.Now().Add(-8 * 24 * time.Hour) })
	o, _, err := old.CreateOrder(context.Background(), f.draft("old"))
	if err != nil {
		t.Fatal(err)
	}
	f.wantReview(f.noticeFor(o, "evt-expired"), ReasonOrderNotPending)
	if f.orderStatus(o.ID) != StatusPending {
		t.Fatal("the expired order changed")
	}
}

func TestReadOnlyWorkspaceNoticeNeedsReviewApplyFailed(t *testing.T) {
	f := newReconFixture(t)
	f.seedSubscription(1)
	o := f.order()
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO platform_workspace (workspace_id, status, reason) VALUES ($1, 'read_only', $2)`, f.ws, ReasonBilling); err != nil {
		t.Fatal(err)
	}
	n := f.noticeFor(o, "evt-readonly")
	got := f.reconcile(n)
	if got.State != paymentnotice.StateNeedsReview || got.ReviewReason != ReasonApplyFailed || f.orderStatus(o.ID) != StatusPending {
		t.Fatalf("notice = %s/%s, order %s; want needs_review/apply_failed and a pending order", got.State, got.ReviewReason, f.orderStatus(o.ID))
	}
	// Staff can still resolve it, and the payment reactivates the workspace.
	if _, err := f.rec.Resolve(context.Background(), billingActor(f), n.ID, o.ID, "paid by transfer"); err != nil {
		t.Fatal(err)
	}
	if f.orderStatus(o.ID) != StatusPaid {
		t.Fatal("order is not paid after the staff resolve")
	}
}

func TestNeedsReviewChangesNoOrder(t *testing.T) {
	f := newReconFixture(t)
	o := f.order()
	before := f.orderJSON(o.ID)
	for i, n := range []paymentnotice.Notice{
		f.notice("evt-a", o.TransferCode, o.Total()-1, "VND", time.Time{}),
		f.notice("evt-b", o.TransferCode, o.Total(), "USD", time.Time{}),
		f.notice("evt-c", o.TransferCode, o.Total()+1, "VND", time.Time{}),
	} {
		if got := f.reconcile(n); got.State != paymentnotice.StateNeedsReview {
			t.Fatalf("notice %d = %s, want needs_review", i, got.State)
		}
	}
	if after := f.orderJSON(o.ID); after != before {
		t.Fatalf("order row changed:\nbefore %s\nafter  %s", before, after)
	}
}

func TestResolveWritesAuditWithBothAmounts(t *testing.T) {
	f := newReconFixture(t)
	o := f.order()
	n := f.notice("evt-short", o.TransferCode, o.Total()-1000, "VND", time.Time{})
	f.reconcile(n)
	got, err := f.rec.Resolve(context.Background(), billingActor(f), n.ID, o.ID, "customer paid the rest in cash")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != paymentnotice.StateResolved || got.OrderID != o.ID || got.DecidedBy != f.staff || got.DecisionReason == "" {
		t.Fatalf("notice = %+v; want resolved to the order by the staff member with a reason", got)
	}
	if f.orderStatus(o.ID) != StatusPaid {
		t.Fatal("order is not paid")
	}
	var actorType, actorID, label string
	var noticeAmount, orderTotal float64
	var reason string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT actor_type, actor_id::text, actor_label, (detail->>'notice_amount')::float8, (detail->>'order_total')::float8, detail->>'reason'
		 FROM audit_event WHERE action = $1 AND target_id = $2`, audit.ActionPaymentNoticeResolved, n.ID,
	).Scan(&actorType, &actorID, &label, &noticeAmount, &orderTotal, &reason); err != nil {
		t.Fatal(err)
	}
	if actorType != audit.ActorStaff || actorID != f.staff || label != "billing@example.com" ||
		int64(noticeAmount) != o.Total()-1000 || int64(orderTotal) != o.Total() || reason != "customer paid the rest in cash" {
		t.Fatalf("audit row = %s/%s/%s notice %v order %v reason %q", actorType, actorID, label, noticeAmount, orderTotal, reason)
	}
	if f.auditCount(audit.ActionBillingOrderPaid, o.ID) != 1 {
		t.Fatal("want one billing.order_paid row")
	}
}

func TestDismissWritesAudit(t *testing.T) {
	f := newReconFixture(t)
	n := f.notice("evt-dismiss", "NOSUCHCODE999", 5000, "VND", time.Time{})
	f.reconcile(n)
	got, err := f.rec.Dismiss(context.Background(), billingActor(f), n.ID, "refunded outside the system")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != paymentnotice.StateDismissed || got.DecisionReason != "refunded outside the system" || got.OrderID != "" {
		t.Fatalf("notice = %+v; want dismissed with the reason and no order", got)
	}
	if f.auditCount(audit.ActionPaymentNoticeDismissed, n.ID) != 1 {
		t.Fatal("want one payment.notice_dismissed row")
	}
}

func TestClosedNoticeReturns409(t *testing.T) {
	f := newReconFixture(t)
	ctx := context.Background()
	o1, o2 := f.order(), f.order()
	resolved := f.notice("evt-r", "NOSUCHCODE1", 1, "VND", time.Time{})
	dismissed := f.notice("evt-d", "NOSUCHCODE2", 1, "VND", time.Time{})
	received := f.notice("evt-rec", "NOSUCHCODE3", 1, "VND", time.Time{})
	f.reconcile(resolved)
	f.reconcile(dismissed)
	if _, err := f.rec.Resolve(ctx, billingActor(f), resolved.ID, o1.ID, "ok"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rec.Dismiss(ctx, billingActor(f), dismissed.ID, "ok"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		do   func() error
		want paymentnotice.State
	}{
		{"resolve a resolved notice with another order", func() error { _, err := f.rec.Resolve(ctx, billingActor(f), resolved.ID, o2.ID, "ok"); return err }, paymentnotice.StateResolved},
		{"resolve a dismissed notice", func() error { _, err := f.rec.Resolve(ctx, billingActor(f), dismissed.ID, o2.ID, "ok"); return err }, paymentnotice.StateDismissed},
		{"dismiss a resolved notice", func() error { _, err := f.rec.Dismiss(ctx, billingActor(f), resolved.ID, "ok"); return err }, paymentnotice.StateResolved},
		{"dismiss a dismissed notice", func() error { _, err := f.rec.Dismiss(ctx, billingActor(f), dismissed.ID, "ok"); return err }, paymentnotice.StateDismissed},
		{"resolve a received notice", func() error { _, err := f.rec.Resolve(ctx, billingActor(f), received.ID, o2.ID, "ok"); return err }, paymentnotice.StateReceived},
	} {
		var closed *NoticeClosedError
		if err := tc.do(); !errors.As(err, &closed) || closed.State != tc.want {
			t.Errorf("%s: err = %v, want a closed error in state %s", tc.name, err, tc.want)
		}
	}
	if f.orderStatus(o2.ID) != StatusPending {
		t.Fatal("a refused resolve changed the order")
	}
}

func TestResolveRepeatIsNoop(t *testing.T) {
	f := newReconFixture(t)
	ctx := context.Background()
	o := f.order()
	n := f.notice("evt-repeat", o.TransferCode, o.Total()+1, "VND", time.Time{})
	f.reconcile(n)
	first, err := f.rec.Resolve(ctx, billingActor(f), n.ID, o.ID, "ok")
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.rec.Resolve(ctx, billingActor(f), n.ID, o.ID, "ok")
	if err != nil || second.State != paymentnotice.StateResolved || !second.DecidedAt.Equal(first.DecidedAt) {
		t.Fatalf("repeat Resolve = %+v, %v; want the resolved notice unchanged", second, err)
	}
	if f.auditCount(audit.ActionPaymentNoticeResolved, n.ID) != 1 || f.auditCount(audit.ActionBillingOrderPaid, o.ID) != 1 {
		t.Fatal("the repeat wrote an audit row")
	}
}

func TestResolveOrderNotPending(t *testing.T) {
	f := newReconFixture(t)
	ctx := context.Background()
	o := f.order()
	if _, err := f.svc.ApplyPayment(ctx, Confirmation{OrderID: o.ID, ConfirmedBy: f.staff}); err != nil {
		t.Fatal(err)
	}
	n := f.notice("evt-np", "NOSUCHCODE4", 1, "VND", time.Time{})
	f.reconcile(n)
	if _, err := f.rec.Resolve(ctx, billingActor(f), n.ID, o.ID, "ok"); !errors.Is(err, ErrOrderNotPending) {
		t.Fatalf("Resolve on a paid order = %v, want ErrOrderNotPending", err)
	}
	if got := f.get(n.ID); got.State != paymentnotice.StateNeedsReview {
		t.Fatalf("notice = %s, want it still needs_review", got.State)
	}
}

func TestApplyPaymentAutomaticSourceAuditedAsSystem(t *testing.T) {
	f := newReconFixture(t)
	o := f.order()
	n := f.noticeFor(o, "evt-system")
	f.reconcile(n)
	var actorType, label string
	var actorID *string
	var confirmedBy *string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT actor_type, actor_id::text, actor_label FROM audit_event WHERE action = $1 AND target_id = $2`,
		audit.ActionBillingOrderPaid, o.ID).Scan(&actorType, &actorID, &label); err != nil {
		t.Fatal(err)
	}
	if actorType != audit.ActorSystem || actorID != nil || label != "provider:"+f.provider {
		t.Fatalf("billing.order_paid actor = %s/%v/%s; want system, no id, provider:%s", actorType, actorID, label, f.provider)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT confirmed_by::text FROM billing_order WHERE id = $1`, o.ID).Scan(&confirmedBy); err != nil || confirmedBy != nil {
		t.Fatalf("confirmed_by = %v, %v; want NULL", confirmedBy, err)
	}
}
