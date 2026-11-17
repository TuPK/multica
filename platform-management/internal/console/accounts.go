package console

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/accountrequest"
)

const (
	defaultAccountPage = 50
	maxAccountPage     = 200
)

// AccountRequests is the account request service (spec 7b.5).
// *accountrequest.Service is one.
type AccountRequests interface {
	Create(ctx context.Context, by accountrequest.Actor, in accountrequest.Input) (r accountrequest.Request, created bool, err error)
	Cancel(ctx context.Context, by accountrequest.Actor, id string) (accountrequest.Request, error)
	ResendWelcome(ctx context.Context, by accountrequest.Actor, id string) (accountrequest.Request, error)
	List(ctx context.Context, q accountrequest.ListQuery) ([]accountrequest.Request, error)
	Get(ctx context.Context, id string) (r accountrequest.Request, found bool, err error)
}

// accountRequestJSON is a request as the console shows it. It leaves out the
// billing profile: it is a third party's data, kept only until the platform has
// written the workspace's profile.
type accountRequestJSON struct {
	ID              string  `json:"id"`
	Kind            string  `json:"kind"`
	Email           string  `json:"email"`
	DisplayName     string  `json:"display_name"`
	WorkspaceName   string  `json:"workspace_name"`
	WorkspaceSlug   string  `json:"workspace_slug"`
	PlanKey         string  `json:"plan_key"`
	Seats           int     `json:"seats"`
	Interval        string  `json:"interval"`
	Trial           bool    `json:"trial"`
	CompReason      string  `json:"comp_reason"`
	State           string  `json:"state"`
	ErrorCode       string  `json:"error_code"`
	Attempts        int     `json:"attempts"`
	UserID          string  `json:"user_id,omitempty"`
	WorkspaceID     string  `json:"workspace_id,omitempty"`
	CreatedBy       string  `json:"created_by"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
	CompletedAt     *string `json:"completed_at"`
	WelcomeSentAt   *string `json:"welcome_sent_at"`
	WelcomeAttempts int     `json:"welcome_attempts"`
}

func toAccountRequestJSON(r accountrequest.Request) accountRequestJSON {
	ts := func(t *time.Time) *string {
		if t == nil {
			return nil
		}
		s := t.UTC().Format(time.RFC3339)
		return &s
	}
	return accountRequestJSON{
		ID: r.ID, Kind: r.Kind, Email: r.Email, DisplayName: r.DisplayName, WorkspaceName: r.WorkspaceName,
		WorkspaceSlug: r.WorkspaceSlug, PlanKey: r.PlanKey, Seats: r.Seats, Interval: r.Interval, Trial: r.Trial,
		CompReason: r.CompReason, State: r.State, ErrorCode: r.ErrorCode, Attempts: r.Attempts,
		UserID: r.UserID, WorkspaceID: r.WorkspaceID, CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: r.UpdatedAt.UTC().Format(time.RFC3339),
		CompletedAt: ts(r.CompletedAt), WelcomeSentAt: ts(r.WelcomeSentAt), WelcomeAttempts: r.WelcomeAttempts,
	}
}

// failAccount answers a service refusal with its status and code, an unknown
// request with the 404 of an unknown id, and anything else as fail does.
func (h *Handler) failAccount(w http.ResponseWriter, r *http.Request, op string, err error) {
	var ae *accountrequest.Error
	switch {
	case errors.As(err, &ae):
		body := map[string]any{"error": ae.Message, "code": ae.Code}
		if len(ae.Fields) > 0 {
			body["fields"] = ae.Fields
		}
		if ae.State != "" {
			body["state"] = ae.State
		}
		if ae.RetryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(ae.RetryAfter.Seconds()))))
		}
		writeJSON(w, ae.Status, body)
	case errors.Is(err, accountrequest.ErrNotFound):
		writeError(w, notFound())
	default:
		h.fail(w, r, op, err)
	}
}

func (h *Handler) accountActor(r *http.Request) (accountrequest.Actor, error) {
	a, err := h.currentActor(r)
	if err != nil {
		return accountrequest.Actor{}, err
	}
	return accountrequest.Actor{ID: a.ID, Label: a.Email}, nil
}

// createAccountRequest stores a pending account request. It answers 202 with
// the request, or 200 with the live request that already holds the email.
func (h *Handler) createAccountRequest(w http.ResponseWriter, r *http.Request) {
	var in accountrequest.Input
	if perr := decodeBody(w, r, &in); perr != nil {
		writeError(w, perr)
		return
	}
	by, err := h.accountActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	req, created, err := h.Accounts.Create(r.Context(), by, in)
	if err != nil {
		h.failAccount(w, r, "create account request", err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusAccepted
	}
	writeJSON(w, status, toAccountRequestJSON(req))
}

func (h *Handler) listAccountRequests(w http.ResponseWriter, r *http.Request) {
	q := accountrequest.ListQuery{Limit: defaultAccountPage}
	if s := r.URL.Query().Get("state"); s != "" {
		switch s {
		case accountrequest.StatePending, accountrequest.StateProvisioned, accountrequest.StateCompleted,
			accountrequest.StateFailed, accountrequest.StateCanceled:
			q.State = s
		default:
			writeError(w, invalidFilter("state is not a request state"))
			return
		}
	}
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxAccountPage {
			writeError(w, invalidFilter("limit must be a whole number from 1 to 200"))
			return
		}
		q.Limit = n
	}
	rows, err := h.Accounts.List(r.Context(), q)
	if err != nil {
		h.fail(w, r, "list account requests", err)
		return
	}
	out := make([]accountRequestJSON, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAccountRequestJSON(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": out})
}

func (h *Handler) getAccountRequest(w http.ResponseWriter, r *http.Request) {
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	req, found, err := h.Accounts.Get(r.Context(), id)
	if err != nil {
		h.fail(w, r, "get account request", err)
		return
	}
	if !found {
		writeError(w, notFound())
		return
	}
	writeJSON(w, http.StatusOK, toAccountRequestJSON(req))
}

func (h *Handler) cancelAccountRequest(w http.ResponseWriter, r *http.Request) {
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	by, err := h.accountActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	req, err := h.Accounts.Cancel(r.Context(), by, id)
	if err != nil {
		h.failAccount(w, r, "cancel account request", err)
		return
	}
	writeJSON(w, http.StatusOK, toAccountRequestJSON(req))
}

// resendWelcome sends the welcome email again, in the request of the staff
// member, and answers with the request.
func (h *Handler) resendWelcome(w http.ResponseWriter, r *http.Request) {
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	by, err := h.accountActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	req, err := h.Accounts.ResendWelcome(r.Context(), by, id)
	if err != nil {
		h.failAccount(w, r, "resend welcome", err)
		return
	}
	writeJSON(w, http.StatusOK, toAccountRequestJSON(req))
}
