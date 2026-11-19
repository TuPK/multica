package tenantstatus

import (
	"slices"
	"testing"
)

// Spec R6 section 3c.9 rule 1: a tenant lock writes every workspace, and an
// unlock restores only the workspaces the tenant lock set.

func TestLockWritesEveryWorkspaceNotAlreadyInThatState(t *testing.T) {
	rows := []WorkspaceStatus{
		{ID: "a", Status: StatusActive},
		{ID: "b", Status: StatusReadOnly, Reason: "billing"},
		{ID: "c", Status: StatusSuspended, Reason: ReasonTenant},
	}
	got := Targets(rows, StatusSuspended)
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("lock targets = %v", got)
	}
}

func TestUnlockRestoresOnlyTenantLockedWorkspaces(t *testing.T) {
	rows := []WorkspaceStatus{
		{ID: "a", Status: StatusSuspended, Reason: ReasonTenant},
		{ID: "b", Status: StatusSuspended, Reason: "fraud review"},
		{ID: "c", Status: StatusActive},
	}
	got := Targets(rows, StatusActive)
	if !slices.Equal(got, []string{"a"}) {
		t.Fatalf("unlock targets = %v, want only the tenant-locked workspace", got)
	}
}

func TestValidStatuses(t *testing.T) {
	for _, ok := range []string{StatusActive, StatusReadOnly, StatusSuspended} {
		if !Valid(ok) {
			t.Errorf("%s must be valid", ok)
		}
	}
	if Valid("pending_deletion") {
		t.Error("pending_deletion is not a tenant status")
	}
}
