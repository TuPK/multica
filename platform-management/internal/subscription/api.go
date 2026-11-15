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

// The Cloud routes Multica's billing proxy forwards (server/internal/handler/
// cloud_billing.go). They carry X-User-ID and no credential, so the caller wraps
// them in trust.Peers.RequireUser.
const (
	SummaryPattern = "GET /api/v1/subscriptions/{ws}/summary"
	PricesPattern  = "GET /api/v1/subscriptions/{ws}/prices"
)

// APIPatterns lists the patterns API.Handler serves.
var APIPatterns = []string{SummaryPattern, PricesPattern}

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

// APIDeps are the API's dependencies.
type APIDeps struct {
	Subscriptions Reader
	Plans         plan.Reader
	Members       MemberReader
	Log           *slog.Logger
}

// API serves the billing page's two reads.
type API struct{ APIDeps }

// NewAPI returns an API.
func NewAPI(d APIDeps) *API {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &API{d}
}

// Handler serves SummaryPattern and PricesPattern. requireUser is
// trust.Peers.RequireUser.
func (a *API) Handler(requireUser func(http.Handler) http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(SummaryPattern, requireUser(http.HandlerFunc(a.summary)))
	mux.Handle(PricesPattern, requireUser(http.HandlerFunc(a.prices)))
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
}

type actionsJSON struct {
	Checkout      bool `json:"checkout"`
	Portal        bool `json:"portal"`
	PurchaseSeats bool `json:"purchase_seats"`
}

type summaryJSON struct {
	Entitlement       entitlementJSON   `json:"entitlement"`
	BillingInterval   *string           `json:"billing_interval"`
	HumanMembers      int64             `json:"human_members"`
	SeatCapacity      *seatCapacityJSON `json:"seat_capacity"`
	CancelAtPeriodEnd bool              `json:"cancel_at_period_end"`
	GraceUntil        *string           `json:"grace_until"`
	HasStripeCustomer bool              `json:"has_stripe_customer"`
	AvailableActions  actionsJSON       `json:"available_actions"`
	PaymentProvider   string            `json:"payment_provider"`
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

// buildSummary assembles the response. sub is nil for a workspace with no row.
func buildSummary(workspaceID string, sub *Subscription, p plan.Plan, members, reserved int64) summaryJSON {
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
	}
	// Checkout, the portal and seat purchases are not served yet, so the page is
	// not offered them.
	return out
}

func (a *API) summary(w http.ResponseWriter, r *http.Request) {
	ws, ok := trust.CanonicalUUID(r.PathValue("ws"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	user, ok := trust.UserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "X-User-ID is required")
		return
	}

	role, found, err := a.Members.Role(r.Context(), ws, user)
	if err != nil {
		a.internalError(w, r, "read member role", err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	if role != "owner" && role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "owner or admin role required")
		return
	}

	sub, hasSub, err := a.Subscriptions.Get(r.Context(), ws)
	if err != nil {
		a.internalError(w, r, "read subscription", err)
		return
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
		return
	}
	members, err := a.Members.Count(r.Context(), ws)
	if err != nil {
		a.internalError(w, r, "count members", err)
		return
	}
	var reserved int64
	if _, limited := PurchasedSeats(p, subPtr); limited {
		if reserved, err = a.Members.Reserved(r.Context(), ws); err != nil {
			a.internalError(w, r, "count reserved seats", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, buildSummary(ws, subPtr, p, members, reserved))
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

// buildPrices returns the response, and false when no public plan has a price.
// The page shows one price per interval, so month and year come from the first
// public plan, in display order, that sells seats in both; plans lists every
// public plan.
func buildPrices(plans []plan.Plan) (pricesJSON, bool) {
	var out pricesJSON
	found := false
	for _, p := range plans {
		if !p.IsPublic || p.ArchivedAt != nil {
			continue
		}
		out.Plans = append(out.Plans, listedPlanJSON{
			Key: p.Key, Name: p.Name, Currency: p.Currency, SeatPriceMonth: p.SeatPriceMonth,
			SeatPriceYear: p.SeatPriceYear, SeatsFixed: p.SeatsFixed, MinSeats: p.MinSeats, TrialDays: p.TrialDays,
		})
		if !found && positive(p.SeatPriceMonth) && positive(p.SeatPriceYear) {
			out.Month = priceJSON{Currency: p.Currency, UnitAmount: *p.SeatPriceMonth, Interval: IntervalMonth, IntervalCount: 1}
			out.Year = priceJSON{Currency: p.Currency, UnitAmount: *p.SeatPriceYear, Interval: IntervalYear, IntervalCount: 1}
			found = true
		}
	}
	if out.Plans == nil {
		out.Plans = []listedPlanJSON{}
	}
	return out, found
}

func (a *API) prices(w http.ResponseWriter, r *http.Request) {
	if _, ok := trust.CanonicalUUID(r.PathValue("ws")); !ok {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	plans, err := a.Plans.List(r.Context())
	if err != nil {
		a.internalError(w, r, "list plans", err)
		return
	}
	out, ok := buildPrices(plans)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "prices_unavailable", "no public plan has a price")
		return
	}
	writeJSON(w, http.StatusOK, out)
}
