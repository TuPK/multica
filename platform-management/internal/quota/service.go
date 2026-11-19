package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// Beginner opens a transaction; *pgxpool.Pool is one.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Deps are the Service's dependencies. Cache drops a workspace's cached policy
// after an approval commits.
type Deps struct {
	Pool  Beginner
	Cache subscription.Invalidator
	Now   func() time.Time
}

// Service creates, approves and denies quota requests.
type Service struct{ d Deps }

// New returns a Service. A nil Now uses time.Now.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d}
}

// Create asks for one gate of a workspace to be raised (M48). It answers
// ErrNotFound for an unknown workspace and a 409 when a pending request exists.
func (s *Service) Create(ctx context.Context, by Actor, workspaceID, tenantID, gate string, requested int64, reason string) (Request, error) {
	if e := checkGate(gate); e != nil {
		return Request{}, e
	}
	reason, e := cleanReason(reason)
	if e != nil {
		return Request{}, e
	}
	tx, err := s.d.Pool.Begin(ctx)
	if err != nil {
		return Request{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return Request{}, err
	}
	eff, err := q.WorkspaceEffectiveLimit(ctx, db.WorkspaceEffectiveLimitParams{Gate: gate, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrNotFound
	}
	if err != nil {
		return Request{}, fmt.Errorf("read effective limit: %w", err)
	}
	if e := checkRequested(gate, eff.Value, eff.Unlimited, requested); e != nil {
		return Request{}, e
	}
	current := pgtype.Int8{}
	if !eff.Unlimited {
		current = pgtype.Int8{Int64: eff.Value, Valid: true}
	}
	by2, err := optionalUUID(by.ID)
	if err != nil {
		return Request{}, err
	}
	tenant, err := optionalUUID(tenantID)
	if err != nil {
		return Request{}, err
	}
	row, err := q.InsertQuotaRequest(ctx, db.InsertQuotaRequestParams{
		WorkspaceID: ws, TenantID: tenant, Gate: gate, CurrentValue: current, RequestedValue: requested,
		Reason: reason, RequestedBy: by2, RequestedByLabel: by.Label,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, &Error{Status: http.StatusConflict, Code: CodePending, Message: "a request for this gate is already pending"}
	}
	if err != nil {
		return Request{}, fmt.Errorf("insert quota request: %w", err)
	}
	if err := recordAudit(ctx, tx, audit.Entry{
		WorkspaceID: workspaceID, TenantID: tenantID, ActorType: audit.ActorStaff, ActorID: by.ID, ActorLabel: by.Label,
		Action: audit.ActionQuotaRequestCreated, TargetType: "quota_request", TargetID: uuidText(row.ID), TargetLabel: gate,
		Detail: map[string]any{"gate": gate, "requested_value": requested, "reason": reason},
	}); err != nil {
		return Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Request{}, fmt.Errorf("commit: %w", err)
	}
	return fromRow(row), nil
}

// Approve applies a pending request (M47). value, when set, approves a lower
// value than requested. The workspace's override and the request's state commit
// together; the cached policy is dropped after the commit.
func (s *Service) Approve(ctx context.Context, by Actor, id string, value *int64, reason string) (Request, error) {
	reason, e := cleanReason(reason)
	if e != nil {
		return Request{}, e
	}
	var out Request
	var ws, tenantID, gate string
	var approved int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		q := db.New(tx)
		cur, err := lockRequest(ctx, q, id)
		if err != nil {
			return err
		}
		if cur.State != StatePending {
			return &Error{Status: http.StatusConflict, Code: CodeNotPending, Message: "the request is already closed", Extra: map[string]any{"state": cur.State}}
		}
		approved = cur.RequestedValue
		if value != nil {
			approved = *value
		}
		current := int64(0)
		if cur.CurrentValue.Valid {
			current = cur.CurrentValue.Int64
		}
		if e := checkApproved(current, cur.RequestedValue, approved); e != nil {
			return e
		}
		byID, err := optionalUUID(by.ID)
		if err != nil {
			return err
		}
		row, err := q.DecideQuotaRequest(ctx, db.DecideQuotaRequestParams{
			ID: cur.ID, State: StateApproved, ApprovedValue: pgtype.Int8{Int64: approved, Valid: true},
			DecisionReason: reason, DecidedBy: byID,
		})
		if err != nil {
			return fmt.Errorf("decide quota request: %w", err)
		}
		ws, tenantID, gate = uuidText(cur.WorkspaceID), uuidText(cur.TenantID), cur.Gate
		if err := subscription.SetLimitOverrideIn(ctx, tx, tenantID, ws,
			subscription.LimitOverride{Gate: gate, Value: &approved}, reason,
			subscription.OverrideActor{ID: by.ID, Label: by.Label}); err != nil {
			return err
		}
		out = fromRow(row)
		return recordAudit(ctx, tx, audit.Entry{
			WorkspaceID: ws, TenantID: tenantID, ActorType: audit.ActorStaff, ActorID: by.ID, ActorLabel: by.Label,
			Action: audit.ActionQuotaRequestApproved, TargetType: "quota_request", TargetID: id, TargetLabel: gate,
			Detail: map[string]any{"gate": gate, "approved_value": approved, "reason": reason},
		})
	})
	if err != nil {
		return Request{}, err
	}
	if s.d.Cache != nil {
		s.d.Cache.Invalidate(ws)
	}
	return out, nil
}

// Deny closes a pending request without a change (M47). A reason is required.
func (s *Service) Deny(ctx context.Context, by Actor, id, reason string) (Request, error) {
	reason, e := cleanReason(reason)
	if e != nil {
		return Request{}, e
	}
	var out Request
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		q := db.New(tx)
		cur, err := lockRequest(ctx, q, id)
		if err != nil {
			return err
		}
		if cur.State != StatePending {
			return &Error{Status: http.StatusConflict, Code: CodeNotPending, Message: "the request is already closed", Extra: map[string]any{"state": cur.State}}
		}
		byID, err := optionalUUID(by.ID)
		if err != nil {
			return err
		}
		row, err := q.DecideQuotaRequest(ctx, db.DecideQuotaRequestParams{
			ID: cur.ID, State: StateDenied, DecisionReason: reason, DecidedBy: byID,
		})
		if err != nil {
			return fmt.Errorf("decide quota request: %w", err)
		}
		out = fromRow(row)
		return recordAudit(ctx, tx, audit.Entry{
			WorkspaceID: uuidText(cur.WorkspaceID), TenantID: uuidText(cur.TenantID), ActorType: audit.ActorStaff,
			ActorID: by.ID, ActorLabel: by.Label, Action: audit.ActionQuotaRequestDenied,
			TargetType: "quota_request", TargetID: id, TargetLabel: cur.Gate,
			Detail: map[string]any{"gate": cur.Gate, "reason": reason},
		})
	})
	if err != nil {
		return Request{}, err
	}
	return out, nil
}

func (s *Service) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.d.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func lockRequest(ctx context.Context, q *db.Queries, id string) (db.QuotaRequest, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return db.QuotaRequest{}, ErrNotFound
	}
	row, err := q.LockQuotaRequest(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.QuotaRequest{}, ErrNotFound
	}
	if err != nil {
		return db.QuotaRequest{}, fmt.Errorf("lock quota request: %w", err)
	}
	return row, nil
}

func fromRow(r db.QuotaRequest) Request {
	out := Request{
		ID: uuidText(r.ID), WorkspaceID: uuidText(r.WorkspaceID), TenantID: uuidText(r.TenantID), Gate: r.Gate,
		RequestedValue: r.RequestedValue, State: r.State, Reason: r.Reason, DecisionReason: r.DecisionReason,
		RequestedBy: uuidText(r.RequestedBy), RequestedByLabel: r.RequestedByLabel, DecidedBy: uuidText(r.DecidedBy),
		CreatedAt: r.CreatedAt.Time,
	}
	if r.CurrentValue.Valid {
		v := r.CurrentValue.Int64
		out.CurrentValue = &v
	}
	if r.ApprovedValue.Valid {
		v := r.ApprovedValue.Int64
		out.ApprovedValue = &v
	}
	if r.DecidedAt.Valid {
		t := r.DecidedAt.Time
		out.DecidedAt = &t
	}
	return out
}

// recordAudit writes one audit row inside the transaction.
func recordAudit(ctx context.Context, tx pgx.Tx, e audit.Entry) error {
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
	return db.New(tx).InsertAuditEvent(ctx, db.InsertAuditEventParams{
		WorkspaceID: ws, ActorType: e.ActorType, ActorID: pgtype.Text{String: e.ActorID, Valid: e.ActorID != ""},
		ActorLabel: e.ActorLabel, Action: e.Action, TargetType: e.TargetType, TargetID: e.TargetID,
		TargetLabel: e.TargetLabel, Detail: raw, Ip: e.IP, UserAgent: e.UserAgent, RequestID: e.RequestID, TenantID: tenant,
	})
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
