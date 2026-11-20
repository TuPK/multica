package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"reflect"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/platformauth"
	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

const (
	maxBodyBytes = 64 << 10

	targetPlan = "plan"
)

// EmailLookup returns a staff member's email for the audit label.
// *staff.Store is one.
type EmailLookup interface {
	EmailByUserID(ctx context.Context, userID string) (string, error)
}

// ConsoleDeps are the staff routes' dependencies. Now defaults to time.Now and
// Log to slog.Default.
type ConsoleDeps struct {
	// RequireStaff is platformauth.Handler.RequirePlatformStaff.
	RequireStaff func(roles ...staff.Role) func(http.Handler) http.Handler
	// RequireOrigin is platformauth.Handler.RequireOrigin. It guards every
	// method but GET.
	RequireOrigin func(http.Handler) http.Handler

	Plans      plan.Reader
	PlanRunner plan.Runner
	// PlanHistory reads the plan's audit rows for GET /plans/{key}/history.
	PlanHistory plan.HistoryReader
	// Tenants reads the tenants that a tenant-wide limit override applies to.
	Tenants       TenantWorkspaces
	Subscriptions *Service
	Profiles      *ProfileService
	Emails        EmailLookup
	// Cache is cleared for every workspace after a plan changes.
	Cache Invalidator
	// EnabledCurrencies are the currencies the platform sells in
	// (PLATFORM_CURRENCIES). A plan in another currency is refused on save.
	EnabledCurrencies []string

	Log *slog.Logger
	Now func() time.Time
}

// Console serves the staff routes for plans and subscriptions under
// /api/platform/.
type Console struct {
	ConsoleDeps
	mux *http.ServeMux
}

type handlerFunc func(*Console, http.ResponseWriter, *http.Request)

// routeSpec is one path with the matrix action it needs (spec R6 section 3.6)
// and a handler for each method it answers.
type routeSpec struct {
	pattern string
	action  string
	// methodActions overrides action for the methods it names.
	methodActions map[string]string
	methods       map[string]handlerFunc
}

// actionFor returns the matrix action of the route for method.
func (rt routeSpec) actionFor(method string) string {
	if a, ok := rt.methodActions[method]; ok {
		return a
	}
	return rt.action
}

// rolesFor returns the staff roles that may call the route with method: the
// roles the matrix gives access to every object.
func (rt routeSpec) rolesFor(method string) []staff.Role {
	return staff.RolesWith(rt.actionFor(method))
}

func routeTable() []routeSpec {
	const base = "/api/platform"
	return []routeSpec{
		{base + "/plans", staff.ActionPlansManage, map[string]string{http.MethodGet: staff.ActionPlansRead}, map[string]handlerFunc{
			http.MethodGet:  (*Console).listPlans,
			http.MethodPost: (*Console).postPlan,
		}},
		{base + "/plans/{key}", staff.ActionPlansManage, map[string]string{http.MethodGet: staff.ActionPlansRead}, map[string]handlerFunc{
			http.MethodGet:    (*Console).getPlan,
			http.MethodPut:    (*Console).putPlan,
			http.MethodDelete: (*Console).deletePlan,
		}},
		{base + "/plans/{key}/activate", staff.ActionPlansManage, nil, map[string]handlerFunc{
			http.MethodPost: (*Console).activatePlan,
		}},
		{base + "/plans/{key}/deactivate", staff.ActionPlansManage, nil, map[string]handlerFunc{
			http.MethodPost: (*Console).deactivatePlan,
		}},
		{base + "/tenants/{id}/limit-overrides", staff.ActionTenantLimitOverride, nil, map[string]handlerFunc{
			http.MethodPost: (*Console).postTenantLimitOverride,
		}},
		{base + "/plans/{key}/history", staff.ActionPlanHistoryRead, nil, map[string]handlerFunc{
			http.MethodGet: (*Console).planHistory,
		}},
		{base + "/workspaces/{id}/subscription", staff.ActionSubscriptionsManage, nil, map[string]handlerFunc{
			http.MethodPut: (*Console).putSubscription,
		}},
		{base + "/workspaces/{id}/billing-profile", staff.ActionWorkspacesRead,
			map[string]string{http.MethodPut: staff.ActionSubscriptionsManage},
			map[string]handlerFunc{
				http.MethodGet: (*Console).getProfile,
				http.MethodPut: (*Console).putProfile,
			}},
		{base + "/workspaces/{id}/billing-profile/verify", staff.ActionSubscriptionsManage, nil, map[string]handlerFunc{
			http.MethodPost: (*Console).verifyProfile,
		}},
		{base + "/workspaces/{id}/billing-profile/unverify", staff.ActionSubscriptionsManage, nil, map[string]handlerFunc{
			http.MethodPost: (*Console).unverifyProfile,
		}},
	}
}

// ConsolePatterns returns the path patterns the Console serves, for the API mux.
func ConsolePatterns() []string {
	var out []string
	for _, r := range routeTable() {
		out = append(out, r.pattern)
	}
	return out
}

// NewConsole builds the Console. Every method is wrapped by the staff middleware
// for its roles, so a refused request and a wrong method both get the 404 of an
// unknown route.
func NewConsole(d ConsoleDeps) *Console {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	c := &Console{ConsoleDeps: d, mux: http.NewServeMux()}
	for _, rt := range routeTable() {
		for method := range rt.methods {
			if len(rt.rolesFor(method)) == 0 {
				// An empty list would admit every staff role.
				panic("subscription: " + method + " " + rt.pattern + " lists no staff role")
			}
		}
		c.mux.Handle(rt.pattern, c.methodSwitch(rt))
	}
	return c
}

func (c *Console) ServeHTTP(w http.ResponseWriter, r *http.Request) { c.mux.ServeHTTP(w, r) }

func (c *Console) methodSwitch(rt routeSpec) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fn, ok := rt.methods[r.Method]
		if !ok {
			http.NotFound(w, r)
			return
		}
		var call http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fn(c, w, r) })
		if r.Method != http.MethodGet {
			call = c.RequireOrigin(call)
		}
		c.RequireStaff(rt.rolesFor(r.Method)...)(call).ServeHTTP(w, r)
	})
}

func (c *Console) fail(w http.ResponseWriter, r *http.Request, op string, err error) {
	var e *Error
	if errors.As(err, &e) {
		writeAPIError(w, e)
		return
	}
	// The cause may name the database, so it goes to the log only.
	c.Log.Error("console: "+op+" failed", "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, "internal", "internal error")
}

// decodeBody reads one JSON object into v. Unknown fields and trailing data are
// refused.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) *Error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return badRequest("invalid_body", "the request body is not a valid JSON object for this route")
	}
	if dec.More() {
		return badRequest("invalid_body", "the request body has more than one JSON value")
	}
	return nil
}

type actor struct{ ID, Label string }

func (c *Console) currentActor(r *http.Request) (actor, error) {
	id, ok := platformauth.IdentityFrom(r.Context())
	if !ok {
		return actor{}, errors.New("no staff identity in the request context")
	}
	email, err := c.Emails.EmailByUserID(r.Context(), id.UserID)
	if err != nil {
		return actor{}, err
	}
	return actor{ID: id.UserID, Label: email}, nil
}

func (a actor) entry(e audit.Entry) audit.Entry {
	e.ActorType, e.ActorID, e.ActorLabel = audit.ActorStaff, a.ID, a.Label
	return e
}

// --- subscription

type putSubscriptionBody struct {
	PlanKey            string      `json:"plan_key"`
	Seats              int         `json:"seats"`
	Interval           string      `json:"interval"`
	CurrentPeriodStart *time.Time  `json:"current_period_start"`
	CurrentPeriodEnd   *time.Time  `json:"current_period_end"`
	TrialEnd           *time.Time  `json:"trial_end"`
	GraceUntil         *time.Time  `json:"grace_until"`
	CancelAtPeriodEnd  bool        `json:"cancel_at_period_end"`
	LimitOverrides     plan.Limits `json:"limit_overrides"`
	Reason             string      `json:"reason"`
}

type subscriptionJSON struct {
	WorkspaceID        string      `json:"workspace_id"`
	PlanKey            string      `json:"plan_key"`
	Status             string      `json:"status"`
	Seats              int         `json:"seats"`
	Interval           string      `json:"interval"`
	CurrentPeriodStart *string     `json:"current_period_start"`
	CurrentPeriodEnd   *string     `json:"current_period_end"`
	TrialEnd           *string     `json:"trial_end"`
	GraceUntil         *string     `json:"grace_until"`
	CancelAtPeriodEnd  bool        `json:"cancel_at_period_end"`
	LimitOverrides     plan.Limits `json:"limit_overrides"`
	Version            int64       `json:"version"`
	UpdatedAt          *string     `json:"updated_at"`
}

func toSubscriptionJSON(s Subscription) subscriptionJSON {
	var updated *string
	if !s.UpdatedAt.IsZero() {
		updated = rfc3339(&s.UpdatedAt)
	}
	return subscriptionJSON{
		WorkspaceID: s.WorkspaceID, PlanKey: s.PlanKey, Status: s.Status, Seats: s.Seats, Interval: s.Interval,
		CurrentPeriodStart: rfc3339(s.CurrentPeriodStart), CurrentPeriodEnd: rfc3339(s.CurrentPeriodEnd),
		TrialEnd: rfc3339(s.TrialEnd), GraceUntil: rfc3339(s.GraceUntil),
		CancelAtPeriodEnd: s.CancelAtPeriodEnd, LimitOverrides: orEmpty(s.LimitOverrides),
		Version: s.Version, UpdatedAt: updated,
	}
}

// putSubscription replaces a workspace's subscription fields. Every field of the
// body is the new value: one the body leaves out is cleared (times, overrides,
// cancellation) or defaulted (seats 0, interval month).
func (c *Console) putSubscription(w http.ResponseWriter, r *http.Request) {
	ws, ok := trust.CanonicalUUID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	var body putSubscriptionBody
	if perr := decodeBody(w, r, &body); perr != nil {
		c.fail(w, r, "decode subscription", perr)
		return
	}
	if body.Interval == "" {
		body.Interval = IntervalMonth
	}
	a, err := c.currentActor(r)
	if err != nil {
		c.fail(w, r, "resolve staff actor", err)
		return
	}
	res, err := c.Subscriptions.Apply(r.Context(), Change{
		WorkspaceID: ws, PlanKey: body.PlanKey, Seats: body.Seats, Interval: body.Interval,
		CurrentPeriodStart: body.CurrentPeriodStart, CurrentPeriodEnd: body.CurrentPeriodEnd,
		TrialEnd: body.TrialEnd, GraceUntil: body.GraceUntil, CancelAtPeriodEnd: body.CancelAtPeriodEnd,
		LimitOverrides: body.LimitOverrides, Reason: body.Reason, ActorID: a.ID, ActorLabel: a.Label,
	})
	if err != nil {
		c.fail(w, r, "change subscription", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": res.Changed, "subscription": toSubscriptionJSON(res.Subscription)})
}

// --- billing profile

// getProfile returns the tenant's billing profile for the console. It carries
// verified_at and omits verified_by, like the customer's read.
func (c *Console) getProfile(w http.ResponseWriter, r *http.Request) {
	ws, ok := trust.CanonicalUUID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	p, found, err := c.Profiles.Read(r.Context(), ws)
	if err != nil {
		c.fail(w, r, "read billing profile", err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_id": ws, "account_kind": p.AccountKind, "legal_name": p.LegalName, "tax_code": p.TaxCode,
		"address": p.Address, "invoice_email": p.InvoiceEmail,
		"verified": p.VerifiedAt != nil, "verified_at": rfc3339(p.VerifiedAt),
	})
}

// putProfile sets a tenant's account kind and the four profile fields, with the
// same validation as the customer's route. It never sets verified_at; changing
// the legal name or tax code clears it (spec 7b.1, rule 12).
func (c *Console) putProfile(w http.ResponseWriter, r *http.Request) {
	ws, ok := trust.CanonicalUUID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	var in profileRequest
	if perr := decodeBody(w, r, &in); perr != nil {
		c.fail(w, r, "decode billing profile", perr)
		return
	}
	change := KindChange{WorkspaceID: ws, Kind: in.AccountKind, ActorType: audit.ActorStaff}
	if in.LegalName != nil || in.TaxCode != nil || in.Address != nil || in.InvoiceEmail != nil {
		f := ProfileFields{LegalName: deref(in.LegalName), TaxCode: deref(in.TaxCode), Address: deref(in.Address), InvoiceEmail: deref(in.InvoiceEmail)}.trimmed()
		if e := checkProfileFormat(f); e != nil {
			c.fail(w, r, "check billing profile", e)
			return
		}
		change.Profile = &f
	}
	a, err := c.currentActor(r)
	if err != nil {
		c.fail(w, r, "resolve staff actor", err)
		return
	}
	change.ActorID, change.ActorLabel = a.ID, a.Label
	res, err := c.Profiles.ChangeAccountKind(r.Context(), change)
	if err != nil {
		c.fail(w, r, "change billing profile", err)
		return
	}
	p := res.Profile
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_id": ws, "account_kind": p.AccountKind, "legal_name": p.LegalName, "tax_code": p.TaxCode,
		"address": p.Address, "invoice_email": p.InvoiceEmail,
		"verified": p.VerifiedAt != nil, "verified_at": rfc3339(p.VerifiedAt), "changed": res.Changed,
	})
}

// --- billing profile verification

func (c *Console) verifyProfile(w http.ResponseWriter, r *http.Request)   { c.setVerified(w, r, true) }
func (c *Console) unverifyProfile(w http.ResponseWriter, r *http.Request) { c.setVerified(w, r, false) }

// setVerified marks the workspace's billing profile verified or unverified after
// the staff member's out-of-system check of the tax code (spec 7b.1, rule 12).
// The routes take no body.
func (c *Console) setVerified(w http.ResponseWriter, r *http.Request, verified bool) {
	ws, ok := trust.CanonicalUUID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	a, err := c.currentActor(r)
	if err != nil {
		c.fail(w, r, "resolve staff actor", err)
		return
	}
	res, err := c.Profiles.Verify(r.Context(), VerifyChange{
		WorkspaceID: ws, Verified: verified, At: c.Now().UTC().Truncate(time.Microsecond),
		ActorType: audit.ActorStaff, ActorID: a.ID, ActorLabel: a.Label,
	})
	if err != nil {
		c.fail(w, r, "verify billing profile", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_id": ws, "account_kind": res.Profile.AccountKind,
		"verified": res.Profile.VerifiedAt != nil, "verified_at": rfc3339(res.Profile.VerifiedAt),
	})
}

// --- plans

func (c *Console) listPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := c.Plans.List(r.Context())
	if err != nil {
		c.fail(w, r, "list plans", err)
		return
	}
	if plans == nil {
		plans = []plan.Plan{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"plans": plans})
}

type putPlanBody struct {
	ExpectedVersion *int64          `json:"expected_version"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Limits          plan.Limits     `json:"limits"`
	Features        map[string]bool `json:"features"`
	Currency        string          `json:"currency"`
	SeatPriceMonth  *int64          `json:"seat_price_month"`
	SeatPriceYear   *int64          `json:"seat_price_year"`
	SeatsFixed      *int            `json:"seats_fixed"`
	MinSeats        *int            `json:"min_seats"`
	TrialDays       int             `json:"trial_days"`
	IsPublic        bool            `json:"is_public"`
	Sort            int             `json:"sort"`
	Archived        bool            `json:"archived"`
	// ConfirmAffectsSubscribers acknowledges that a lowered limit applies to the
	// plan's existing subscribers (spec 3b.1 rule 3).
	ConfirmAffectsSubscribers bool `json:"confirm_affects_subscribers"`
}

// planDoc is a plan as the JSON object clients see, without its key, so two
// plans can be compared and diffed field by field.
func planDoc(p plan.Plan) (map[string]any, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	delete(doc, "key")
	return doc, nil
}

// planChanges returns {field: {from, to}} for each field that differs. from is
// nil for every field of a plan that is new.
// limitsLowered reports whether next lowers any gate of old. A nil value is
// unlimited, so going from unlimited to a number is a lowering, and the reverse is not.
func limitsLowered(old, next plan.Limits) bool {
	for gate := range old {
		if v, ok := next[gate]; ok && lowered(old[gate], v) {
			return true
		}
	}
	for gate, v := range next {
		if _, ok := old[gate]; !ok && v != nil {
			return true
		}
	}
	return false
}

func lowered(old, next *int64) bool {
	if next == nil {
		return false
	}
	return old == nil || *next < *old
}

func planChanges(from *plan.Plan, to plan.Plan) (map[string]any, error) {
	newDoc, err := planDoc(to)
	if err != nil {
		return nil, err
	}
	oldDoc := map[string]any{}
	if from != nil {
		if oldDoc, err = planDoc(*from); err != nil {
			return nil, err
		}
	}
	out := map[string]any{}
	for field, v := range newDoc {
		old, had := oldDoc[field]
		if from != nil && had && reflect.DeepEqual(old, v) {
			continue
		}
		out[field] = map[string]any{"from": old, "to": v}
	}
	return out, nil
}

func planKey(w http.ResponseWriter, r *http.Request, onInvalid func() *Error) (string, bool) {
	key := r.PathValue("key")
	if err := plan.ValidateKey(key); err != nil {
		writeFromError(w, onInvalid())
		return "", false
	}
	return key, true
}

func writeFromError(w http.ResponseWriter, e *Error) { writeAPIError(w, e) }

// putPlan creates a plan or replaces every field of it. Its key is the path's;
// the body names no key. A plan that is archived is kept for its subscribers and
// takes no new ones.
func (c *Console) putPlan(w http.ResponseWriter, r *http.Request) {
	key, ok := planKey(w, r, func() *Error {
		return badRequest("invalid_plan", "key must be 1 to 32 lower-case letters, digits or underscores, starting with a letter")
	})
	if !ok {
		return
	}
	var body putPlanBody
	if perr := decodeBody(w, r, &body); perr != nil {
		c.fail(w, r, "decode plan", perr)
		return
	}
	a, err := c.currentActor(r)
	if err != nil {
		c.fail(w, r, "resolve staff actor", err)
		return
	}

	var stored plan.Plan
	changed := false
	err = c.PlanRunner.InTx(r.Context(), func(tx plan.Tx) error {
		changed = false
		old, exists, err := tx.Get(r.Context(), key)
		if err != nil {
			return err
		}
		// Get locked the row, so this check and the write below are one atomic step.
		if exists && body.ExpectedVersion != nil && *body.ExpectedVersion != old.Version {
			return &Error{Status: http.StatusConflict, Code: "plan_version_conflict",
				Message: "the plan changed since it was read; reload it and retry"}
		}
		if exists && body.Currency != "" && body.Currency != old.Currency {
			inUse, err := tx.InUse(r.Context(), key)
			if err != nil {
				return err
			}
			if inUse {
				return &Error{Status: http.StatusConflict, Code: "plan_currency_locked",
					Message: "the currency of a plan that has a subscription or an order cannot change"}
			}
		}
		next := plan.Plan{
			Key: key, Name: body.Name, Description: body.Description, Limits: body.Limits, Features: body.Features, Currency: body.Currency,
			SeatPriceMonth: body.SeatPriceMonth, SeatPriceYear: body.SeatPriceYear, SeatsFixed: body.SeatsFixed,
			MinSeats: body.MinSeats, TrialDays: body.TrialDays, IsPublic: body.IsPublic, Sort: body.Sort,
		}
		// A body that omits a price or the currency keeps the stored value, so a
		// rename does not write NULL prices (prices change only by a scheduled price
		// change, spec 7b.4).
		if exists {
			if body.SeatPriceMonth == nil {
				next.SeatPriceMonth = old.SeatPriceMonth
			}
			if body.SeatPriceYear == nil {
				next.SeatPriceYear = old.SeatPriceYear
			}
			if next.Currency == "" {
				next.Currency = old.Currency
			}
		}
		if next.Limits == nil {
			next.Limits = plan.Limits{}
		}
		if next.Features == nil {
			next.Features = map[string]bool{}
		}
		if body.Archived {
			if exists && old.ArchivedAt != nil {
				next.ArchivedAt = old.ArchivedAt
			} else {
				at := c.Now().UTC().Truncate(time.Microsecond)
				next.ArchivedAt = &at
			}
		}
		if err := plan.Validate(next); err != nil {
			return badRequest("invalid_plan", err.Error())
		}
		// Lowering a limit only blocks new purchases, so it still needs the caller
		// to confirm that subscribers are affected (spec 3b.1 rule 3, D88).
		if exists && limitsLowered(old.Limits, next.Limits) && !body.ConfirmAffectsSubscribers {
			n, err := tx.CountSubscriptions(r.Context(), key)
			if err != nil {
				return err
			}
			if n > 0 {
				e := &Error{Status: http.StatusConflict, Code: "plan_limit_change_confirm_required",
					Message: "lowering a limit of a plan with subscribers affects them; resend with confirm_affects_subscribers = true"}
				e.Detail = map[string]any{"affected_subscriptions": n}
				return e
			}
		}
		if err := plan.ValidateCurrency(next.Currency, c.EnabledCurrencies); err != nil {
			e := badRequest("price_invalid", err.Error())
			e.Detail = map[string]any{"field": "currency"}
			return e
		}
		var before *plan.Plan
		if exists {
			before = &old
		}
		diff, err := planChanges(before, next)
		if err != nil {
			return err
		}
		stored = next
		if len(diff) == 0 {
			return nil
		}
		if err := tx.Upsert(r.Context(), next); err != nil {
			return err
		}
		changed = true
		return audit.Record(r.Context(), tx, a.entry(audit.Entry{
			Action: audit.ActionPlanChanged, TargetType: targetPlan, TargetID: key, TargetLabel: next.Name,
			Detail: map[string]any{"created": !exists, "changes": diff},
		}))
	})
	if err != nil {
		c.fail(w, r, "put plan", err)
		return
	}
	if changed {
		c.Cache.InvalidateAll()
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed, "plan": stored})
}

type postPlanBody struct {
	Key            string          `json:"key"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Limits         plan.Limits     `json:"limits"`
	Features       map[string]bool `json:"features"`
	Currency       string          `json:"currency"`
	SeatPriceMonth *int64          `json:"seat_price_month"`
	SeatPriceYear  *int64          `json:"seat_price_year"`
	SeatsFixed     *int            `json:"seats_fixed"`
	MinSeats       *int            `json:"min_seats"`
	TrialDays      int             `json:"trial_days"`
	IsPublic       bool            `json:"is_public"`
	Sort           int             `json:"sort"`
	Audience       string          `json:"audience"`
	BillingModel   string          `json:"billing_model"`
}

// postPlan creates a plan, a private one included, with its initial prices
// (spec 7b.4 rules 2 and 13). A key that exists is a 409; changing a plan is PUT.
func (c *Console) postPlan(w http.ResponseWriter, r *http.Request) {
	var body postPlanBody
	if perr := decodeBody(w, r, &body); perr != nil {
		c.fail(w, r, "decode plan", perr)
		return
	}
	a, err := c.currentActor(r)
	if err != nil {
		c.fail(w, r, "resolve staff actor", err)
		return
	}
	next := plan.Plan{
		Key: body.Key, Name: body.Name, Limits: body.Limits, Features: body.Features, Currency: body.Currency,
		SeatPriceMonth: body.SeatPriceMonth, SeatPriceYear: body.SeatPriceYear, SeatsFixed: body.SeatsFixed,
		MinSeats: body.MinSeats, TrialDays: body.TrialDays, IsPublic: body.IsPublic, Sort: body.Sort,
		Audience: body.Audience, BillingModel: body.BillingModel, Description: body.Description,
	}
	if next.Limits == nil {
		next.Limits = plan.Limits{}
	}
	if next.Features == nil {
		next.Features = map[string]bool{}
	}
	if err := plan.Validate(next); err != nil {
		c.fail(w, r, "post plan", badRequest("invalid_plan", err.Error()))
		return
	}
	if err := plan.ValidateCurrency(next.Currency, c.EnabledCurrencies); err != nil {
		e := badRequest("price_invalid", err.Error())
		e.Detail = map[string]any{"field": "currency"}
		c.fail(w, r, "post plan", e)
		return
	}
	diff, err := planChanges(nil, next)
	if err != nil {
		c.fail(w, r, "post plan", err)
		return
	}
	err = c.PlanRunner.InTx(r.Context(), func(tx plan.Tx) error {
		inserted, err := tx.InsertIfAbsent(r.Context(), next)
		if err != nil {
			return err
		}
		if !inserted {
			return &Error{Status: http.StatusConflict, Code: "plan_exists", Message: "a plan with this key exists; change it with PUT"}
		}
		return audit.Record(r.Context(), tx, a.entry(audit.Entry{
			Action: audit.ActionPlanChanged, TargetType: targetPlan, TargetID: next.Key, TargetLabel: next.Name,
			Detail: map[string]any{"created": true, "changes": diff},
		}))
	})
	if err != nil {
		c.fail(w, r, "post plan", err)
		return
	}
	c.Cache.InvalidateAll()
	stored, _, err := c.Plans.Get(r.Context(), next.Key)
	if err != nil {
		c.fail(w, r, "read created plan", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"plan": stored})
}

func planInUse(msg string) *Error {
	return &Error{Status: http.StatusConflict, Code: "plan_in_use", Message: msg}
}

// deletePlan removes a plan nobody uses. A plan that subscriptions name can only
// be archived (putPlan with "archived": true), and the free plan, which every
// workspace without a subscription row is on, cannot be removed.
func (c *Console) deletePlan(w http.ResponseWriter, r *http.Request) {
	key, ok := planKey(w, r, NotFound)
	if !ok {
		return
	}
	a, err := c.currentActor(r)
	if err != nil {
		c.fail(w, r, "resolve staff actor", err)
		return
	}
	err = c.PlanRunner.InTx(r.Context(), func(tx plan.Tx) error {
		p, exists, err := tx.Get(r.Context(), key)
		if err != nil {
			return err
		}
		if !exists {
			return NotFound()
		}
		if key == plan.FreeKey {
			return planInUse("workspaces with no subscription are on the free plan; archive it instead")
		}
		n, err := tx.CountSubscriptions(r.Context(), key)
		if err != nil {
			return err
		}
		if n > 0 {
			return planInUse("subscriptions use this plan; archive it instead")
		}
		old, err := planDoc(p)
		if err != nil {
			return err
		}
		gone := make(map[string]any, len(old))
		for field, v := range old {
			gone[field] = map[string]any{"from": v, "to": nil}
		}
		if _, err := tx.Delete(r.Context(), key); err != nil {
			return err
		}
		return audit.Record(r.Context(), tx, a.entry(audit.Entry{
			Action: audit.ActionPlanChanged, TargetType: targetPlan, TargetID: key, TargetLabel: p.Name,
			Detail: map[string]any{"deleted": true, "changes": gone},
		}))
	})
	if err != nil {
		c.fail(w, r, "delete plan", err)
		return
	}
	c.Cache.InvalidateAll()
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}
