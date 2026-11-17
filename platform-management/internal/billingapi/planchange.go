package billingapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// The self-service subscription routes (spec 7b.1, rules 5 to 8). They are served
// under the workspace path and called by Multica for an owner or an admin.
const (
	PlanChangePreviewPattern   = "POST /api/v1/subscriptions/{ws}/plan-changes/preview"
	PlanChangePattern          = "POST /api/v1/subscriptions/{ws}/plan-changes"
	PlanChangeScheduledPattern = "DELETE /api/v1/subscriptions/{ws}/plan-changes/scheduled"
	CancelPattern              = "POST /api/v1/subscriptions/{ws}/cancel"
	ResumePattern              = "POST /api/v1/subscriptions/{ws}/resume"
)

// PlanChangePatterns lists the self-service routes Handler serves.
var PlanChangePatterns = []string{
	PlanChangePreviewPattern, PlanChangePattern, PlanChangeScheduledPattern, CancelPattern, ResumePattern,
}

// codeAccountKindRequired is the refusal of seats above an individual account's limit.
const codeAccountKindRequired = "account_kind_required"

// PlanChanger writes a workspace's scheduled change and its cancel flag.
// *subscription.PlanChanges is one.
type PlanChanger interface {
	Schedule(ctx context.Context, c subscription.ScheduleChange, actor subscription.Actor) (subscription.Result, error)
	CancelScheduled(ctx context.Context, workspaceID string, actor subscription.Actor) (subscription.Result, error)
	SetCancel(ctx context.Context, workspaceID string, cancel bool, actor subscription.Actor) (subscription.Result, error)
}

// ChangeOrderCreator writes the order of a charged change and reads back an
// order by its idempotency key. *billing.Service is one.
type ChangeOrderCreator interface {
	CreateChangeOrder(ctx context.Context, c billing.ChangeOrder) (billing.Order, bool, error)
	OrderForKey(ctx context.Context, workspaceID, key string) (billing.Order, bool, error)
}

// planChangeRequest is the body of the plan-change routes. The preview reads
// the first three fields; the write also reads the rest.
type planChangeRequest struct {
	PlanKey  string `json:"plan_key"`
	Interval string `json:"interval"`
	Seats    int    `json:"seats"`
	// AcceptedAmount is the amount before VAT the client was quoted for a
	// charged change. It is required for one and must still match.
	AcceptedAmount *int64 `json:"accepted_amount"`
	// ExpectedVersion is the subscription version the quote was made at; 0 skips
	// the check.
	ExpectedVersion int64  `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type planChangePreviewResponse struct {
	Mode            string `json:"mode"`
	PlanKey         string `json:"plan_key"`
	Interval        string `json:"interval"`
	CurrentSeats    int    `json:"current_seats"`
	ResultingSeats  int    `json:"resulting_seats"`
	Currency        string `json:"currency"`
	Amount          int64  `json:"amount"`
	VATAmount       int64  `json:"vat_amount"`
	EffectiveAt     string `json:"effective_at"`
	ExpectedVersion int64  `json:"expected_version"`
	QuotedAt        string `json:"quoted_at"`
}

type planChangeOrderResponse struct {
	RequestID string `json:"request_id"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	PlanKey   string `json:"plan_key"`
	Interval  string `json:"interval"`
	Seats     int    `json:"seats"`
	Currency  string `json:"currency"`
	Amount    int64  `json:"amount"`
	VATAmount int64  `json:"vat_amount"`
	ExpiresAt string `json:"expires_at"`
}

type planChangeScheduledResponse struct {
	Mode        string `json:"mode"`
	PlanKey     string `json:"plan_key"`
	Interval    string `json:"interval"`
	Seats       int    `json:"seats"`
	EffectiveAt string `json:"effective_at"`
	Version     int64  `json:"version"`
	Changed     bool   `json:"changed"`
}

type cancelResponse struct {
	CancelAtPeriodEnd bool    `json:"cancel_at_period_end"`
	Status            string  `json:"status"`
	CurrentPeriodEnd  *string `json:"current_period_end"`
	Version           int64   `json:"version"`
	Changed           bool    `json:"changed"`
}

// writeRefusal writes a *subscription.Error as the JSON error body, with its
// detail fields beside "error" and "code".
func writeRefusal(w http.ResponseWriter, e *subscription.Error) {
	body := map[string]any{"error": e.Message, "code": e.Code}
	for k, v := range e.Detail {
		body[k] = v
	}
	writeJSON(w, e.Status, body)
}

// fail writes the response of a write that did not happen: a refusal of the
// subscription or billing rules, else an internal error.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, op string, err error) {
	var refusal *subscription.Error
	if errors.As(err, &refusal) {
		writeRefusal(w, refusal)
		return
	}
	h.internalError(w, r, op, err)
}

func rfc3339Text(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// quotedChange is a quote for a request, with the target plan it prices.
type quotedChange struct {
	subscription.Quote
	target plan.Plan
	cur    subscription.Subscription
}

// quotePlanChange checks a request against the subscription as it is now and
// prices it. It writes the refusal and returns false when the request cannot be
// made. The write checks it again against the locked row (spec 7b.1, rule 5).
func (h *Handler) quotePlanChange(w http.ResponseWriter, r *http.Request, ws string, in planChangeRequest) (quotedChange, bool) {
	ctx := r.Context()
	key := strings.TrimSpace(in.PlanKey)
	if key == "" {
		writeError(w, http.StatusBadRequest, codeInvalid, "plan_key is required")
		return quotedChange{}, false
	}
	if !validInterval(in.Interval) {
		writeError(w, http.StatusBadRequest, codeInvalid, "interval must be month or year")
		return quotedChange{}, false
	}
	target, found, err := h.Plans.Get(ctx, key)
	if err != nil {
		h.internalError(w, r, "read plan", err)
		return quotedChange{}, false
	}
	if !found {
		writeError(w, http.StatusNotFound, codePlanNotFound, "plan not found")
		return quotedChange{}, false
	}
	if err := subscription.CheckPlanForWorkspace(ctx, h.Subscriptions, ws, target); err != nil {
		h.fail(w, r, "check the plan audience", err)
		return quotedChange{}, false
	}
	if h.refuseUnpaidUsage(w, r, ws, target) {
		return quotedChange{}, false
	}
	sub, ok := h.subscriptionOf(w, r, ws)
	if !ok {
		return quotedChange{}, false
	}
	var cur subscription.Subscription
	var curPlan plan.Plan
	if sub != nil {
		cur = *sub
		if curPlan, _, err = h.Plans.Get(ctx, cur.PlanKey); err != nil {
			h.internalError(w, r, "read current plan", err)
			return quotedChange{}, false
		}
	}
	q, refusal := subscription.QuotePlanChange(subscription.QuoteInput{
		Current: cur, HasRow: sub != nil, CurrentPlan: curPlan, Target: target,
		Request: subscription.Request{PlanKey: key, Interval: in.Interval, Seats: in.Seats}, Now: h.Now().UTC(),
	})
	if refusal != nil {
		writeRefusal(w, refusal)
		return quotedChange{}, false
	}
	if !h.withinAccountSeats(w, r, ws, q.Seats) {
		return quotedChange{}, false
	}
	return quotedChange{Quote: q, target: target, cur: cur}, true
}

// withinAccountSeats refuses seats above PLATFORM_INDIVIDUAL_MAX_SEATS for an
// individual workspace, which the client answers by offering a business account
// (spec 7b.1, rule 4).
func (h *Handler) withinAccountSeats(w http.ResponseWriter, r *http.Request, ws string, seats int) bool {
	kind, err := subscription.AccountKind(r.Context(), h.Subscriptions, ws)
	if err != nil {
		h.internalError(w, r, "read account kind", err)
		return false
	}
	if kind == subscription.AccountIndividual && seats > h.IndividualMaxSeats {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":     "an individual account allows at most " + strconv.Itoa(h.IndividualMaxSeats) + " seats",
			"code":      codeAccountKindRequired,
			"max_seats": h.IndividualMaxSeats,
		})
		return false
	}
	return true
}

// previewPlanChange quotes a change without writing anything (spec 7b.1, rule 5).
func (h *Handler) previewPlanChange(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.adminOfPath(w, r)
	if !ok {
		return
	}
	var in planChangeRequest
	if !decodeBody(w, r, &in) {
		return
	}
	q, ok := h.quotePlanChange(w, r, ws, in)
	if !ok {
		return
	}
	vat, _, err := billing.WithVAT(q.Amount, h.VATRate)
	if err != nil {
		h.internalError(w, r, "compute VAT", err)
		return
	}
	now := h.Now().UTC()
	effective := now
	if q.Mode == subscription.ModeSchedule && q.cur.CurrentPeriodEnd != nil {
		effective = *q.cur.CurrentPeriodEnd
	}
	writeJSON(w, http.StatusOK, planChangePreviewResponse{
		Mode: string(q.Mode), PlanKey: q.target.Key, Interval: in.Interval,
		CurrentSeats: q.cur.Seats, ResultingSeats: q.Seats,
		Currency: q.target.Currency, Amount: q.Amount, VATAmount: vat,
		EffectiveAt: rfc3339Text(effective), ExpectedVersion: q.cur.Version, QuotedAt: rfc3339Text(now),
	})
}

// changePlan applies a plan change. A charged change writes its order (rule 5);
// a downgrade, seat reduction or interval change is stored as pending_change
// (rule 6). A repeated idempotency key returns the order it made (rule 11).
func (h *Handler) changePlan(w http.ResponseWriter, r *http.Request) {
	ws, user, ok := h.adminOfPath(w, r)
	if !ok {
		return
	}
	var in planChangeRequest
	if !decodeBody(w, r, &in) {
		return
	}
	key, ok := idempotencyKey(w, r, in.IdempotencyKey)
	if !ok {
		return
	}
	ctx := r.Context()
	if o, found, err := h.ChangeOrders.OrderForKey(ctx, ws, key); err != nil {
		h.internalError(w, r, "read order by key", err)
		return
	} else if found {
		writeChangeOrder(w, o)
		return
	}
	q, ok := h.quotePlanChange(w, r, ws, in)
	if !ok {
		return
	}
	actor := subscription.Actor{ID: user}
	if q.Mode == subscription.ModeSchedule {
		// A usage plan is billed by the calendar month, whatever interval was sent.
		if q.target.BillingModel == plan.BillingUsage {
			in.Interval = subscription.IntervalMonth
		}
		res, err := h.Changes.Schedule(ctx, subscription.ScheduleChange{
			WorkspaceID: ws, PlanKey: q.target.Key, Interval: in.Interval, Seats: q.Seats,
			ExpectedVersion: in.ExpectedVersion,
		}, actor)
		if err != nil {
			h.fail(w, r, "schedule the plan change", err)
			return
		}
		effective := h.Now().UTC()
		if res.Subscription.CurrentPeriodEnd != nil {
			effective = *res.Subscription.CurrentPeriodEnd
		}
		writeJSON(w, http.StatusOK, planChangeScheduledResponse{
			Mode: string(q.Mode), PlanKey: q.target.Key, Interval: in.Interval, Seats: q.Seats,
			EffectiveAt: rfc3339Text(effective), Version: res.Subscription.Version, Changed: res.Changed,
		})
		return
	}
	if in.AcceptedAmount == nil {
		writeError(w, http.StatusBadRequest, codeInvalid, "accepted_amount is required for a change that is charged now")
		return
	}
	o, _, err := h.ChangeOrders.CreateChangeOrder(ctx, billing.ChangeOrder{
		WorkspaceID: ws, Request: subscription.Request{PlanKey: q.target.Key, Interval: in.Interval, Seats: in.Seats},
		AcceptedAmount: *in.AcceptedAmount, ExpectedVersion: in.ExpectedVersion, VATRate: h.VATRate,
		IdempotencyKey: key, CreatedBy: user,
	})
	if err != nil {
		h.fail(w, r, "create the change order", err)
		return
	}
	writeChangeOrder(w, o)
}

func writeChangeOrder(w http.ResponseWriter, o billing.Order) {
	writeJSON(w, http.StatusOK, planChangeOrderResponse{
		RequestID: o.ID, Kind: string(o.Kind), Status: string(o.Status), PlanKey: o.PlanKey,
		Interval: o.Interval, Seats: o.Seats, Currency: o.Currency, Amount: o.Amount, VATAmount: o.VATAmount,
		ExpiresAt: rfc3339Text(o.ExpiresAt),
	})
}

// cancelScheduledPlanChange withdraws the pending change (spec 7b.1, rule 6).
func (h *Handler) cancelScheduledPlanChange(w http.ResponseWriter, r *http.Request) {
	ws, user, ok := h.adminOfPath(w, r)
	if !ok {
		return
	}
	if _, ok := idempotencyKey(w, r, ""); !ok {
		return
	}
	res, err := h.Changes.CancelScheduled(r.Context(), ws, subscription.Actor{ID: user})
	if err != nil {
		h.fail(w, r, "cancel the scheduled change", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pending_change": nil, "version": res.Subscription.Version, "changed": res.Changed})
}

// setCancel cancels (true) or resumes (false) the subscription at the period end
// (spec 7b.1, rule 8).
func (h *Handler) setCancel(cancel bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws, user, ok := h.adminOfPath(w, r)
		if !ok {
			return
		}
		if _, ok := idempotencyKey(w, r, ""); !ok {
			return
		}
		res, err := h.Changes.SetCancel(r.Context(), ws, cancel, subscription.Actor{ID: user})
		if err != nil {
			h.fail(w, r, "set cancel_at_period_end", err)
			return
		}
		var end *string
		if res.Subscription.CurrentPeriodEnd != nil {
			s := rfc3339Text(*res.Subscription.CurrentPeriodEnd)
			end = &s
		}
		writeJSON(w, http.StatusOK, cancelResponse{
			CancelAtPeriodEnd: res.Subscription.CancelAtPeriodEnd, Status: res.Subscription.Status,
			CurrentPeriodEnd: end, Version: res.Subscription.Version, Changed: res.Changed,
		})
	}
}

// registerPlanChangeRoutes mounts the self-service routes on mux.
func (h *Handler) registerPlanChangeRoutes(mux *http.ServeMux, requireUser func(http.Handler) http.Handler) {
	mux.Handle(PlanChangePreviewPattern, requireUser(http.HandlerFunc(h.previewPlanChange)))
	mux.Handle(PlanChangePattern, requireUser(http.HandlerFunc(h.changePlan)))
	mux.Handle(PlanChangeScheduledPattern, requireUser(http.HandlerFunc(h.cancelScheduledPlanChange)))
	mux.Handle(CancelPattern, requireUser(h.setCancel(true)))
	mux.Handle(ResumePattern, requireUser(h.setCancel(false)))
}
