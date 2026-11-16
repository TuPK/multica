package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/billing"
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

// InTx implements Runner. It returns fn's error as it is, so a caller can still
// recognise an *apiError.
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
	return fmtUUID(u.Bytes)
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func cursorArgs(c *Cursor) (pgtype.Timestamptz, pgtype.UUID, error) {
	if c == nil {
		return pgtype.Timestamptz{}, pgtype.UUID{}, nil
	}
	id, err := parseUUID(c.ID)
	if err != nil {
		return pgtype.Timestamptz{}, pgtype.UUID{}, err
	}
	return pgtype.Timestamptz{Time: c.CreatedAt, Valid: true}, id, nil
}

func textArg(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func (r *PgxReader) SearchTenants(ctx context.Context, q TenantSearch) ([]TenantRow, error) {
	at, id, err := cursorArgs(q.Cursor)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.SearchTenants(ctx, db.SearchTenantsParams{
		Pattern: textArg(q.Pattern), CursorCreatedAt: at, CursorID: id, RowLimit: int32(q.Limit + 1),
	})
	if err != nil {
		return nil, err
	}
	out := make([]TenantRow, len(rows))
	for i, row := range rows {
		out[i] = TenantRow{
			ID: uuidText(row.ID), Name: row.Name, Slug: row.Slug, Status: row.Status,
			MemberCount: row.MemberCount, CreatedAt: row.CreatedAt.Time, LastActivityAt: row.LastActivityAt.Time,
		}
	}
	return out, nil
}

func (r *PgxReader) GetTenant(ctx context.Context, id string) (TenantDetail, bool, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return TenantDetail{}, false, err
	}
	row, err := r.q.GetTenant(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return TenantDetail{}, false, nil
	}
	if err != nil {
		return TenantDetail{}, false, err
	}
	return TenantDetail{
		TenantRow: TenantRow{
			ID: uuidText(row.ID), Name: row.Name, Slug: row.Slug, Status: row.Status,
			MemberCount: row.MemberCount, CreatedAt: row.CreatedAt.Time, LastActivityAt: row.LastActivityAt.Time,
		},
		StateReason:         row.StateReason.String,
		StateChangedBy:      uuidText(row.StateChangedBy),
		StateChangedAt:      timePtr(row.StateChangedAt),
		DeletionScheduledAt: timePtr(row.DeletionScheduledAt),
		NotesCount:          row.NotesCount,
		OwnerID:             uuidText(row.OwnerID),
		OwnerEmail:          row.OwnerEmail.String,
	}, true, nil
}

func (r *PgxReader) SearchUsers(ctx context.Context, q UserSearch) ([]UserRow, error) {
	at, id, err := cursorArgs(q.Cursor)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.SearchUsers(ctx, db.SearchUsersParams{
		Pattern: textArg(q.Pattern), CursorCreatedAt: at, CursorID: id, RowLimit: int32(q.Limit + 1),
	})
	if err != nil {
		return nil, err
	}
	out := make([]UserRow, len(rows))
	for i, row := range rows {
		out[i] = UserRow{
			ID: uuidText(row.ID), Email: row.Email, Name: row.Name, Status: row.Status,
			MembershipsCount: row.MembershipsCount, CreatedAt: row.CreatedAt.Time,
		}
	}
	return out, nil
}

func (r *PgxReader) GetUser(ctx context.Context, id string) (UserDetail, bool, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return UserDetail{}, false, err
	}
	row, err := r.q.GetUser(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserDetail{}, false, nil
	}
	if err != nil {
		return UserDetail{}, false, err
	}
	ms, err := r.q.ListUserMemberships(ctx, uid)
	if err != nil {
		return UserDetail{}, false, err
	}
	d := UserDetail{
		UserRow: UserRow{
			ID: uuidText(row.ID), Email: row.Email, Name: row.Name, Status: row.Status,
			MembershipsCount: row.MembershipsCount, CreatedAt: row.CreatedAt.Time,
		},
		Memberships:        make([]Membership, len(ms)),
		AccountReason:      row.AccountReason.String,
		AccountChangedAt:   timePtr(row.AccountChangedAt),
		SessionsValidAfter: timePtr(row.SessionsValidAfter),
	}
	for i, m := range ms {
		d.Memberships[i] = Membership{WorkspaceID: uuidText(m.WorkspaceID), Name: m.WorkspaceName, Role: m.Role}
	}
	return d, true, nil
}

func (r *PgxReader) ListNotes(ctx context.Context, q NoteQuery) ([]Note, error) {
	ws, err := parseUUID(q.WorkspaceID)
	if err != nil {
		return nil, err
	}
	at, id, err := cursorArgs(q.Cursor)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListPlatformNotes(ctx, db.ListPlatformNotesParams{
		WorkspaceID: ws, CursorCreatedAt: at, CursorID: id, RowLimit: int32(q.Limit + 1),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Note, len(rows))
	for i, row := range rows {
		out[i] = Note{
			ID: uuidText(row.ID), WorkspaceID: uuidText(row.WorkspaceID), AuthorID: uuidText(row.AuthorID),
			AuthorEmail: row.AuthorEmail.String, Body: row.Body, CreatedAt: row.CreatedAt.Time,
		}
	}
	return out, nil
}

func (r *PgxReader) ListStaff(ctx context.Context) ([]StaffRow, error) {
	rows, err := r.q.ListActiveStaffWithEmail(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]StaffRow, len(rows))
	for i, row := range rows {
		out[i] = StaffRow{
			UserID: uuidText(row.UserID), Email: row.Email, Role: row.Role,
			CreatedBy: uuidText(row.CreatedBy), CreatedAt: row.CreatedAt.Time,
		}
	}
	return out, nil
}

func toAuditRow(e db.AuditEvent) AuditRow {
	return AuditRow{
		ID: uuidText(e.ID), WorkspaceID: uuidText(e.WorkspaceID), ActorType: e.ActorType,
		ActorID: e.ActorID.String, ActorLabel: e.ActorLabel, Action: e.Action,
		TargetType: e.TargetType, TargetID: e.TargetID, TargetLabel: e.TargetLabel,
		Detail: e.Detail, IP: e.Ip, UserAgent: e.UserAgent, RequestID: e.RequestID,
		CreatedAt: e.CreatedAt.Time,
	}
}

func timeArg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func (r *PgxReader) ListAuditEvents(ctx context.Context, q AuditQuery) ([]AuditRow, error) {
	at, id, err := cursorArgs(q.Cursor)
	if err != nil {
		return nil, err
	}
	var events []db.AuditEvent
	if q.WorkspaceID == "" {
		events, err = r.q.ListAllAuditEvents(ctx, db.ListAllAuditEventsParams{
			Action: textArg(q.Action), ActorID: textArg(q.ActorID),
			CreatedFrom: timeArg(q.From), CreatedTo: timeArg(q.To),
			CursorCreatedAt: at, CursorID: id, RowLimit: int32(q.Limit + 1),
		})
	} else {
		var ws pgtype.UUID
		if ws, err = parseUUID(q.WorkspaceID); err != nil {
			return nil, err
		}
		events, err = r.q.ListWorkspaceAuditEvents(ctx, db.ListWorkspaceAuditEventsParams{
			WorkspaceID: ws, Action: textArg(q.Action), ActorID: textArg(q.ActorID),
			CreatedFrom: timeArg(q.From), CreatedTo: timeArg(q.To),
			CursorCreatedAt: at, CursorID: id, RowLimit: int32(q.Limit + 1),
		})
	}
	if err != nil {
		return nil, err
	}
	out := make([]AuditRow, len(events))
	for i, e := range events {
		out[i] = toAuditRow(e)
	}
	return out, nil
}

func (r *PgxReader) Overview(ctx context.Context, signupsSince time.Time) (Overview, error) {
	total, err := r.q.CountWorkspaces(ctx)
	if err != nil {
		return Overview{}, err
	}
	rows, err := r.q.CountNonActiveWorkspacesByStatus(ctx)
	if err != nil {
		return Overview{}, err
	}
	signups, err := r.q.CountSignupsSince(ctx, pgtype.Timestamptz{Time: signupsSince, Valid: true})
	if err != nil {
		return Overview{}, err
	}
	plans, err := r.q.CountWorkspacesByPlan(ctx)
	if err != nil {
		return Overview{}, err
	}
	pending, err := r.q.CountPendingBillingOrders(ctx)
	if err != nil {
		return Overview{}, err
	}
	o := Overview{
		PendingOrders:   pending,
		WorkspacesTotal: total, NonActive: make(map[string]int64, len(rows)), Signups: signups,
		ByPlan: make(map[string]int64, len(plans)),
	}
	for _, row := range rows {
		o.NonActive[row.Status] = row.Workspaces
	}
	for _, row := range plans {
		o.ByPlan[row.PlanKey] = row.Workspaces
	}
	return o, nil
}

func orderFromRow(r db.BillingOrder, workspaceName pgtype.Text) OrderRow {
	return OrderRow{
		Order: billing.Order{
			ID: uuidText(r.ID), WorkspaceID: uuidText(r.WorkspaceID), Kind: billing.Kind(r.Kind), PlanKey: r.PlanKey,
			Interval: r.Interval, Seats: int(r.Seats), Amount: r.Amount, VATAmount: r.VatAmount, Currency: r.Currency,
			Status: billing.Status(r.Status), Provider: r.Provider, ProviderRef: r.ProviderRef, TransferCode: r.TransferCode,
			IdempotencyKey: r.IdempotencyKey, Buyer: json.RawMessage(r.Buyer),
			PeriodStart: timePtr(r.PeriodStart), PeriodEnd: timePtr(r.PeriodEnd), EInvoiceRef: r.EinvoiceRef,
			CreatedBy: uuidText(r.CreatedBy), ConfirmedBy: uuidText(r.ConfirmedBy), PaidAt: timePtr(r.PaidAt),
			ExpiresAt: r.ExpiresAt.Time.UTC(), CreatedAt: r.CreatedAt.Time.UTC(),
		},
		WorkspaceName: workspaceName.String,
	}
}

// ListOrders implements Reader.
func (r *PgxReader) ListOrders(ctx context.Context, status billing.Status, newestFirst bool, limit int) ([]OrderRow, error) {
	var out []OrderRow
	if newestFirst {
		rows, err := r.q.ListBillingOrdersByStatusNewestFirst(ctx, db.ListBillingOrdersByStatusNewestFirstParams{Status: string(status), RowLimit: int32(limit)})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, orderFromRow(row.BillingOrder, row.WorkspaceName))
		}
		return nonNil(out), nil
	}
	rows, err := r.q.ListBillingOrdersByStatusOldestFirst(ctx, db.ListBillingOrdersByStatusOldestFirstParams{Status: string(status), RowLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out = append(out, orderFromRow(row.BillingOrder, row.WorkspaceName))
	}
	return nonNil(out), nil
}

func nonNil(rows []OrderRow) []OrderRow {
	if rows == nil {
		return []OrderRow{}
	}
	return rows
}

// GetOrder implements Reader.
func (r *PgxReader) GetOrder(ctx context.Context, id string) (OrderRow, bool, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return OrderRow{}, false, err
	}
	row, err := r.q.GetBillingOrderWithWorkspace(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrderRow{}, false, nil
	}
	if err != nil {
		return OrderRow{}, false, err
	}
	return orderFromRow(row.BillingOrder, row.WorkspaceName), true, nil
}

// RecentOrders implements Reader.
func (r *PgxReader) RecentOrders(ctx context.Context, workspaceID string, limit int) ([]OrderRow, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListBillingOrdersByWorkspace(ctx, db.ListBillingOrdersByWorkspaceParams{WorkspaceID: ws, RowLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]OrderRow, len(rows))
	for i, row := range rows {
		out[i] = orderFromRow(row.BillingOrder, row.WorkspaceName)
	}
	return out, nil
}

type pgxTx struct{ q *db.Queries }

// LockOrder implements Tx.
func (t *pgxTx) LockOrder(ctx context.Context, id string) (billing.Order, bool, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return billing.Order{}, false, err
	}
	row, err := t.q.GetBillingOrderForUpdate(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.Order{}, false, nil
	}
	if err != nil {
		return billing.Order{}, false, err
	}
	return orderFromRow(row, pgtype.Text{}).Order, true, nil
}

// CancelOrder implements Tx. It returns pgx.ErrNoRows when the locked order is
// not pending, which LockOrder's caller has already ruled out.
func (t *pgxTx) CancelOrder(ctx context.Context, id string) (billing.Order, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return billing.Order{}, err
	}
	row, err := t.q.CancelBillingOrder(ctx, uid)
	if err != nil {
		return billing.Order{}, err
	}
	return orderFromRow(row, pgtype.Text{}).Order, nil
}

func (t *pgxTx) WorkspaceName(ctx context.Context, id string) (string, bool, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return "", false, err
	}
	name, err := t.q.GetWorkspaceName(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return name, err == nil, err
}

func (t *pgxTx) InsertNote(ctx context.Context, workspaceID, authorID, body string) (string, time.Time, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return "", time.Time{}, err
	}
	author, err := parseUUID(authorID)
	if err != nil {
		return "", time.Time{}, err
	}
	row, err := t.q.InsertPlatformNote(ctx, db.InsertPlatformNoteParams{WorkspaceID: ws, AuthorID: author, Body: body})
	if err != nil {
		return "", time.Time{}, err
	}
	return uuidText(row.ID), row.CreatedAt.Time, nil
}

func (t *pgxTx) UserIDByEmail(ctx context.Context, email string) (string, bool, error) {
	id, err := t.q.GetUserIDByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return uuidText(id), true, nil
}

func (t *pgxTx) UserEmail(ctx context.Context, id string) (string, bool, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return "", false, err
	}
	email, err := t.q.GetUserEmailByID(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return email, true, nil
}

func (t *pgxTx) Account(ctx context.Context, userID string) (Account, bool, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return Account{}, false, err
	}
	row, err := t.q.GetPlatformAccountForUpdate(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, false, nil
	}
	if err != nil {
		return Account{}, false, err
	}
	return Account{Status: row.Status}, true, nil
}

func (t *pgxTx) SetAccountStatus(ctx context.Context, userID, status, reason, changedBy string) error {
	uid, err := parseUUID(userID)
	if err != nil {
		return err
	}
	by, err := parseUUID(changedBy)
	if err != nil {
		return err
	}
	return t.q.UpsertAccountStatus(ctx, db.UpsertAccountStatusParams{UserID: uid, Status: status, Reason: reason, ChangedBy: by})
}

func (t *pgxTx) SetSessionsValidAfter(ctx context.Context, userID, changedBy string, at time.Time) error {
	uid, err := parseUUID(userID)
	if err != nil {
		return err
	}
	by, err := parseUUID(changedBy)
	if err != nil {
		return err
	}
	return t.q.UpsertAccountSessionsValidAfter(ctx, db.UpsertAccountSessionsValidAfterParams{
		UserID: uid, ChangedBy: by, SessionsValidAfter: pgtype.Timestamptz{Time: at, Valid: true},
	})
}

func (t *pgxTx) ActiveAdmins(ctx context.Context) ([]string, error) {
	ids, err := t.q.LockActiveAdmins(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = uuidText(id)
	}
	return out, nil
}

func (t *pgxTx) ActiveStaffRole(ctx context.Context, userID string) (string, bool, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return "", false, err
	}
	row, err := t.q.GetActiveStaffByUserID(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return row.Role, true, nil
}

func (t *pgxTx) ReactivateStaff(ctx context.Context, userID, role string) (bool, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return false, err
	}
	n, err := t.q.ReactivateStaff(ctx, db.ReactivateStaffParams{UserID: uid, Role: role})
	return n > 0, err
}

func (t *pgxTx) InsertStaff(ctx context.Context, userID, role, createdBy string) (bool, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return false, err
	}
	by, err := parseUUID(createdBy)
	if err != nil {
		return false, err
	}
	n, err := t.q.InsertStaff(ctx, db.InsertStaffParams{UserID: uid, Role: role, CreatedBy: by})
	return n > 0, err
}

func (t *pgxTx) RevokeStaff(ctx context.Context, userID string) (bool, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return false, err
	}
	n, err := t.q.RevokeStaff(ctx, uid)
	return n > 0, err
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
