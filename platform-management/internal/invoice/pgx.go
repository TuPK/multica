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

	"github.com/multica-ai/multica/platform-management/audit"
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

// Insert stores the pending record and its einvoice.requested audit row in one
// transaction. A unique-key loss returns the error and writes no audit row.
func (s *PgxStore) Insert(ctx context.Context, n NewRecord) (Record, error) {
	var rec Record
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if rec, err = insert(ctx, q, n); err != nil {
			return err
		}
		return recordAudit(ctx, q, audit.Entry{
			WorkspaceID: rec.WorkspaceID,
			ActorType:   audit.ActorSystem,
			ActorLabel:  "einvoice enqueue",
			Action:      audit.ActionEinvoiceRequested,
			TargetType:  auditTarget,
			TargetID:    rec.OrderID,
			TargetLabel: revisionLabel(rec.Revision),
			Detail:      map[string]any{"order_id": rec.OrderID, "revision": rec.Revision, "provider": rec.Provider},
		})
	})
	if err != nil {
		return Record{}, err
	}
	return rec, nil
}

const auditTarget = "einvoice"

func revisionLabel(revision int) string { return fmt.Sprintf("revision %d", revision) }

// inTx runs fn on a transaction and commits it when fn succeeds. fn's error is
// returned as it is, so callers can still match sentinels and pg error codes.
func (s *PgxStore) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op once committed
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// auditWriter implements audit.Writer on the queries of one transaction.
type auditWriter struct{ q *db.Queries }

func recordAudit(ctx context.Context, q *db.Queries, e audit.Entry) error {
	if err := audit.Record(ctx, auditWriter{q}, e); err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}
	return nil
}

func (w auditWriter) InsertAuditEvent(ctx context.Context, e audit.Entry) error {
	var ws pgtype.UUID
	if e.WorkspaceID != "" {
		if err := ws.Scan(e.WorkspaceID); err != nil {
			return fmt.Errorf("invalid workspace id: %w", err)
		}
	}
	detail := e.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	return w.q.InsertAuditEvent(ctx, db.InsertAuditEventParams{
		WorkspaceID: ws,
		ActorType:   e.ActorType,
		ActorID:     pgtype.Text{String: e.ActorID, Valid: e.ActorID != ""},
		ActorLabel:  e.ActorLabel,
		Action:      e.Action,
		TargetType:  e.TargetType,
		TargetID:    e.TargetID,
		TargetLabel: e.TargetLabel,
		Detail:      raw,
		Ip:          e.IP,
		UserAgent:   e.UserAgent,
		RequestID:   e.RequestID,
	})
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

// GetLatest returns the highest revision of the order.
func (s *PgxStore) GetLatest(ctx context.Context, orderID string) (Record, error) {
	oid, err := parseID(orderID)
	if err != nil {
		return Record{}, err
	}
	row, err := s.q.GetLatestEinvoiceIssuance(ctx, oid)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("get latest einvoice issuance: %w", err)
	}
	return fromRow(row), nil
}

// RetryRecord moves a failed or needs_info record to pending and writes
// einvoice.retried in the same transaction. The actor must be a staff UUID.
func (s *PgxStore) RetryRecord(ctx context.Context, orderID string, revision int, by string, now time.Time) (Record, error) {
	oid, err := parseID(orderID)
	if err != nil {
		return Record{}, err
	}
	var actor pgtype.UUID
	if err := actor.Scan(by); err != nil || !actor.Valid {
		return Record{}, ErrInvalidActor
	}
	var out Record
	err = s.inTx(ctx, func(q *db.Queries) error {
		prev, err := get(ctx, q, orderID, revision)
		if err != nil {
			return err
		}
		// The conditional UPDATE takes the row lock and checks the state again.
		n, err := q.RetryEinvoiceIssuance(ctx, db.RetryEinvoiceIssuanceParams{
			Now: pgtype.Timestamptz{Time: now, Valid: true}, OrderID: oid, Revision: int32(revision),
		})
		if err != nil {
			return fmt.Errorf("retry einvoice issuance: %w", err)
		}
		if n == 0 {
			return ErrStateConflict
		}
		if out, err = get(ctx, q, orderID, revision); err != nil {
			return err
		}
		return recordAudit(ctx, q, audit.Entry{
			WorkspaceID: prev.WorkspaceID,
			ActorType:   audit.ActorStaff,
			ActorID:     by,
			ActorLabel:  by,
			Action:      audit.ActionEinvoiceRetried,
			TargetType:  auditTarget,
			TargetID:    orderID,
			TargetLabel: revisionLabel(revision),
			Detail: map[string]any{
				"order_id": orderID, "revision": revision, "previous_state": string(prev.State),
				"attempts_reset": true, "previous_attempts": prev.Attempts,
			},
		})
	})
	if err != nil {
		return Record{}, err
	}
	return out, nil
}

// MarkIssued moves the record to issued and writes einvoice.issued in the same
// transaction. A record that is not pending writes nothing.
func (s *PgxStore) MarkIssued(ctx context.Context, orderID string, revision int, i Issued) error {
	oid, err := parseID(orderID)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(q *db.Queries) error {
		n, err := q.MarkEinvoiceIssued(ctx, db.MarkEinvoiceIssuedParams{
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
		if n == 0 {
			return conflictOrNotFound(ctx, q, orderID, revision)
		}
		rec, err := get(ctx, q, orderID, revision)
		if err != nil {
			return err
		}
		return recordAudit(ctx, q, audit.Entry{
			WorkspaceID: rec.WorkspaceID,
			ActorType:   audit.ActorSystem,
			ActorLabel:  "einvoice issue",
			Action:      audit.ActionEinvoiceIssued,
			TargetType:  auditTarget,
			TargetID:    orderID,
			TargetLabel: revisionLabel(revision),
			Detail: map[string]any{
				"order_id": orderID, "revision": revision,
				"invoice_number": i.InvoiceNumber, "provider_invoice_id": i.ProviderInvoiceID,
			},
		})
	})
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

// ReplaceIssued cancels the issued record, inserts the next revision and writes
// einvoice.replaced, all in one transaction. The actor must be a staff UUID.
func (s *PgxStore) ReplaceIssued(ctx context.Context, orderID string, revision int, buyer []byte, by, reason string) (Record, error) {
	oid, err := parseID(orderID)
	if err != nil {
		return Record{}, err
	}
	var actor pgtype.UUID
	if err := actor.Scan(by); err != nil || !actor.Valid {
		return Record{}, ErrInvalidActor
	}
	var next Record
	err = s.inTx(ctx, func(q *db.Queries) error {
		// The conditional UPDATE takes the row lock, so a concurrent replace of the
		// same original sees state cancelled and gets ErrStateConflict.
		n, err := q.CancelEinvoiceIssued(ctx, db.CancelEinvoiceIssuedParams{
			CancelledBy:  by,
			CancelReason: reason,
			OrderID:      oid,
			Revision:     int32(revision),
		})
		if err != nil {
			return fmt.Errorf("cancel einvoice issuance: %w", err)
		}
		if n == 0 {
			return conflictOrNotFound(ctx, q, orderID, revision)
		}
		orig, err := get(ctx, q, orderID, revision)
		if err != nil {
			return err
		}
		next, err = insert(ctx, q, NewRecord{
			OrderID:     orig.OrderID,
			WorkspaceID: orig.WorkspaceID,
			Provider:    orig.Provider,
			Currency:    orig.Currency,
			Amount:      orig.Amount,
			VATAmount:   orig.VATAmount,
			Buyer:       buyer,
		})
		if err != nil {
			return err
		}
		return recordAudit(ctx, q, audit.Entry{
			WorkspaceID: orig.WorkspaceID,
			ActorType:   audit.ActorStaff,
			ActorID:     by,
			ActorLabel:  by,
			Action:      audit.ActionEinvoiceReplaced,
			TargetType:  auditTarget,
			TargetID:    orderID,
			TargetLabel: revisionLabel(revision),
			Detail: map[string]any{
				"order_id": orderID, "revision": revision, "new_revision": next.Revision, "reason": reason,
			},
		})
	})
	if err != nil {
		return Record{}, err
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
	return s.inTx(ctx, func(q *db.Queries) error {
		n, err := q.SetEinvoiceAttempt(ctx, db.SetEinvoiceAttemptParams{
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
		if n == 0 {
			return conflictOrNotFound(ctx, q, orderID, revision)
		}
		// A retry that leaves the record pending is not an outcome yet.
		if a.State != StateFailed && a.State != StateNeedsInfo {
			return nil
		}
		rec, err := get(ctx, q, orderID, revision)
		if err != nil {
			return err
		}
		e := audit.Entry{
			WorkspaceID: rec.WorkspaceID,
			ActorType:   audit.ActorSystem,
			ActorLabel:  "einvoice issue",
			TargetType:  auditTarget,
			TargetID:    orderID,
			TargetLabel: revisionLabel(revision),
		}
		if a.State == StateNeedsInfo {
			e.Action = audit.ActionEinvoiceNeedsInfo
			e.Detail = map[string]any{"order_id": orderID, "revision": revision, "missing_fields": fields}
		} else {
			e.Action = audit.ActionEinvoiceFailed
			e.Detail = map[string]any{
				"order_id": orderID, "revision": revision,
				"error_code": a.LastErrorCode, "attempts": rec.Attempts,
			}
		}
		return recordAudit(ctx, q, e)
	})
}
