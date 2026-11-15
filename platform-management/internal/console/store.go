package console

import (
	"context"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
)

// TenantRow is one workspace in a list.
type TenantRow struct {
	ID             string
	Name           string
	Slug           string
	Status         string
	MemberCount    int64
	CreatedAt      time.Time
	LastActivityAt time.Time
}

// TenantDetail is a workspace with its state record, notes count and owner.
// The state fields are zero when the workspace has no platform row.
type TenantDetail struct {
	TenantRow
	StateReason         string
	StateChangedBy      string
	StateChangedAt      *time.Time
	DeletionScheduledAt *time.Time
	NotesCount          int64
	OwnerID             string
	OwnerEmail          string
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
	// NonActive counts the workspaces in each state other than active.
	NonActive map[string]int64
	Signups   int64
}

// TenantSearch asks for a page of workspaces. Pattern is an escaped ILIKE
// pattern, nil for no filter. Readers return up to Limit+1 rows so the caller
// can tell whether another page exists.
type TenantSearch struct {
	Pattern *string
	Cursor  *Cursor
	Limit   int
}

// UserSearch is the same for accounts.
type UserSearch = TenantSearch

// NoteQuery asks for a page of one workspace's notes.
type NoteQuery struct {
	WorkspaceID string
	Cursor      *Cursor
	Limit       int
}

// AuditQuery asks for a page of audit rows. An empty WorkspaceID asks for every
// row.
type AuditQuery struct {
	WorkspaceID string
	Action      *string
	ActorID     *string
	From        *time.Time
	To          *time.Time
	Cursor      *Cursor
	Limit       int
}

// Reader is the console's read side.
type Reader interface {
	SearchTenants(ctx context.Context, q TenantSearch) ([]TenantRow, error)
	// GetTenant returns found == false for an unknown workspace.
	GetTenant(ctx context.Context, id string) (d TenantDetail, found bool, err error)
	SearchUsers(ctx context.Context, q UserSearch) ([]UserRow, error)
	GetUser(ctx context.Context, id string) (d UserDetail, found bool, err error)
	ListNotes(ctx context.Context, q NoteQuery) ([]Note, error)
	ListStaff(ctx context.Context) ([]StaffRow, error)
	ListAuditEvents(ctx context.Context, q AuditQuery) ([]AuditRow, error)
	// Overview counts the workspaces and the accounts created since signupsSince.
	Overview(ctx context.Context, signupsSince time.Time) (Overview, error)
}

// Account is the stored state of an account. found is false for an account
// with no platform row, which is active.
type Account struct {
	Status string
}

// Tx is the writes of one console operation, all inside one transaction with
// its audit row.
type Tx interface {
	// WorkspaceName returns found == false for an unknown workspace.
	WorkspaceName(ctx context.Context, id string) (name string, found bool, err error)
	InsertNote(ctx context.Context, workspaceID, authorID, body string) (id string, createdAt time.Time, err error)

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

	// ActiveAdmins locks and returns the user ids of the active administrators.
	ActiveAdmins(ctx context.Context) ([]string, error)
	// ActiveStaffRole returns the role of the user's active staff row.
	ActiveStaffRole(ctx context.Context, userID string) (role string, found bool, err error)
	// ReactivateStaff re-activates the user's latest revoked row with role, and
	// reports whether there was one.
	ReactivateStaff(ctx context.Context, userID, role string) (bool, error)
	// InsertStaff adds a row, and reports whether it did.
	InsertStaff(ctx context.Context, userID, role, createdBy string) (bool, error)
	// RevokeStaff sets revoked_at on the user's active row.
	RevokeStaff(ctx context.Context, userID string) (bool, error)

	audit.Writer
}

// Runner runs fn in one transaction: it commits when fn returns nil and rolls
// back otherwise.
type Runner interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}
