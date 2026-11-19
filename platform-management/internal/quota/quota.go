// Package quota holds the quota requests of spec R6 section 3c.5 (QT7): a
// request to raise one plan gate of a workspace, approved or denied by a
// system_admin. An approval writes the request and the workspace's override in
// one transaction (D102). Alerts at 80 and 100 percent are a separate job.
package quota

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// States of a quota request.
const (
	StatePending  = "pending"
	StateApproved = "approved"
	StateDenied   = "denied"
)

// Error codes of spec R6 section 3c.5.
const (
	CodeInvalid    = "quota_request_invalid"
	CodePending    = "quota_request_pending"
	CodeNotPending = "quota_request_not_pending"
	maxReasonRunes = 500
)

// ErrNotFound is returned for an unknown request id.
var ErrNotFound = errors.New("quota: not found")

// Error is a rejection with an HTTP status, a stable code and the fields it names.
type Error struct {
	Status  int
	Code    string
	Message string
	Extra   map[string]any
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Actor is the staff member who decides or asks, for the audit row.
type Actor struct {
	ID    string
	Label string
}

// Request is one quota request.
type Request struct {
	ID               string
	WorkspaceID      string
	TenantID         string
	Gate             string
	CurrentValue     *int64
	RequestedValue   int64
	ApprovedValue    *int64
	State            string
	Reason           string
	DecisionReason   string
	RequestedBy      string
	RequestedByLabel string
	DecidedBy        string
	CreatedAt        time.Time
	DecidedAt        *time.Time
}

// checkGate reports a gate the plans limit.
func checkGate(gate string) *Error {
	if !slices.Contains(plan.LimitGates, gate) {
		return &Error{Status: http.StatusBadRequest, Code: CodeInvalid, Message: "gate is not a plan limit", Extra: map[string]any{"fields": []string{"gate"}}}
	}
	return nil
}

// cleanReason trims a reason and checks it: 1 to 500 characters.
func cleanReason(reason string) (string, *Error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > maxReasonRunes {
		return "", &Error{Status: http.StatusBadRequest, Code: CodeInvalid, Message: "reason must be 1 to 500 characters", Extra: map[string]any{"fields": []string{"reason"}}}
	}
	return reason, nil
}

// checkRequested applies rule 2: the requested value must be above the effective
// value. An unlimited gate has nothing to raise.
func checkRequested(gate string, current int64, unlimited bool, requested int64) *Error {
	if unlimited {
		return &Error{Status: http.StatusBadRequest, Code: CodeInvalid, Message: "the gate has no limit to raise", Extra: map[string]any{"fields": []string{"gate"}}}
	}
	if requested <= current {
		return &Error{Status: http.StatusBadRequest, Code: CodeInvalid, Message: "requested_value must be above the current value", Extra: map[string]any{"fields": []string{"requested_value"}}}
	}
	return nil
}

// checkApproved applies rule 3: an approval may be lower than the request but
// not higher, and not at or below the current value.
func checkApproved(current, requested, approved int64) *Error {
	if approved > requested || approved <= current {
		return &Error{Status: http.StatusBadRequest, Code: CodeInvalid, Message: "approved_value must be above the current value and not above the requested value", Extra: map[string]any{"fields": []string{"value"}}}
	}
	return nil
}
