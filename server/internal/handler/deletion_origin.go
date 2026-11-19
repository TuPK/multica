package handler

import (
	"github.com/multica-ai/multica/platform-management/audit"
)

// DeletionOriginKind says what asked for a workspace deletion. It picks the
// actor and the detail of the workspace.deleted audit row.
type DeletionOriginKind string

const (
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
