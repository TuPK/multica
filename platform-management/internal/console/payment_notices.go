package console

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/paymentnotice"
)

const (
	noticeListDefault = 50
	noticeListMax     = 200
	// noticeCandidateLimit bounds the orders a notice's detail offers.
	noticeCandidateLimit = 20
)

// NoticeResolver decides payment notices. *billing.Reconciler is one.
type NoticeResolver interface {
	Resolve(ctx context.Context, actor billing.Actor, noticeID, orderID, reason string) (paymentnotice.Notice, error)
	Dismiss(ctx context.Context, actor billing.Actor, noticeID, reason string) (paymentnotice.Notice, error)
}

// NoticeCandidate is a pending order that may be the one a notice paid for.
type NoticeCandidate struct {
	OrderID      string
	WorkspaceID  string
	State        string
	TransferCode string
	Amount       int64
	VATAmount    int64
	Currency     string
	CreatedAt    time.Time
	// Match is "code" or "amount"; an order that matches both says "code".
	Match string
}

// NoticeReader is the payment notice reads. *PgxReader is one; the handler
// finds it on Deps.Reader, so a Reader that is not one answers 500.
type NoticeReader interface {
	// ListPaymentNotices returns up to limit notices in state: oldest first for
	// needs_review, newest first for the others.
	ListPaymentNotices(ctx context.Context, state paymentnotice.State, limit int) ([]paymentnotice.Notice, error)
	// GetPaymentNotice returns found == false for an unknown id.
	GetPaymentNotice(ctx context.Context, id string) (n paymentnotice.Notice, found bool, err error)
	// NoticeCandidates returns the pending, unexpired orders matching the
	// notice by transfer code or by total and currency, up to limit.
	NoticeCandidates(ctx context.Context, n paymentnotice.Notice, limit int) ([]NoticeCandidate, error)
}

// SQL of the payment notice reads. The notice's body hash and the decision
// columns the console does not show are not selected.
const (
	noticeColumns = `id::text, provider, provider_event_id, state, review_reason, transfer_code, amount, currency,
		order_id, received_at, decided_at, decided_by, decision_reason`

	noticeListOldestSQL = `SELECT ` + noticeColumns + ` FROM payment_notice WHERE state = $1
		ORDER BY received_at ASC, id ASC LIMIT $2`
	noticeListNewestSQL = `SELECT ` + noticeColumns + ` FROM payment_notice WHERE state = $1
		ORDER BY received_at DESC, id DESC LIMIT $2`
	noticeGetSQL = `SELECT ` + noticeColumns + ` FROM payment_notice WHERE id = $1`

	// An order with no expiry stores the zero time, which is before 1900.
	noticeCandidatesSQL = `SELECT id::text, workspace_id::text, status, transfer_code, amount, vat_amount, currency, created_at,
			CASE WHEN $1 <> '' AND transfer_code = $1 THEN 'code' ELSE 'amount' END AS match
		FROM billing_order
		WHERE status = 'pending' AND (expires_at > now() OR expires_at < '1900-01-01')
		  AND (($1 <> '' AND transfer_code = $1) OR (amount + vat_amount = $2 AND lower(currency) = lower($3)))
		ORDER BY (CASE WHEN $1 <> '' AND transfer_code = $1 THEN 0 ELSE 1 END), created_at ASC, id ASC
		LIMIT $4`
)

func scanNotice(row pgx.Row) (paymentnotice.Notice, error) {
	var (
		n         paymentnotice.Notice
		state     string
		decidedAt *time.Time
	)
	err := row.Scan(&n.ID, &n.Provider, &n.ProviderEventID, &state, &n.ReviewReason, &n.TransferCode, &n.Amount, &n.Currency,
		&n.OrderID, &n.ReceivedAt, &decidedAt, &n.DecidedBy, &n.DecisionReason)
	if err != nil {
		return paymentnotice.Notice{}, err
	}
	n.State = paymentnotice.State(state)
	n.ReceivedAt = n.ReceivedAt.UTC()
	if decidedAt != nil {
		n.DecidedAt = decidedAt.UTC()
	}
	return n, nil
}

// ListPaymentNotices implements NoticeReader.
func (r *PgxReader) ListPaymentNotices(ctx context.Context, state paymentnotice.State, limit int) ([]paymentnotice.Notice, error) {
	query := noticeListNewestSQL
	if state == paymentnotice.StateNeedsReview {
		query = noticeListOldestSQL
	}
	rows, err := r.pool.Query(ctx, query, string(state), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []paymentnotice.Notice{}
	for rows.Next() {
		n, err := scanNotice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// GetPaymentNotice implements NoticeReader.
func (r *PgxReader) GetPaymentNotice(ctx context.Context, id string) (paymentnotice.Notice, bool, error) {
	n, err := scanNotice(r.pool.QueryRow(ctx, noticeGetSQL, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return paymentnotice.Notice{}, false, nil
	}
	if err != nil {
		return paymentnotice.Notice{}, false, err
	}
	return n, true, nil
}

// NoticeCandidates implements NoticeReader.
func (r *PgxReader) NoticeCandidates(ctx context.Context, n paymentnotice.Notice, limit int) ([]NoticeCandidate, error) {
	rows, err := r.pool.Query(ctx, noticeCandidatesSQL, n.TransferCode, n.Amount, n.Currency, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NoticeCandidate{}
	for rows.Next() {
		var c NoticeCandidate
		if err := rows.Scan(&c.OrderID, &c.WorkspaceID, &c.State, &c.TransferCode, &c.Amount, &c.VATAmount, &c.Currency, &c.CreatedAt, &c.Match); err != nil {
			return nil, err
		}
		c.CreatedAt = c.CreatedAt.UTC()
		out = append(out, c)
	}
	return out, rows.Err()
}

type noticeJSON struct {
	ID              string  `json:"id"`
	Provider        string  `json:"provider"`
	ProviderEventID string  `json:"provider_event_id"`
	State           string  `json:"state"`
	TransferCode    string  `json:"transfer_code"`
	Amount          int64   `json:"amount"`
	Currency        string  `json:"currency"`
	ReviewReason    string  `json:"review_reason"`
	OrderID         string  `json:"order_id"`
	ReceivedAt      string  `json:"received_at"`
	DecidedAt       *string `json:"decided_at"`
}

type candidateJSON struct {
	OrderID      string `json:"order_id"`
	WorkspaceID  string `json:"workspace_id"`
	State        string `json:"state"`
	TransferCode string `json:"transfer_code"`
	Amount       int64  `json:"amount"`
	VATAmount    int64  `json:"vat_amount"`
	Currency     string `json:"currency"`
	CreatedAt    string `json:"created_at"`
	Match        string `json:"match"`
}

type noticeDetailJSON struct {
	noticeJSON
	Candidates []candidateJSON `json:"candidates"`
}

// toNoticeJSON never carries the body hash, the decision's author or its
// reason: the console shows what the notice says and where it stands.
func toNoticeJSON(n paymentnotice.Notice) noticeJSON {
	out := noticeJSON{
		ID: n.ID, Provider: n.Provider, ProviderEventID: n.ProviderEventID, State: string(n.State),
		TransferCode: n.TransferCode, Amount: n.Amount, Currency: n.Currency, ReviewReason: n.ReviewReason,
		OrderID: n.OrderID, ReceivedAt: ts(n.ReceivedAt),
	}
	if !n.DecidedAt.IsZero() {
		out.DecidedAt = tsPtr(&n.DecidedAt)
	}
	return out
}

func noticeNotFound() *apiError { return notFound() }

func (h *Handler) noticeReader() (NoticeReader, error) {
	nr, ok := h.Reader.(NoticeReader)
	if !ok {
		return nil, errors.New("the console reader does not read payment notices")
	}
	return nr, nil
}

// parseNoticeState reads the state filter: needs_review when absent or empty.
func parseNoticeState(r *http.Request) (paymentnotice.State, *apiError) {
	values := r.URL.Query()["state"]
	if len(values) > 1 {
		return "", badRequest("invalid_state", "state is given more than once")
	}
	if len(values) == 0 || values[0] == "" {
		return paymentnotice.StateNeedsReview, nil
	}
	switch s := paymentnotice.State(values[0]); s {
	case paymentnotice.StateReceived, paymentnotice.StateNeedsReview, paymentnotice.StateApplied,
		paymentnotice.StateResolved, paymentnotice.StateDismissed:
		return s, nil
	}
	return "", badRequest("invalid_state", "state must be received, needs_review, applied, resolved or dismissed")
}

// parseListLimit reads limit: def when absent, otherwise 1 to max.
func parseListLimit(r *http.Request, def, max int) (int, *apiError) {
	values := r.URL.Query()["limit"]
	if len(values) == 0 {
		return def, nil
	}
	n, err := strconv.Atoi(values[0])
	if len(values) > 1 || err != nil || n < 1 || n > max {
		return 0, invalidFilter("limit must be an integer from 1 to " + strconv.Itoa(max))
	}
	return n, nil
}

// noticePathID reads the notice id. A value that is not a UUID is a 400.
func noticePathID(r *http.Request) (string, *apiError) {
	id, ok := canonicalUUID(r.PathValue("id"))
	if !ok {
		return "", badRequest("invalid_id", "the notice id is not a UUID")
	}
	return id, nil
}

func (h *Handler) listPaymentNotices(w http.ResponseWriter, r *http.Request) {
	state, perr := parseNoticeState(r)
	if perr != nil {
		writeError(w, perr)
		return
	}
	limit, perr := parseListLimit(r, noticeListDefault, noticeListMax)
	if perr != nil {
		writeError(w, perr)
		return
	}
	nr, err := h.noticeReader()
	if err != nil {
		h.fail(w, r, "list payment notices", err)
		return
	}
	rows, err := nr.ListPaymentNotices(r.Context(), state, limit)
	if err != nil {
		h.fail(w, r, "list payment notices", err)
		return
	}
	out := make([]noticeJSON, len(rows))
	for i, n := range rows {
		out[i] = toNoticeJSON(n)
	}
	writeJSON(w, http.StatusOK, map[string]any{"notices": out})
}

func (h *Handler) getPaymentNotice(w http.ResponseWriter, r *http.Request) {
	id, perr := noticePathID(r)
	if perr != nil {
		writeError(w, perr)
		return
	}
	nr, err := h.noticeReader()
	if err != nil {
		h.fail(w, r, "read payment notice", err)
		return
	}
	n, found, err := nr.GetPaymentNotice(r.Context(), id)
	if err != nil {
		h.fail(w, r, "read payment notice", err)
		return
	}
	if !found {
		writeError(w, noticeNotFound())
		return
	}
	cands, err := nr.NoticeCandidates(r.Context(), n, noticeCandidateLimit)
	if err != nil {
		h.fail(w, r, "read payment notice candidates", err)
		return
	}
	out := noticeDetailJSON{noticeJSON: toNoticeJSON(n), Candidates: make([]candidateJSON, len(cands))}
	for i, c := range cands {
		out.Candidates[i] = candidateJSON{
			OrderID: c.OrderID, WorkspaceID: c.WorkspaceID, State: c.State, TransferCode: c.TransferCode,
			Amount: c.Amount, VATAmount: c.VATAmount, Currency: c.Currency, CreatedAt: ts(c.CreatedAt), Match: c.Match,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type resolveBody struct {
	OrderID string `json:"order_id"`
	Reason  string `json:"reason"`
}

type dismissBody struct {
	Reason string `json:"reason"`
}

func noticeClosed() *apiError {
	return &apiError{Status: http.StatusConflict, Code: "payment_notice_closed", Message: "the payment notice is no longer waiting for review"}
}

// writeNoticeError answers a refusal of the reconciler. State is the field a
// closed notice adds to the envelope.
func (h *Handler) noticeError(w http.ResponseWriter, r *http.Request, op string, err error) {
	var closed *billing.NoticeClosedError
	switch {
	case errors.As(err, &closed):
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": noticeClosed().Message, "code": "payment_notice_closed", "state": string(closed.State),
		})
		return
	case errors.Is(err, billing.ErrReasonRequired):
		err = badRequest("reason_required", "a reason is required")
	case errors.Is(err, paymentnotice.ErrNotFound):
		err = noticeNotFound()
	case errors.Is(err, billing.ErrNoticeConflict):
		err = orderConflict("payment_notice_conflict", "the payment notice changed while it was decided; reload it")
	case errors.Is(err, billing.ErrForbidden):
		err = forbidden()
	case errors.Is(err, billing.ErrOrderNotPending):
		err = orderNotPending()
	case errors.Is(err, billing.ErrOrderNotFound):
		err = orderNotFound()
	case errors.Is(err, billing.ErrOrderExpired):
		err = orderExpired()
	case errors.Is(err, billing.ErrNoSubscription), errors.Is(err, billing.ErrPlanMismatch):
		err = orderConflict("order_not_applicable", "the order does not fit the workspace's subscription")
	case errors.Is(err, billing.ErrSubscriptionRaced):
		err = orderConflict("order_conflict", "the subscription changed while the order was applied; try again")
	}
	h.fail(w, r, op, err)
}

func (a actor) billingActor() billing.Actor {
	return billing.Actor{ID: a.ID, Label: a.Email, Role: a.Role}
}

func (h *Handler) resolvePaymentNotice(w http.ResponseWriter, r *http.Request) {
	id, perr := noticePathID(r)
	if perr != nil {
		writeError(w, perr)
		return
	}
	var body resolveBody
	if perr := decodeBody(w, r, &body); perr != nil {
		writeError(w, perr)
		return
	}
	orderID, ok := canonicalUUID(body.OrderID)
	if !ok {
		writeError(w, badRequest("invalid_order_id", "order_id is not a UUID"))
		return
	}
	reason, perr := cleanReason(body.Reason)
	if perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	n, err := h.Reconciler.Resolve(r.Context(), a.billingActor(), id, orderID, reason)
	if err != nil {
		h.noticeError(w, r, "resolve payment notice", err)
		return
	}
	writeJSON(w, http.StatusOK, toNoticeJSON(n))
}

func (h *Handler) dismissPaymentNotice(w http.ResponseWriter, r *http.Request) {
	id, perr := noticePathID(r)
	if perr != nil {
		writeError(w, perr)
		return
	}
	var body dismissBody
	if perr := decodeBody(w, r, &body); perr != nil {
		writeError(w, perr)
		return
	}
	reason, perr := cleanReason(body.Reason)
	if perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	n, err := h.Reconciler.Dismiss(r.Context(), a.billingActor(), id, reason)
	if err != nil {
		h.noticeError(w, r, "dismiss payment notice", err)
		return
	}
	writeJSON(w, http.StatusOK, toNoticeJSON(n))
}
