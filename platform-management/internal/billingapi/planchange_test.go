package billingapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// fakeChanger keeps the calls the scheduled and cancel routes make.
type fakeChanger struct {
	schedules  []subscription.ScheduleChange
	cancels    []bool
	clearCalls int
	err        error
}

func (f *fakeChanger) Schedule(_ context.Context, c subscription.ScheduleChange, _ subscription.Actor) (subscription.Result, error) {
	f.schedules = append(f.schedules, c)
	if f.err != nil {
		return subscription.Result{}, f.err
	}
	end := periodEnd
	return subscription.Result{Subscription: subscription.Subscription{WorkspaceID: c.WorkspaceID, Version: 5, CurrentPeriodEnd: &end}, Changed: true}, nil
}

func (f *fakeChanger) CancelScheduled(_ context.Context, ws string, _ subscription.Actor) (subscription.Result, error) {
	f.clearCalls++
	return subscription.Result{Subscription: subscription.Subscription{WorkspaceID: ws, Version: 5}, Changed: true}, f.err
}

func (f *fakeChanger) SetCancel(_ context.Context, ws string, cancel bool, _ subscription.Actor) (subscription.Result, error) {
	f.cancels = append(f.cancels, cancel)
	if f.err != nil {
		return subscription.Result{}, f.err
	}
	end := periodEnd
	return subscription.Result{Subscription: subscription.Subscription{
		WorkspaceID: ws, Status: subscription.StatusActive, CancelAtPeriodEnd: cancel, Version: 5, CurrentPeriodEnd: &end,
	}, Changed: true}, nil
}

// fakeChangeOrders prices a charged change the way the billing service does: it
// refuses a different accepted amount, and repeats the order of a key.
type fakeChangeOrders struct {
	creates int
	byKey   map[string]billing.Order
	err     error
	// amount is the quoted amount the fake checks the accepted amount against.
	amount int64
}

func (f *fakeChangeOrders) OrderForKey(_ context.Context, ws, key string) (billing.Order, bool, error) {
	o, ok := f.byKey[ws+"/"+key]
	return o, ok && key != "", nil
}

func (f *fakeChangeOrders) CreateChangeOrder(_ context.Context, c billing.ChangeOrder) (billing.Order, bool, error) {
	f.creates++
	if f.err != nil {
		return billing.Order{}, false, f.err
	}
	if c.AcceptedAmount != f.amount {
		return billing.Order{}, false, subscription.QuoteChanged()
	}
	if f.byKey == nil {
		f.byKey = map[string]billing.Order{}
	}
	o := billing.Order{
		ID: "eeeeeeee-0000-0000-0000-000000000001", WorkspaceID: c.WorkspaceID, Kind: billing.KindChangePlan,
		PlanKey: c.Request.PlanKey, Interval: c.Request.Interval, Seats: 5, Amount: f.amount,
		Currency: "vnd", Status: billing.StatusPending, ExpiresAt: now.Add(billing.OrderLifetime),
	}
	f.byKey[c.WorkspaceID+"/"+c.IdempotencyKey] = o
	return o, true, nil
}

// The upgrade of the pro workspace of paid() to unlimited: 5 seats, 20 of 30 days
// left, the delta 500000 prorated to 333333 before VAT.
const upgradeAmount = 333333

func planChangePath(ws string) string { return "/api/v1/subscriptions/" + ws + "/plan-changes" }

func upgradeBody(key string) obj {
	return obj{"plan_key": "unlimited", "interval": "month", "accepted_amount": upgradeAmount, "idempotency_key": key}
}

func TestPlanChangePreviewQuotesAnUpgradeWithoutWriting(t *testing.T) {
	e := newEnv(t)
	e.paid()
	e.changeOrder.amount = upgradeAmount
	rec := e.do(http.MethodPost, planChangePath(wsID)+"/preview", ownerID, nil, obj{"plan_key": "unlimited", "interval": "month"})
	wantCode(t, rec, http.StatusOK, "")
	got := decode(t, rec)
	if got["mode"] != "charge" || got["amount"] != float64(upgradeAmount) || got["resulting_seats"] != float64(5) || got["expected_version"] != float64(4) {
		t.Fatalf("preview = %v", got)
	}
	if e.changeOrder.creates != 0 || len(e.changes.schedules) != 0 {
		t.Fatal("a preview wrote")
	}
}

func TestPlanChangeRepeatedKeyReturnsTheSameOrder(t *testing.T) {
	e := newEnv(t)
	e.paid()
	e.changeOrder.amount = upgradeAmount
	first := decode(t, e.do(http.MethodPost, planChangePath(wsID), ownerID, nil, upgradeBody("pc-1")))
	second := decode(t, e.do(http.MethodPost, planChangePath(wsID), ownerID, nil, upgradeBody("pc-1")))
	if first["request_id"] == nil || first["request_id"] != second["request_id"] {
		t.Fatalf("first %v, second %v: want one order", first, second)
	}
	if e.changeOrder.creates != 1 {
		t.Fatalf("CreateChangeOrder calls = %d, want 1", e.changeOrder.creates)
	}
	if first["status"] != "pending" || first["kind"] != "change_plan" {
		t.Fatalf("order = %v", first)
	}
}

func TestPlanChangeRefusesAPendingOrder(t *testing.T) {
	e := newEnv(t)
	e.paid()
	e.changeOrder.err = subscription.OrderPending("dddddddd-0000-0000-0000-000000000009")
	rec := e.do(http.MethodPost, planChangePath(wsID), ownerID, nil, upgradeBody("pc-2"))
	wantCode(t, rec, http.StatusConflict, "order_pending")
	if decode(t, rec)["order_id"] != "dddddddd-0000-0000-0000-000000000009" {
		t.Fatalf("body = %s, want the pending order's id", rec.Body)
	}
}

func TestPlanChangeRefusesAnAcceptedAmountThatNoLongerMatches(t *testing.T) {
	e := newEnv(t)
	e.paid()
	e.changeOrder.amount = upgradeAmount
	body := upgradeBody("pc-3")
	body["accepted_amount"] = upgradeAmount - 1
	wantCode(t, e.do(http.MethodPost, planChangePath(wsID), ownerID, nil, body), http.StatusConflict, "quote_changed")
}

func TestDowngradeIsStoredForTheNextRenewalAndNotCharged(t *testing.T) {
	e := newEnv(t)
	e.paid()
	rec := e.do(http.MethodPost, planChangePath(wsID), ownerID, nil, obj{"plan_key": "pro", "interval": "year", "idempotency_key": "pc-4"})
	wantCode(t, rec, http.StatusOK, "")
	got := decode(t, rec)
	if got["mode"] != "schedule" || got["interval"] != "year" || got["seats"] != float64(5) {
		t.Fatalf("response = %v", got)
	}
	if len(e.changes.schedules) != 1 || e.changes.schedules[0].Interval != "year" || e.changes.schedules[0].PlanKey != "pro" {
		t.Fatalf("schedules = %+v", e.changes.schedules)
	}
	if e.changeOrder.creates != 0 {
		t.Fatal("a scheduled change created an order")
	}
}

func TestChargedChangeNeedsAnAcceptedAmount(t *testing.T) {
	e := newEnv(t)
	e.paid()
	body := upgradeBody("pc-5")
	delete(body, "accepted_amount")
	wantCode(t, e.do(http.MethodPost, planChangePath(wsID), ownerID, nil, body), http.StatusBadRequest, "invalid_request")
}

func TestScheduledSeatsAboveTheIndividualLimitAreRefused(t *testing.T) {
	e := newEnv(t)
	e.paid()
	rec := e.do(http.MethodPost, planChangePath(wsID), ownerID, nil,
		obj{"plan_key": "pro", "interval": "year", "seats": 7, "idempotency_key": "pc-6"})
	wantCode(t, rec, http.StatusConflict, "account_kind_required")
	if decode(t, rec)["max_seats"] != float64(5) || len(e.changes.schedules) != 0 {
		t.Fatalf("body = %s, schedules = %d", rec.Body, len(e.changes.schedules))
	}
}

func TestPlanChangeToAnUnknownPlanIsNotFound(t *testing.T) {
	e := newEnv(t)
	e.paid()
	wantCode(t, e.do(http.MethodPost, planChangePath(wsID), ownerID, nil,
		obj{"plan_key": "nope", "interval": "month", "accepted_amount": 1, "idempotency_key": "pc-7"}), http.StatusNotFound, "plan_not_found")
}

func TestCancelAndResumeNeedAKeyAndSetTheFlag(t *testing.T) {
	e := newEnv(t)
	e.paid()
	wantCode(t, e.do(http.MethodPost, "/api/v1/subscriptions/"+wsID+"/cancel", ownerID, nil, nil), http.StatusBadRequest, "")
	h := map[string]string{"Idempotency-Key": "cancel-1"}
	wantCode(t, e.do(http.MethodPost, "/api/v1/subscriptions/"+wsID+"/cancel", ownerID, h, nil), http.StatusOK, "")
	wantCode(t, e.do(http.MethodPost, "/api/v1/subscriptions/"+wsID+"/resume", ownerID, map[string]string{"Idempotency-Key": "resume-1"}, nil), http.StatusOK, "")
	if len(e.changes.cancels) != 2 || e.changes.cancels[0] != true || e.changes.cancels[1] != false {
		t.Fatalf("cancel calls = %v, want [true false]", e.changes.cancels)
	}
}

func TestCancelOutsideTheTableIsRefused(t *testing.T) {
	e := newEnv(t)
	e.paid()
	e.changes.err = &subscription.Error{Status: http.StatusConflict, Code: "subscription_not_cancelable", Message: "no"}
	h := map[string]string{"Idempotency-Key": "cancel-2"}
	wantCode(t, e.do(http.MethodPost, "/api/v1/subscriptions/"+wsID+"/cancel", ownerID, h, nil), http.StatusConflict, "subscription_not_cancelable")
}

func TestCancelScheduledNeedsAKeyAndClearsOnlyThePendingChange(t *testing.T) {
	e := newEnv(t)
	e.paid()
	path := "/api/v1/subscriptions/" + wsID + "/plan-changes/scheduled"
	wantCode(t, e.do(http.MethodDelete, path, ownerID, nil, nil), http.StatusBadRequest, "")
	wantCode(t, e.do(http.MethodDelete, path, ownerID, map[string]string{"Idempotency-Key": "clear-1"}, nil), http.StatusOK, "")
	if e.changes.clearCalls != 1 {
		t.Fatalf("clear calls = %d, want 1", e.changes.clearCalls)
	}
}

func TestPlanChangeRoutesAreForOwnersAndAdminsOnly(t *testing.T) {
	e := newEnv(t)
	e.paid()
	h := map[string]string{"Idempotency-Key": "pc-role"}
	wantCode(t, e.do(http.MethodPost, planChangePath(wsID)+"/preview", memberID, nil, obj{"plan_key": "unlimited", "interval": "month"}), http.StatusForbidden, "forbidden")
	wantCode(t, e.do(http.MethodPost, "/api/v1/subscriptions/"+wsID+"/cancel", memberID, h, nil), http.StatusForbidden, "forbidden")
	wantCode(t, e.do(http.MethodPost, "/api/v1/subscriptions/"+wsID+"/cancel", strangID, h, nil), http.StatusNotFound, "not_found")
	wantCode(t, e.do(http.MethodPost, "/api/v1/subscriptions/"+wsID+"/cancel", adminID, h, nil), http.StatusOK, "")
	if len(e.changes.cancels) != 1 {
		t.Fatalf("the admin's cancel did not reach the service")
	}
}

func TestCheckoutRefusesSeatsAboveTheIndividualLimit(t *testing.T) {
	e := newEnv(t)
	e.setKind(subscription.AccountIndividual)
	body := func(seats int, key string) obj {
		return obj{"workspace_id": wsID, "interval": "month", "idempotency_key": key, "plan_key": "pro", "seats": seats}
	}
	rec := e.checkout(adminID, body(6, "k-over"))
	wantCode(t, rec, http.StatusConflict, "account_kind_required")
	if decode(t, rec)["max_seats"] != float64(5) || len(e.orders.drafts) != 0 {
		t.Fatalf("body = %s, orders = %d", rec.Body, len(e.orders.drafts))
	}
	if rec := e.checkout(adminID, body(5, "k-at")); rec.Code != http.StatusOK {
		t.Fatalf("at the limit: status %d: %s", rec.Code, rec.Body)
	}
}

func TestSeatPurchaseRefusesSeatsAboveTheIndividualLimit(t *testing.T) {
	e := newEnv(t)
	sub := e.paid() // five seats, an individual workspace
	rec := e.do("POST", previewPath(wsID), ownerID, nil, obj{"additional_seats": 1})
	wantCode(t, rec, http.StatusConflict, "account_kind_required")
	q := obj{"current_seats": float64(5), "purchase_version": float64(sub.Version), "proration_amount": float64(0), "currency": "vnd"}
	body := confirm(q, "buy-over")
	body["additional_seats"] = 1
	wantCode(t, e.do("POST", purchasePath(wsID), adminID, nil, body), http.StatusConflict, "account_kind_required")
	if len(e.orders.drafts) != 0 {
		t.Fatalf("a refused purchase wrote %d orders", len(e.orders.drafts))
	}

	sub.Seats = 4
	e.subs.rows[wsID] = sub
	if rec := e.do("POST", previewPath(wsID), ownerID, nil, obj{"additional_seats": 1}); rec.Code != http.StatusOK {
		t.Fatalf("at the limit: status %d: %s", rec.Code, rec.Body)
	}
}
