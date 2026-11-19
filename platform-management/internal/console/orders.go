package console

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/invoice"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

const (
	// orderListLimit bounds GET /orders.
	orderListLimit = 200
	// recentOrdersLimit is how many orders a workspace's detail carries.
	recentOrdersLimit = 20
	// maxOrderReasonChars bounds the reason of a confirm or a cancel.
	maxOrderReasonChars = 500
	// maxTransferRefChars bounds the bank's reference of a confirmed transfer.
	maxTransferRefChars = 100

	targetOrder = "billing_order"
)

type buyerJSON struct {
	Name         string `json:"name"`
	TaxCode      string `json:"tax_code"`
	Address      string `json:"address"`
	InvoiceEmail string `json:"invoice_email"`
}

type orderJSON struct {
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	Kind          string `json:"kind"`
	PlanKey       string `json:"plan_key"`
	Interval      string `json:"interval"`
	Seats         int    `json:"seats"`
	Amount        int64  `json:"amount"`
	VATAmount     int64  `json:"vat_amount"`
	Currency      string `json:"currency"`
	Status        string `json:"status"`
	TransferCode  string `json:"transfer_code"`
	// ExpiresAt is null for a usage invoice, which does not expire.
	ExpiresAt *string   `json:"expires_at"`
	CreatedAt string    `json:"created_at"`
	PaidAt    *string   `json:"paid_at"`
	Buyer     buyerJSON `json:"buyer"`
}

func toOrderJSON(o OrderRow) orderJSON {
	// The buyer is a snapshot written by the Multica server. A snapshot that is
	// not an object with these string fields reads as an empty buyer rather
	// than failing the whole list.
	var buyer buyerJSON
	if err := json.Unmarshal(o.Buyer, &buyer); err != nil {
		buyer = buyerJSON{}
	}
	return orderJSON{
		ID: o.ID, WorkspaceID: o.WorkspaceID, WorkspaceName: o.WorkspaceName, Kind: string(o.Kind),
		PlanKey: o.PlanKey, Interval: o.Interval, Seats: o.Seats, Amount: o.Amount, VATAmount: o.VATAmount,
		Currency: o.Currency, Status: string(o.Status), TransferCode: o.TransferCode,
		ExpiresAt: expiry(o.ExpiresAt), CreatedAt: ts(o.CreatedAt), PaidAt: tsPtr(o.PaidAt), Buyer: buyer,
	}
}

func expiry(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	return tsPtr(&t)
}

// orderListJSON is an order of GET /orders: the order plus the state of its
// highest e-invoice revision.
type orderListJSON struct {
	orderJSON
	// InvoiceState is null for an order with no record and for a state the
	// console does not show (for example cancelled).
	InvoiceState *string `json:"invoice_state"`
}

func toOrdersJSON(rows []OrderRow) []orderJSON {
	out := make([]orderJSON, len(rows))
	for i, o := range rows {
		out[i] = toOrderJSON(o)
	}
	return out
}

func toOrderListJSON(rows []OrderRow) []orderListJSON {
	out := make([]orderListJSON, len(rows))
	for i, o := range rows {
		out[i] = orderListJSON{orderJSON: toOrderJSON(o), InvoiceState: shownInvoiceState(o.InvoiceState)}
	}
	return out
}

func shownInvoiceState(state string) *string {
	switch s := invoice.State(state); s {
	case invoice.StatePending, invoice.StateIssued, invoice.StateNeedsInfo, invoice.StateFailed:
		return &state
	}
	return nil
}

func orderNotFound() *apiError {
	return &apiError{Status: http.StatusNotFound, Code: "order_not_found", Message: "order not found"}
}

func orderNotPending() *apiError {
	return &apiError{Status: http.StatusConflict, Code: "order_not_pending", Message: "only a pending order can be confirmed or canceled"}
}

func orderExpired() *apiError {
	return &apiError{Status: http.StatusConflict, Code: "order_expired", Message: "the order has expired"}
}

func orderConflict(code, msg string) *apiError {
	return &apiError{Status: http.StatusConflict, Code: code, Message: msg}
}

// parseOrderStatus reads the status filter: one of the five statuses, pending
// when absent or empty.
func parseOrderStatus(r *http.Request) (billing.Status, *apiError) {
	values := r.URL.Query()["status"]
	if len(values) > 1 {
		return "", badRequest("invalid_status", "status is given more than once")
	}
	if len(values) == 0 || values[0] == "" {
		return billing.StatusPending, nil
	}
	switch s := billing.Status(values[0]); s {
	case billing.StatusPending, billing.StatusPaid, billing.StatusExpired, billing.StatusCanceled, billing.StatusRefunded:
		return s, nil
	}
	return "", badRequest("invalid_status", "status must be pending, paid, expired, canceled or refunded")
}

// listOrders returns the orders in one status. The pending queue is oldest
// first, so the longest wait is at the top; the others are newest first.
func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionOrdersRead)
	if !ok {
		return
	}
	status, perr := parseOrderStatus(r)
	if perr != nil {
		writeError(w, perr)
		return
	}
	rows, err := h.Reader.ListOrders(r.Context(), status, status != billing.StatusPending, orderListLimit, scope)
	if err != nil {
		h.fail(w, r, "list orders", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": toOrderListJSON(rows)})
}

type orderBody struct {
	Reason            string `json:"reason"`
	TransferReference string `json:"transfer_reference"`
}

// cleanOrderReason is cleanReason with the shorter bound orders use.
func cleanOrderReason(reason string) (string, *apiError) {
	reason, perr := cleanReason(reason)
	if perr != nil {
		return "", perr
	}
	if utf8.RuneCountInString(reason) > maxOrderReasonChars {
		return "", badRequest("reason_too_long", "reason is longer than 500 characters")
	}
	return reason, nil
}

func cleanTransferReference(ref string) (string, *apiError) {
	ref = strings.TrimSpace(ref)
	if hasNUL(ref) {
		return "", badRequest("invalid_body", "transfer_reference contains a NUL character")
	}
	if utf8.RuneCountInString(ref) > maxTransferRefChars {
		return "", badRequest("transfer_reference_too_long", "transfer_reference is longer than 100 characters")
	}
	return ref, nil
}

// readOrderRequest reads the order id and a body with a required reason.
func readOrderRequest(w http.ResponseWriter, r *http.Request) (id, reason string, ref string, perr *apiError) {
	if id, perr = pathID(r, "id"); perr != nil {
		return "", "", "", orderNotFound()
	}
	var body orderBody
	if perr = decodeBody(w, r, &body); perr != nil {
		return "", "", "", perr
	}
	if reason, perr = cleanOrderReason(body.Reason); perr != nil {
		return "", "", "", perr
	}
	if ref, perr = cleanTransferReference(body.TransferReference); perr != nil {
		return "", "", "", perr
	}
	return id, reason, ref, nil
}

// confirmOrder applies a bank transfer to an order through
// billing.Service.ApplyPayment, which writes the order, the subscription and the
// billing.order_paid audit row in one transaction. Confirming an order that is
// already paid answers 200 with applied false and writes nothing.
func (h *Handler) confirmOrder(w http.ResponseWriter, r *http.Request) {
	id, _, _, perr := readOrderRequest(w, r)
	if perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	applied, err := h.Payments.ApplyPayment(r.Context(), billing.Confirmation{OrderID: id, ConfirmedBy: a.ID, ActorLabel: a.Email})
	if err != nil {
		h.fail(w, r, "confirm order", h.confirmError(r, id, err))
		return
	}
	// The payment is committed. A failed read of the workspace name must not
	// turn it into an error response, so the name stays empty.
	row := OrderRow{Order: applied.Order}
	if got, found, err := h.Reader.GetOrder(r.Context(), id); err != nil {
		h.Log.Error("console: read confirmed order failed", "order_id", id, "error", err)
	} else if found {
		row = got
	}
	writeJSON(w, http.StatusOK, map[string]any{"order": toOrderJSON(row), "applied": !applied.AlreadyPaid})
}

// confirmError maps what ApplyPayment refuses to the order error codes. Any
// other error is returned as it is, for a 500.
func (h *Handler) confirmError(r *http.Request, id string, err error) error {
	switch {
	case errors.Is(err, billing.ErrOrderNotFound):
		return orderNotFound()
	case errors.Is(err, billing.ErrOrderExpired):
		return orderExpired()
	case errors.Is(err, billing.ErrInvalidTransition):
		// Which status the order is in tells an order the expiry job closed
		// from one staff canceled.
		if o, found, rerr := h.Reader.GetOrder(r.Context(), id); rerr == nil && found && o.Status == billing.StatusExpired {
			return orderExpired()
		}
		return orderNotPending()
	case errors.Is(err, billing.ErrNoSubscription), errors.Is(err, billing.ErrPlanMismatch):
		return orderConflict("order_not_applicable", "the order does not fit the workspace's subscription")
	case errors.Is(err, billing.ErrSubscriptionRaced):
		return orderConflict("order_conflict", "the subscription changed while the order was applied; try again")
	}
	return err
}

// cancelOrder moves a pending order to canceled and writes
// billing.order_canceled with the reason, in one transaction.
func (h *Handler) cancelOrder(w http.ResponseWriter, r *http.Request) {
	id, reason, _, perr := readOrderRequest(w, r)
	if perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	if row, found, err := h.Reader.GetOrder(r.Context(), id); err != nil {
		h.fail(w, r, "read order", err)
		return
	} else if found && row.Kind == billing.KindUsage {
		h.writeOffUsage(w, r, a, id, reason)
		return
	}
	var out OrderRow
	err = h.Runner.InTx(r.Context(), func(tx Tx) error {
		o, found, err := tx.LockOrder(r.Context(), id)
		if err != nil {
			return err
		}
		if !found {
			return orderNotFound()
		}
		if err := billing.Transition(o.Status, billing.StatusCanceled, o.ExpiresAt, h.Now()); err != nil {
			return orderNotPending()
		}
		if o, err = tx.CancelOrder(r.Context(), id); err != nil {
			return err
		}
		name, _, err := tx.WorkspaceName(r.Context(), o.WorkspaceID)
		if err != nil {
			return err
		}
		out = OrderRow{Order: o, WorkspaceName: name}
		return audit.Record(r.Context(), tx, a.staffEntry(audit.Entry{
			WorkspaceID: o.WorkspaceID, Action: audit.ActionBillingOrderCanceled,
			TargetType: targetOrder, TargetID: o.ID,
			Detail: map[string]any{
				"reason": reason, "kind": string(o.Kind), "plan": o.PlanKey, "interval": o.Interval, "seats": o.Seats,
				"amount": o.Amount, "vat_amount": o.VATAmount, "currency": o.Currency, "transfer_code": o.TransferCode,
			},
		}))
	})
	if err != nil {
		h.fail(w, r, "cancel order", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"order": toOrderJSON(out), "applied": true})
}

// writeOffUsage cancels a usage invoice, the debt it records, as a write-off:
// system_admin only, with the reason (spec 7b.3, rule 14; spec R6 row M15). billing.Service writes the
// order, billing.usage_written_off and the restore of the workspace in one
// transaction.
func (h *Handler) writeOffUsage(w http.ResponseWriter, r *http.Request, a actor, id, reason string) {
	if staff.Allowed(a.Role, staff.ActionUsageWriteOff) != staff.All {
		writeError(w, &apiError{Status: http.StatusForbidden, Code: "usage_cancel_admin_only", Message: "only an admin can write off a usage invoice"})
		return
	}
	o, err := h.Payments.WriteOffUsage(r.Context(), billing.WriteOff{OrderID: id, Reason: reason, StaffID: a.ID, ActorLabel: a.Email})
	switch {
	case errors.Is(err, billing.ErrOrderNotFound):
		err = orderNotFound()
	case errors.Is(err, billing.ErrInvalidTransition):
		err = orderNotPending()
	}
	if err != nil {
		h.fail(w, r, "write off usage invoice", err)
		return
	}
	// The write-off is committed. A failed read of the workspace name must not
	// turn it into an error response, so the name stays empty.
	row := OrderRow{Order: o}
	if got, found, err := h.Reader.GetOrder(r.Context(), id); err != nil {
		h.Log.Error("console: read written-off order failed", "order_id", id, "error", err)
	} else if found {
		row = got
	}
	writeJSON(w, http.StatusOK, map[string]any{"order": toOrderJSON(row), "applied": true})
}
