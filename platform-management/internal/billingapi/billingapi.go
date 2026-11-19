// Package billingapi serves the Cloud billing routes that Multica's checkout
// and seat-purchase handlers forward (server/internal/handler/cloud_billing.go),
// so the existing billing page works with the manual payment provider (spec 5.8,
// Appendix A). Summary and prices are served by package subscription.
//
// A checkout or a seat purchase creates a pending order. The customer pays it by
// bank transfer and staff confirm it; nothing here touches a subscription.
package billingapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

// The routes of Multica's billing proxy. Checkout carries the workspace in its
// body, so its path has no workspace segment; the others carry it in the path.
const (
	CheckoutPattern  = "POST /api/v1/subscriptions/checkout-sessions"
	PreviewPattern   = "POST /api/v1/subscriptions/{ws}/seats/purchase-preview"
	PurchasePattern  = "POST /api/v1/subscriptions/{ws}/seats/purchases"
	ReconcilePattern = "POST /api/v1/subscriptions/{ws}/seats/reconcile"
	PortalPattern    = "POST /api/v1/subscriptions/{ws}/portal-sessions"
)

// Patterns lists the patterns Handler.Handler serves.
var Patterns = append([]string{CheckoutPattern, PreviewPattern, PurchasePattern, ReconcilePattern, PortalPattern}, PlanChangePatterns...)

const (
	maxBody           = 64 << 10
	maxIdempotencyKey = 200
	roleOwner         = "owner"
	roleAdmin         = "admin"
)

// Error codes of the JSON error body.
const (
	codeInvalid         = "invalid_request"
	codeInvalidSeats    = "invalid_seats"
	codeNotFound        = "not_found"
	codePlanNotFound    = "plan_not_found"
	codeNotPurchaseable = "plan_not_purchasable"
	codeForbidden       = "forbidden"
	codeUnavailable     = "seat_purchase_unavailable"
	codeStale           = "seat_purchase_stale"
	codeProration       = "proration_mismatch"
	codeCurrency        = "currency_mismatch"
	// codePlanChangeUnavailable is spec 7b.1's plan_change_unavailable.
	codePlanChangeUnavailable = "plan_change_unavailable"
	// codeUsageInvoiceUnpaid is spec 7b.3's usage_invoice_unpaid.
	codeUsageInvoiceUnpaid = "usage_invoice_unpaid"
	codeInternal           = "internal"

	codePaymentProviderUnavailable = "payment_provider_unavailable"
)

// UsageActivator activates a workspace on a pay as you go plan. *billing.Service
// is one.
type UsageActivator interface {
	ActivateUsage(ctx context.Context, a billing.UsageActivation) (o billing.Order, created bool, err error)
}

// UsageDebts finds a workspace's unpaid usage invoice (spec 7b.3, rule 13).
// *subscription.PgxPendingOrders is one.
type UsageDebts interface {
	// UnpaidUsageOrder returns found == false when the workspace owes nothing.
	UnpaidUsageOrder(ctx context.Context, workspaceID string) (orderID string, found bool, err error)
}

// OrderCreator creates pending orders. *billing.Service is one.
type OrderCreator interface {
	// CreateOrder returns created == false with the existing order when the
	// workspace already has one with the draft's idempotency key.
	CreateOrder(ctx context.Context, d billing.Draft) (o billing.Order, created bool, err error)
}

// WorkspaceReader reads what the address of an order page needs.
type WorkspaceReader interface {
	// Slug returns found == false for an unknown workspace.
	Slug(ctx context.Context, workspaceID string) (slug string, found bool, err error)
}

// SubscriptionReader reads a workspace's subscription and its billing profile.
// *subscription.PgxReader is one.
type SubscriptionReader interface {
	subscription.Reader
	subscription.KindReader
}

// Deps are the handler's dependencies. All but Now and Log are required.
type Deps struct {
	Subscriptions SubscriptionReader
	Plans         plan.Reader
	Members       subscription.MemberReader
	Orders        OrderCreator
	// Activator activates a pay as you go plan at checkout. *billing.Service is one.
	Activator  UsageActivator
	Workspaces WorkspaceReader
	// UsageDebts refuses a purchase while a usage invoice is unpaid. Nil means
	// no workspace owes.
	UsageDebts UsageDebts
	VATRate    billing.Rate
	// Changes and ChangeOrders serve the self-service routes (spec 7b.1).
	Changes      PlanChanger
	ChangeOrders ChangeOrderCreator
	// IndividualMaxSeats is PLATFORM_INDIVIDUAL_MAX_SEATS (spec 7b.1, rule 4).
	IndividualMaxSeats int
	// AppURL is the origin of the Multica web app, PLATFORM_APP_URL.
	AppURL string
	Now    func() time.Time
	Log    *slog.Logger
}

// Handler serves Patterns.
type Handler struct {
	Deps
}

// New returns a Handler. It refuses an AppURL that is not an absolute http or
// https origin, because every checkout response is built from it.
func New(d Deps) (*Handler, error) {
	u, err := url.Parse(d.AppURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("billingapi: AppURL must be an absolute http or https origin")
	}
	d.AppURL = u.Scheme + "://" + u.Host
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Handler{d}, nil
}

// Handler returns the mux of Patterns. requireUser is trust.Peers.RequireUser:
// the routes carry X-User-ID and no credential.
func (h *Handler) Handler(requireUser func(http.Handler) http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(CheckoutPattern, requireUser(http.HandlerFunc(h.checkout)))
	mux.Handle(PreviewPattern, requireUser(http.HandlerFunc(h.preview)))
	mux.Handle(PurchasePattern, requireUser(http.HandlerFunc(h.purchase)))
	mux.Handle(ReconcilePattern, requireUser(http.HandlerFunc(h.reconcile)))
	mux.Handle(PortalPattern, requireUser(http.HandlerFunc(h.portal)))
	h.registerPlanChangeRoutes(mux, requireUser)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}

// orderError answers a failed CreateOrder. A currency without a provider is a
// retryable 503, not a plan-limit 402.
func (h *Handler) orderError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, billing.ErrNoProvider) {
		w.Header().Set("Retry-After", "300")
		writeError(w, http.StatusServiceUnavailable, codePaymentProviderUnavailable, "payment provider unavailable for this currency")
		return
	}
	h.internalError(w, r, "create order", err)
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, op string, err error) {
	// The cause may name the database, so it goes to the log only.
	h.Log.Error("billing api: "+op+" failed", "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
}

func decodeBody(w http.ResponseWriter, r *http.Request, into any) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err == nil {
		err = json.Unmarshal(raw, into)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, codeInvalid, "body must be a JSON object")
		return false
	}
	return true
}

// idempotencyKey returns the key of the body, else the header's. It writes a 400
// and returns false when there is none or it is too long.
func idempotencyKey(w http.ResponseWriter, r *http.Request, bodyKey string) (string, bool) {
	key := strings.TrimSpace(bodyKey)
	if key == "" {
		key = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	if key == "" || len(key) > maxIdempotencyKey {
		writeError(w, http.StatusBadRequest, codeInvalid, "an idempotency key of at most 200 bytes is required")
		return "", false
	}
	return key, true
}

// admin checks that the caller is an owner or admin of the workspace: a
// non-member gets 404 and any other role 403, as the summary does.
func (h *Handler) admin(w http.ResponseWriter, r *http.Request, ws string) (user string, ok bool) {
	user, ok = trust.UserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "X-User-ID is required")
		return "", false
	}
	role, found, err := h.Members.Role(r.Context(), ws, user)
	if err != nil {
		h.internalError(w, r, "read member role", err)
		return "", false
	}
	if !found {
		writeError(w, http.StatusNotFound, codeNotFound, "not found")
		return "", false
	}
	if role != roleOwner && role != roleAdmin {
		writeError(w, http.StatusForbidden, codeForbidden, "owner or admin role required")
		return "", false
	}
	return user, true
}

// adminOfPath is admin for the routes that name the workspace in their path.
func (h *Handler) adminOfPath(w http.ResponseWriter, r *http.Request) (ws, user string, ok bool) {
	ws, ok = trust.CanonicalUUID(r.PathValue("ws"))
	if !ok {
		writeError(w, http.StatusNotFound, codeNotFound, "not found")
		return "", "", false
	}
	user, ok = h.admin(w, r, ws)
	return ws, user, ok
}

// subscriptionOf returns the workspace's subscription, or nil when it has none.
func (h *Handler) subscriptionOf(w http.ResponseWriter, r *http.Request, ws string) (*subscription.Subscription, bool) {
	sub, found, err := h.Subscriptions.Get(r.Context(), ws)
	if err != nil {
		h.internalError(w, r, "read subscription", err)
		return nil, false
	}
	if !found {
		return nil, true
	}
	return &sub, true
}

// --- checkout-sessions

// checkoutRequest is what Multica's checkout handler forwards, plus the
// platform's optional plan_key and seats. Other fields, such as the payer's
// email, are ignored.
type checkoutRequest struct {
	WorkspaceID    string `json:"workspace_id"`
	Interval       string `json:"interval"`
	IdempotencyKey string `json:"idempotency_key"`
	PlanKey        string `json:"plan_key"`
	Seats          *int   `json:"seats"`
}

type checkoutResponse struct {
	RequestID string `json:"request_id"`
	SessionID string `json:"session_id"`
	URL       string `json:"url"`
}

func validInterval(v string) bool {
	return v == subscription.IntervalMonth || v == subscription.IntervalYear
}

// pickPlan chooses the plan an order is for. A named plan must be listed
// (public and not archived) or be the workspace's own. Without a name the
// workspace's own plan is used when it sells seats, else the cheapest public
// paid plan.
func pickPlan(plans []plan.Plan, sub *subscription.Subscription, key string) (p plan.Plan, found bool) {
	if key != "" {
		for _, c := range plans {
			if c.Key == key && ((c.IsPublic && c.ArchivedAt == nil) || (sub != nil && sub.PlanKey == key)) {
				return c, true
			}
		}
		return plan.Plan{}, false
	}
	if sub != nil {
		for _, c := range plans {
			if c.Key == sub.PlanKey && subscription.SellsSeats(c) {
				return c, true
			}
		}
	}
	return subscription.CheapestPublicPaidPlan(plans)
}

// refuseUnpaidUsage answers 409 usage_invoice_unpaid, with the invoice's id,
// when the workspace has an unpaid usage invoice and the target plan is not the
// free plan (spec 7b.3, rule 13). It reports whether it answered.
func (h *Handler) refuseUnpaidUsage(w http.ResponseWriter, r *http.Request, ws string, target plan.Plan) bool {
	if h.UsageDebts == nil || target.Key == plan.FreeKey {
		return false
	}
	orderID, owes, err := h.UsageDebts.UnpaidUsageOrder(r.Context(), ws)
	if err != nil {
		h.internalError(w, r, "read unpaid usage invoice", err)
		return true
	}
	if !owes {
		return false
	}
	writeJSON(w, http.StatusConflict, map[string]any{
		"error": "the workspace has an unpaid usage invoice; pay it first",
		"code":  codeUsageInvoiceUnpaid, "order_id": orderID,
	})
	return true
}

// checkoutUsage activates a pay as you go plan at once (spec 7b.3, rule 6): no
// order waits for payment, and the activation's own order is paid for 0 in the plan currency. A
// workspace on a paid plan cannot switch here: that switch waits for the end of
// the paid period and goes through plan-changes (spec 7b.1, rule 7).
func (h *Handler) checkoutUsage(w http.ResponseWriter, r *http.Request, ws, user, key string, p plan.Plan, sub *subscription.Subscription) {
	if sub != nil && sub.PlanKey != plan.FreeKey && sub.PlanKey != p.Key {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "the workspace is on a paid plan; the switch to a usage plan is made with a plan change and applies at the end of the paid period",
			"code":  codePlanChangeUnavailable, "reason": "from_paid_plan",
		})
		return
	}
	profile, err := subscription.ProfileOrDefault(r.Context(), h.Subscriptions, ws)
	if err != nil {
		h.internalError(w, r, "read the billing profile", err)
		return
	}
	if refusal := subscription.UsageProfileReady(profile); refusal != nil {
		body := map[string]any{"error": refusal.Message, "code": refusal.Code}
		for k, v := range refusal.Detail {
			body[k] = v
		}
		writeJSON(w, refusal.Status, body)
		return
	}
	slug, found, err := h.Workspaces.Slug(r.Context(), ws)
	if err != nil {
		h.internalError(w, r, "read workspace", err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, codeNotFound, "not found")
		return
	}
	order, _, err := h.Activator.ActivateUsage(r.Context(), billing.UsageActivation{
		WorkspaceID: ws, PlanKey: p.Key, CreatedBy: user, IdempotencyKey: key, Currency: p.Currency,
	})
	if err != nil {
		h.internalError(w, r, "activate usage plan", err)
		return
	}
	writeJSON(w, http.StatusOK, checkoutResponse{
		RequestID: order.ID, SessionID: order.ID,
		URL: h.AppURL + "/" + url.PathEscape(slug) + "/billing/orders/" + order.ID,
	})
}

func (h *Handler) checkout(w http.ResponseWriter, r *http.Request) {
	var in checkoutRequest
	if !decodeBody(w, r, &in) {
		return
	}
	ws, ok := trust.CanonicalUUID(in.WorkspaceID)
	if !ok {
		writeError(w, http.StatusBadRequest, codeInvalid, "workspace_id must be a UUID")
		return
	}
	if !validInterval(in.Interval) {
		writeError(w, http.StatusBadRequest, codeInvalid, "interval must be month or year")
		return
	}
	key, ok := idempotencyKey(w, r, in.IdempotencyKey)
	if !ok {
		return
	}
	user, ok := h.admin(w, r, ws)
	if !ok {
		return
	}

	sub, ok := h.subscriptionOf(w, r, ws)
	if !ok {
		return
	}
	plans, err := h.Plans.List(r.Context())
	if err != nil {
		h.internalError(w, r, "list plans", err)
		return
	}
	p, found := pickPlan(plans, sub, in.PlanKey)
	if !found {
		writeError(w, http.StatusNotFound, codePlanNotFound, "plan not found")
		return
	}
	if err := subscription.CheckPlanForWorkspace(r.Context(), h.Subscriptions, ws, p); err != nil {
		var refusal *subscription.Error
		if errors.As(err, &refusal) {
			body := map[string]any{"error": refusal.Message, "code": refusal.Code}
			for k, v := range refusal.Detail {
				body[k] = v
			}
			writeJSON(w, refusal.Status, body)
			return
		}
		h.internalError(w, r, "check the plan audience", err)
		return
	}
	if h.refuseUnpaidUsage(w, r, ws, p) {
		return
	}
	if p.BillingModel == plan.BillingUsage {
		h.checkoutUsage(w, r, ws, user, key, p, sub)
		return
	}
	if !subscription.SellsSeats(p) {
		writeError(w, http.StatusBadRequest, codeNotPurchaseable, "the plan has no seat price")
		return
	}

	// An existing paid subscription changes plan; any other workspace subscribes.
	kind := billing.KindSubscribe
	var current *plan.Plan
	if sub != nil {
		for i := range plans {
			if plans[i].Key != sub.PlanKey {
				continue
			}
			if refuseCurrencyChange(w, *sub, plans[i], p) {
				return
			}
			if subscription.SellsSeats(plans[i]) {
				kind, current = billing.KindChangePlan, &plans[i]
			}
		}
	}

	seats, ok := h.checkoutSeats(w, r, ws, p, sub, current, in.Seats)
	if !ok || !h.withinAccountSeats(w, r, ws, seats) {
		return
	}
	amount, err := billing.Price(p, in.Interval, seats)
	if err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidSeats, "the price of these seats is too large")
		return
	}
	vat, _, err := billing.WithVAT(amount, h.VATRate)
	if err != nil {
		h.internalError(w, r, "compute VAT", err)
		return
	}
	start := h.Now().UTC()
	end, err := billing.PeriodEnd(start, in.Interval)
	if err != nil {
		h.internalError(w, r, "compute the period", err)
		return
	}

	slug, found, err := h.Workspaces.Slug(r.Context(), ws)
	if err != nil {
		h.internalError(w, r, "read workspace", err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, codeNotFound, "not found")
		return
	}
	order, _, err := h.Orders.CreateOrder(r.Context(), billing.Draft{
		WorkspaceID: ws, Kind: kind, PlanKey: p.Key, Interval: in.Interval, Seats: seats,
		Amount: amount, VATAmount: vat, Currency: p.Currency, PeriodStart: &start, PeriodEnd: &end,
		IdempotencyKey: key, CreatedBy: user,
	})
	if err != nil {
		h.orderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, checkoutResponse{
		RequestID: order.ID, SessionID: order.ID,
		URL: h.AppURL + "/" + url.PathEscape(slug) + "/billing/orders/" + order.ID,
	})
}

// checkoutSeats returns the seat count of a checkout. Without one in the
// request it is the larger of the plan's minimum, the human members, and the
// seats of the workspace's current paid subscription.
func (h *Handler) checkoutSeats(w http.ResponseWriter, r *http.Request, ws string, p plan.Plan, sub *subscription.Subscription, current *plan.Plan, asked *int) (int, bool) {
	minSeats := 1
	if p.MinSeats != nil {
		minSeats = *p.MinSeats
	}
	seats := 0
	if asked != nil {
		seats = *asked
		if seats < minSeats || seats > billing.MaxSeats {
			writeError(w, http.StatusBadRequest, codeInvalidSeats,
				fmt.Sprintf("seats must be from %d to %d for this plan", minSeats, billing.MaxSeats))
			return 0, false
		}
		return seats, true
	}
	members, err := h.Members.Count(r.Context(), ws)
	if err != nil {
		h.internalError(w, r, "count members", err)
		return 0, false
	}
	seats = max(minSeats, int(min(members, billing.MaxSeats)))
	if current != nil {
		seats = max(seats, sub.Seats)
	}
	return min(seats, billing.MaxSeats), true
}

// --- seats/purchase-preview and seats/purchases

// seatQuote is the price of extra seats at one moment.
type seatQuote struct {
	sub       subscription.Subscription
	plan      plan.Plan
	additions int
	resulting int
	// proration is the price of the extra seats for the rest of the period and
	// nextInvoice the price of all seats for a full one, both before VAT.
	proration, nextInvoice int64
}

// quote prices additional seats for the workspace's subscription. It writes the
// response and returns false when the seats cannot be bought.
func (h *Handler) quote(w http.ResponseWriter, r *http.Request, ws string, additional int) (seatQuote, bool) {
	if additional < 1 || additional > billing.MaxSeats {
		writeError(w, http.StatusBadRequest, codeInvalidSeats, "additional_seats must be positive")
		return seatQuote{}, false
	}
	sub, ok := h.subscriptionOf(w, r, ws)
	if !ok {
		return seatQuote{}, false
	}
	unavailable := func() (seatQuote, bool) {
		writeError(w, http.StatusConflict, codeUnavailable, "seats can be bought only on an active paid subscription, inside its period")
		return seatQuote{}, false
	}
	if sub == nil {
		return unavailable()
	}
	p, found, err := h.Plans.Get(r.Context(), sub.PlanKey)
	if err != nil {
		h.internalError(w, r, "read plan", err)
		return seatQuote{}, false
	}
	now := h.Now().UTC()
	if !found || !subscription.CanPurchaseSeats(p, *sub, now) {
		return unavailable()
	}
	resulting := sub.Seats + additional
	if resulting > billing.MaxSeats {
		writeError(w, http.StatusBadRequest, codeInvalidSeats, "the subscription would have too many seats")
		return seatQuote{}, false
	}
	if !h.withinAccountSeats(w, r, ws, resulting) {
		return seatQuote{}, false
	}
	unit := p.SeatPriceMonth
	if sub.Interval == subscription.IntervalYear {
		unit = p.SeatPriceYear
	}
	proration, err := billing.ProrateAdditionalSeats(*unit, additional, *sub.CurrentPeriodStart, *sub.CurrentPeriodEnd, now)
	if err != nil {
		return unavailable()
	}
	next, err := billing.Price(p, sub.Interval, resulting)
	if err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidSeats, "the price of these seats is too large")
		return seatQuote{}, false
	}
	return seatQuote{sub: *sub, plan: p, additions: additional, resulting: resulting, proration: proration, nextInvoice: next}, true
}

type previewRequest struct {
	AdditionalSeats int `json:"additional_seats"`
}

type previewResponse struct {
	CurrentSeats      int    `json:"current_seats"`
	AdditionalSeats   int    `json:"additional_seats"`
	ResultingSeats    int    `json:"resulting_seats"`
	PurchaseVersion   int64  `json:"purchase_version"`
	Currency          string `json:"currency"`
	ProrationAmount   int64  `json:"proration_amount"`
	NextInvoiceAmount int64  `json:"next_invoice_amount"`
	QuotedAt          string `json:"quoted_at"`
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.adminOfPath(w, r)
	if !ok {
		return
	}
	var in previewRequest
	if !decodeBody(w, r, &in) {
		return
	}
	q, ok := h.quote(w, r, ws, in.AdditionalSeats)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, previewResponse{
		CurrentSeats: q.sub.Seats, AdditionalSeats: q.additions, ResultingSeats: q.resulting,
		// The subscription's version moves whenever its seats or plan do, so a
		// quote taken before such a change is refused at purchase.
		PurchaseVersion: q.sub.Version, Currency: strings.ToLower(q.plan.Currency),
		ProrationAmount: q.proration, NextInvoiceAmount: q.nextInvoice,
		QuotedAt: h.Now().UTC().Format(time.RFC3339Nano),
	})
}

type purchaseRequest struct {
	AdditionalSeats         int    `json:"additional_seats"`
	ExpectedCurrentSeats    int    `json:"expected_current_seats"`
	ExpectedPurchaseVersion int64  `json:"expected_purchase_version"`
	AcceptedProrationAmount int64  `json:"accepted_proration_amount"`
	Currency                string `json:"currency"`
	IdempotencyKey          string `json:"idempotency_key"`
}

type purchaseResponse struct {
	RequestID         string `json:"request_id"`
	CurrentSeats      int    `json:"current_seats"`
	AdditionalSeats   int    `json:"additional_seats"`
	ResultingSeats    int    `json:"resulting_seats"`
	Currency          string `json:"currency"`
	ProrationAmount   int64  `json:"proration_amount"`
	NextInvoiceAmount int64  `json:"next_invoice_amount"`
	Status            string `json:"status"`
}

func (h *Handler) purchase(w http.ResponseWriter, r *http.Request) {
	ws, user, ok := h.adminOfPath(w, r)
	if !ok {
		return
	}
	var in purchaseRequest
	if !decodeBody(w, r, &in) {
		return
	}
	if in.ExpectedCurrentSeats < 1 || in.ExpectedPurchaseVersion < 1 || in.AcceptedProrationAmount < 0 || len(in.Currency) != 3 {
		writeError(w, http.StatusBadRequest, codeInvalid, "invalid seat purchase confirmation")
		return
	}
	key, ok := idempotencyKey(w, r, in.IdempotencyKey)
	if !ok {
		return
	}
	q, ok := h.quote(w, r, ws, in.AdditionalSeats)
	if !ok {
		return
	}
	currency := strings.ToLower(q.plan.Currency)
	if strings.ToLower(in.Currency) != currency {
		writeError(w, http.StatusConflict, codeCurrency, "currency does not match the subscription")
		return
	}
	if in.ExpectedCurrentSeats != q.sub.Seats || in.ExpectedPurchaseVersion != q.sub.Version {
		writeError(w, http.StatusConflict, codeStale, "the subscription changed since the quote; ask for a new one")
		return
	}
	if in.AcceptedProrationAmount != q.proration {
		writeError(w, http.StatusConflict, codeProration, "the price changed since the quote; ask for a new one")
		return
	}
	vat, _, err := billing.WithVAT(q.proration, h.VATRate)
	if err != nil {
		h.internalError(w, r, "compute VAT", err)
		return
	}
	order, _, err := h.Orders.CreateOrder(r.Context(), billing.Draft{
		WorkspaceID: ws, Kind: billing.KindAddSeats, PlanKey: q.sub.PlanKey, Interval: q.sub.Interval, Seats: q.additions,
		Amount: q.proration, VATAmount: vat, Currency: currency,
		PeriodStart: q.sub.CurrentPeriodStart, PeriodEnd: q.sub.CurrentPeriodEnd,
		IdempotencyKey: key, CreatedBy: user,
	})
	if err != nil {
		h.orderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, purchaseResponse{
		RequestID: order.ID, CurrentSeats: q.sub.Seats, AdditionalSeats: q.additions, ResultingSeats: q.resulting,
		Currency: currency, ProrationAmount: q.proration, NextInvoiceAmount: q.nextInvoice,
		Status: string(billing.StatusPending),
	})
}

// --- seats/reconcile and portal-sessions

type reconcileResponse struct {
	WorkspaceID string `json:"workspace_id"`
	Action      string `json:"action"`
	Version     int64  `json:"version"`
}

// reconcile has nothing to do: the seat ledger counts the member table
// directly (Appendix B rule 4), so there is no external quantity to correct.
func (h *Handler) reconcile(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.adminOfPath(w, r)
	if !ok {
		return
	}
	sub, ok := h.subscriptionOf(w, r, ws)
	if !ok {
		return
	}
	var version int64
	if sub != nil {
		version = sub.Version
	}
	writeJSON(w, http.StatusOK, reconcileResponse{WorkspaceID: ws, Action: "noop", Version: version})
}

// portal answers 404: there is no payment portal. The billing page does not call
// it, because the summary says portal = false.
func (h *Handler) portal(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, codeNotFound, "there is no payment portal")
}
