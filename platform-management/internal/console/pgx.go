package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/multica-ai/multica/platform-management/internal/aicost"
	"github.com/multica-ai/multica/platform-management/internal/quota"
	"github.com/multica-ai/multica/platform-management/internal/tenantpolicy"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/entitlement"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
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

// overviewSeriesDays is the length of the daily series on the overview.
const overviewSeriesDays = 30

func (r *PgxReader) Overview(ctx context.Context, now time.Time) (Overview, error) {
	signupsSince := now.Add(-signupWindow)
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
	series, err := r.overviewSeries(ctx, now)
	if err != nil {
		return Overview{}, err
	}
	runtimes, err := r.q.CountRuntimesByStatus(ctx)
	if err != nil {
		return Overview{}, err
	}
	queue, err := r.q.CountOperationsQueue(ctx)
	if err != nil {
		return Overview{}, err
	}
	o := Overview{
		WorkspacesWithoutTenant: withoutTenant,
		PendingOrders:           pending, PendingByCurrency: byCurrency, Usage: totals,
		WorkspacesTotal: total, NonActive: make(map[string]int64, len(rows)), Signups: signups,
		ByPlan: make(map[string]int64, len(plans)),
		Series: series, RuntimesByStatus: map[string]int64{"online": 0, "offline": 0},
		Operations: OperationsQueue{
			PendingApprovals: queue.PendingApprovals, PendingDeletions: queue.PendingDeletions,
			PendingQuotaRequests: queue.PendingQuotaRequests, FailedEinvoices: queue.FailedEinvoices,
			ActiveLegalHolds: queue.ActiveLegalHolds,
		},
	}
	for _, row := range rows {
		o.NonActive[row.Status] = row.Workspaces
	}
	for _, row := range plans {
		o.ByPlan[row.PlanKey] = row.Workspaces
	}
	for _, row := range runtimes {
		o.RuntimesByStatus[row.Status] = row.Total
	}
	return o, nil
}

// overviewSeries returns the last overviewSeriesDays UTC days, oldest first. Each
// count query returns only the days that had activity; the others stay zero.
func (r *PgxReader) overviewSeries(ctx context.Context, now time.Time) ([]DayCount, error) {
	first := seriesStart(now)
	days := make([]DayCount, overviewSeriesDays)
	index := make(map[string]int, overviewSeriesDays)
	for i := range days {
		key := first.AddDate(0, 0, i).Format(time.DateOnly)
		days[i].Day = key
		index[key] = i
	}
	since := pgtype.Timestamptz{Time: first, Valid: true}

	created, err := r.q.CountWorkspacesCreatedByDay(ctx, since)
	if err != nil {
		return nil, err
	}
	for _, row := range created {
		if i, ok := index[row.Day.Time.Format(time.DateOnly)]; ok {
			days[i].WorkspacesCreated = row.Total
		}
	}
	ordered, err := r.q.CountBillingOrdersCreatedByDay(ctx, since)
	if err != nil {
		return nil, err
	}
	for _, row := range ordered {
		if i, ok := index[row.Day.Time.Format(time.DateOnly)]; ok {
			days[i].OrdersCreated = row.Total
		}
	}
	paid, err := r.q.CountBillingOrdersPaidByDay(ctx, since)
	if err != nil {
		return nil, err
	}
	for _, row := range paid {
		if i, ok := index[row.Day.Time.Format(time.DateOnly)]; ok {
			days[i].OrdersPaid = row.Total
		}
	}
	usageDays, err := r.q.SumIssuesAndTasksByDay(ctx, pgtype.Date{Time: first, Valid: true})
	if err != nil {
		return nil, err
	}
	for _, row := range usageDays {
		if i, ok := index[row.Day.Time.Format(time.DateOnly)]; ok {
			days[i].IssuesCreated = row.IssuesCreated
			days[i].TasksRun = row.TasksRun
		}
	}
	return days, nil
}

// seriesStart is midnight UTC of the first day of the overview series: the
// current UTC day and the overviewSeriesDays-1 days before it.
func seriesStart(now time.Time) time.Time {
	u := now.UTC()
	today := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	return today.AddDate(0, 0, -(overviewSeriesDays - 1))
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

// WorkspaceAICostRows implements Reader.
func (r *PgxReader) WorkspaceAICostRows(ctx context.Context, workspaceID string, since time.Time) ([]aicost.Row, error) {
	return aicost.Load(ctx, r.pool, workspaceID, since)
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

// SearchWorkspaces implements Reader.
func (r *PgxReader) SearchWorkspaces(ctx context.Context, q WorkspaceSearch) ([]WorkspaceListRow, error) {
	at, id, err := cursorArgs(q.Cursor)
	if err != nil {
		return nil, err
	}
	var tenantFilter pgtype.UUID
	if q.TenantFilter != "" {
		if tenantFilter, err = parseUUID(q.TenantFilter); err != nil {
			return nil, err
		}
	}
	scoped, scopeTenant := scopeArgs(q.Scope)
	rows, err := r.q.SearchWorkspaces(ctx, db.SearchWorkspacesParams{
		Pattern: textArg(q.Pattern), Status: q.Status, PlanKey: q.PlanKey,
		Scoped: scoped, ScopeTenant: scopeTenant, TenantFilter: tenantFilter,
		CursorCreatedAt: at, CursorID: id, RowLimit: int32(q.Limit + 1),
	})
	if err != nil {
		return nil, err
	}
	out := make([]WorkspaceListRow, len(rows))
	for i, row := range rows {
		out[i] = WorkspaceListRow{
			ID: uuidText(row.ID), Name: row.Name, Slug: row.Slug, Status: row.Status, PlanKey: row.PlanKey,
			TenantID: uuidText(row.TenantID), TenantName: row.TenantName, OwnerEmail: row.OwnerEmail,
			MemberCount: row.MemberCount, CreatedAt: row.CreatedAt.Time,
		}
	}
	return out, nil
}

// WorkspaceTenant implements Reader.
func (r *PgxReader) WorkspaceTenant(ctx context.Context, workspaceID string) (string, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return "", err
	}
	tenant, err := r.q.GetWorkspaceTenantID(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return uuidText(tenant), nil
}

// RunningExport implements Tx.
func (t *pgxTx) RunningExport(ctx context.Context, workspaceID string) (bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return false, err
	}
	return t.q.WorkspaceExportInFlight(ctx, ws)
}

// InsertExport implements Tx.
func (t *pgxTx) InsertExport(ctx context.Context, workspaceID, requestedBy string) (string, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return "", err
	}
	by, err := parseUUID(requestedBy)
	if err != nil {
		return "", err
	}
	id, err := t.q.InsertWorkspaceExport(ctx, db.InsertWorkspaceExportParams{WorkspaceID: ws, RequestedBy: by})
	if err != nil {
		return "", err
	}
	return uuidText(id), nil
}

// TenantUsers implements Reader: one page of users, each with its memberships in the tenant.
func (r *PgxReader) TenantUsers(ctx context.Context, q TenantUserSearch) ([]TenantUser, error) {
	tenant, err := parseUUID(q.TenantID)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListTenantUserRows(ctx, db.ListTenantUserRowsParams{
		TenantID: tenant, Pattern: q.Pattern, CursorUserID: q.CursorUserID, RowLimit: int32(q.Limit + 1),
	})
	if err != nil {
		return nil, err
	}
	var out []TenantUser
	index := map[string]int{}
	for _, row := range rows {
		id := uuidText(row.UserID)
		i, ok := index[id]
		if !ok {
			out = append(out, TenantUser{
				ID: id, Email: row.Email, Name: row.Name, AccountStatus: row.AccountStatus,
				IsStaff: row.IsStaff, LastLoginAt: timePtr(row.LastLoginAt),
			})
			i = len(out) - 1
			index[id] = i
		}
		out[i].Workspaces = append(out[i].Workspaces, TenantUserWorkspace{
			ID: uuidText(row.WorkspaceID), Name: row.WorkspaceName, Role: row.MemberRole,
		})
	}
	return out, nil
}

// TenantUser implements Reader.
func (r *PgxReader) TenantUser(ctx context.Context, tenantID, userID string) (TenantUser, bool, error) {
	tenant, err := parseUUID(tenantID)
	if err != nil {
		return TenantUser{}, false, err
	}
	user, err := parseUUID(userID)
	if err != nil {
		return TenantUser{}, false, err
	}
	rows, err := r.q.GetTenantUserMemberships(ctx, db.GetTenantUserMembershipsParams{TenantID: tenant, ID: user})
	if err != nil {
		return TenantUser{}, false, err
	}
	if len(rows) == 0 {
		return TenantUser{}, false, nil
	}
	u := TenantUser{
		ID: userID, Email: rows[0].Email, Name: rows[0].Name, AccountStatus: rows[0].AccountStatus, IsStaff: rows[0].IsStaff,
	}
	for _, row := range rows {
		u.Workspaces = append(u.Workspaces, TenantUserWorkspace{ID: uuidText(row.WorkspaceID), Name: row.WorkspaceName, Role: row.MemberRole})
	}
	return u, true, nil
}

// WorkspaceMember implements Reader.
func (r *PgxReader) WorkspaceMember(ctx context.Context, workspaceID, userID string) (string, string, bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return "", "", false, err
	}
	user, err := parseUUID(userID)
	if err != nil {
		return "", "", false, err
	}
	row, err := r.q.GetWorkspaceMemberRole(ctx, db.GetWorkspaceMemberRoleParams{WorkspaceID: ws, UserID: user})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return row.Role, row.Email, true, nil
}

// IsActiveStaff implements Reader.
func (r *PgxReader) IsActiveStaff(ctx context.Context, userID string) (bool, error) {
	user, err := parseUUID(userID)
	if err != nil {
		return false, err
	}
	_, err = r.q.GetActiveStaffByUserID(ctx, user)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// InsertMemberChange implements Tx.
func (t *pgxTx) InsertMemberChange(ctx context.Context, c MemberChange, byID, byLabel string) (string, bool, error) {
	ws, err := parseUUID(c.WorkspaceID)
	if err != nil {
		return "", false, err
	}
	target, err := parseUUID(c.TargetUserID)
	if err != nil {
		return "", false, err
	}
	by, err := parseUUID(byID)
	if err != nil {
		return "", false, err
	}
	row, err := t.q.InsertMemberRequest(ctx, db.InsertMemberRequestParams{
		WorkspaceID: ws, Kind: c.Kind, TargetUserID: target, PreviousRole: c.PreviousRole,
		Email: c.Email, Role: c.Role, RequestedBy: by, RequestedByLabel: byLabel,
	})
	if err == nil {
		return uuidText(row.ID), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, err
	}
	live, err := t.q.GetLiveMemberRequest(ctx, db.GetLiveMemberRequestParams{WorkspaceID: ws, Kind: c.Kind, Email: c.Email})
	if err != nil {
		return "", false, err
	}
	return uuidText(live.ID), false, nil
}

// SearchRuntimes implements Reader.
func (r *PgxReader) SearchRuntimes(ctx context.Context, q RuntimeSearch) ([]RuntimeRow, error) {
	at, id, err := cursorArgs(q.Cursor)
	if err != nil {
		return nil, err
	}
	tenantFilter, wsFilter, err := optionalUUIDs(q.TenantFilter, q.WorkspaceFilter)
	if err != nil {
		return nil, err
	}
	scoped, scopeTenant := scopeArgs(q.Scope)
	rows, err := r.q.SearchRuntimes(ctx, db.SearchRuntimesParams{
		Pattern: textArg(q.Pattern), Status: q.Status, Scoped: scoped, ScopeTenant: scopeTenant,
		TenantFilter: tenantFilter, WorkspaceFilter: wsFilter, DisabledOnly: q.DisabledOnly,
		CursorCreatedAt: at, CursorID: id, RowLimit: int32(q.Limit + 1),
	})
	if err != nil {
		return nil, err
	}
	out := make([]RuntimeRow, len(rows))
	for i, x := range rows {
		out[i] = RuntimeRow{
			ID: uuidText(x.ID), Name: x.Name, WorkspaceID: uuidText(x.WorkspaceID), TenantID: x.TenantID,
			Status: x.Status, OS: osOf(x.DeviceInfo), DaemonVersion: x.DaemonVersion, AgentVersion: x.AgentVersion,
			OwnerID: uuidText(x.OwnerID), Disabled: x.Disabled, RunningTasks: x.RunningTasks,
			LastSeenAt: timePtr(x.LastSeenAt), CreatedAt: x.CreatedAt.Time,
		}
	}
	return out, nil
}

// RuntimeDetail implements Reader.
func (r *PgxReader) RuntimeDetail(ctx context.Context, id string) (RuntimeRow, bool, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return RuntimeRow{}, false, err
	}
	x, err := r.q.GetRuntimeForAdmin(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return RuntimeRow{}, false, nil
	}
	if err != nil {
		return RuntimeRow{}, false, err
	}
	return RuntimeRow{
		ID: uuidText(x.ID), Name: x.Name, WorkspaceID: uuidText(x.WorkspaceID), TenantID: x.TenantID,
		Status: x.Status, OS: osOf(x.DeviceInfo), DaemonVersion: x.DaemonVersion, AgentVersion: x.AgentVersion,
		OwnerID: uuidText(x.OwnerID), Disabled: x.Disabled, RunningTasks: x.RunningTasks,
		LastSeenAt: timePtr(x.LastSeenAt), CreatedAt: x.CreatedAt.Time,
	}, true, nil
}

// RuntimeTasks implements Reader.
func (r *PgxReader) RuntimeTasks(ctx context.Context, id string) ([]RuntimeTask, int64, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.q.ListRuntimeRecentTasks(ctx, uid)
	if err != nil {
		return nil, 0, err
	}
	failed, err := r.q.CountRuntimeFailedTasks(ctx, uid)
	if err != nil {
		return nil, 0, err
	}
	out := make([]RuntimeTask, len(rows))
	for i, x := range rows {
		out[i] = RuntimeTask{
			ID: uuidText(x.ID), Status: x.Status, HasError: x.Error.Valid && x.Error.String != "",
			CreatedAt: x.CreatedAt.Time, CompletedAt: timePtr(x.CompletedAt),
		}
	}
	return out, failed, nil
}

// SetRuntimeDisabled implements Tx. It reports whether the state changed.
func (t *pgxTx) SetRuntimeDisabled(ctx context.Context, runtimeID string, disabled bool, reason, byID string) (bool, error) {
	rt, err := parseUUID(runtimeID)
	if err != nil {
		return false, err
	}
	was, err := t.q.PlatformRuntimeDisabled(ctx, rt)
	if err != nil {
		return false, err
	}
	if disabled {
		if was {
			return false, nil
		}
		by, err := parseUUID(byID)
		if err != nil {
			return false, err
		}
		if err := t.q.DisablePlatformRuntime(ctx, db.DisablePlatformRuntimeParams{RuntimeID: rt, Reason: reason, ChangedBy: by}); err != nil {
			return false, err
		}
		return true, nil
	}
	if !was {
		return false, nil
	}
	n, err := t.q.EnablePlatformRuntime(ctx, rt)
	return n > 0, err
}

// optionalUUIDs parses two optional ids; an empty one is null.
func optionalUUIDs(a, b string) (pgtype.UUID, pgtype.UUID, error) {
	var ua, ub pgtype.UUID
	var err error
	if a != "" {
		if ua, err = parseUUID(a); err != nil {
			return ua, ub, err
		}
	}
	if b != "" {
		if ub, err = parseUUID(b); err != nil {
			return ua, ub, err
		}
	}
	return ua, ub, nil
}

// TenantPolicy implements Reader.
func (r *PgxReader) TenantPolicy(ctx context.Context, tenantID string) (tenantpolicy.Stored, bool, error) {
	id, err := parseUUID(tenantID)
	if err != nil {
		return tenantpolicy.Stored{}, false, err
	}
	row, err := r.q.GetTenantPolicy(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return tenantpolicy.Stored{}, false, nil
	}
	if err != nil {
		return tenantpolicy.Stored{}, false, err
	}
	return storedFromRow(row.AllowedInviteDomains, row.ShareLinksEnabled, row.RequireSso, row.RequireMfa, row.SessionMaxAgeHours, row.Version), true, nil
}

// TenantHasPolicyFeature implements Reader.
func (r *PgxReader) TenantHasPolicyFeature(ctx context.Context, tenantID string) (bool, error) {
	id, err := parseUUID(tenantID)
	if err != nil {
		return false, err
	}
	return r.q.TenantHasPolicyFeature(ctx, id)
}

// TenantPolicyForUpdate implements Tx.
func (t *pgxTx) TenantPolicyForUpdate(ctx context.Context, tenantID string) (tenantpolicy.Stored, bool, error) {
	id, err := parseUUID(tenantID)
	if err != nil {
		return tenantpolicy.Stored{}, false, err
	}
	row, err := t.q.GetTenantPolicyForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return tenantpolicy.Stored{}, false, nil
	}
	if err != nil {
		return tenantpolicy.Stored{}, false, err
	}
	return storedFromRow(row.AllowedInviteDomains, row.ShareLinksEnabled, row.RequireSso, row.RequireMfa, row.SessionMaxAgeHours, row.Version), true, nil
}

// SaveTenantPolicy implements Tx.
func (t *pgxTx) SaveTenantPolicy(ctx context.Context, tenantID string, s tenantpolicy.Stored, byID string) error {
	id, err := parseUUID(tenantID)
	if err != nil {
		return err
	}
	by, err := parseUUID(byID)
	if err != nil {
		return err
	}
	domains := s.AllowedInviteDomains
	if domains == nil {
		domains = []string{}
	}
	return t.q.UpsertTenantPolicy(ctx, db.UpsertTenantPolicyParams{
		TenantID: id, AllowedInviteDomains: domains, ShareLinksEnabled: s.ShareLinksEnabled,
		RequireSso: s.RequireSSO, RequireMfa: s.RequireMFA, SessionMaxAgeHours: s.SessionMaxAgeHours,
		Version: s.Version, UpdatedBy: by,
	})
}

func storedFromRow(domains []string, share, sso, mfa bool, hours int32, version int64) tenantpolicy.Stored {
	if domains == nil {
		domains = []string{}
	}
	return tenantpolicy.Stored{
		Policy: tenantpolicy.Policy{
			AllowedInviteDomains: domains, ShareLinksEnabled: share, RequireSSO: sso, RequireMFA: mfa, SessionMaxAgeHours: hours,
		},
		Version: version,
	}
}

// ListQuotaRequests implements Reader.
func (r *PgxReader) ListQuotaRequests(ctx context.Context, q QuotaSearch) ([]quota.Request, error) {
	at, id, err := cursorArgs(q.Cursor)
	if err != nil {
		return nil, err
	}
	scoped, scopeTenant := scopeArgs(q.Scope)
	rows, err := r.q.ListQuotaRequests(ctx, db.ListQuotaRequestsParams{
		Scoped: scoped, ScopeTenant: scopeTenant, State: q.State,
		CursorCreatedAt: at, CursorID: id, RowLimit: int32(q.Limit + 1),
	})
	if err != nil {
		return nil, err
	}
	out := make([]quota.Request, len(rows))
	for i, x := range rows {
		out[i] = quotaFromRow(x)
	}
	return out, nil
}

// GetQuotaRequest implements Reader.
func (r *PgxReader) GetQuotaRequest(ctx context.Context, id string) (quota.Request, bool, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return quota.Request{}, false, err
	}
	x, err := r.q.GetQuotaRequest(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return quota.Request{}, false, nil
	}
	if err != nil {
		return quota.Request{}, false, err
	}
	return quotaFromRow(x), true, nil
}

func quotaFromRow(x db.QuotaRequest) quota.Request {
	out := quota.Request{
		ID: uuidText(x.ID), WorkspaceID: uuidText(x.WorkspaceID), TenantID: uuidText(x.TenantID), Gate: x.Gate,
		RequestedValue: x.RequestedValue, State: x.State, Reason: x.Reason, DecisionReason: x.DecisionReason,
		RequestedBy: uuidText(x.RequestedBy), RequestedByLabel: x.RequestedByLabel, DecidedBy: uuidText(x.DecidedBy),
		CreatedAt: x.CreatedAt.Time,
	}
	if x.CurrentValue.Valid {
		v := x.CurrentValue.Int64
		out.CurrentValue = &v
	}
	if x.ApprovedValue.Valid {
		v := x.ApprovedValue.Int64
		out.ApprovedValue = &v
	}
	if x.DecidedAt.Valid {
		t := x.DecidedAt.Time
		out.DecidedAt = &t
	}
	return out
}

// AutopilotRuns returns the autopilot runs a workspace used this period.
func (r *PgxReader) AutopilotRuns(ctx context.Context, workspaceID string, now time.Time) (int64, error) {
	return r.usage.AutopilotRuns(ctx, workspaceID, now)
}

// SeatLimit returns the seats a workspace bought: the plan's fixed seats, else the
// subscription's seats. nil means the workspace has no seat limit.
func (r *PgxReader) SeatLimit(ctx context.Context, workspaceID string) (*int64, error) {
	in, err := entitlement.NewPgxLoader(r.pool).Inputs(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load plan: %w", err)
	}
	sub, found, err := subscription.NewPgxReader(r.pool).Get(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load subscription: %w", err)
	}
	var subPtr *subscription.Subscription
	if found {
		subPtr = &sub
	}
	n, ok := subscription.PurchasedSeats(in.Plan, subPtr)
	if !ok {
		return nil, nil
	}
	return &n, nil
}

// HeldSeats counts the seat operations of a workspace that hold a seat at now.
func (r *PgxReader) HeldSeats(ctx context.Context, workspaceID string, now time.Time) (int64, error) {
	var id pgtype.UUID
	if err := id.Scan(workspaceID); err != nil || !id.Valid {
		return 0, fmt.Errorf("invalid workspace id %q", workspaceID)
	}
	return r.q.CountHeldSeatOperations(ctx, db.CountHeldSeatOperationsParams{
		WorkspaceID: id, ExpiresAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
}

// Flows returns a workspace's daily flows from since on.
func (r *PgxReader) Flows(ctx context.Context, workspaceID string, since time.Time) ([]usage.DayFlow, error) {
	return r.usage.Flows(ctx, workspaceID, since)
}

// WorkspaceLimits builds the workspace's entitlement policy and returns the
// limit of each gate. The loader reads the subscription, the plan and the
// tenant, the same way the Multica policy does.
func (r *PgxReader) WorkspaceLimits(ctx context.Context, workspaceID string, now time.Time) (map[string]*int64, error) {
	in, err := entitlement.NewPgxLoader(r.pool).Inputs(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load entitlement inputs: %w", err)
	}
	gates := entitlement.Build(in, now).Gates
	out := make(map[string]*int64, len(gates))
	for name, g := range gates {
		out[name] = g.Limit
	}
	return out, nil
}
