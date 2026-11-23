// Package memberrequest holds the staff add-member requests of spec R6 section
// 4: a system_admin, or an admin inside their tenant, asks for a person to be
// added to a workspace without waiting for an invitation to be accepted. This
// package checks the input, stores the request as pending with its audit row,
// and cancels a pending one. Delegora's member_provisioning job then finds or
// creates the user and the member row.
package memberrequest

import (
	"context"
	"errors"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
)

// Kinds of request. Only add is made in this slice; remove and change_role
// are the rows of spec R6 section 3b (QT3).
const (
	KindAdd        = "add"
	KindRemove     = "remove"
	KindChangeRole = "change_role"
)

// States of a request (spec R6 section 4.3).
const (
	StatePending   = "pending"
	StateCompleted = "completed"
	StateFailed    = "failed"
	StateCanceled  = "canceled"
)

// The member roles a request may give (spec R6 section 4.2, rule 2). owner is
// not among them (D63).
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleViewer = "viewer"
)

// Roles lists the roles a request may give.
var Roles = []string{RoleAdmin, RoleMember, RoleViewer}

// Error codes of spec R6 section 4.4.
const (
	CodeInvalid            = "member_request_invalid"
	CodeRoleInvalid        = "member_role_invalid"
	CodeWorkspaceNotActive = "workspace_not_active"
	CodeStaffCannotBeAdded = "staff_cannot_be_added"
	CodeAlreadyMember      = "already_member"
	CodeNotCancelable      = "member_request_not_cancelable"
)

// WorkspaceActive is the platform status a workspace must have.
const WorkspaceActive = "active"

// Input is what staff submit.
type Input struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// Request is one row of platform_member_request.
type Request struct {
	ID               string
	WorkspaceID      string
	Kind             string
	TargetUserID     string
	PreviousRole     string
	Email            string
	Role             string
	State            string
	ErrorCode        string
	Attempts         int
	UserID           string
	MemberID         string
	UserCreated      bool
	RequestedBy      string
	RequestedByLabel string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CompletedAt      *time.Time
}

// Actor is the staff member who acts, as the audit log records them.
type Actor struct {
	ID    string
	Label string
}

// Workspace is what the request path reads of a workspace.
type Workspace struct {
	ID   string
	Name string
	// Status is the platform status: active when the workspace has no row.
	Status string
	// TenantID is the tenant the workspace belongs to, empty without a link.
	TenantID string
}

// ErrNotFound is returned for a workspace or request id that names no row.
var ErrNotFound = errors.New("member request not found")

// Error is a refusal with the HTTP status and code of spec R6 section 4.4.
type Error struct {
	Status  int
	Code    string
	Message string
	// Fields names the input fields of member_request_invalid.
	Fields []string
	// State is the request's state for member_request_not_cancelable, and the
	// workspace's status for workspace_not_active.
	State string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Tx is the writes and checks of one operation, all inside one transaction with
// its audit row.
type Tx interface {
	// Workspace returns found == false for an unknown workspace.
	Workspace(ctx context.Context, id string) (w Workspace, found bool, err error)
	// IsActiveStaff reports whether a lower-cased email belongs to a staff member
	// who is active now.
	IsActiveStaff(ctx context.Context, email string) (bool, error)
	// IsMember reports whether a lower-cased email is a member of the workspace.
	IsMember(ctx context.Context, workspaceID, email string) (bool, error)
	// Live returns the pending request of the workspace, kind and email.
	Live(ctx context.Context, workspaceID, kind, email string) (r Request, found bool, err error)
	// Insert adds a pending add request. inserted is false when a live request
	// already holds the key, which is how the unique index answers a lost race.
	Insert(ctx context.Context, workspaceID, email, role string, by Actor) (r Request, inserted bool, err error)
	// Lock reads the request and locks its row.
	Lock(ctx context.Context, id string) (r Request, found bool, err error)
	// Cancel moves a pending request to canceled.
	Cancel(ctx context.Context, id string) (Request, error)

	audit.Writer
}

// Runner runs fn in one transaction: it commits when fn returns nil and rolls
// back otherwise. It returns fn's error as it is.
type Runner interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

// Reader reads requests.
type Reader interface {
	// List returns up to limit requests of the workspace, newest first.
	List(ctx context.Context, workspaceID string, limit int) ([]Request, error)
	// Get returns found == false for an unknown id.
	Get(ctx context.Context, id string) (r Request, found bool, err error)
	// Workspace returns found == false for an unknown workspace.
	Workspace(ctx context.Context, id string) (w Workspace, found bool, err error)
}

// SeatReleaser frees the seat a request may hold. *seat.Service is one, through
// ReleaseSeat.
type SeatReleaser interface {
	Release(ctx context.Context, workspaceID, token string) error
}
