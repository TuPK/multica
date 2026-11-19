package console

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/invoice"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// EinvoiceRow is an e-invoice record as the console lists it. It has no buyer:
// the buyer is personal data the list does not need.
type EinvoiceRow struct {
	OrderID       string
	WorkspaceID   string
	Revision      int
	State         invoice.State
	Currency      string
	Amount        int64
	VATAmount     int64
	Provider      string
	InvoiceNumber string
	LookupURL     string
	Attempts      int
	LastErrorCode string
	// MissingFields are field names only, never values.
	MissingFields []string
	NextAttemptAt time.Time
	UpdatedAt     time.Time
}

// EinvoiceHistoryRow is one revision of an order's e-invoice. Buyer is the
// revision's snapshot, personal data that must never be logged.
type EinvoiceHistoryRow struct {
	Revision      int
	State         invoice.State
	InvoiceNumber string
	LookupURL     string
	Buyer         []byte
	Attempts      int
	LastErrorCode string
	CreatedAt     time.Time
	IssuedAt      *time.Time
	UpdatedAt     time.Time
}

// EinvoiceReader is the e-invoice read. *PgxReader is one; the handler finds
// it on Deps.Reader.
type EinvoiceReader interface {
	// ListEinvoices returns up to limit records in any of states, newest
	// update first.
	ListEinvoices(ctx context.Context, states []invoice.State, limit int) ([]EinvoiceRow, error)
}

// OrderEinvoiceReader reads the e-invoice history of one order. *PgxReader is
// one; the handler finds it on Deps.Reader.
type OrderEinvoiceReader interface {
	// ListOrderEinvoices returns every revision of the order, newest first.
	ListOrderEinvoices(ctx context.Context, orderID string) ([]EinvoiceHistoryRow, error)
}

// EinvoiceOps is what the staff retry route needs.
// *invoice.Service is one.
type EinvoiceOps interface {
	// Latest returns the highest revision of the order; invoice.ErrNotFound when there is none.
	Latest(ctx context.Context, orderID string) (invoice.Record, error)
	Retry(ctx context.Context, orderID string, revision int, by string) (invoice.Record, error)
}

var errNoEinvoiceReader = errors.New("the console reader does not read e-invoices")

// einvoiceListSQL does not select the buyer column.
const einvoiceListSQL = `SELECT order_id::text, workspace_id::text, revision, state, currency, amount, vat_amount, provider,
		invoice_number, lookup_url, attempts, last_error_code, missing_fields, next_attempt_at, updated_at
	FROM einvoice_issuance
	WHERE state = ANY($1)
	ORDER BY updated_at DESC, order_id DESC, revision DESC
	LIMIT $2`

// ListEinvoices implements EinvoiceReader.
func (r *PgxReader) ListEinvoices(ctx context.Context, states []invoice.State, limit int) ([]EinvoiceRow, error) {
	names := make([]string, len(states))
	for i, s := range states {
		names[i] = string(s)
	}
	rows, err := r.pool.Query(ctx, einvoiceListSQL, names, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EinvoiceRow{}
	for rows.Next() {
		var (
			e       EinvoiceRow
			state   string
			missing []byte
		)
		if err := rows.Scan(&e.OrderID, &e.WorkspaceID, &e.Revision, &state, &e.Currency, &e.Amount, &e.VATAmount, &e.Provider,
			&e.InvoiceNumber, &e.LookupURL, &e.Attempts, &e.LastErrorCode, &missing, &e.NextAttemptAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.State = invoice.State(state)
		e.NextAttemptAt, e.UpdatedAt = e.NextAttemptAt.UTC(), e.UpdatedAt.UTC()
		// A value that is not an array of strings reads as no fields rather
		// than failing the list.
		if err := json.Unmarshal(missing, &e.MissingFields); err != nil {
			e.MissingFields = nil
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// issuedAtColumn is joined from the state constant: the isolation scan reads
// a string literal that starts with "issue" as the content table.
var issuedAtColumn = string(invoice.StateIssued) + "_at"

// einvoiceHistorySQL reads the buyer snapshot of each revision, not the order's.
var einvoiceHistorySQL = `SELECT revision, state, invoice_number, lookup_url, buyer, attempts, last_error_code, created_at, ` +
	issuedAtColumn + `, updated_at
	FROM einvoice_issuance
	WHERE order_id = $1
	ORDER BY revision DESC`

// ListOrderEinvoices implements OrderEinvoiceReader.
func (r *PgxReader) ListOrderEinvoices(ctx context.Context, orderID string) ([]EinvoiceHistoryRow, error) {
	id, err := parseUUID(orderID)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, einvoiceHistorySQL, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EinvoiceHistoryRow{}
	for rows.Next() {
		var (
			h      EinvoiceHistoryRow
			state  string
			issued pgtype.Timestamptz
		)
		if err := rows.Scan(&h.Revision, &state, &h.InvoiceNumber, &h.LookupURL, &h.Buyer, &h.Attempts, &h.LastErrorCode,
			&h.CreatedAt, &issued, &h.UpdatedAt); err != nil {
			return nil, err
		}
		h.State = invoice.State(state)
		h.CreatedAt, h.UpdatedAt, h.IssuedAt = h.CreatedAt.UTC(), h.UpdatedAt.UTC(), timePtr(issued)
		out = append(out, h)
	}
	return out, rows.Err()
}

// toHistoryJSON is a map because the key of the issue time cannot be written
// as a struct tag (see issuedAtColumn).
func toHistoryJSON(h EinvoiceHistoryRow) map[string]any {
	// A snapshot that does not parse reads as an empty buyer, as on the orders list.
	var buyer invoice.Buyer
	if err := json.Unmarshal(h.Buyer, &buyer); err != nil {
		buyer = invoice.Buyer{}
	}
	return map[string]any{
		"revision": h.Revision, "state": string(h.State), "invoice_number": h.InvoiceNumber, "lookup_url": h.LookupURL,
		"buyer":    buyerJSON{Name: buyer.Name, TaxCode: buyer.TaxCode, Address: buyer.Address, InvoiceEmail: buyer.Email},
		"attempts": h.Attempts, "last_error_code": h.LastErrorCode, "created_at": ts(h.CreatedAt),
		issuedAtColumn: tsPtr(h.IssuedAt), "updated_at": ts(h.UpdatedAt),
	}
}

// listOrderEinvoices returns every revision of one order's e-invoice, newest
// first. An order without a record answers 200 with an empty list.
func (h *Handler) listOrderEinvoices(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionOrdersRead)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	hr, ok := h.Reader.(OrderEinvoiceReader)
	if !ok {
		h.fail(w, r, "list order e-invoices", errNoEinvoiceReader)
		return
	}
	order, found, err := h.Reader.GetOrder(r.Context(), id)
	if err != nil {
		h.fail(w, r, "read order", err)
		return
	}
	if !found {
		writeError(w, orderNotFound())
		return
	}
	// An order of another tenant reads as one that does not exist.
	if visible, err := h.workspaceVisible(r.Context(), scope, order.WorkspaceID); err != nil {
		h.fail(w, r, "check order scope", err)
		return
	} else if !visible {
		writeError(w, orderNotFound())
		return
	}
	rows, err := hr.ListOrderEinvoices(r.Context(), id)
	if err != nil {
		h.fail(w, r, "list order e-invoices", err)
		return
	}
	out := make([]map[string]any, len(rows))
	for i, row := range rows {
		out[i] = toHistoryJSON(row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"einvoices": out})
}

type einvoiceJSON struct {
	OrderID       string   `json:"order_id"`
	WorkspaceID   string   `json:"workspace_id"`
	Revision      int      `json:"revision"`
	State         string   `json:"state"`
	Currency      string   `json:"currency"`
	Amount        int64    `json:"amount"`
	VATAmount     int64    `json:"vat_amount"`
	Provider      string   `json:"provider"`
	InvoiceNumber string   `json:"invoice_number"`
	LookupURL     string   `json:"lookup_url"`
	Attempts      int      `json:"attempts"`
	LastErrorCode string   `json:"last_error_code"`
	MissingFields []string `json:"missing_fields"`
	NextAttemptAt string   `json:"next_attempt_at"`
	UpdatedAt     string   `json:"updated_at"`
}

func toEinvoiceJSON(e EinvoiceRow) einvoiceJSON {
	missing := e.MissingFields
	if missing == nil {
		missing = []string{}
	}
	return einvoiceJSON{
		OrderID: e.OrderID, WorkspaceID: e.WorkspaceID, Revision: e.Revision, State: string(e.State),
		Currency: e.Currency, Amount: e.Amount, VATAmount: e.VATAmount, Provider: e.Provider,
		InvoiceNumber: e.InvoiceNumber, LookupURL: e.LookupURL, Attempts: e.Attempts, LastErrorCode: e.LastErrorCode,
		MissingFields: missing, NextAttemptAt: ts(e.NextAttemptAt), UpdatedAt: ts(e.UpdatedAt),
	}
}

// parseEinvoiceStates reads the state filter: needs_info and failed together
// when absent or empty.
func parseEinvoiceStates(r *http.Request) ([]invoice.State, *apiError) {
	values := r.URL.Query()["state"]
	if len(values) > 1 {
		return nil, badRequest("invalid_state", "state is given more than once")
	}
	if len(values) == 0 || values[0] == "" {
		return []invoice.State{invoice.StateNeedsInfo, invoice.StateFailed}, nil
	}
	switch s := invoice.State(values[0]); s {
	case invoice.StatePending, invoice.StateIssued, invoice.StateNeedsInfo, invoice.StateFailed:
		return []invoice.State{s}, nil
	}
	// The state names are joined from the constants: the isolation scan reads
	// a string literal that starts with "issue" as the content table.
	return nil, badRequest("invalid_state", "state must be one of "+strings.Join([]string{
		string(invoice.StatePending), string(invoice.StateIssued), string(invoice.StateNeedsInfo), string(invoice.StateFailed)}, ", "))
}

func (h *Handler) listEinvoices(w http.ResponseWriter, r *http.Request) {
	states, perr := parseEinvoiceStates(r)
	if perr != nil {
		writeError(w, perr)
		return
	}
	limit, perr := parseListLimit(r, noticeListDefault, noticeListMax)
	if perr != nil {
		writeError(w, perr)
		return
	}
	er, ok := h.Reader.(EinvoiceReader)
	if !ok {
		h.fail(w, r, "list e-invoices", errNoEinvoiceReader)
		return
	}
	rows, err := er.ListEinvoices(r.Context(), states, limit)
	if err != nil {
		h.fail(w, r, "list e-invoices", err)
		return
	}
	out := make([]einvoiceJSON, len(rows))
	for i, e := range rows {
		out[i] = toEinvoiceJSON(e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"einvoices": out})
}

func recordToRow(rec invoice.Record) EinvoiceRow {
	return EinvoiceRow{
		OrderID: rec.OrderID, WorkspaceID: rec.WorkspaceID, Revision: rec.Revision, State: rec.State,
		Currency: rec.Currency, Amount: rec.Amount, VATAmount: rec.VATAmount, Provider: rec.Provider,
		InvoiceNumber: rec.InvoiceNumber, LookupURL: rec.LookupURL, Attempts: rec.Attempts,
		LastErrorCode: rec.LastErrorCode, MissingFields: rec.MissingFields,
		NextAttemptAt: rec.NextAttemptAt, UpdatedAt: rec.UpdatedAt,
	}
}

// einvoiceError answers an e-invoice write error. A buyer that lacks fields
// lists the field names only, never a value.
func (h *Handler) einvoiceError(w http.ResponseWriter, r *http.Request, op string, err error) {
	var pe *invoice.ProviderError
	switch {
	case errors.Is(err, invoice.ErrNotFound):
		writeError(w, notFound())
	case errors.Is(err, invoice.ErrStateConflict):
		writeError(w, &apiError{Status: http.StatusConflict, Code: "einvoice_state_conflict",
			Message: "the invoice is not in a state this action accepts"})
	case errors.Is(err, invoice.ErrProviderNotConfigured):
		writeError(w, &apiError{Status: http.StatusServiceUnavailable, Code: "einvoice_provider_not_configured",
			Message: "e-invoicing is not configured on this platform"})
	case errors.As(err, &pe) && pe.Kind == invoice.KindMissingFields:
		fields := pe.Fields
		if fields == nil {
			fields = []string{}
		}
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "the buyer lacks required fields", "code": "einvoice_buyer_incomplete", "fields": fields,
		})
	default:
		h.fail(w, r, op, err)
	}
}

// latestEinvoice loads the latest revision of the order in the path.
func (h *Handler) latestEinvoice(w http.ResponseWriter, r *http.Request, op string) (invoice.Record, bool) {
	orderID, perr := pathID(r, "orderId")
	if perr != nil {
		writeError(w, perr)
		return invoice.Record{}, false
	}
	if h.Einvoices == nil {
		h.einvoiceError(w, r, op, invoice.ErrProviderNotConfigured)
		return invoice.Record{}, false
	}
	rec, err := h.Einvoices.Latest(r.Context(), orderID)
	if err != nil {
		h.einvoiceError(w, r, op, err)
		return invoice.Record{}, false
	}
	return rec, true
}

func (h *Handler) retryEinvoice(w http.ResponseWriter, r *http.Request) {
	const op = "retry e-invoice"
	rec, ok := h.latestEinvoice(w, r, op)
	if !ok {
		return
	}
	if rec.State != invoice.StateFailed && rec.State != invoice.StateNeedsInfo {
		h.einvoiceError(w, r, op, invoice.ErrStateConflict)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	out, err := h.Einvoices.Retry(r.Context(), rec.OrderID, rec.Revision, a.ID)
	if err != nil {
		h.einvoiceError(w, r, op, err)
		return
	}
	writeJSON(w, http.StatusOK, toEinvoiceJSON(recordToRow(out)))
}
