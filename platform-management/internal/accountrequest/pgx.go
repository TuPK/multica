package accountrequest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/db"
)

// Beginner starts a transaction. *pgxpool.Pool is one.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// PgxRunner is the Runner over a Postgres pool.
type PgxRunner struct{ pool Beginner }

// NewPgxRunner returns a Runner that opens one transaction per call.
func NewPgxRunner(pool Beginner) *PgxRunner { return &PgxRunner{pool: pool} }

// InTx implements Runner.
func (r *PgxRunner) InTx(ctx context.Context, fn func(Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op once committed
	if err := fn(&pgxTx{q: db.New(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// PgxReader is the Reader over a Postgres pool.
type PgxReader struct{ q *db.Queries }

// NewPgxReader returns a Reader over pool.
func NewPgxReader(pool db.DBTX) *PgxReader { return &PgxReader{q: db.New(pool)} }

// List implements Reader.
func (r *PgxReader) List(ctx context.Context, q ListQuery) ([]Request, error) {
	rows, err := r.q.ListAccountRequests(ctx, db.ListAccountRequestsParams{State: q.State, RowLimit: int32(q.Limit)})
	if err != nil {
		return nil, fmt.Errorf("list account requests: %w", err)
	}
	out := make([]Request, 0, len(rows))
	for _, row := range rows {
		req, err := fromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, nil
}

// Get implements Reader.
func (r *PgxReader) Get(ctx context.Context, id string) (Request, bool, error) {
	u, err := parseUUID(id)
	if err != nil {
		return Request{}, false, err
	}
	row, err := r.q.GetAccountRequest(ctx, u)
	return one(row, err)
}

type pgxTx struct{ q *db.Queries }

func (t *pgxTx) UserIDByEmail(ctx context.Context, email string) (string, bool, error) {
	id, err := t.q.GetUserIDByEmailIgnoringCase(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("look up user by email: %w", err)
	}
	return uuidText(id), true, nil
}

func (t *pgxTx) LiveByEmail(ctx context.Context, email string) (Request, bool, error) {
	row, err := t.q.GetLiveAccountRequestByEmail(ctx, email)
	return one(row, err)
}

func (t *pgxTx) Insert(ctx context.Context, in Input, createdBy string) (Request, bool, error) {
	by, err := parseUUID(createdBy)
	if err != nil {
		return Request{}, false, err
	}
	var profile []byte
	if in.BillingProfile != nil {
		if profile, err = json.Marshal(in.BillingProfile); err != nil {
			return Request{}, false, err
		}
	}
	row, err := t.q.InsertAccountRequest(ctx, db.InsertAccountRequestParams{
		Kind: in.Kind, Email: in.Email, DisplayName: in.DisplayName, WorkspaceName: in.WorkspaceName,
		WorkspaceSlug: in.WorkspaceSlug, PlanKey: in.PlanKey, Seats: int32(in.Seats), Interval: in.Interval,
		Trial: in.Trial, CompReason: in.CompReason, BillingProfile: profile, CreatedBy: by,
	})
	r, found, err := one(row, err)
	return r, found, err
}

func (t *pgxTx) Lock(ctx context.Context, id string) (Request, bool, error) {
	u, err := parseUUID(id)
	if err != nil {
		return Request{}, false, err
	}
	row, err := t.q.LockAccountRequest(ctx, u)
	return one(row, err)
}

func (t *pgxTx) Cancel(ctx context.Context, id string) (Request, error) {
	u, err := parseUUID(id)
	if err != nil {
		return Request{}, err
	}
	row, err := t.q.CancelAccountRequest(ctx, u)
	if err != nil {
		return Request{}, fmt.Errorf("cancel account request: %w", err)
	}
	return fromRow(row)
}

// InsertAuditEvent implements audit.Writer inside the transaction.
func (t *pgxTx) InsertAuditEvent(ctx context.Context, e audit.Entry) error {
	var ws pgtype.UUID
	if e.WorkspaceID != "" {
		u, err := parseUUID(e.WorkspaceID)
		if err != nil {
			return err
		}
		ws = u
	}
	detail := e.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	return t.q.InsertAuditEvent(ctx, db.InsertAuditEventParams{
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

// one turns a single-row query result into found/not found.
func one(row db.PlatformAccountRequest, err error) (Request, bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, false, nil
	}
	if err != nil {
		return Request{}, false, fmt.Errorf("read account request: %w", err)
	}
	r, err := fromRow(row)
	return r, err == nil, err
}

func fromRow(row db.PlatformAccountRequest) (Request, error) {
	r := Request{
		ID: uuidText(row.ID), Kind: row.Kind, Email: row.Email, DisplayName: row.DisplayName,
		WorkspaceName: row.WorkspaceName, WorkspaceSlug: row.WorkspaceSlug, PlanKey: row.PlanKey,
		Seats: int(row.Seats), Interval: row.Interval, Trial: row.Trial, CompReason: row.CompReason,
		State: row.State, ErrorCode: row.ErrorCode, Attempts: int(row.Attempts),
		UserID: uuidText(row.UserID), WorkspaceID: uuidText(row.WorkspaceID), CreatedBy: uuidText(row.CreatedBy),
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		CompletedAt: timePtr(row.CompletedAt), WelcomeSentAt: timePtr(row.WelcomeSentAt),
		WelcomeAttempts: int(row.WelcomeAttempts),
	}
	if len(row.BillingProfile) > 0 {
		var p BillingProfile
		if err := json.Unmarshal(row.BillingProfile, &p); err != nil {
			return Request{}, fmt.Errorf("account request %s: billing profile: %w", r.ID, err)
		}
		r.BillingProfile = &p
	}
	return r, nil
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func parseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil || !u.Valid {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid %q", s)
	}
	return u, nil
}

func uuidText(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	h := fmt.Sprintf("%x", b)
	return strings.Join([]string{h[0:8], h[8:12], h[12:16], h[16:20], h[20:32]}, "-")
}
