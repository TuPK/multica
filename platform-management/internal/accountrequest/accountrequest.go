// Package accountrequest holds the account requests of spec 7b.5: an admin asks
// for an account to be created for a person. This package checks the input,
// stores the request as pending, counts the staff member's requests and cancels
// a pending one. Multica's job creates the user and workspace, and the
// platform's later job finishes the subscription; neither is here.
package accountrequest

import (
	"context"
	"errors"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
)

// Kinds of account (spec 7b.1, rule 1).
const (
	KindIndividual = "individual"
	KindBusiness   = "business"
)

// States of a request (spec 7b.5, rule 7).
const (
	StatePending     = "pending"
	StateProvisioned = "provisioned"
	StateCompleted   = "completed"
	StateFailed      = "failed"
	StateCanceled    = "canceled"
)

// Billing intervals.
const (
	IntervalMonth = "month"
	IntervalYear  = "year"
)

// Error codes of spec 7b.5, plus the audience code of 7b.1.
const (
	CodeInvalid          = "account_request_invalid"
	CodeActivationNeeded = "account_activation_required"
	CodeEmailExists      = "account_email_exists"
	CodeNotCancelable    = "account_request_not_cancelable"
	CodeRateLimited      = "account_create_rate_limited"
	CodeAudienceMismatch = "plan_audience_mismatch"
)

// BillingProfile is the buyer's identity a business account needs (spec 7b.1,
// rule 3). The request keeps it until the platform has written the workspace's
// billing profile.
type BillingProfile struct {
	LegalName    string `json:"legal_name"`
	TaxCode      string `json:"tax_code"`
	Address      string `json:"address"`
	InvoiceEmail string `json:"invoice_email"`
}

// Input is what staff submit (spec 7b.5, rule 3).
type Input struct {
	Kind           string          `json:"kind"`
	Email          string          `json:"email"`
	DisplayName    string          `json:"display_name"`
	WorkspaceName  string          `json:"workspace_name"`
	WorkspaceSlug  string          `json:"workspace_slug"`
	PlanKey        string          `json:"plan_key"`
	Seats          int             `json:"seats"`
	Interval       string          `json:"interval"`
	Trial          bool            `json:"trial"`
	CompReason     string          `json:"comp_reason"`
	BillingProfile *BillingProfile `json:"billing_profile"`
}

// Request is one row of platform_account_request. UserID and WorkspaceID are
// empty until Multica's job has created them.
type Request struct {
	ID              string
	Kind            string
	Email           string
	DisplayName     string
	WorkspaceName   string
	WorkspaceSlug   string
	PlanKey         string
	Seats           int
	Interval        string
	Trial           bool
	CompReason      string
	BillingProfile  *BillingProfile
	State           string
	ErrorCode       string
	Attempts        int
	UserID          string
	WorkspaceID     string
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     *time.Time
	WelcomeSentAt   *time.Time
	WelcomeAttempts int
}

// Actor is the staff member who acts, as the audit log records them.
type Actor struct {
	ID    string
	Label string
}

// ListQuery asks for the newest requests, optionally in one state.
type ListQuery struct {
	State string
	Limit int
}

// ErrNotFound is returned for a request id that names no row.
var ErrNotFound = errors.New("account request not found")

// Error is a refusal with the HTTP status and code of spec 7b.5. Fields names
// the input fields of account_request_invalid, State the request's state for
// account_request_not_cancelable, and RetryAfter the wait for a 429.
type Error struct {
	Status     int
	Code       string
	Message    string
	Fields     []string
	State      string
	RetryAfter time.Duration
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func invalid(msg string, fields ...string) *Error {
	return &Error{Status: 400, Code: CodeInvalid, Message: msg, Fields: fields}
}

// Tx is the writes of one operation, all inside one transaction with its audit
// row.
type Tx interface {
	// UserIDByEmail takes a lower-cased email and ignores the case of the stored
	// one. found is false when no user has it.
	UserIDByEmail(ctx context.Context, email string) (id string, found bool, err error)
	// LiveByEmail returns the pending or provisioned request for the email.
	LiveByEmail(ctx context.Context, email string) (r Request, found bool, err error)
	// Insert adds a pending request. inserted is false when a live request holds
	// the email, which is how the unique index answers a lost race.
	Insert(ctx context.Context, in Input, createdBy string) (r Request, inserted bool, err error)
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
	// List returns up to q.Limit requests, newest first.
	List(ctx context.Context, q ListQuery) ([]Request, error)
	// Get returns found == false for an unknown id.
	Get(ctx context.Context, id string) (r Request, found bool, err error)
}
