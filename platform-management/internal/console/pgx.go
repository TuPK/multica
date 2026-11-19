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
	"github.com/multica-ai/multica/platform-management/internal/entitlement"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/usage"
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
type PgxReader struct {
	q     *db.Queries
	pool  db.DBTX
	usage *usage.Reader
}

// NewPgxReader returns a Reader over pool.
func NewPgxReader(pool db.DBTX) *PgxReader {
	return &PgxReader{q: db.New(pool), pool: pool, usage: usage.NewReader(pool)}
}

// groupCount counts a workspace's account groups. member_group belongs to
// Multica, and a platform database may not have the table yet, so a missing
// table reads as zero rather than failing the tenant page.
func (r *PgxReader) groupCount(ctx context.Context, id pgtype.UUID) (int64, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT to_regclass('member_group') IS NOT NULL`).Scan(&exists); err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM member_group WHERE workspace_id = $1`, id).Scan(&n)
	return n, err
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

// scopeArgs turns a Scope into the arguments of the scoped queries. A scoped
// caller with no usable tenant id gets a null tenant, which matches nothing.
func scopeArgs(s Scope) (scoped bool, tenant pgtype.UUID) {
	if s.All {
		return false, pgtype.UUID{}
	}
	u, err := parseUUID(s.TenantID)
	if err != nil {
		return true, pgtype.UUID{}
	}
	return true, u
}

func parseLimits(raw []byte) (plan.Limits, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var l plan.Limits
	if err := json.Unmarshal(raw, &l); err != nil {
		return nil, err
	}
	return l, nil
}

// tenantSummary is what the console shows of the workspaces of one tenant.
type tenantSummary struct {
	max        *int64
	bestPlan   string
	status     map[string]int64
	workspaces []TenantWorkspace
}

// summarize reduces the workspaces of one tenant: the effective max_workspaces
// (the largest among them, nil when any has none), the plan of the
// highest-ranked one, and the count of workspaces by status.
func summarize(rows []db.ListTenantWorkspaceSummariesRow) (tenantSummary, error) {
	out := tenantSummary{status: map[string]int64{}}
	var siblings []entitlement.Sibling
	bestSort := -1
	for _, row := range rows {
		limits, err := parseLimits(row.PlanLimits)
		if err != nil {
			return tenantSummary{}, fmt.Errorf("plan %q limits: %w", row.PlanKey, err)
		}
		overrides, err := parseLimits(row.LimitOverrides)
		if err != nil {
			return tenantSummary{}, fmt.Errorf("limit overrides of workspace %s: %w", uuidText(row.WorkspaceID), err)
		}
		siblings = append(siblings, entitlement.Sibling{Plan: plan.Plan{Key: row.PlanKey, Limits: limits}, Overrides: overrides})
		if rank := int(row.PlanSort); rank > bestSort || (rank == bestSort && row.PlanKey < out.bestPlan) {
			bestSort, out.bestPlan = rank, row.PlanKey
		}
		out.status[row.Status]++
		out.workspaces = append(out.workspaces, TenantWorkspace{
			ID: uuidText(row.WorkspaceID), Name: row.Name, Slug: row.Slug, Status: row.Status, PlanKey: row.PlanKey,
			MemberCount: row.MemberCount, OwnerID: uuidText(row.OwnerID), OwnerEmail: row.OwnerEmail.String,
			CreatedAt: row.CreatedAt.Time,
		})
	}
	if len(siblings) > 0 {
		out.max = entitlement.TenantMaxWorkspaces(siblings)
	}
	return out, nil
}

// summaries reads the workspaces of the tenants and groups them by tenant.
func (r *PgxReader) summaries(ctx context.Context, tenantIDs []pgtype.UUID) (map[string]tenantSummary, error) {
	out := map[string]tenantSummary{}
	if len(tenantIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.ListTenantWorkspaceSummaries(ctx, tenantIDs)
	if err != nil {
		return nil, err
	}
	byTenant := map[string][]db.ListTenantWorkspaceSummariesRow{}
	for _, row := range rows {
		key := uuidText(row.TenantID)
		byTenant[key] = append(byTenant[key], row)
	}
	for key, group := range byTenant {
		sum, err := summarize(group)
		if err != nil {
			return nil, err
		}
		out[key] = sum
	}
	return out, nil
}

func tenantRow(id pgtype.UUID, name, status string, createdAt, lastActivity pgtype.Timestamptz, count int64, sum tenantSummary) TenantRow {
	if sum.status == nil {
		sum.status = map[string]int64{}
	}
	return TenantRow{
		ID: uuidText(id), Name: name, Status: status, WorkspaceCount: count, MaxWorkspaces: sum.max,
		BestPlanKey: sum.bestPlan, StatusSummary: sum.status, CreatedAt: createdAt.Time, LastActivityAt: lastActivity.Time,
	}
}

func (r *PgxReader) SearchTenants(ctx context.Context, q TenantSearch) ([]TenantRow, error) {
	at, id, err := cursorArgs(q.Cursor)
	if err != nil {
		return nil, err
	}
	scoped, scopeTenant := scopeArgs(q.Scope)
	rows, err := r.q.SearchTenants(ctx, db.SearchTenantsParams{
		Scoped: scoped, ScopeTenant: scopeTenant,
		Pattern: textArg(q.Pattern), CursorCreatedAt: at, CursorID: id, RowLimit: int32(q.Limit + 1),
	})
	if err != nil {
		return nil, err
	}
	ids := make([]pgtype.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	sums, err := r.summaries(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]TenantRow, len(rows))
	for i, row := range rows {
		out[i] = tenantRow(row.ID, row.Name, row.Status, row.CreatedAt, row.LastActivityAt, row.WorkspaceCount, sums[uuidText(row.ID)])
	}
	return out, nil
}

func (r *PgxReader) GetTenant(ctx context.Context, id string, scope Scope) (TenantDetail, bool, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return TenantDetail{}, false, err
	}
	scoped, scopeTenant := scopeArgs(scope)
	row, err := r.q.GetTenant(ctx, db.GetTenantParams{ID: uid, Scoped: scoped, ScopeTenant: scopeTenant})
	if errors.Is(err, pgx.ErrNoRows) {
		return TenantDetail{}, false, nil
	}
	if err != nil {
		return TenantDetail{}, false, err
	}
	sums, err := r.summaries(ctx, []pgtype.UUID{row.ID})
	if err != nil {
		return TenantDetail{}, false, err
	}
	sum := sums[uuidText(row.ID)]
	d := TenantDetail{
		TenantRow:  tenantRow(row.ID, row.Name, row.Status, row.CreatedAt, row.LastActivityAt, row.WorkspaceCount, sum),
		CreatedBy:  uuidText(row.CreatedBy),
		Workspaces: sum.workspaces,
	}
	ids := make([]string, len(d.Workspaces))
	for i, w := range d.Workspaces {
		ids[i] = w.ID
	}
	latest, err := r.usage.Latest(ctx, ids)
	if err != nil {
		return TenantDetail{}, false, err
	}
	for i := range d.Workspaces {
		d.Workspaces[i].Usage = latest[d.Workspaces[i].ID]
	}
	if d.Workspaces == nil {
		d.Workspaces = []TenantWorkspace{}
	}
	return d, true, nil
}

func (r *PgxReader) WorkspaceInScope(ctx context.Context, workspaceID string, scope Scope) (bool, error) {
	if scope.All {
		return true, nil
	}
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return false, nil
	}
	_, tenant := scopeArgs(scope)
	return r.q.WorkspaceInScope(ctx, db.WorkspaceInScopeParams{WorkspaceID: ws, ScopeTenant: tenant})
}

func (r *PgxReader) UserInScope(ctx context.Context, userID string, scope Scope) (bool, error) {
	if scope.All {
		return true, nil
	}
	uid, err := parseUUID(userID)
	if err != nil {
		return false, nil
	}
	_, tenant := scopeArgs(scope)
	return r.q.UserInScope(ctx, db.UserInScopeParams{UserID: uid, ScopeTenant: tenant})
}

func (r *PgxReader) GetWorkspace(ctx context.Context, id string) (WorkspaceDetail, bool, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return WorkspaceDetail{}, false, err
	}
	row, err := r.q.GetWorkspace(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkspaceDetail{}, false, nil
	}
	if err != nil {
		return WorkspaceDetail{}, false, err
	}
	latest, err := r.usage.Latest(ctx, []string{uuidText(row.ID)})
	if err != nil {
		return WorkspaceDetail{}, false, err
	}
	current, err := r.usage.Current(ctx, uuidText(row.ID))
	if err != nil {
		return WorkspaceDetail{}, false, err
	}
	groups, err := r.groupCount(ctx, row.ID)
	if err != nil {
		return WorkspaceDetail{}, false, err
	}
	return WorkspaceDetail{
		GroupCount: groups,
		WorkspaceRow: WorkspaceRow{
			ID: uuidText(row.ID), Name: row.Name, Slug: row.Slug, Status: row.Status,
			MemberCount: row.MemberCount, CreatedAt: row.CreatedAt.Time, LastActivityAt: row.LastActivityAt.Time,
			Usage: latest[uuidText(row.ID)],
		},
		Current:             current,
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
	scoped, scopeTenant := scopeArgs(q.Scope)
	rows, err := r.q.SearchUsers(ctx, db.SearchUsersParams{
		Scoped: scoped, ScopeTenant: scopeTenant,
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
			UserID: uuidText(row.UserID), Email: row.Email, Role: row.Role, TenantID: optionalUUIDText(row.TenantID),
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
		scoped, scopeTenant := scopeArgs(q.Scope)
		events, err = r.q.ListAllAuditEvents(ctx, db.ListAllAuditEventsParams{
			Scoped: scoped, ScopeTenant: scopeTenant,
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
	byCurrency, err := r.pendingByCurrency(ctx)
	if err != nil {
		return Overview{}, err
	}
	totals, err := r.usage.Totals(ctx, time.Now())
	if err != nil {
		return Overview{}, err
	}
	withoutTenant, err := r.q.CountWorkspacesWithoutTenant(ctx)
	if err != nil {
		return Overview{}, err
	}
	o := Overview{
		WorkspacesWithoutTenant: withoutTenant,
		PendingOrders:           pending, PendingByCurrency: byCurrency, Usage: totals,
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

// pendingByCurrencySQL sums the pending orders once per currency.
const pendingByCurrencySQL = `SELECT currency, count(*)::bigint, COALESCE(sum(amount), 0)::bigint, COALESCE(sum(vat_amount), 0)::bigint
	FROM billing_order WHERE status = 'pending' GROUP BY currency`

func (r *PgxReader) pendingByCurrency(ctx context.Context) (map[string]CurrencyTotal, error) {
	rows, err := r.pool.Query(ctx, pendingByCurrencySQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]CurrencyTotal{}
	for rows.Next() {
		var (
			currency string
			t        CurrencyTotal
		)
		if err := rows.Scan(&currency, &t.Orders, &t.Amount, &t.VATAmount); err != nil {
			return nil, err
		}
		out[currency] = t
	}
	return out, rows.Err()
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

// listOrdersSQL reads the orders of one status with the state of each order's
// highest e-invoice revision in a single query. The order direction is the
// only text added to it.
const listOrdersSQL = `SELECT o.id, o.workspace_id, o.kind, o.plan_key, o.interval, o.seats, o.amount, o.vat_amount, o.currency,
		o.status, o.provider, o.provider_ref, o.transfer_code, o.idempotency_key, o.buyer, o.period_start, o.period_end,
		o.einvoice_ref, o.created_by, o.confirmed_by, o.paid_at, o.expires_at, o.created_at, w.name AS workspace_name,
		inv.state AS invoice_state
	FROM billing_order o
	LEFT JOIN workspace w ON w.id = o.workspace_id
	LEFT JOIN LATERAL (
		SELECT e.state FROM einvoice_issuance e WHERE e.order_id = o.id ORDER BY e.revision DESC LIMIT 1
	) inv ON true
	WHERE o.status = $1
	  AND (NOT $3::boolean OR EXISTS (
		SELECT 1 FROM tenant_workspace tw WHERE tw.workspace_id = o.workspace_id AND tw.tenant_id = $4::uuid
	  ))
	ORDER BY o.created_at %[1]s, o.id %[1]s
	LIMIT $2`

// ListOrders implements Reader. The pending queue is oldest first.
func (r *PgxReader) ListOrders(ctx context.Context, status billing.Status, newestFirst bool, limit int, scope Scope) ([]OrderRow, error) {
	dir := "ASC"
	if newestFirst {
		dir = "DESC"
	}
	scoped, scopeTenant := scopeArgs(scope)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(listOrdersSQL, dir), string(status), int32(limit), scoped, scopeTenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OrderRow{}
	for rows.Next() {
		var (
			o     db.BillingOrder
			name  pgtype.Text
			state pgtype.Text
		)
		if err := rows.Scan(&o.ID, &o.WorkspaceID, &o.Kind, &o.PlanKey, &o.Interval, &o.Seats, &o.Amount, &o.VatAmount, &o.Currency,
			&o.Status, &o.Provider, &o.ProviderRef, &o.TransferCode, &o.IdempotencyKey, &o.Buyer, &o.PeriodStart, &o.PeriodEnd,
			&o.EinvoiceRef, &o.CreatedBy, &o.ConfirmedBy, &o.PaidAt, &o.ExpiresAt, &o.CreatedAt, &name, &state); err != nil {
			return nil, err
		}
		row := orderFromRow(o, name)
		row.InvoiceState = state.String
		out = append(out, row)
	}
	return out, rows.Err()
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

// WorkspaceUsage implements Reader.
func (r *PgxReader) WorkspaceUsage(ctx context.Context, workspaceID string, now time.Time) (usage.Month, error) {
	return r.usage.Month(ctx, workspaceID, now)
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

func (t *pgxTx) TenantExists(ctx context.Context, id string) (bool, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return false, nil
	}
	return t.q.TenantExists(ctx, uid)
}

func (t *pgxTx) ActiveSystemAdmins(ctx context.Context) ([]string, error) {
	ids, err := t.q.LockActiveSystemAdmins(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = uuidText(id)
	}
	return out, nil
}

func (t *pgxTx) ActiveStaffRole(ctx context.Context, userID string) (string, string, bool, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return "", "", false, err
	}
	row, err := t.q.GetActiveStaffByUserID(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return row.Role, optionalUUIDText(row.TenantID), true, nil
}

func (t *pgxTx) ReactivateStaff(ctx context.Context, userID, role, tenantID string) (bool, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return false, err
	}
	tenant, err := optionalUUID(tenantID)
	if err != nil {
		return false, err
	}
	n, err := t.q.ReactivateStaff(ctx, db.ReactivateStaffParams{UserID: uid, Role: role, TenantID: tenant})
	return n > 0, err
}

func (t *pgxTx) InsertStaff(ctx context.Context, userID, role, tenantID, createdBy string) (bool, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return false, err
	}
	by, err := parseUUID(createdBy)
	if err != nil {
		return false, err
	}
	tenant, err := optionalUUID(tenantID)
	if err != nil {
		return false, err
	}
	n, err := t.q.InsertStaff(ctx, db.InsertStaffParams{UserID: uid, Role: role, CreatedBy: by, TenantID: tenant})
	return n > 0, err
}

func (t *pgxTx) UpdateStaff(ctx context.Context, userID, role, tenantID string) (bool, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return false, err
	}
	tenant, err := optionalUUID(tenantID)
	if err != nil {
		return false, err
	}
	n, err := t.q.UpdateStaffRoleAndTenant(ctx, db.UpdateStaffRoleAndTenantParams{UserID: uid, Role: role, TenantID: tenant})
	return n > 0, err
}

// optionalUUID parses a UUID, and returns NULL for empty text.
func optionalUUID(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, nil
	}
	return parseUUID(s)
}

// optionalUUIDText is the text of a UUID, and "" for NULL.
func optionalUUIDText(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuidText(u)
}

func (t *pgxTx) RevokeStaff(ctx context.Context, userID string) (bool, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return false, err
	}
	n, err := t.q.RevokeStaff(ctx, uid)
	return n > 0, err
}

// DeleteUserMFA implements Tx.
func (t *pgxTx) DeleteUserMFA(ctx context.Context, userID string) (bool, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return false, err
	}
	n, err := t.q.DeleteUserMFA(ctx, uid)
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

// PgxLogins is the LoginLookup over a Postgres pool.
type PgxLogins struct{ q *db.Queries }

// NewPgxLogins returns a LoginLookup over pool.
func NewPgxLogins(pool db.DBTX) *PgxLogins { return &PgxLogins{q: db.New(pool)} }

func (l *PgxLogins) UsersWithLogin(ctx context.Context, userIDs []string) (map[string]bool, error) {
	ids, err := l.q.ListUserIDsWithLogin(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}
