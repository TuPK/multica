package legalhold

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/db"
)

// Beginner opens a transaction; *pgxpool.Pool is one.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Pool is what the Service needs: transactions and reads; *pgxpool.Pool is one.
type Pool interface {
	Beginner
	db.DBTX
}

// Service places, releases and lists holds.
type Service struct{ pool Pool }

// New returns a Service over pool.
func New(pool Pool) *Service { return &Service{pool: pool} }

// Place puts a hold on a scope. A live hold on the same scope is 409 legal_hold_exists.
// tenantID is the tenant the audit row belongs to ("" for an account).
func (s *Service) Place(ctx context.Context, by Actor, kind, scopeID, tenantID, reason, caseRef string) (Hold, error) {
	reason, caseRef, e := CheckInput(kind, reason, caseRef)
	if e != nil {
		return Hold{}, e
	}
	sid, err := parseUUID(scopeID)
	if err != nil {
		return Hold{}, &Error{Status: http.StatusBadRequest, Code: CodeInvalid, Message: "scope_id must be an id"}
	}
	var out Hold
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		by2, err := optionalUUID(by.ID)
		if err != nil {
			return err
		}
		row, err := db.New(tx).InsertLegalHold(ctx, db.InsertLegalHoldParams{
			ScopeKind: kind, ScopeID: sid, Reason: reason, CaseRef: caseRef, PlacedBy: by2,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return &Error{Status: http.StatusConflict, Code: CodeExists, Message: "a live hold already covers this scope"}
		}
		if err != nil {
			return fmt.Errorf("insert legal hold: %w", err)
		}
		out = fromRow(row)
		// The audit row names the hold only: the reason and case reference stay out of it.
		return recordAudit(ctx, tx, audit.Entry{
			TenantID: tenantID, ActorType: audit.ActorStaff, ActorID: by.ID, ActorLabel: by.Label,
			Action: audit.ActionLegalHoldPlaced, TargetType: "legal_hold", TargetID: out.ID, TargetLabel: kind,
			Detail: map[string]any{"legal_hold_id": out.ID},
		})
	})
	return out, err
}

// Release lifts a live hold. An unknown or released hold is ErrNotFound.
func (s *Service) Release(ctx context.Context, by Actor, id, tenantID string) (Hold, error) {
	hid, err := parseUUID(id)
	if err != nil {
		return Hold{}, ErrNotFound
	}
	var out Hold
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		by2, err := optionalUUID(by.ID)
		if err != nil {
			return err
		}
		row, err := db.New(tx).ReleaseLegalHold(ctx, db.ReleaseLegalHoldParams{ID: hid, ReleasedBy: by2})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("release legal hold: %w", err)
		}
		out = fromRow(row)
		return recordAudit(ctx, tx, audit.Entry{
			TenantID: tenantID, ActorType: audit.ActorStaff, ActorID: by.ID, ActorLabel: by.Label,
			Action: audit.ActionLegalHoldReleased, TargetType: "legal_hold", TargetID: out.ID, TargetLabel: out.ScopeKind,
			Detail: map[string]any{"legal_hold_id": out.ID},
		})
	})
	return out, err
}

// List returns the holds, live ones first.
func (s *Service) List(ctx context.Context, limit int) ([]Hold, error) {
	rows, err := db.New(s.pool).ListLegalHolds(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("list legal holds: %w", err)
	}
	out := make([]Hold, len(rows))
	for i, r := range rows {
		out[i] = fromRow(r)
	}
	return out, nil
}

func (s *Service) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func fromRow(r db.LegalHold) Hold {
	return Hold{
		ID: uuidText(r.ID), ScopeKind: r.ScopeKind, ScopeID: uuidText(r.ScopeID), Reason: r.Reason, CaseRef: r.CaseRef,
		PlacedBy: uuidText(r.PlacedBy), ReleasedBy: uuidText(r.ReleasedBy), Released: r.ReleasedAt.Valid,
	}
}

func parseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil || !u.Valid {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid %q", s)
	}
	return u, nil
}

func optionalUUID(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, nil
	}
	return parseUUID(s)
}

func uuidText(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// txWriter is the audit writer of a transaction.
type txWriter struct{ tx pgx.Tx }

// InsertAuditEvent implements audit.Writer inside the transaction.
func (w txWriter) InsertAuditEvent(ctx context.Context, e audit.Entry) error {
	ws, err := optionalUUID(e.WorkspaceID)
	if err != nil {
		return err
	}
	tenant, err := optionalUUID(e.TenantID)
	if err != nil {
		return err
	}
	detail := e.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	return db.New(w.tx).InsertAuditEvent(ctx, db.InsertAuditEventParams{
		WorkspaceID: ws, ActorType: e.ActorType, ActorID: pgtype.Text{String: e.ActorID, Valid: e.ActorID != ""},
		ActorLabel: e.ActorLabel, Action: e.Action, TargetType: e.TargetType, TargetID: e.TargetID,
		TargetLabel: e.TargetLabel, Detail: raw, Ip: e.IP, UserAgent: e.UserAgent, RequestID: e.RequestID, TenantID: tenant,
	})
}

func recordAudit(ctx context.Context, tx pgx.Tx, e audit.Entry) error {
	return audit.Record(ctx, txWriter{tx: tx}, e)
}
