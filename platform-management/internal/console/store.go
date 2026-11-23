package console

import (
	"context"
	"github.com/multica-ai/multica/platform-management/internal/aicost"

	"github.com/multica-ai/multica/platform-management/internal/quota"
	"github.com/multica-ai/multica/platform-management/internal/tenantpolicy"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/usage"
)

// WorkspaceRow is one workspace in a list.
type WorkspaceRow struct {
	ID             string
	Name           string
	Slug           string
	Status         string
	MemberCount    int64
	CreatedAt      time.Time
	LastActivityAt time.Time
	// Usage is the workspace's newest daily usage row. The zero value means it
	// has none.
	Usage usage.Snapshot
}

// WorkspaceDetail is a workspace with its state record, notes count and owner.
// The state fields are zero when the workspace has no platform row.
type WorkspaceDetail struct {
	WorkspaceRow
	StateReason         string
	StateChangedBy      string
	StateChangedAt      *time.Time
	DeletionScheduledAt *time.Time
	NotesCount          int64
	// GroupCount counts the workspace's account groups (spec 7b.2). Only the
	// count is read: names, members and keys stay in Delegora (isolation rule I6).
	GroupCount int64
	OwnerID    string
	OwnerEmail string
	// Current counts the workspace now, from one query per number.
	Current usage.Current
}

// Scope limits what a staff member's request reads or writes (spec R6 section 3.5).
// It comes from the staff row the middleware read for this request: All for a
// role with access to every object, a tenant for an admin. A Scope that is not
// All and names no tenant matches nothing.
type Scope struct {
	All      bool
	TenantID string
}

// AllScope is the scope of a role with access to every object.
var AllScope = Scope{All: true}

// TenantRow is one tenant in a list.
type TenantRow struct {
	ID             string
	Name           string
	Status         string
	WorkspaceCount int64
	// MaxWorkspaces is the effective limit of the tenant (the largest among its
	// workspaces); nil means no limit.
	MaxWorkspaces *int64
	// BestPlanKey is the plan of the workspace on the highest-ranked plan.
	BestPlanKey string
	// StatusSummary counts the tenant's workspaces by platform status.
	StatusSummary  map[string]int64
	CreatedAt      time.Time
	LastActivityAt time.Time
}

// TenantWorkspace is the summary of one workspace of a tenant.
type TenantWorkspace struct {
	ID          string
	Name        string
	Slug        string
	Status      string
	PlanKey     string
	MemberCount int64
	OwnerID     string
	OwnerEmail  string
	CreatedAt   time.Time
	// Usage is the workspace's newest daily usage row; the zero value means it
	// has none.
	Usage usage.Snapshot
}

// TenantDetail is a tenant with its workspaces.
type TenantDetail struct {
	TenantRow
	CreatedBy  string
	Workspaces []TenantWorkspace
}

// UserRow is one account in a list.
type UserRow struct {
	ID               string
	Email            string
	Name             string
	Status           string
	MembershipsCount int64
	CreatedAt        time.Time
}

// Membership is one workspace a user belongs to.
type Membership struct {
	WorkspaceID string
	Name        string
	Role        string
}

// UserDetail is an account with its memberships and its account record.
type UserDetail struct {
	UserRow
	Memberships        []Membership
	AccountReason      string
	AccountChangedAt   *time.Time
	SessionsValidAfter *time.Time
}

// Note is an internal staff note about a workspace.
type Note struct {
	ID          string
	WorkspaceID string
	AuthorID    string
	AuthorEmail string
	Body        string
	CreatedAt   time.Time
}

// StaffRow is one active staff member.
type StaffRow struct {
	UserID    string
	Email     string
	Role      string
	TenantID  string
	CreatedBy string
	CreatedAt time.Time
}

// AuditRow is one audit_event row.
type AuditRow struct {
	ID          string
	WorkspaceID string
	ActorType   string
	ActorID     string
	ActorLabel  string
	Action      string
	TargetType  string
	TargetID    string
	TargetLabel string
	Detail      []byte
	IP          string
	UserAgent   string
	RequestID   string
	CreatedAt   time.Time
}

// Overview is the data behind GET /overview.
type Overview struct {
	WorkspacesTotal int64
	// WorkspacesWithoutTenant counts the workspaces with no tenant link: what
	// `platform-backfill tenants` repairs. It should be 0.
	WorkspacesWithoutTenant int64
	// NonActive counts the workspaces in each state other than active.
	NonActive map[string]int64
	Signups   int64
	// ByPlan counts the workspaces on each plan key. A workspace with no
	// subscription row is counted under the free plan.
	ByPlan map[string]int64
	// PendingOrders counts the orders waiting for a payment confirmation.
	PendingOrders int64
	// PendingByCurrency sums the pending orders per order currency. Amounts of
	// different currencies are never added together (spec 7.4, rule 5).
	PendingByCurrency map[string]CurrencyTotal
	// Usage sums the newest daily usage row of each workspace that has one.
	Usage usage.Totals
	// Series has one entry per UTC day of the last overviewSeriesDays days,
	// oldest first, with zero where nothing happened.
	Series []DayCount
	// RuntimesByStatus counts the runtimes of every workspace in each status
	// (online, offline). Both keys are always present.
	RuntimesByStatus map[string]int64
	// Operations counts the items waiting in the operations queue.
	Operations OperationsQueue
}

// DayCount is the activity of one UTC day. Day is YYYY-MM-DD.
type DayCount struct {
	Day               string
	WorkspacesCreated int64
	OrdersCreated     int64
	OrdersPaid        int64
	IssuesCreated     int64
	TasksRun          int64
}

// OperationsQueue counts the items the list screens show as waiting.
type OperationsQueue struct {
	PendingApprovals     int64
	PendingDeletions     int64
	PendingQuotaRequests int64
	FailedEinvoices      int64
	ActiveLegalHolds     int64
}

// CurrencyTotal is the count and the amounts, in minor units of one currency,
// of a group of orders.
type CurrencyTotal struct {
	Orders    int64
	Amount    int64
	VATAmount int64
}

// OrderRow is a billing order with the name of its workspace, which is empty
// when the workspace row is gone.
type OrderRow struct {
	billing.Order
	WorkspaceName string
	// InvoiceState is the state of the order's highest e-invoice revision. It is
	// set by ListOrders only and is empty when the order has no record.
	InvoiceState string
}

// ListSearch asks for a page. Pattern is an escaped ILIKE pattern, nil for no
// filter. Readers return up to Limit+1 rows so the caller can tell whether
// another page exists. Scope limits the rows to the staff member's tenant.
type ListSearch struct {
	Pattern *string
	Cursor  *Cursor
	Limit   int
	Scope   Scope
}

// TenantSearch asks for a page of tenants, UserSearch for a page of accounts.
type (
	TenantSearch = ListSearch
	UserSearch   = ListSearch
)

// NoteQuery asks for a page of one workspace's notes.
type NoteQuery struct {
	WorkspaceID string
	Cursor      *Cursor
	Limit       int
}

// AuditQuery asks for a page of audit rows. An empty WorkspaceID asks for every
// row.
type AuditQuery struct {
	Scope       Scope
	WorkspaceID string
	Action      *string
	ActorID     *string
	From        *time.Time
	To          *time.Time
	Cursor      *Cursor
	Limit       int
}

// RuntimeRow is one runtime of the staff list. Device details are reduced to the
// operating system (spec R6 section 3b.5, rule 2): no device_info, metadata or IP.
type RuntimeRow struct {
	ID            string
	Name          string
	WorkspaceID   string
	TenantID      string
	Status        string
	OS            string
	DaemonVersion string
	AgentVersion  string
	OwnerID       string
	Disabled      bool
	RunningTasks  int64
	LastSeenAt    *time.Time
	CreatedAt     time.Time
}

// RuntimeSearch is one page of the runtime list.
type RuntimeSearch struct {
	Pattern         *string
	Status          string
	TenantFilter    string
	WorkspaceFilter string
	DisabledOnly    bool
	Cursor          *Cursor
	Limit           int
	Scope           Scope
}

// RuntimeTask is a task of a runtime: its state and times, and whether it failed
// with an error. Its content is never read (spec R6 section 3b.5, rule 3).
type RuntimeTask struct {
	ID          string
	Status      string
	HasError    bool
	CreatedAt   time.Time
	CompletedAt *time.Time
}

// WorkspaceListRow is one workspace of the staff list (spec R6 section 3b.2).
type WorkspaceListRow struct {
	ID          string
	Name        string
	Slug        string
	Status      string
	PlanKey     string
	TenantID    string
	TenantName  string
	OwnerEmail  string
	MemberCount int64
	CreatedAt   time.Time
}

// WorkspaceSearch is one page of the workspace list. Status, PlanKey and
// TenantFilter narrow it when set; Scope limits it to a tenant for an admin.
type WorkspaceSearch struct {
	Pattern      *string
	Status       string
	PlanKey      string
	TenantFilter string
	Cursor       *Cursor
	Limit        int
	Scope        Scope
}

// Reader is the console's read side.
type Reader interface {
	// SearchWorkspaces pages the workspaces, newest first, within q.Scope.
	SearchWorkspaces(ctx context.Context, q WorkspaceSearch) ([]WorkspaceListRow, error)
	// WorkspaceTenant returns the tenant id of a workspace, or "" when it has none.
	WorkspaceTenant(ctx context.Context, workspaceID string) (string, error)
	// TenantPolicy returns the tenant's stored policy floor; found is false for a tenant with no row.
	TenantPolicy(ctx context.Context, tenantID string) (tenantpolicy.Stored, bool, error)
	// TenantHasPolicyFeature reports whether a workspace of the tenant has the workspace_policies feature (spec R6 section 3c.4, rule 4).
	TenantHasPolicyFeature(ctx context.Context, tenantID string) (bool, error)
	// ListQuotaRequests pages the quota requests, newest first, within q.Scope (spec R6 section 3c.5).
	ListQuotaRequests(ctx context.Context, q QuotaSearch) ([]quota.Request, error)
	// GetQuotaRequest returns found == false for an unknown request.
	GetQuotaRequest(ctx context.Context, id string) (quota.Request, bool, error)
	// SearchRuntimes pages the runtimes, newest first, within q.Scope (spec R6 section 3b.5).
	SearchRuntimes(ctx context.Context, q RuntimeSearch) ([]RuntimeRow, error)
	// RuntimeDetail returns found == false for an unknown runtime.
	RuntimeDetail(ctx context.Context, id string) (RuntimeRow, bool, error)
	// RuntimeTasks returns the newest tasks of a runtime and its failed count of the last seven days.
	RuntimeTasks(ctx context.Context, id string) ([]RuntimeTask, int64, error)
	// TenantUsers pages the users of a tenant, by user id (spec R6 section 3b.3).
	TenantUsers(ctx context.Context, q TenantUserSearch) ([]TenantUser, error)
	// TenantUser returns found == false when the user is in no workspace of the tenant.
	TenantUser(ctx context.Context, tenantID, userID string) (TenantUser, bool, error)
	// WorkspaceMember returns the role and email of a member; found is false for a non-member.
	WorkspaceMember(ctx context.Context, workspaceID, userID string) (role, email string, found bool, err error)
	// IsActiveStaff reports whether the user has an active platform_staff row.
	IsActiveStaff(ctx context.Context, userID string) (bool, error)
	// SearchTenants pages the tenants, newest first, within q.Scope.
	SearchTenants(ctx context.Context, q TenantSearch) ([]TenantRow, error)
	// GetTenant returns found == false for an unknown tenant and for one outside
	// scope.
	GetTenant(ctx context.Context, id string, scope Scope) (d TenantDetail, found bool, err error)
	// WorkspaceInScope reports whether the workspace belongs to a tenant of scope.
	WorkspaceInScope(ctx context.Context, workspaceID string, scope Scope) (bool, error)
	// UserInScope reports whether the user is a member of a workspace of scope.
	UserInScope(ctx context.Context, userID string, scope Scope) (bool, error)

	// GetWorkspace returns found == false for an unknown workspace.
	GetWorkspace(ctx context.Context, id string) (d WorkspaceDetail, found bool, err error)
	SearchUsers(ctx context.Context, q UserSearch) ([]UserRow, error)
	GetUser(ctx context.Context, id string) (d UserDetail, found bool, err error)
	ListNotes(ctx context.Context, q NoteQuery) ([]Note, error)
	ListStaff(ctx context.Context) ([]StaffRow, error)
	ListAuditEvents(ctx context.Context, q AuditQuery) ([]AuditRow, error)
	// Overview counts the platform as of now: the workspaces and accounts created
	// in the last week, and the last overviewSeriesDays days of activity.
	Overview(ctx context.Context, now time.Time) (Overview, error)
	// ListOrders returns up to limit orders in status, ordered by creation time,
	// of the workspaces within scope.
	ListOrders(ctx context.Context, status billing.Status, newestFirst bool, limit int, scope Scope) ([]OrderRow, error)
	// WorkspaceUsage returns the usage tab of a workspace for the UTC month of now.
	WorkspaceUsage(ctx context.Context, workspaceID string, now time.Time) (usage.Month, error)
	// WorkspaceAICostRows reads the AI spend rows of a workspace since the given instant.
	WorkspaceAICostRows(ctx context.Context, workspaceID string, since time.Time) ([]aicost.Row, error)
	// WorkspaceLimits returns the effective limit of every gate of a workspace at
	// now: its plan, its overrides and its tenant (spec R6 section 3b.4). A nil
	// limit is no limit.
	WorkspaceLimits(ctx context.Context, workspaceID string, now time.Time) (map[string]*int64, error)
	// AutopilotRuns returns the autopilot runs a workspace used in the period that holds now (spec R6 section 3b.4).
	AutopilotRuns(ctx context.Context, workspaceID string, now time.Time) (int64, error)
	// SeatLimit returns the seats a workspace bought, nil when it has no seat limit (spec R6 section 3b.4).
	SeatLimit(ctx context.Context, workspaceID string) (*int64, error)
	// HeldSeats counts the seat operations that hold a seat at now (spec R6 section 3b.4).
	HeldSeats(ctx context.Context, workspaceID string, now time.Time) (int64, error)
	// Flows returns a workspace's daily issues created and tasks run from since on, oldest first (spec R6 section 3b.4, rule 1).
	Flows(ctx context.Context, workspaceID string, since time.Time) ([]usage.DayFlow, error)
	// GetOrder returns found == false for an unknown order.
	GetOrder(ctx context.Context, id string) (o OrderRow, found bool, err error)
	// RecentOrders returns a workspace's newest orders first, up to limit.
	RecentOrders(ctx context.Context, workspaceID string, limit int) ([]OrderRow, error)
}

// Account is the stored state of an account. found is false for an account
// with no platform row, which is active.
type Account struct {
	Status string
}

// Tx is the writes of one console operation, all inside one transaction with
// its audit row.
type Tx interface {
	// TenantPolicyForUpdate reads the tenant's floor and locks its row; found is false without a row.
	TenantPolicyForUpdate(ctx context.Context, tenantID string) (tenantpolicy.Stored, bool, error)
	// SaveTenantPolicy writes the floor with its version and the actor.
	SaveTenantPolicy(ctx context.Context, tenantID string, s tenantpolicy.Stored, byID string) error
	// SetRuntimeDisabled writes or clears the disabled row of a runtime and reports
	// whether the state changed (spec R6 section 3b.5, rule 4).
	SetRuntimeDisabled(ctx context.Context, runtimeID string, disabled bool, reason, byID string) (bool, error)
	// InsertMemberChange queues a remove or change_role request. A live request
	// for the same workspace, kind and email is returned with inserted == false.
	InsertMemberChange(ctx context.Context, c MemberChange, byID, byLabel string) (id string, inserted bool, err error)
	// RunningExport reports whether the workspace has a queued or running export.
	RunningExport(ctx context.Context, workspaceID string) (bool, error)
	// InsertExport queues an export for the workspace and returns its id.
	InsertExport(ctx context.Context, workspaceID, requestedBy string) (string, error)
	// WorkspaceName returns found == false for an unknown workspace.
	WorkspaceName(ctx context.Context, id string) (name string, found bool, err error)
	InsertNote(ctx context.Context, workspaceID, authorID, body string) (id string, createdAt time.Time, err error)

	// TenantExists reports whether a tenant has this id.
	TenantExists(ctx context.Context, id string) (bool, error)
	// UserIDByEmail takes a lower-cased email.
	UserIDByEmail(ctx context.Context, email string) (id string, found bool, err error)
	// UserEmail returns found == false for an unknown account.
	UserEmail(ctx context.Context, id string) (email string, found bool, err error)
	// Account reads the account row and locks it.
	Account(ctx context.Context, userID string) (a Account, found bool, err error)
	// SetAccountStatus upserts the account row. It leaves sessions_valid_after
	// and deletion_scheduled_at as they are.
	SetAccountStatus(ctx context.Context, userID, status, reason, changedBy string) error
	// SetSessionsValidAfter upserts only the revocation mark.
	SetSessionsValidAfter(ctx context.Context, userID, changedBy string, at time.Time) error

	// DeleteUserMFA deletes the user's user_mfa row, confirmed or pending, and
	// reports whether there was one.
	DeleteUserMFA(ctx context.Context, userID string) (bool, error)

	// ActiveSystemAdmins locks and returns the user ids of the active system
	// administrators.
	ActiveSystemAdmins(ctx context.Context) ([]string, error)
	// ActiveStaffRole returns the role and tenant of the user's active staff
	// row. The tenant is empty unless the role is admin.
	ActiveStaffRole(ctx context.Context, userID string) (role, tenantID string, found bool, err error)
	// ReactivateStaff re-activates the user's latest revoked row with role and
	// tenant (empty for none), and reports whether there was one.
	ReactivateStaff(ctx context.Context, userID, role, tenantID string) (bool, error)
	// InsertStaff adds a row, and reports whether it did.
	InsertStaff(ctx context.Context, userID, role, tenantID, createdBy string) (bool, error)
	// UpdateStaff sets the role and tenant of the user's active row, and
	// reports whether there was one.
	UpdateStaff(ctx context.Context, userID, role, tenantID string) (bool, error)
	// RevokeStaff sets revoked_at on the user's active row.
	RevokeStaff(ctx context.Context, userID string) (bool, error)

	// LockOrder reads the order and locks its row.
	LockOrder(ctx context.Context, id string) (o billing.Order, found bool, err error)
	// CancelOrder moves a pending order to canceled.
	CancelOrder(ctx context.Context, id string) (billing.Order, error)

	audit.Writer
}

// Runner runs fn in one transaction: it commits when fn returns nil and rolls
// back otherwise.
type Runner interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}
