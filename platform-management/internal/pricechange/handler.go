package pricechange

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/platformauth"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

const maxBodyBytes = 64 << 10

// EmailLookup returns a staff member's email for the audit label.
// *staff.Store is one.
type EmailLookup interface {
	EmailByUserID(ctx context.Context, userID string) (string, error)
}

// HandlerDeps are the staff routes' dependencies. Now defaults to time.Now and
// Log to slog.Default.
type HandlerDeps struct {
	// RequireStaff is platformauth.Handler.RequirePlatformStaff.
	RequireStaff func(roles ...staff.Role) func(http.Handler) http.Handler
	// RequireOrigin is platformauth.Handler.RequireOrigin. It guards every
	// method but GET.
	RequireOrigin func(http.Handler) http.Handler

	Service *Service
	Emails  EmailLookup
	Log     *slog.Logger
	Now     func() time.Time
}

// Patterns are the routes the handler serves, under /api/platform.
var Patterns = []string{
	"/api/platform/plans/{key}/price-changes",
	"/api/platform/plans/{key}/price-changes/scheduled",
}

type handler struct{ HandlerDeps }

// endpoint is a handler with the matrix action it needs (spec R6 section 3.6).
type endpoint struct {
	action string
	fn     http.HandlerFunc
}

type route struct {
	pattern string
	methods map[string]endpoint
}

// routes is the route table. h may be nil when only the actions are read.
func routes(h *handler) []route {
	return []route{
		{Patterns[0], map[string]endpoint{
			http.MethodGet:  {staff.ActionPriceChangeRead, h.history},
			http.MethodPost: {staff.ActionPriceChangeManage, h.schedule},
		}},
		{Patterns[1], map[string]endpoint{http.MethodDelete: {staff.ActionPriceChangeManage, h.cancel}}},
	}
}

// NewHandler returns the staff routes of the price-change schedule. Every route
// is wrapped by the staff middleware outside its method switch, so a refused
// request and a wrong method both get the 404 of an unknown route. Only system_admin
// schedules and cancels; billing and system_admin read the history (spec 7b.4 rule 1).
func NewHandler(d HandlerDeps) http.Handler {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	h := &handler{d}
	mux := http.NewServeMux()
	for _, rt := range routes(h) {
		calls := map[string]http.Handler{}
		for method, ep := range rt.methods {
			roles := staff.RolesWith(ep.action)
			if len(roles) == 0 {
				// An empty list would admit every staff role.
				panic("pricechange: " + method + " " + rt.pattern + " lists no staff role")
			}
			var call http.Handler = ep.fn
			if method != http.MethodGet {
				call = d.RequireOrigin(call)
			}
			calls[method] = d.RequireStaff(roles...)(call)
		}
		mux.HandleFunc(rt.pattern, func(w http.ResponseWriter, r *http.Request) {
			call, ok := calls[r.Method]
			if !ok {
				http.NotFound(w, r)
				return
			}
			call.ServeHTTP(w, r)
		})
	}
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, e *Error) {
	body := map[string]any{}
	for k, v := range e.Detail {
		body[k] = v
	}
	body["error"], body["code"] = e.Message, e.Code
	writeJSON(w, e.Status, body)
}

func (h *handler) fail(w http.ResponseWriter, r *http.Request, op string, err error) {
	var e *Error
	if errors.As(err, &e) {
		writeError(w, e)
		return
	}
	// The cause may name the database, so it goes to the log only.
	h.Log.Error("price changes: "+op+" failed", "path", r.URL.Path, "error", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error", "code": "internal"})
}

func (h *handler) actor(r *http.Request) (Actor, error) {
	id, ok := platformauth.IdentityFrom(r.Context())
	if !ok {
		return Actor{}, errors.New("no staff identity in the request context")
	}
	email, err := h.Emails.EmailByUserID(r.Context(), id.UserID)
	if err != nil {
		return Actor{}, err
	}
	return Actor{ID: id.UserID, Label: email}, nil
}

func (h *handler) history(w http.ResponseWriter, r *http.Request) {
	changes, err := h.Service.History(r.Context(), r.PathValue("key"))
	if err != nil {
		h.fail(w, r, "history", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"price_changes": changes})
}

func (h *handler) schedule(w http.ResponseWriter, r *http.Request) {
	var req Request
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || dec.More() {
		writeError(w, &Error{Status: http.StatusBadRequest, Code: "invalid_body",
			Message: "the request body is not a valid JSON object for this route"})
		return
	}
	a, err := h.actor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	dryRun := r.URL.Query().Get("dry_run") == "1"
	res, err := h.Service.Schedule(r.Context(), a, r.PathValue("key"), req, dryRun, h.Now())
	if err != nil {
		h.fail(w, r, "schedule", err)
		return
	}
	if dryRun {
		writeJSON(w, http.StatusOK, map[string]any{
			"dry_run": true, "affected_subscribers": res.Affected, "notice_date": noticeDate(res.NoticeDate),
		})
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

// noticeDate is null in the JSON when no notice goes out.
func noticeDate(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (h *handler) cancel(w http.ResponseWriter, r *http.Request) {
	a, err := h.actor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	c, err := h.Service.Cancel(r.Context(), a, r.PathValue("key"), h.Now())
	if err != nil {
		h.fail(w, r, "cancel", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"price_change": c})
}

// RefusePriceFields wraps the handler of PUT /plans/{key}: a body that holds a
// price field is a 400 price_field_via_schedule naming the fields (spec 7b.4
// rule 2). Other methods, other bodies and bodies that are not a JSON object
// reach next unchanged.
func RefusePriceFields(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))
		var fields map[string]json.RawMessage
		if err != nil || json.Unmarshal(body, &fields) != nil {
			next.ServeHTTP(w, r)
			return
		}
		var refused []string
		for _, f := range PriceFields {
			if _, ok := fields[f]; ok {
				refused = append(refused, f)
			}
		}
		if len(refused) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, &Error{
			Status: http.StatusBadRequest, Code: "price_field_via_schedule",
			Message: "prices change only through a scheduled price change",
			Detail:  map[string]any{"fields": refused},
		})
	})
}
