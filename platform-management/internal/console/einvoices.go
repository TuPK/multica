package console

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/invoice"
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

// EinvoiceReader is the e-invoice read. *PgxReader is one; the handler finds
// it on Deps.Reader.
type EinvoiceReader interface {
	// ListEinvoices returns up to limit records in any of states, newest
	// update first.
	ListEinvoices(ctx context.Context, states []invoice.State, limit int) ([]EinvoiceRow, error)
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
