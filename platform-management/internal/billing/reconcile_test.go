package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/paymentnotice"
	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

func TestMatchNotice(t *testing.T) {
	now := time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)
	order := Order{Status: StatusPending, Amount: 1000, VATAmount: 100, Currency: "vnd", ExpiresAt: now.Add(time.Hour)}
	notice := paymentnotice.Notice{Amount: 1100, Currency: "VND"}
	with := func(f func(*Order, *paymentnotice.Notice)) (Order, paymentnotice.Notice) {
		o, n := order, notice
		f(&o, &n)
		return o, n
	}
	cases := []struct {
		name  string
		found bool
		edit  func(*Order, *paymentnotice.Notice)
		want  string
	}{
		{"exact match, currency case differs", true, func(*Order, *paymentnotice.Notice) {}, ""},
		{"no order", false, func(*Order, *paymentnotice.Notice) {}, ReasonUnknownCode},
		{"paid", true, func(o *Order, _ *paymentnotice.Notice) { o.Status = StatusPaid }, ReasonOrderNotPending},
		{"canceled", true, func(o *Order, _ *paymentnotice.Notice) { o.Status = StatusCanceled }, ReasonOrderNotPending},
		{"expired status", true, func(o *Order, _ *paymentnotice.Notice) { o.Status = StatusExpired }, ReasonOrderNotPending},
		{"expires_at passed", true, func(o *Order, _ *paymentnotice.Notice) { o.ExpiresAt = now.Add(-time.Second) }, ReasonOrderNotPending},
		{"expires_at is now", true, func(o *Order, _ *paymentnotice.Notice) { o.ExpiresAt = now }, ReasonOrderNotPending},
		{"currency", true, func(_ *Order, n *paymentnotice.Notice) { n.Currency = "USD" }, ReasonCurrencyMismatch},
		{"short", true, func(_ *Order, n *paymentnotice.Notice) { n.Amount = 1099 }, ReasonAmountMismatch},
		{"over", true, func(_ *Order, n *paymentnotice.Notice) { n.Amount = 1101 }, ReasonAmountMismatch},
		{"amount without VAT", true, func(_ *Order, n *paymentnotice.Notice) { n.Amount = 1000 }, ReasonAmountMismatch},
		{"not pending wins over currency", true, func(o *Order, n *paymentnotice.Notice) { o.Status = StatusPaid; n.Currency = "USD" }, ReasonOrderNotPending},
		{"currency wins over amount", true, func(_ *Order, n *paymentnotice.Notice) { n.Currency = "USD"; n.Amount = 1 }, ReasonCurrencyMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o, n := with(tc.edit)
			if got := matchNotice(n, o, tc.found, now); got != tc.want {
				t.Fatalf("matchNotice = %q, want %q", got, tc.want)
			}
		})
	}
}

// O-R4-8: billing and admin decide notices; support does not.
func TestSupportCannotResolve(t *testing.T) {
	r := &Reconciler{}
	ctx := context.Background()
	support := Actor{ID: tStaff, Role: staff.RoleSupport}
	if _, err := r.Resolve(ctx, support, tStaff, tStaff, "reason"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Resolve by support = %v, want ErrForbidden", err)
	}
	if _, err := r.Dismiss(ctx, support, tStaff, "reason"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Dismiss by support = %v, want ErrForbidden", err)
	}
	if _, err := r.Resolve(ctx, Actor{ID: tStaff}, tStaff, tStaff, "reason"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Resolve with no role = %v, want ErrForbidden", err)
	}
}

func TestDismissRequiresReason(t *testing.T) {
	r := &Reconciler{}
	for _, reason := range []string{"", "  \t"} {
		if _, err := r.Dismiss(context.Background(), Actor{ID: tStaff, Role: staff.RoleBilling}, tStaff, reason); !errors.Is(err, ErrReasonRequired) {
			t.Fatalf("Dismiss(%q) = %v, want ErrReasonRequired", reason, err)
		}
		if _, err := r.Resolve(context.Background(), Actor{ID: tStaff, Role: staff.RoleAdmin}, tStaff, tStaff, reason); !errors.Is(err, ErrReasonRequired) {
			t.Fatalf("Resolve(%q) = %v, want ErrReasonRequired", reason, err)
		}
	}
}

func reconPending(t *testing.T, s *Service) Order {
	t.Helper()
	start, end := clock, clock.AddDate(0, 1, 0)
	o, _, err := s.CreateOrder(context.Background(), Draft{
		WorkspaceID: tWS, Kind: KindSubscribe, PlanKey: "pro", Interval: "month", Seats: 3,
		Amount: 447000, VATAmount: 44700, Currency: "vnd", PeriodStart: &start, PeriodEnd: &end, IdempotencyKey: "k",
	})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestAutomaticApplyNeedsStaffOrProvider(t *testing.T) {
	m := newMemory()
	s, _ := newTestService(t, m)
	o := reconPending(t, s)
	if _, err := s.ApplyPayment(context.Background(), Confirmation{OrderID: o.ID}); err == nil {
		t.Fatal("ApplyPayment with no staff member and no provider succeeded")
	}
	if m.orders[o.ID].Status != StatusPending {
		t.Fatalf("order status = %s, want pending", m.orders[o.ID].Status)
	}
}

func TestAutomaticApplyRefusesReadOnlyWorkspace(t *testing.T) {
	m := newMemory()
	s, _ := newTestService(t, m)
	o := reconPending(t, s)
	m.ws[tWS] = workspacestate.Record{Status: workspacestate.ReadOnly}
	m.wsReason[tWS] = ReasonBilling
	_, err := s.ApplyPayment(context.Background(), Confirmation{OrderID: o.ID, Provider: "sepay"})
	if !errors.Is(err, ErrAutomaticReactivation) {
		t.Fatalf("ApplyPayment = %v, want ErrAutomaticReactivation", err)
	}
	if m.orders[o.ID].Status != StatusPending {
		t.Fatalf("order status = %s, want pending", m.orders[o.ID].Status)
	}
	// A staff confirmation of the same order still works and reactivates the workspace.
	if _, err := s.ApplyPayment(context.Background(), Confirmation{OrderID: o.ID, ConfirmedBy: tStaff}); err != nil {
		t.Fatalf("staff ApplyPayment = %v", err)
	}
}

func TestNoticeDecisionRollsBackWithPayment(t *testing.T) {
	m := newMemory()
	s, _ := newTestService(t, m)
	o := reconPending(t, s)
	n := paymentnotice.Notice{ID: tUser, Provider: "sepay", ProviderEventID: "1", State: paymentnotice.StateReceived, Amount: o.Total(), Currency: "VND"}
	m.notices[n.ID] = n
	m.auditFail = audit.ActionBillingOrderPaid // the last write fails, so the notice move must roll back
	_, err := s.ApplyPayment(context.Background(), Confirmation{
		OrderID: o.ID, Provider: "sepay", Notice: &NoticeDecision{Notice: n, To: paymentnotice.StateApplied},
	})
	if err == nil {
		t.Fatal("ApplyPayment succeeded with a failing audit write")
	}
	if got := m.notices[n.ID].State; got != paymentnotice.StateReceived {
		t.Fatalf("notice state = %s, want received after rollback", got)
	}

	m.auditFail = ""
	m.notices[n.ID] = paymentnotice.Notice{ID: n.ID, State: paymentnotice.StateApplied}
	_, err = s.ApplyPayment(context.Background(), Confirmation{
		OrderID: o.ID, Provider: "sepay", Notice: &NoticeDecision{Notice: n, To: paymentnotice.StateApplied},
	})
	if !errors.Is(err, ErrNoticeConflict) {
		t.Fatalf("ApplyPayment on a decided notice = %v, want ErrNoticeConflict", err)
	}
	if m.orders[o.ID].Status != StatusPending {
		t.Fatalf("order status = %s, want pending", m.orders[o.ID].Status)
	}
}
