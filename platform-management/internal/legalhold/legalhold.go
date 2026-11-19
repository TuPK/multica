// Package legalhold places and releases legal holds (spec R6 section 3c.7, QT11,
// D104). A hold on a tenant covers every workspace of it; a hold on a workspace
// or an account blocks its deletion until a system_admin releases it. The
// deletion paths in internal/deletionrequest read the holds in their transactions.
package legalhold

import (
	"errors"
	"net/http"
	"strings"
)

// Scope kinds of a hold.
const (
	KindTenant    = "tenant"
	KindWorkspace = "workspace"
	KindAccount   = "account"
)

// Error codes of spec R6 section 3c.7.
const (
	CodeExists   = "legal_hold_exists"
	CodeNotFound = "legal_hold_not_found"
	CodeInvalid  = "legal_hold_invalid"
	// CodeBlocked is the answer of a deletion that a hold blocks (every deletion path).
	CodeBlocked = "legal_hold"
)

// ErrNotFound is returned for an unknown or released hold.
var ErrNotFound = errors.New("legalhold: not found")

// Error is a rejection with an HTTP status, a stable code and the fields it names.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Actor is the system_admin who places or releases a hold, for the audit row.
type Actor struct {
	ID    string
	Label string
}

// Hold is one legal hold.
type Hold struct {
	ID         string
	ScopeKind  string
	ScopeID    string
	Reason     string
	CaseRef    string
	PlacedBy   string
	ReleasedBy string
	Released   bool
}

// CheckInput applies the input rules of spec R6 section 3c.7 rule 2: a known scope
// kind, a reason and a case reference, both required.
func CheckInput(kind, reason, caseRef string) (string, string, *Error) {
	if kind != KindTenant && kind != KindWorkspace && kind != KindAccount {
		return "", "", &Error{Status: http.StatusBadRequest, Code: CodeInvalid, Message: "scope_kind must be tenant, workspace or account"}
	}
	reason = strings.TrimSpace(reason)
	caseRef = strings.TrimSpace(caseRef)
	if reason == "" || len([]rune(reason)) > 500 {
		return "", "", &Error{Status: http.StatusBadRequest, Code: CodeInvalid, Message: "reason must be 1 to 500 characters"}
	}
	if caseRef == "" || len([]rune(caseRef)) > 100 {
		return "", "", &Error{Status: http.StatusBadRequest, Code: CodeInvalid, Message: "case_ref must be 1 to 100 characters"}
	}
	return reason, caseRef, nil
}
