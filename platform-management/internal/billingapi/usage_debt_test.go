package billingapi

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

const unpaidOrderID = "dddddddd-0000-0000-0000-0000000000a1"

func TestUsageInvoiceUnpaidBlocksPaidPurchase(t *testing.T) {
	e := newEnv(t)
	e.plans.plans = append(e.plans.plans, plan.Plan{
		Key: "payg", Name: "Pay as you go", Currency: "vnd", IsPublic: true, Audience: plan.AudienceAny,
		BillingModel: plan.BillingUsage, Limits: plan.Limits{}, Features: map[string]bool{},
	})
	e.paid()
	e.debts.unpaid[wsID] = unpaidOrderID

	for _, key := range []string{"unlimited", "payg"} {
		rec := e.checkout(ownerID, obj{"workspace_id": wsID, "interval": "month", "idempotency_key": "ck-" + key, "plan_key": key})
		got := decode(t, rec)
		if rec.Code != http.StatusConflict || got["code"] != "usage_invoice_unpaid" || got["order_id"] != unpaidOrderID {
			t.Errorf("checkout of %s: status %d body %v, want 409 usage_invoice_unpaid with order_id", key, rec.Code, got)
		}

		e.changeOrder.amount = upgradeAmount
		rec = e.do(http.MethodPost, planChangePath(wsID), ownerID, nil,
			obj{"plan_key": key, "interval": "month", "accepted_amount": upgradeAmount, "idempotency_key": "pc-" + key})
		got = decode(t, rec)
		if rec.Code != http.StatusConflict || got["code"] != "usage_invoice_unpaid" || got["order_id"] != unpaidOrderID {
			t.Errorf("plan change to %s: status %d body %v, want 409 usage_invoice_unpaid with order_id", key, rec.Code, got)
		}
	}
	if len(e.orders.drafts) != 0 || e.changeOrder.creates != 0 || len(e.changes.schedules) != 0 {
		t.Errorf("a refused request wrote: %d drafts, %d change orders, %d schedules", len(e.orders.drafts), e.changeOrder.creates, len(e.changes.schedules))
	}

	// The free plan is never refused for the debt: the answer is the one the
	// workspace gets without it (a free plan has no seat price to buy, so it is
	// not purchasable through plan-changes; the downgrade is a cancel).
	freeBody := obj{"plan_key": "free", "interval": "month", "idempotency_key": "pc-free"}
	withDebt := e.do(http.MethodPost, planChangePath(wsID), ownerID, nil, freeBody)
	delete(e.debts.unpaid, wsID)
	without := e.do(http.MethodPost, planChangePath(wsID), ownerID, nil, freeBody)
	if withDebt.Code != without.Code || withDebt.Body.String() != without.Body.String() || decode(t, withDebt)["code"] == "usage_invoice_unpaid" {
		t.Errorf("plan change to free with a debt: %d %s, without: %d %s; want the same answer", withDebt.Code, withDebt.Body, without.Code, without.Body)
	}

	// Once the invoice is paid or written off, the purchase goes through again.
	rec := e.do(http.MethodPost, planChangePath(wsID), ownerID, nil, upgradeBody("pc-after"))
	if rec.Code != http.StatusOK {
		t.Errorf("plan change without a debt: status %d: %s", rec.Code, rec.Body)
	}
}
