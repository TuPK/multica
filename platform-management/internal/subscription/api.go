package subscription

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

// The Cloud routes Delegora's billing proxy forwards (server/internal/handler/
// cloud_billing.go). They carry X-User-ID and no credential, so the caller wraps
// them in trust.Peers.RequireUser.
const (
	SummaryPattern = "GET /api/v1/subscriptions/{ws}/summary"
	PricesPattern  = "GET /api/v1/subscriptions/{ws}/prices"
	UsagePattern   = "GET /api/v1/subscriptions/{ws}/usage"
)

// APIPatterns lists the patterns API.Handler serves.
var APIPatterns = []string{SummaryPattern, PricesPattern, UsagePattern}

// paymentProvider is the platform's addition to the summary: payment is by
// bank transfer, confirmed by staff.
const paymentProvider = "manual"

// MemberReader reads the shared member table.
type MemberReader interface {
	// Role returns found == false when the user is not a member of the workspace.
	Role(ctx context.Context, workspaceID, userID string) (role string, found bool, err error)
	// Count returns the number of members of the workspace.
	Count(ctx context.Context, workspaceID string) (int64, error)
	// Reserved returns the number of seats held by live seat operations that
	// are not yet members (Appendix B rule 4).
	Reserved(ctx context.Context, workspaceID string) (int64, error)
}

// PendingPurchase is an add_seats order that can still be paid.
type PendingPurchase struct {
	// OrderID is the order's id, which the page knows as the request id.
	OrderID string
	// Seats is the number of seats the order adds.
	Seats     int64
	ExpiresAt time.Time
}

// PendingReader reads the seat purchase a workspace is waiting on.
type PendingReader interface {
	// PendingSeats returns found == false when the workspace has no add_seats
	// order that is pending and unexpired at now. With several, it returns the
	// newest.
	PendingSeats(ctx context.Context, workspaceID string, now time.Time) (p PendingPurchase, found bool, err error)
}

// PriceUpdate is one price field of a scheduled change. Set is false when the
// change leaves the price as it is; Value is nil when it sets no listed price.
type PriceUpdate struct {
	Set   bool
	Value *int64
}

// PriceChange is a plan's scheduled price change (spec 7b.4).
type PriceChange struct {
	PlanKey        string
	EffectiveFrom  time.Time
	NoticeSentAt   *time.Time
	SeatPriceMonth PriceUpdate
	SeatPriceYear  PriceUpdate
}

// PriceChangeReader reads the schedule of plan price changes.
type PriceChangeReader interface {
	// ScheduledPriceChange returns found == false when the plan has no
	// scheduled change.
	ScheduledPriceChange(ctx context.Context, planKey string) (c PriceChange, found bool, err error)
}

// APIDeps are the API's dependencies.
type APIDeps struct {
	Subscriptions Reader
	Plans         plan.Reader
	Members       MemberReader
	Pending       PendingReader
	// PriceChanges reads the plan's scheduled price change.
	PriceChanges PriceChangeReader
	// Usage reads the usage block of a usage plan.
	Usage UsageReader
	// Now is the clock. Nil means time.Now.
	Now func() time.Time
	Log *slog.Logger
}

// API serves the billing page's reads.
type API struct{ APIDeps }

// NewAPI returns an API.
func NewAPI(d APIDeps) *API {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &API{d}
}

// Handler serves SummaryPattern, PricesPattern and UsagePattern. requireUser is
// trust.Peers.RequireUser.
func (a *API) Handler(requireUser func(http.Handler) http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(SummaryPattern, requireUser(http.HandlerFunc(a.summary)))
	mux.Handle(PricesPattern, requireUser(http.HandlerFunc(a.prices)))
	mux.Handle(UsagePattern, requireUser(http.HandlerFunc(a.usage)))
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

// errorBody is the JSON body of a refusal: error and code, then its Detail.
func errorBody(e *Error) map[string]any {
	body := map[string]any{}
	for k, v := range e.Detail {
		body[k] = v
	}
	body["error"], body["code"] = e.Message, e.Code
	return body
}

func writeAPIError(w http.ResponseWriter, e *Error) { writeJSON(w, e.Status, errorBody(e)) }

func (a *API) internalError(w http.ResponseWriter, r *http.Request, op string, err error) {
	// The cause may name the database, so it goes to the log only.
	a.Log.Error("subscription api: "+op+" failed", "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, "internal", "internal error")
}

// --- summary

type limitJSON struct {
	Mode  string `json:"mode"`
	Limit *int64 `json:"limit,omitempty"`
}

type entitlementJSON struct {
	WorkspaceID       string               `json:"workspace_id"`
	Plan              string               `json:"plan"`
	Status            string               `json:"status"`
	Seats             int64                `json:"seats"`
	Limits            map[string]limitJSON `json:"limits"`
	CurrentPeriodEnd  *string              `json:"current_period_end"`
	SnapshotExpiresAt *string              `json:"snapshot_expires_at"`
	Version           int64                `json:"version"`
}

type seatCapacityJSON struct {
	Purchased       int64  `json:"purchased"`
	Used            int64  `json:"used"`
	Reserved        int64  `json:"reserved"`
	Available       int64  `json:"available"`
	Overcommitted   bool   `json:"overcommitted"`
	Version         int64  `json:"version"`
	PendingQuantity *int64 `json:"pending_quantity"`
	// ActivePurchase is the add_seats order the workspace is waiting on.
	ActivePurchase *activePurchaseJSON `json:"active_purchase,omitempty"`
}

type activePurchaseJSON struct {
	RequestID   string `json:"request_id"`
	TargetSeats int64  `json:"target_seats"`
	Status      string `json:"status"`
	ExpiresAt   string `json:"expires_at"`
}

type actionsJSON struct {
	Checkout      bool `json:"checkout"`
	Portal        bool `json:"portal"`
	PurchaseSeats bool `json:"purchase_seats"`
	ChangePlan    bool `json:"change_plan"`
	Cancel        bool `json:"cancel"`
	Resume        bool `json:"resume"`
}

// pendingChangeJSON is the change that applies at the next renewal (spec 7b.1,
// rule 6), or null.
type pendingChangeJSON struct {
	PlanKey     string `json:"plan_key"`
	Seats       int    `json:"seats"`
	Interval    string `json:"interval"`
	RequestedAt string `json:"requested_at"`
}

type summaryJSON struct {
	Entitlement       entitlementJSON    `json:"entitlement"`
	BillingInterval   *string            `json:"billing_interval"`
	HumanMembers      int64              `json:"human_members"`
	SeatCapacity      *seatCapacityJSON  `json:"seat_capacity"`
	CancelAtPeriodEnd bool               `json:"cancel_at_period_end"`
	GraceUntil        *string            `json:"grace_until"`
	HasStripeCustomer bool               `json:"has_stripe_customer"`
	AvailableActions  actionsJSON        `json:"available_actions"`
	PaymentProvider   string             `json:"payment_provider"`
	PendingChange     *pendingChangeJSON `json:"pending_change"`
	// ScheduledPriceChange is the plan's announced price change, or null.
	ScheduledPriceChange *scheduledPriceChangeJSON `json:"scheduled_price_change"`
	// BillingModel is the plan's, seat or usage (spec 7b.3).
	BillingModel string `json:"billing_model"`
	// Usage is the usage block, only for a usage plan.
	Usage *Usage `json:"usage,omitempty"`
}

// scheduledPriceChangeJSON is a price change the plan in use will take, once its
// notice is sent (spec 7b.4 rule 11).
type scheduledPriceChangeJSON struct {
	EffectiveFrom  string `json:"effective_from"`
	SeatPriceMonth *int64 `json:"seat_price_month"`
	SeatPriceYear  *int64 `json:"seat_price_year"`
}

// scheduledJSON returns the change as the page sees it, or nil when it is not
// for plan p, has no notice yet, or is none. A price the change does not set is
// the plan's current one.
func scheduledJSON(p plan.Plan, c PriceChange, found bool) *scheduledPriceChangeJSON {
	if !found || c.PlanKey != p.Key || c.NoticeSentAt == nil {
		return nil
	}
	price := func(u PriceUpdate, current *int64) *int64 {
		if u.Set {
			return u.Value
		}
		return current
	}
	return &scheduledPriceChangeJSON{
		EffectiveFrom:  c.EffectiveFrom.UTC().Format(time.RFC3339),
		SeatPriceMonth: price(c.SeatPriceMonth, p.SeatPriceMonth),
		SeatPriceYear:  price(c.SeatPriceYear, p.SeatPriceYear),
	}
}

func pendingJSON(p *PendingChange) *pendingChangeJSON {
	if p == nil {
		return nil
	}
	return &pendingChangeJSON{
		PlanKey: p.PlanKey, Seats: p.Seats, Interval: p.Interval,
		RequestedAt: p.RequestedAt.UTC().Format(time.RFC3339Nano),
	}
}

func rfc3339(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339Nano)
	return &s
}

// PurchasedSeats is the seat capacity: the plan's fixed seats, else the
// subscription's seats. ok is false for a workspace with no seat limit.
func PurchasedSeats(p plan.Plan, sub *Subscription) (n int64, ok bool) {
	if p.SeatsFixed != nil && *p.SeatsFixed > 0 {
		return int64(*p.SeatsFixed), true
	}
	if sub != nil && sub.Seats > 0 {
		return int64(sub.Seats), true
	}
	return 0, false
}

// CanPurchaseSeats reports whether a workspace on plan p with subscription s
// can buy extra seats at now: the subscription is active, inside its current
// period, and its plan sells seats for its interval. Seats bought mid-period are
// prorated over that period.
func CanPurchaseSeats(p plan.Plan, s Subscription, now time.Time) bool {
	if s.Status != StatusActive || s.Seats < 1 || s.CurrentPeriodStart == nil || s.CurrentPeriodEnd == nil {
		return false
	}
	price := p.SeatPriceMonth
	if s.Interval == IntervalYear {
		price = p.SeatPriceYear
	}
	return positive(price) && !now.Before(*s.CurrentPeriodStart) && now.Before(*s.CurrentPeriodEnd)
}

// buildSummary assembles the response. sub is nil for a workspace with no row,
// and pending is nil when no seat purchase is waiting.
func buildSummary(workspaceID string, sub *Subscription, p plan.Plan, members, reserved int64, pending *PendingPurchase, now time.Time) summaryJSON {
	var overrides plan.Limits
	status, version := StatusActive, int64(0)
	if sub != nil {
		overrides, status, version = sub.LimitOverrides, sub.Status, sub.Version
	}

	limits := map[string]limitJSON{}
	for _, gate := range []string{plan.GateIssueCount, plan.GateAutopilotRuns} {
		if l := plan.EffectiveLimit(p.Limits, overrides, gate); l != nil {
			v := *l
			limits[gate] = limitJSON{Mode: "limited", Limit: &v}
		} else {
			limits[gate] = limitJSON{Mode: "unlimited"}
		}
	}

	out := summaryJSON{
		Entitlement: entitlementJSON{
			WorkspaceID: workspaceID, Plan: p.Key, Status: status, Limits: limits, Version: version,
		},
		HumanMembers:    members,
		PaymentProvider: paymentProvider,
		BillingModel:    p.BillingModel,
	}
	if sub != nil {
		out.Entitlement.Seats = int64(sub.Seats)
		out.Entitlement.CurrentPeriodEnd = rfc3339(sub.CurrentPeriodEnd)
		out.CancelAtPeriodEnd = sub.CancelAtPeriodEnd
		out.GraceUntil = rfc3339(sub.GraceUntil)
		// A free plan has no billing interval; a contract plan (no listed price) does.
		if p.SeatPriceMonth == nil || *p.SeatPriceMonth > 0 {
			interval := sub.Interval
			out.BillingInterval = &interval
		}
	}
	if purchased, ok := PurchasedSeats(p, sub); ok {
		out.Entitlement.Seats = purchased
		out.SeatCapacity = &seatCapacityJSON{
			Purchased:     purchased,
			Used:          members,
			Reserved:      reserved,
			Available:     max(purchased-members-reserved, 0),
			Overcommitted: members > purchased,
			// The subscription's version is 0 with no row, and the page needs a
			// positive one.
			Version: version + 1,
		}
		if pending != nil {
			target := purchased + pending.Seats
			out.SeatCapacity.PendingQuantity = &target
			out.SeatCapacity.ActivePurchase = &activePurchaseJSON{
				RequestID: pending.OrderID, TargetSeats: target, Status: "pending",
				ExpiresAt: pending.ExpiresAt.UTC().Format(time.RFC3339Nano),
			}
		}
	}
	// There is no payment portal: payment is a bank transfer.
	out.AvailableActions = actionsJSON{Checkout: true}
	if sub != nil {
		out.AvailableActions.PurchaseSeats = CanPurchaseSeats(p, *sub, now)
		out.PendingChange = pendingJSON(sub.PendingChange)
		// A free workspace (no row) changes plan by buying a period (rule 5).
		out.AvailableActions.ChangePlan = CanCancel(*sub) && !sub.CancelAtPeriodEnd
		out.AvailableActions.Cancel = CanCancel(*sub) && !sub.CancelAtPeriodEnd
		out.AvailableActions.Resume = CanResume(*sub, now)
	} else {
		out.AvailableActions.ChangePlan = true
	}
	return out
}

// authorize checks that the caller is an owner or admin of the workspace in the
// path: a non-member gets 404 and any other role 403. It writes the refusal and
// returns false when the request must stop.
func (a *API) authorize(w http.ResponseWriter, r *http.Request) (ws string, ok bool) {
	ws, ok = trust.CanonicalUUID(r.PathValue("ws"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return "", false
	}
	user, ok := trust.UserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "X-User-ID is required")
		return "", false
	}

	role, found, err := a.Members.Role(r.Context(), ws, user)
	if err != nil {
		a.internalError(w, r, "read member role", err)
		return "", false
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return "", false
	}
	if role != "owner" && role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "owner or admin role required")
		return "", false
	}
	return ws, true
}

// planOf returns the workspace's subscription (nil when it has no row) and its
// plan, which is the free plan without a row. It writes the failure and returns
// false when it cannot read them.
func (a *API) planOf(w http.ResponseWriter, r *http.Request, ws string) (*Subscription, plan.Plan, bool) {
	sub, hasSub, err := a.Subscriptions.Get(r.Context(), ws)
	if err != nil {
		a.internalError(w, r, "read subscription", err)
		return nil, plan.Plan{}, false
	}
	key, subPtr := plan.FreeKey, (*Subscription)(nil)
	if hasSub {
		key, subPtr = sub.PlanKey, &sub
	}
	p, found, err := a.Plans.Get(r.Context(), key)
	if err != nil || !found {
		if err == nil {
			err = &missingPlanError{key}
		}
		a.internalError(w, r, "read plan", err)
		return nil, plan.Plan{}, false
	}
	return subPtr, p, true
}

func (a *API) summary(w http.ResponseWriter, r *http.Request) {
	ws, ok := a.authorize(w, r)
	if !ok {
		return
	}
	subPtr, p, ok := a.planOf(w, r, ws)
	if !ok {
		return
	}
	members, err := a.Members.Count(r.Context(), ws)
	if err != nil {
		a.internalError(w, r, "count members", err)
		return
	}
	var reserved int64
	var pending *PendingPurchase
	now := a.Now()
	if _, limited := PurchasedSeats(p, subPtr); limited {
		if reserved, err = a.Members.Reserved(r.Context(), ws); err != nil {
			a.internalError(w, r, "count reserved seats", err)
			return
		}
		order, has, err := a.Pending.PendingSeats(r.Context(), ws, now)
		if err != nil {
			a.internalError(w, r, "read pending seat purchase", err)
			return
		}
		if has {
			pending = &order
		}
	}
	out := buildSummary(ws, subPtr, p, members, reserved, pending, now)
	if a.PriceChanges != nil {
		change, has, err := a.PriceChanges.ScheduledPriceChange(r.Context(), p.Key)
		if err != nil {
			a.internalError(w, r, "read scheduled price change", err)
			return
		}
		out.ScheduledPriceChange = scheduledJSON(p, change, has)
	}
	if p.BillingModel == plan.BillingUsage {
		u, err := a.Usage.Usage(r.Context(), ws, now)
		if err != nil {
			a.internalError(w, r, "read usage", err)
			return
		}
		out.Usage = &u
	}
	writeJSON(w, http.StatusOK, out)
}

type usageResponse struct {
	BillingModel string `json:"billing_model"`
	Usage        *Usage `json:"usage"`
}

// usage serves the customer's usage of the current month: the usage block of a
// usage plan, and null for a seat plan.
func (a *API) usage(w http.ResponseWriter, r *http.Request) {
	ws, ok := a.authorize(w, r)
	if !ok {
		return
	}
	_, p, ok := a.planOf(w, r, ws)
	if !ok {
		return
	}
	out := usageResponse{BillingModel: p.BillingModel}
	if p.BillingModel == plan.BillingUsage {
		u, err := a.Usage.Usage(r.Context(), ws, a.Now())
		if err != nil {
			a.internalError(w, r, "read usage", err)
			return
		}
		out.Usage = &u
	}
	writeJSON(w, http.StatusOK, out)
}

type missingPlanError struct{ key string }

func (e *missingPlanError) Error() string { return "plan " + e.key + " does not exist" }

// --- prices

type priceJSON struct {
	Currency      string `json:"currency"`
	UnitAmount    int64  `json:"unit_amount"`
	Interval      string `json:"interval"`
	IntervalCount int    `json:"interval_count"`
}

type listedPlanJSON struct {
	Key            string `json:"key"`
	Name           string `json:"name"`
	Currency       string `json:"currency"`
	SeatPriceMonth *int64 `json:"seat_price_month"`
	SeatPriceYear  *int64 `json:"seat_price_year"`
	SeatsFixed     *int   `json:"seats_fixed"`
	MinSeats       *int   `json:"min_seats"`
	TrialDays      int    `json:"trial_days"`
}

type pricesJSON struct {
	Month priceJSON        `json:"month"`
	Year  priceJSON        `json:"year"`
	Plans []listedPlanJSON `json:"plans"`
}

func positive(v *int64) bool { return v != nil && *v > 0 }

// SellsSeats reports whether p has a seat price for both intervals.
func SellsSeats(p plan.Plan) bool { return positive(p.SeatPriceMonth) && positive(p.SeatPriceYear) }

func listed(p plan.Plan) bool { return p.IsPublic && p.ArchivedAt == nil }

// CheapestPublicPaidPlan returns the listed plan that sells seats at the lowest
// monthly price. Of equal prices, the earlier in plans, which is display order,
// wins.
func CheapestPublicPaidPlan(plans []plan.Plan) (plan.Plan, bool) {
	var best plan.Plan
	found := false
	for _, p := range plans {
		if listed(p) && SellsSeats(p) && (!found || *p.SeatPriceMonth < *best.SeatPriceMonth) {
			best, found = p, true
		}
	}
	return best, found
}

// buildPrices returns the response for plan p; plans lists every public plan.
// A usage plan is never listed: its price is not a seat price (spec 7b.3, rule 15).
func buildPrices(p plan.Plan, plans []plan.Plan) pricesJSON {
	out := pricesJSON{
		Month: priceJSON{Currency: p.Currency, UnitAmount: *p.SeatPriceMonth, Interval: IntervalMonth, IntervalCount: 1},
		Year:  priceJSON{Currency: p.Currency, UnitAmount: *p.SeatPriceYear, Interval: IntervalYear, IntervalCount: 1},
		Plans: []listedPlanJSON{},
	}
	for _, l := range plans {
		if listed(l) && l.BillingModel != plan.BillingUsage {
			out.Plans = append(out.Plans, listedPlanJSON{
				Key: l.Key, Name: l.Name, Currency: l.Currency, SeatPriceMonth: l.SeatPriceMonth,
				SeatPriceYear: l.SeatPriceYear, SeatsFixed: l.SeatsFixed, MinSeats: l.MinSeats, TrialDays: l.TrialDays,
			})
		}
	}
	return out
}

// prices serves the seat prices of the plan in question: the one named by
// ?plan= when the workspace may see it, else the workspace's own plan when it is
// paid, else the cheapest public paid plan. A plan is visible when it is public
// and not archived, or when it is the workspace's own.
func (a *API) prices(w http.ResponseWriter, r *http.Request) {
	ws, ok := trust.CanonicalUUID(r.PathValue("ws"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	plans, err := a.Plans.List(r.Context())
	if err != nil {
		a.internalError(w, r, "list plans", err)
		return
	}
	sub, hasSub, err := a.Subscriptions.Get(r.Context(), ws)
	if err != nil {
		a.internalError(w, r, "read subscription", err)
		return
	}

	var chosen plan.Plan
	found := false
	if r.URL.Query().Has("plan") {
		key := r.URL.Query().Get("plan")
		for _, p := range plans {
			if p.Key == key && (listed(p) || (hasSub && sub.PlanKey == key)) {
				chosen, found = p, true
			}
		}
		if !found {
			writeError(w, http.StatusNotFound, "plan_not_found", "plan not found")
			return
		}
	} else {
		if hasSub {
			for _, p := range plans {
				if p.Key == sub.PlanKey && SellsSeats(p) {
					chosen, found = p, true
				}
			}
		}
		if !found {
			chosen, found = CheapestPublicPaidPlan(plans)
		}
	}
	if !found || !SellsSeats(chosen) {
		writeError(w, http.StatusServiceUnavailable, "prices_unavailable", "no plan with a price")
		return
	}
	writeJSON(w, http.StatusOK, buildPrices(chosen, plans))
}
