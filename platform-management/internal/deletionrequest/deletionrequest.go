// Package deletionrequest is phase 1 of an immediate deletion of a workspace or
// a tenant (spec R6 section 6.3). It checks the input, writes the request with
// its audit rows and the pending_deletion status in one transaction, and cancels
// or retries a request. Multica's staff_deletion job (phase 2) deletes the data
// through the existing deletion path; this package deletes nothing.
package deletionrequest

import (
	"context"
	"errors"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
)

// Kinds of request. Runtime requests come later (spec R6 section 11).
const (
	KindWorkspace = "workspace"
	KindTenant    = "tenant"
	KindAccount   = "account"
)

// States of a request (spec R6 section 6.3 and the platform_deletion_request table).
const (
	StatePending   = "pending"
	StateRunning   = "running"
	StateCompleted = "completed"
	StateFailed    = "failed"
	StateCanceled  = "canceled"
)

// ReasonImmediate is the workspace status reason of an immediate deletion (spec R6 section 6.3, step 4).
const ReasonImmediate = "staff_immediate_deletion"

// ReasonCanceled is the workspace status reason written when a request is canceled.
const ReasonCanceled = "staff_deletion_canceled"

// Error codes of spec R6 section 6.5 and section 7.5.
const (
	CodeReasonRequired       = "reason_required"
	CodeReasonTooLong        = "reason_too_long"
	CodeConfirmationMismatch = "confirmation_mismatch"
	CodeNotCancelable        = "deletion_request_not_cancelable"
	CodeNotRetryable         = "deletion_request_not_retryable"
	CodeUserStaff            = "user_is_staff"
	CodeCannotDeleteSelf     = "cannot_delete_self"
	CodeSoleOwnerUnresolved  = "sole_owner_unresolved"
	CodeSuccessorInvalid     = "successor_invalid"
	CodeResolutionInvalid    = "resolution_invalid"
	// CodeWorkspaceRequestLive refuses an account request whose workspace choice
	// (B) already has a live request of its own.
	CodeWorkspaceRequestLive = "workspace_request_live"
)

// ErrNotFound is returned for an unknown target or request id.
var ErrNotFound = errors.New("deletionrequest: not found")

// Error is a rejection with an HTTP status and a stable code. Extra holds the
// fields a rejection adds to its body, such as workspace_id or workspace_ids.
type Error struct {
	Status  int
	Code    string
	Message string
	State   string
	Extra   map[string]any
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Actor is the staff member who asks. ID is the audit actor, Label the email.
type Actor struct {
	ID    string
	Label string
}

// Request is one deletion request.
type Request struct {
	ID               string
	Kind             string
	TargetID         string
	TargetLabel      string
	Reason           string
	State            string
	ErrorCode        string
	Attempts         int32
	TenantID         string
	RequestedBy      string
	RequestedByLabel string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Input is what staff submit: the reason and the typed confirmation.
type Input struct {
	Reason  string `json:"reason"`
	Confirm string `json:"confirm"`
}

// Workspace is the target row of a workspace request.
type Workspace struct {
	ID       string
	Name     string
	Slug     string
	Status   string
	TenantID string
}

// Tenant is the target row of a tenant request.
type Tenant struct {
	ID     string
	Name   string
	Status string
}

// NewRequest is a request to insert. Resolutions is JSON; nil stores an empty object.
type NewRequest struct {
	Kind        string
	TargetID    string
	TargetLabel string
	Reason      string
	TenantID    string
	ParentID    string
	Resolutions []byte
	By          Actor
}

// Tx is the reads and writes of one transaction. Its status writes go through
// workspacestatus, so a status change and its audit row commit together.
type Tx interface {
	workspacestatus.Tx
	Workspace(ctx context.Context, id string) (Workspace, bool, error)
	Tenant(ctx context.Context, id string) (Tenant, bool, error)
	// LockTenant takes the max_workspaces gate lock of the tenant, so a workspace
	// creation and a tenant deletion queue on it (spec R6 section 5.7).
	LockTenant(ctx context.Context, tenantID string) error
	TenantWorkspaces(ctx context.Context, tenantID string) ([]string, error)
	// AccountUser returns found == false for an unknown user and for a deleted one.
	AccountUser(ctx context.Context, id string) (AccountUser, bool, error)
	// AccountWorkspaces lists the workspaces the user belongs to (spec R6 section 7.3).
	AccountWorkspaces(ctx context.Context, userID string) ([]AccountWorkspace, error)
	// WorkspaceMember returns the role and account status of a member; found is
	// false when the user is not a member.
	WorkspaceMember(ctx context.Context, workspaceID, userID string) (role, status string, found bool, err error)
	// ScheduleAccount puts the account in pending_deletion now with its sessions
	// revoked as of at (spec R6 section 7.4, phase 1).
	ScheduleAccount(ctx context.Context, userID, reason string, by Actor, at time.Time) error
	Live(ctx context.Context, kind, targetID string) (Request, bool, error)
	Lock(ctx context.Context, id string) (Request, bool, error)
	Insert(ctx context.Context, n NewRequest) (Request, bool, error)
	MarkCanceled(ctx context.Context, id string) (Request, error)
	Retry(ctx context.Context, id string) (Request, error)
}

// Runner runs fn in one transaction: it commits when fn returns nil.
type Runner interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

// Reader reads requests outside a transaction.
type Reader interface {
	List(ctx context.Context, f Filter) ([]Request, error)
	Get(ctx context.Context, id string) (Request, bool, error)
}

// Filter narrows a list. TenantID, when set, limits it to one tenant's requests.
type Filter struct {
	TenantID string
	Kind     string
	State    string
	Limit    int
}

// Invalidator drops the cached state of a workspace. *workspacestate.Guard is one.
type Invalidator interface {
	Invalidate(ctx context.Context, workspaceID string) error
}

// AccountNotifier drops the cached state of an account (Invalidate) and publishes
// a change of kind to the Multica server (Publish), as the console does after a
// status change. Both run after the commit.
type AccountNotifier interface {
	Invalidate(ctx context.Context, userID string) error
	Publish(ctx context.Context, userID, kind string) error
}
