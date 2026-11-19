package handler

import (
	"github.com/multica-ai/multica/platform-management/audit"
)

// DeletionOriginKind says what asked for a workspace deletion. It picks the
// actor and the detail of the workspace.deleted audit row.
type DeletionOriginKind string

const (
	// DeletionOriginSelf is the user's own deletion, or the account job that runs it.
	DeletionOriginSelf DeletionOriginKind = "self"
	// DeletionOriginPeriodEnded is the workspace_deletion job, after the grace period.
	DeletionOriginPeriodEnded DeletionOriginKind = "period_ended"
	// DeletionOriginStaff is the staff_deletion job, running a platform request
	// (spec R6 section 6.3, phase 2).
	DeletionOriginStaff DeletionOriginKind = "staff"
)

// DeletionOrigin is the caller of a teardown that is not a user request.
type DeletionOrigin struct {
	Kind DeletionOriginKind
	// RequestID, StaffID and StaffLabel identify the platform request and its
	// staff member. Set for DeletionOriginStaff only.
	RequestID  string
	StaffID    string
	StaffLabel string
}

// workspaceDeletedEntry is the workspace.deleted audit row for the origin. A
// staff deletion leaves WorkspaceID empty and keeps the tenant, so the row
// outlives the workspace and its tenant's admin still reads it (spec R6 section 6.3).
func (o DeletionOrigin) workspaceDeletedEntry(workspaceID, tenantID, name string) audit.Entry {
	if o.Kind == DeletionOriginStaff {
		return audit.Entry{
			TenantID:    tenantID,
			ActorType:   audit.ActorStaff,
			ActorID:     o.StaffID,
			ActorLabel:  o.StaffLabel,
			Action:      audit.ActionWorkspaceDeleted,
			TargetType:  "workspace",
			TargetID:    workspaceID,
			TargetLabel: name,
			Detail: map[string]any{
				"reason":     "staff_immediate_deletion",
				"origin":     string(DeletionOriginStaff),
				"request_id": o.RequestID,
			},
		}
	}
	return audit.Entry{
		WorkspaceID: workspaceID,
		ActorType:   audit.ActorSystem,
		Action:      audit.ActionWorkspaceDeleted,
		TargetType:  "workspace",
		TargetID:    workspaceID,
		TargetLabel: name,
		Detail:      map[string]any{"reason": "deletion_period_ended"},
	}
}

// auditActor is who an audit row names for an account deletion.
type auditActor struct {
	Type   string
	ID     string
	Label  string
	Detail map[string]any
}

// accountActor is the actor of the audit rows an account deletion writes for
// the user's memberships and sole workspaces. A staff deletion names the staff
// member and the origin; the user's own deletion names the deleted user, as before.
func (o DeletionOrigin) accountActor(userID string) auditActor {
	if o.Kind == DeletionOriginStaff {
		return auditActor{
			Type:   audit.ActorStaff,
			ID:     o.StaffID,
			Label:  o.StaffLabel,
			Detail: map[string]any{"reason": "account_deletion", "origin": string(DeletionOriginStaff), "request_id": o.RequestID},
		}
	}
	return auditActor{
		Type:   audit.ActorMember,
		ID:     userID,
		Label:  deletedUserLabel,
		Detail: map[string]any{"reason": "account_deletion"},
	}
}
