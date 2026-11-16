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

	Plans         plan.Reader
	PlanRunner    plan.Runner
	Subscriptions *Service
	Emails        EmailLookup
	// Cache is cleared for every workspace after a plan changes.
	Cache Invalidator

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

type routeSpec struct {
	pattern string
	roles   []staff.Role
	methods map[string]handlerFunc
}

func routeTable() []routeSpec {
	const base = "/api/platform"
	admin := []staff.Role{staff.RoleAdmin}
	return []routeSpec{
		{base + "/plans", admin, map[string]handlerFunc{http.MethodGet: (*Console).listPlans}},
		{base + "/plans/{key}", admin, map[string]handlerFunc{
			http.MethodPut:    (*Console).putPlan,
			http.MethodDelete: (*Console).deletePlan,
		}},
		{base + "/tenants/{id}/subscription", []staff.Role{staff.RoleBilling, staff.RoleAdmin}, map[string]handlerFunc{
			http.MethodPut: (*Console).putSubscription,
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

// NewConsole builds the Console. Every route is wrapped by the staff middleware
// outside its method switch, so a refused request and a wrong method both get
// the 404 of an unknown route.
func NewConsole(d ConsoleDeps) *Console {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	c := &Console{ConsoleDeps: d, mux: http.NewServeMux()}
	for _, rt := range routeTable() {
		c.mux.Handle(rt.pattern, d.RequireStaff(rt.roles...)(c.methodSwitch(rt)))
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
		call := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fn(c, w, r) })
		if r.Method == http.MethodGet {
			call.ServeHTTP(w, r)
			return
		}
		c.RequireOrigin(call).ServeHTTP(w, r)
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
	Name           string          `json:"name"`
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
	Archived       bool            `json:"archived"`
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
		next := plan.Plan{
			Key: key, Name: body.Name, Limits: body.Limits, Features: body.Features, Currency: body.Currency,
			SeatPriceMonth: body.SeatPriceMonth, SeatPriceYear: body.SeatPriceYear, SeatsFixed: body.SeatsFixed,
			MinSeats: body.MinSeats, TrialDays: body.TrialDays, IsPublic: body.IsPublic, Sort: body.Sort,
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
