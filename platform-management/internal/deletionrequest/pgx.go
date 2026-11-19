package deletionrequest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
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
	if err := fn(&pgxTx{Tx: workspacestatus.NewTx(tx), q: db.New(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type pgxTx struct {
	workspacestatus.Tx
	q *db.Queries
}

// PgxReader is the Reader over a Postgres pool.
type PgxReader struct{ q *db.Queries }

// NewPgxReader returns a Reader over pool.
func NewPgxReader(pool db.DBTX) *PgxReader { return &PgxReader{q: db.New(pool)} }

func (t *pgxTx) Workspace(ctx context.Context, id string) (Workspace, bool, error) {
	u, err := parseUUID(id)
	if err != nil {
		return Workspace{}, false, err
	}
	row, err := t.q.GetDeletionWorkspace(ctx, u)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, false, nil
	}
	if err != nil {
		return Workspace{}, false, fmt.Errorf("read workspace: %w", err)
	}
	return Workspace{ID: uuidText(row.ID), Name: row.Name, Slug: row.Slug, Status: row.Status, TenantID: uuidText(row.TenantID)}, true, nil
}

func (t *pgxTx) Tenant(ctx context.Context, id string) (Tenant, bool, error) {
	u, err := parseUUID(id)
	if err != nil {
		return Tenant{}, false, err
	}
	row, err := t.q.GetDeletionTenant(ctx, u)
	if errors.Is(err, pgx.ErrNoRows) {
		return Tenant{}, false, nil
	}
	if err != nil {
		return Tenant{}, false, fmt.Errorf("read tenant: %w", err)
	}
	return Tenant{ID: uuidText(row.ID), Name: row.Name, Status: row.Status}, true, nil
}

func (t *pgxTx) LockTenant(ctx context.Context, tenantID string) error {
	return t.q.LockDeletionTenant(ctx, tenantID)
}

func (t *pgxTx) TenantWorkspaces(ctx context.Context, tenantID string) ([]string, error) {
	u, err := parseUUID(tenantID)
	if err != nil {
		return nil, err
	}
	rows, err := t.q.ListDeletionTenantWorkspaces(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("list tenant workspaces: %w", err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = uuidText(r)
	}
	return out, nil
}

func (t *pgxTx) Live(ctx context.Context, kind, targetID string) (Request, bool, error) {
	u, err := parseUUID(targetID)
	if err != nil {
		return Request{}, false, err
	}
	row, err := t.q.GetLiveDeletionRequest(ctx, db.GetLiveDeletionRequestParams{Kind: kind, TargetID: u})
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, false, nil
	}
	if err != nil {
		return Request{}, false, fmt.Errorf("read live request: %w", err)
	}
	return fromRow(row), true, nil
}

func (t *pgxTx) Lock(ctx context.Context, id string) (Request, bool, error) {
	u, err := parseUUID(id)
	if err != nil {
		return Request{}, false, err
	}
	row, err := t.q.LockDeletionRequest(ctx, u)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, false, nil
	}
	if err != nil {
		return Request{}, false, fmt.Errorf("lock request: %w", err)
	}
	return fromRow(row), true, nil
}

func (t *pgxTx) Insert(ctx context.Context, n NewRequest) (Request, bool, error) {
	target, err := parseUUID(n.TargetID)
	if err != nil {
		return Request{}, false, err
	}
	by, err := parseUUID(n.By.ID)
	if err != nil {
		return Request{}, false, err
	}
	tenant, err := optionalUUID(n.TenantID)
	if err != nil {
		return Request{}, false, err
	}
	parent, err := optionalUUID(n.ParentID)
	if err != nil {
		return Request{}, false, err
	}
	resolutions := n.Resolutions
	if resolutions == nil {
		resolutions = []byte(`{}`)
	}
	row, err := t.q.InsertDeletionRequest(ctx, db.InsertDeletionRequestParams{
		Kind:             n.Kind,
		TargetID:         target,
		TargetLabel:      n.TargetLabel,
		Reason:           n.Reason,
		RequestedBy:      by,
		RequestedByLabel: n.By.Label,
		ExportFirst:      n.ExportFirst,
		Resolutions:      resolutions,
		ParentID:         parent,
		TenantID:         tenant,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, false, nil
	}
	if err != nil {
		return Request{}, false, fmt.Errorf("insert request: %w", err)
	}
	return fromRow(row), true, nil
}

func (t *pgxTx) MarkCanceled(ctx context.Context, id string) (Request, error) {
	u, err := parseUUID(id)
	if err != nil {
		return Request{}, err
	}
	row, err := t.q.CancelDeletionRequest(ctx, u)
	if err != nil {
		return Request{}, fmt.Errorf("cancel request: %w", err)
	}
	return fromRow(row), nil
}

func (t *pgxTx) Retry(ctx context.Context, id string) (Request, error) {
	u, err := parseUUID(id)
	if err != nil {
		return Request{}, err
	}
	row, err := t.q.RetryDeletionRequest(ctx, u)
	if err != nil {
		return Request{}, fmt.Errorf("retry request: %w", err)
	}
	return fromRow(row), nil
}

// List implements Reader.
func (r *PgxReader) List(ctx context.Context, f Filter) ([]Request, error) {
	var rows []db.PlatformDeletionRequest
	var err error
	if f.TenantID != "" {
		tenant, perr := parseUUID(f.TenantID)
		if perr != nil {
			return nil, perr
		}
		rows, err = r.q.ListDeletionRequestsInTenant(ctx, db.ListDeletionRequestsInTenantParams{
			TenantID: tenant, Column2: f.Kind, Column3: f.State, Limit: int32(f.Limit),
		})
	} else {
		rows, err = r.q.ListDeletionRequests(ctx, db.ListDeletionRequestsParams{
			Column1: f.Kind, Column2: f.State, Limit: int32(f.Limit),
		})
	}
	if err != nil {
		return nil, fmt.Errorf("list requests: %w", err)
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
		return Request{}, false, err
	}
	row, err := r.q.GetDeletionRequest(ctx, u)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, false, nil
	}
	if err != nil {
		return Request{}, false, fmt.Errorf("read request: %w", err)
	}
	return fromRow(row), true, nil
}

func fromRow(r db.PlatformDeletionRequest) Request {
	return Request{
		ID:               uuidText(r.ID),
		Kind:             r.Kind,
		TargetID:         uuidText(r.TargetID),
		TargetLabel:      r.TargetLabel,
		Reason:           r.Reason,
		State:            r.State,
		ErrorCode:        r.ErrorCode,
		Attempts:         r.Attempts,
		TenantID:         uuidText(r.TenantID),
		RequestedBy:      uuidText(r.RequestedBy),
		RequestedByLabel: r.RequestedByLabel,
		CreatedAt:        r.CreatedAt.Time,
		UpdatedAt:        r.UpdatedAt.Time,
	}
}

func parseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid %q: %w", s, err)
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

func (t *pgxTx) AccountUser(ctx context.Context, id string) (AccountUser, bool, error) {
	u, err := parseUUID(id)
	if err != nil {
		return AccountUser{}, false, err
	}
	row, err := t.q.GetDeletionAccount(ctx, u)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountUser{}, false, nil
	}
	if err != nil {
		return AccountUser{}, false, fmt.Errorf("read account: %w", err)
	}
	if row.Status == accountStatusDeleted {
		return AccountUser{}, false, nil
	}
	return AccountUser{ID: uuidText(row.ID), Email: row.Email, Status: row.Status, Staff: row.IsStaff}, true, nil
}

func (t *pgxTx) AccountWorkspaces(ctx context.Context, userID string) ([]AccountWorkspace, error) {
	u, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	rows, err := t.q.ListDeletionAccountWorkspaces(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("list account workspaces: %w", err)
	}
	out := make([]AccountWorkspace, len(rows))
	for i, r := range rows {
		out[i] = AccountWorkspace{
			ID: uuidText(r.WorkspaceID), Slug: r.Slug, Role: r.Role,
			OwnerCount: int(r.OwnerCount), MemberCount: int(r.MemberCount),
		}
	}
	return out, nil
}

func (t *pgxTx) WorkspaceMember(ctx context.Context, workspaceID, userID string) (string, string, bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return "", "", false, err
	}
	u, err := parseUUID(userID)
	if err != nil {
		return "", "", false, err
	}
	row, err := t.q.GetDeletionMember(ctx, db.GetDeletionMemberParams{WorkspaceID: ws, UserID: u})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("read member: %w", err)
	}
	return row.Role, row.Status, true, nil
}

func (t *pgxTx) ScheduleAccount(ctx context.Context, userID, reason string, by Actor, at time.Time) error {
	u, err := parseUUID(userID)
	if err != nil {
		return err
	}
	b, err := parseUUID(by.ID)
	if err != nil {
		return err
	}
	return t.q.ScheduleAccountDeletionNow(ctx, db.ScheduleAccountDeletionNowParams{
		UserID: u, Reason: reason, ChangedBy: b, At: pgtype.Timestamptz{Time: at, Valid: true},
	})
}

// Runtime implements Tx.
func (t *pgxTx) Runtime(ctx context.Context, id string) (RuntimeTarget, bool, error) {
	u, err := parseUUID(id)
	if err != nil {
		return RuntimeTarget{}, false, err
	}
	row, err := t.q.GetDeletionRuntime(ctx, u)
	if errors.Is(err, pgx.ErrNoRows) {
		return RuntimeTarget{}, false, nil
	}
	if err != nil {
		return RuntimeTarget{}, false, fmt.Errorf("read runtime: %w", err)
	}
	return RuntimeTarget{ID: uuidText(row.ID), Name: row.Name, WorkspaceID: uuidText(row.WorkspaceID), TenantID: row.TenantID}, true, nil
}

// InsertExport implements Tx: a queued export of the workspace, unless one is
// queued or running already. It reports whether it queued one.
func (t *pgxTx) InsertExport(ctx context.Context, workspaceID, requestedBy string) (bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return false, err
	}
	busy, err := t.q.WorkspaceExportInFlight(ctx, ws)
	if err != nil {
		return false, fmt.Errorf("read export state: %w", err)
	}
	if busy {
		return false, nil
	}
	by, err := parseUUID(requestedBy)
	if err != nil {
		return false, err
	}
	if _, err := t.q.InsertWorkspaceExport(ctx, db.InsertWorkspaceExportParams{WorkspaceID: ws, RequestedBy: by}); err != nil {
		return false, fmt.Errorf("queue export: %w", err)
	}
	return true, nil
}

// OwnerEmails implements OwnerLookup over the shared member and user tables.
func (r *PgxReader) OwnerEmails(ctx context.Context, workspaceID string) ([]string, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return nil, err
	}
	return r.q.ListWorkspaceOwnerEmails(ctx, ws)
}

// ActiveLegalHold implements Tx.
func (t *pgxTx) ActiveLegalHold(ctx context.Context, workspaceID, tenantID string) (string, bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return "", false, err
	}
	tenant, err := optionalUUID(tenantID)
	if err != nil {
		return "", false, err
	}
	id, err := t.q.ActiveLegalHoldForWorkspace(ctx, db.ActiveLegalHoldForWorkspaceParams{WorkspaceID: ws, TenantID: tenant})
	return holdResult(id, err)
}

// ActiveTenantHold implements Tx.
func (t *pgxTx) ActiveTenantHold(ctx context.Context, tenantID string) (string, bool, error) {
	tn, err := parseUUID(tenantID)
	if err != nil {
		return "", false, err
	}
	id, err := t.q.ActiveLegalHoldForTenant(ctx, tn)
	return holdResult(id, err)
}

// ActiveAccountHold implements Tx.
func (t *pgxTx) ActiveAccountHold(ctx context.Context, userID string) (string, bool, error) {
	u, err := parseUUID(userID)
	if err != nil {
		return "", false, err
	}
	id, err := t.q.ActiveLegalHoldForAccount(ctx, u)
	return holdResult(id, err)
}

func holdResult(id pgtype.UUID, err error) (string, bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read legal hold: %w", err)
	}
	return uuidText(id), true, nil
}
