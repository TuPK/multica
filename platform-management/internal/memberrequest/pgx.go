package memberrequest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
func (r *PgxReader) List(ctx context.Context, workspaceID string, limit int) ([]Request, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListMemberRequestsByWorkspace(ctx, db.ListMemberRequestsByWorkspaceParams{WorkspaceID: ws, Limit: int32(limit)})
	if err != nil {
		return nil, fmt.Errorf("list member requests: %w", err)
	}
	out := make([]Request, len(rows))
	for i, row := range rows {
		out[i] = fromRow(row)
	}
	return out, nil
}

// Get implements Reader.
func (r *PgxReader) Get(ctx context.Context, id string) (Request, bool, error) {
	u, err := parseUUID(id)
	if err != nil {
		return Request{}, false, nil
	}
	return one(r.q.GetMemberRequest(ctx, u))
}

// Workspace implements Reader.
func (r *PgxReader) Workspace(ctx context.Context, id string) (Workspace, bool, error) {
	return workspace(ctx, r.q, id)
}

type pgxTx struct{ q *db.Queries }

func (t *pgxTx) Workspace(ctx context.Context, id string) (Workspace, bool, error) {
	return workspace(ctx, t.q, id)
}

func workspace(ctx context.Context, q *db.Queries, id string) (Workspace, bool, error) {
	u, err := parseUUID(id)
	if err != nil {
		return Workspace{}, false, nil
	}
	row, err := q.GetMemberRequestWorkspace(ctx, u)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, false, nil
	}
	if err != nil {
		return Workspace{}, false, fmt.Errorf("read workspace: %w", err)
	}
	return Workspace{ID: uuidText(row.ID), Name: row.Name, Status: row.Status, TenantID: uuidText(row.TenantID)}, true, nil
}

func (t *pgxTx) IsActiveStaff(ctx context.Context, email string) (bool, error) {
	return t.q.IsActiveStaffEmail(ctx, strings.ToLower(email))
}

func (t *pgxTx) IsMember(ctx context.Context, workspaceID, email string) (bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return false, err
	}
	return t.q.IsWorkspaceMemberEmail(ctx, db.IsWorkspaceMemberEmailParams{WorkspaceID: ws, Email: strings.ToLower(email)})
}

func (t *pgxTx) Live(ctx context.Context, workspaceID, kind, email string) (Request, bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return Request{}, false, err
	}
	return one(t.q.GetLiveMemberRequest(ctx, db.GetLiveMemberRequestParams{WorkspaceID: ws, Kind: kind, Email: email}))
}

func (t *pgxTx) Insert(ctx context.Context, workspaceID, email, role string, by Actor) (Request, bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return Request{}, false, err
	}
	staff, err := parseUUID(by.ID)
	if err != nil {
		return Request{}, false, err
	}
	return one(t.q.InsertMemberRequest(ctx, db.InsertMemberRequestParams{
		WorkspaceID: ws, Kind: KindAdd, Email: email, Role: role, RequestedBy: staff, RequestedByLabel: by.Label,
	}))
}

func (t *pgxTx) Lock(ctx context.Context, id string) (Request, bool, error) {
	u, err := parseUUID(id)
	if err != nil {
		return Request{}, false, nil
	}
	return one(t.q.LockMemberRequest(ctx, u))
}

func (t *pgxTx) Cancel(ctx context.Context, id string) (Request, error) {
	u, err := parseUUID(id)
	if err != nil {
		return Request{}, err
	}
	row, err := t.q.CancelMemberRequest(ctx, u)
	if err != nil {
		return Request{}, fmt.Errorf("cancel member request: %w", err)
	}
	return fromRow(row), nil
}

// InsertAuditEvent implements audit.Writer inside the transaction.
func (t *pgxTx) InsertAuditEvent(ctx context.Context, e audit.Entry) error {
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
		TenantID:    tenant,
	})
}

func one(row db.PlatformMemberRequest, err error) (Request, bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, false, nil
	}
	if err != nil {
		return Request{}, false, err
	}
	return fromRow(row), true, nil
}

func fromRow(r db.PlatformMemberRequest) Request {
	out := Request{
		ID: uuidText(r.ID), WorkspaceID: uuidText(r.WorkspaceID), Kind: r.Kind, TargetUserID: uuidText(r.TargetUserID),
		PreviousRole: r.PreviousRole, Email: r.Email, Role: r.Role, State: r.State, ErrorCode: r.ErrorCode,
		Attempts: int(r.Attempts), UserID: uuidText(r.UserID), MemberID: uuidText(r.MemberID), UserCreated: r.UserCreated,
		RequestedBy: uuidText(r.RequestedBy), RequestedByLabel: r.RequestedByLabel,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.CompletedAt.Valid {
		t := r.CompletedAt.Time
		out.CompletedAt = &t
	}
	return out
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
