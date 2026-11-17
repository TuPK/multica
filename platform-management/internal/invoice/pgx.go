package invoice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/internal/db"
)

type PgxStore struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

var _ Store = (*PgxStore)(nil)

func NewPgxStore(pool *pgxpool.Pool) *PgxStore {
	return &PgxStore{pool: pool, q: db.New(pool)}
}

// Errors below never include buyer data or the input values.

func (s *PgxStore) Insert(ctx context.Context, n NewRecord) (Record, error) {
	return insert(ctx, s.q, n)
}

func insert(ctx context.Context, q *db.Queries, n NewRecord) (Record, error) {
	orderID, err := parseID(n.OrderID)
	if err != nil {
		return Record{}, err
	}
	wsID, err := parseID(n.WorkspaceID)
	if err != nil {
		return Record{}, err
	}
	buyer := n.Buyer
	if buyer == nil {
		buyer = []byte(`{}`)
	}
	row, err := q.InsertEinvoiceIssuance(ctx, db.InsertEinvoiceIssuanceParams{
		OrderID:     orderID,
		WorkspaceID: wsID,
		Provider:    n.Provider,
		Currency:    n.Currency,
		Amount:      n.Amount,
		VatAmount:   n.VATAmount,
		Buyer:       buyer,
	})
	if err != nil {
		return Record{}, fmt.Errorf("insert einvoice issuance: %w", err)
	}
	return fromRow(row), nil
}

func (s *PgxStore) Get(ctx context.Context, orderID string, revision int) (Record, error) {
	return get(ctx, s.q, orderID, revision)
}

func get(ctx context.Context, q *db.Queries, orderID string, revision int) (Record, error) {
	oid, err := parseID(orderID)
	if err != nil {
		return Record{}, err
	}
	row, err := q.GetEinvoiceIssuance(ctx, db.GetEinvoiceIssuanceParams{OrderID: oid, Revision: int32(revision)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("get einvoice issuance: %w", err)
	}
	return fromRow(row), nil
}

func (s *PgxStore) MarkIssued(ctx context.Context, orderID string, revision int, i Issued) error {
	oid, err := parseID(orderID)
	if err != nil {
		return err
	}
	n, err := s.q.MarkEinvoiceIssued(ctx, db.MarkEinvoiceIssuedParams{
		ProviderInvoiceID: i.ProviderInvoiceID,
		InvoiceNumber:     i.InvoiceNumber,
		LookupUrl:         i.LookupURL,
		IssuedAt:          pgtype.Timestamptz{Time: i.IssuedAt, Valid: true},
		OrderID:           oid,
		Revision:          int32(revision),
	})
	if err != nil {
		return fmt.Errorf("mark einvoice issued: %w", err)
	}
	if n > 0 {
		return nil
	}
	return conflictOrNotFound(ctx, s.q, orderID, revision)
}

func (s *PgxStore) ListDue(ctx context.Context, now time.Time, limit int) ([]Record, error) {
	rows, err := s.q.ListDueEinvoiceIssuance(ctx, db.ListDueEinvoiceIssuanceParams{
		NextAttemptAt: pgtype.Timestamptz{Time: now, Valid: true},
		Limit:         int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list due einvoice issuance: %w", err)
	}
	out := make([]Record, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out, nil
}

func (s *PgxStore) ReplaceIssued(ctx context.Context, orderID string, revision int, buyer []byte, by, reason string) (Record, error) {
	oid, err := parseID(orderID)
	if err != nil {
		return Record{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("replace einvoice issuance: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	// The conditional UPDATE takes the row lock, so a concurrent replace of the
	// same original sees state cancelled and gets ErrStateConflict.
	n, err := q.CancelEinvoiceIssued(ctx, db.CancelEinvoiceIssuedParams{
		CancelledBy:  by,
		CancelReason: reason,
		OrderID:      oid,
		Revision:     int32(revision),
	})
	if err != nil {
		return Record{}, fmt.Errorf("cancel einvoice issuance: %w", err)
	}
	if n == 0 {
		return Record{}, conflictOrNotFound(ctx, q, orderID, revision)
	}
	orig, err := get(ctx, q, orderID, revision)
	if err != nil {
		return Record{}, err
	}
	next, err := insert(ctx, q, NewRecord{
		OrderID:     orig.OrderID,
		WorkspaceID: orig.WorkspaceID,
		Provider:    orig.Provider,
		Currency:    orig.Currency,
		Amount:      orig.Amount,
		VATAmount:   orig.VATAmount,
		Buyer:       buyer,
	})
	if err != nil {
		return Record{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Record{}, fmt.Errorf("replace einvoice issuance: %w", err)
	}
	return next, nil
}

// conflictOrNotFound explains a zero-row conditional update.
func conflictOrNotFound(ctx context.Context, q *db.Queries, orderID string, revision int) error {
	if _, err := get(ctx, q, orderID, revision); err != nil {
		return err
	}
	return ErrStateConflict
}

// parseID treats a malformed id as not found: no row can carry it.
func parseID(id string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(id); err != nil {
		return u, ErrNotFound
	}
	return u, nil
}

func fromRow(r db.EinvoiceIssuance) Record {
	rec := Record{
		OrderID:           uuidString(r.OrderID),
		Revision:          int(r.Revision),
		WorkspaceID:       uuidString(r.WorkspaceID),
		State:             State(r.State),
		Provider:          r.Provider,
		ProviderInvoiceID: r.ProviderInvoiceID,
		InvoiceNumber:     r.InvoiceNumber,
		LookupURL:         r.LookupUrl,
		Currency:          r.Currency,
		Amount:            r.Amount,
		VATAmount:         r.VatAmount,
		Buyer:             compactJSON(r.Buyer),
		MissingFields:     []string{},
		Attempts:          int(r.Attempts),
		NextAttemptAt:     r.NextAttemptAt.Time,
		LastErrorCode:     r.LastErrorCode,
		CancelledBy:       r.CancelledBy,
		CancelReason:      r.CancelReason,
		CreatedAt:         r.CreatedAt.Time,
		UpdatedAt:         r.UpdatedAt.Time,
	}
	if len(r.MissingFields) > 0 {
		// A malformed list reads as empty; the column default is [].
		_ = json.Unmarshal(r.MissingFields, &rec.MissingFields)
	}
	rec.LockedUntil = timePtr(r.LockedUntil)
	rec.IssuedAt = timePtr(r.IssuedAt)
	rec.CancelledAt = timePtr(r.CancelledAt)
	return rec
}

// compactJSON strips the spaces jsonb adds when it prints a value, so the
// buyer reads back in the compact form it was written in.
func compactJSON(b []byte) []byte {
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		return b
	}
	return buf.Bytes()
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (s *PgxStore) ListPaidWithoutRecord(ctx context.Context, startAt time.Time, limit int) ([]PaidOrder, error) {
	rows, err := s.q.ListPaidOrdersNeedingEinvoice(ctx, db.ListPaidOrdersNeedingEinvoiceParams{
		StartAt: pgtype.Timestamptz{Time: startAt, Valid: true},
		MaxRows: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list paid orders needing einvoice: %w", err)
	}
	out := make([]PaidOrder, 0, len(rows))
	for _, r := range rows {
		out = append(out, PaidOrder{
			ID:          uuidString(r.ID),
			WorkspaceID: uuidString(r.WorkspaceID),
			Currency:    r.Currency,
			Amount:      r.Amount,
			VATAmount:   r.VatAmount,
			Buyer:       r.Buyer,
		})
	}
	return out, nil
}

func (s *PgxStore) Claim(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]Record, error) {
	rows, err := s.q.ClaimDueEinvoiceIssuance(ctx, db.ClaimDueEinvoiceIssuanceParams{
		LeaseUntil: pgtype.Timestamptz{Time: now.Add(lease), Valid: true},
		Now:        pgtype.Timestamptz{Time: now, Valid: true},
		MaxRows:    int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("claim einvoice issuance: %w", err)
	}
	out := make([]Record, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out, nil
}

func (s *PgxStore) RecordAttempt(ctx context.Context, orderID string, revision int, a Attempt) error {
	oid, err := parseID(orderID)
	if err != nil {
		return err
	}
	fields := a.MissingFields
	if fields == nil {
		fields = []string{}
	}
	missing, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("record einvoice attempt: %w", err)
	}
	n, err := s.q.SetEinvoiceAttempt(ctx, db.SetEinvoiceAttemptParams{
		State:         string(a.State),
		NextAttemptAt: pgtype.Timestamptz{Time: a.NextAttemptAt, Valid: true},
		LastErrorCode: a.LastErrorCode,
		MissingFields: missing,
		OrderID:       oid,
		Revision:      int32(revision),
	})
	if err != nil {
		return fmt.Errorf("record einvoice attempt: %w", err)
	}
	if n > 0 {
		return nil
	}
	return conflictOrNotFound(ctx, s.q, orderID, revision)
}
