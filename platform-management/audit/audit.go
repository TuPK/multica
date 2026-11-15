// Package audit defines the audit-event entry, the secret redaction filter and
// the Record writer. It depends on the standard library only; the server
// supplies the database through the Writer interface.
package audit

import (
	"context"
	"errors"
	"strings"
)

// R1 action catalog (spec 4.3).
const (
	ActionMemberRoleChanged = "member.role_changed"
	ActionMemberRemoved     = "member.removed"
	ActionMemberLeft        = "member.left"

	ActionInvitationCreated  = "invitation.created"
	ActionInvitationRevoked  = "invitation.revoked"
	ActionInvitationAccepted = "invitation.accepted"
	ActionInvitationDeclined = "invitation.declined"

	ActionShareLinkCreated = "share_link.created"
	ActionShareLinkRevoked = "share_link.revoked"
	ActionShareLinkJoined  = "share_link.joined"

	ActionWorkspaceCreated = "workspace.created"
	ActionWorkspaceUpdated = "workspace.updated"
	ActionWorkspaceDeleted = "workspace.deleted"

	ActionAuthLogin               = "auth.login"
	ActionAuthSessionsRevoked     = "auth.sessions_revoked"
	ActionPATCreated              = "pat.created"
	ActionPATRevoked              = "pat.revoked"
	ActionAccountExportRequested  = "account.export_requested"
	ActionAccountDeletionRequest  = "account.deletion_requested"
	ActionAccountDeletionCanceled = "account.deletion_canceled"
	ActionConsentAccepted         = "consent.accepted"
)

// Console actions: written by the platform's staff API, with the staff actor.
const (
	ActionWorkspaceStatusChanged = "workspace.status_changed"
	ActionAccountStatusChanged   = "account.status_changed"
	ActionAccountSessionsRevoked = "account.sessions_revoked"
	ActionStaffAdded             = "staff.added"
	ActionStaffRemoved           = "staff.removed"
	ActionNoteAdded              = "note.added"
)

// Actor types.
const (
	ActorMember = "member"
	ActorAgent  = "agent"
	ActorSystem = "system"
	ActorStaff  = "staff"
)

// Redacted replaces the value of a sensitive detail key.
const Redacted = "[redacted]"

// Entry mirrors the audit_event columns. IDs are plain strings. An empty
// WorkspaceID or ActorID is stored as NULL.
type Entry struct {
	ID          string
	WorkspaceID string
	ActorType   string
	ActorID     string
	ActorLabel  string
	Action      string
	TargetType  string
	TargetID    string
	TargetLabel string
	Detail      map[string]any
	IP          string
	UserAgent   string
	RequestID   string
}

// Writer persists one audit entry.
type Writer interface {
	InsertAuditEvent(ctx context.Context, e Entry) error
}

var sensitiveKeyParts = []string{"secret", "token", "password", "key", "env"}

func sensitive(key string) bool {
	k := strings.ToLower(key)
	for _, p := range sensitiveKeyParts {
		if strings.Contains(k, p) {
			return true
		}
	}
	return false
}

// Redact returns a copy of detail with the value of every sensitive key
// replaced by Redacted, at any depth. The input is not modified.
func Redact(detail map[string]any) map[string]any {
	if detail == nil {
		return nil
	}
	out := make(map[string]any, len(detail))
	for k, v := range detail {
		if sensitive(k) {
			out[k] = Redacted
			continue
		}
		out[k] = redactValue(v)
	}
	return out
}

func redactValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return Redact(t)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = redactValue(e)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(t))
		for i, e := range t {
			out[i] = Redact(e)
		}
		return out
	default:
		return v
	}
}

// Record validates e, redacts its detail and writes it. Write errors are
// returned so the caller can fail the surrounding transaction.
func Record(ctx context.Context, w Writer, e Entry) error {
	if e.Action == "" {
		return errors.New("audit: action is required")
	}
	if e.ActorType == "" {
		return errors.New("audit: actor type is required")
	}
	e.Detail = Redact(e.Detail)
	return w.InsertAuditEvent(ctx, e)
}
