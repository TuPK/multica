package staff

import (
	"slices"
	"testing"
)

// matrixRow is one row of spec R6 section 3.6, written by hand. The values are
// the spec's, not read from the code under test.
type matrixRow struct {
	id     string
	action string
	// systemAdmin, admin, support, billing
	sa, admin, support, billing Access
}

func specMatrix() []matrixRow {
	return []matrixRow{
		{"M1", ActionStaffManage, All, None, None, None},
		{"M2", ActionPlansManage, All, None, None, None},
		{"M3", ActionPriceChangeManage, All, None, None, None},
		{"M4", ActionPriceChangeRead, All, None, None, All},
		{"M5", ActionAccountRequests, All, None, None, None},
		{"M6", ActionTenantsList, All, Own, All, All},
		{"M7", ActionTenantsRead, All, Own, All, All},
		{"M8", ActionWorkspacesRead, All, Own, All, All},
		{"M9", ActionNotesWrite, All, Own, All, All},
		{"M10", ActionWorkspacesLock, All, Own, All, None},
		{"M11", ActionWorkspacesDeletion, All, Own, None, None},
		{"M12", ActionSubscriptionsManage, All, None, None, All},
		{"M13", ActionOrdersRead, All, Own, None, All},
		{"M14", ActionOrdersDecide, All, None, None, All},
		{"M15", ActionUsageWriteOff, All, None, None, None},
		{"M16", ActionPaymentsManage, All, None, None, All},
		{"M17", ActionOverviewRead, All, None, All, All},
		{"M18", ActionUsersRead, All, Own, All, All},
		{"M19", ActionAccountsControl, All, None, All, None},
		{"M20", ActionAuditRead, All, Own, All, All},
		{"M21", ActionMembersAdd, All, Own, None, None},
		{"M22", ActionMemberRequests, All, Own, None, None},
		{"M23", ActionWorkspaceDeleteNow, All, Own, None, None},
		{"M24", ActionTenantDeleteNow, All, None, None, None},
		{"M25", ActionAccountDeleteNow, All, None, None, None},
		{"M26", ActionAccountDeleteChoice, All, None, None, None},
		{"M27", ActionDeletionRequestsRead, All, Own, None, None},
		{"M28", ActionDeletionRequestsAct, All, Own, None, None},
		{"M29", ActionConsoleSignIn, All, All, All, All},
		{"M30", ActionPlansRead, All, All, None, None},
		{"M32", ActionPlanHistoryRead, All, None, None, All},
		{"M33", ActionWorkspacesList, All, Own, All, All},
		{"M34", ActionWorkspaceExport, All, Own, None, None},
		{"M35", ActionTenantUsersRead, All, Own, All, All},
		{"M36", ActionMembersRemove, All, Own, None, None},
		{"M37", ActionMembersChangeRole, All, Own, None, None},
		{"M40", ActionTenantLimitOverride, All, None, None, All},
		{"M41", ActionRuntimesRead, All, Own, All, None},
		{"M44", ActionTenantPolicyRead, All, Own, All, None},
		{"M45", ActionTenantPolicyWrite, All, Own, None, None},
		{"M46", ActionQuotaRequestsRead, All, Own, None, All},
		{"M47", ActionQuotaRequestDecide, All, None, None, None},
		{"M48", ActionQuotaRequestCreate, All, Own, None, None},
		{"M51", ActionAuditExport, All, Own, None, None},
		{"M52", ActionTenantStatus, All, Own, All, None},
		{"M50", ActionLegalHoldManage, All, None, None, None},
		{"M42", ActionRuntimeStatus, All, Own, None, None},
		{"M43", ActionRuntimeUnregister, All, Own, None, None},
	}
}

func TestPermissionMatrixMatchesSpec(t *testing.T) {
	rows := specMatrix()
	if len(rows) != 48 {
		t.Fatalf("the hand-written matrix has %d rows, want M1 to M32", len(rows))
	}
	for _, r := range rows {
		for role, want := range map[Role]Access{
			RoleSystemAdmin: r.sa, RoleAdmin: r.admin, RoleSupport: r.support, RoleBilling: r.billing,
		} {
			if got := Allowed(role, r.action); got != want {
				t.Errorf("%s %s: Allowed(%s) = %d, want %d", r.id, r.action, role, got, want)
			}
		}
	}
}

func TestMatrixListsExactlyTheActionsOfTheSpec(t *testing.T) {
	var want []string
	for _, r := range specMatrix() {
		want = append(want, r.action)
	}
	got := slices.Clone(Actions())
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("Actions() = %v, want %v", got, want)
	}
}

func TestSystemAdminHasEveryAction(t *testing.T) {
	if len(Actions()) == 0 {
		t.Fatal("no actions")
	}
	for _, a := range Actions() {
		if got := Allowed(RoleSystemAdmin, a); got != All {
			t.Errorf("system_admin on %s = %d, want All", a, got)
		}
	}
}

func TestAdminHasNoSystemOnlyAction(t *testing.T) {
	systemOnly := map[string]string{
		"M1": ActionStaffManage, "M2": ActionPlansManage, "M3": ActionPriceChangeManage,
		"M4": ActionPriceChangeRead, "M5": ActionAccountRequests, "M12": ActionSubscriptionsManage,
		"M14": ActionOrdersDecide, "M15": ActionUsageWriteOff, "M16": ActionPaymentsManage,
		"M17": ActionOverviewRead, "M19": ActionAccountsControl, "M24": ActionTenantDeleteNow,
		"M25": ActionAccountDeleteNow,
	}
	for id, a := range systemOnly {
		if got := Allowed(RoleAdmin, a); got != None {
			t.Errorf("admin on %s (%s) = %d, want None", id, a, got)
		}
	}
}

// The behaviour of support and billing at HEAD before the matrix: the route role
// lists of the console, subscription and pricechange route tables (spec 5.2).
// billing cannot lock a workspace or an account.
func TestSupportAndBillingAccessUnchangedFromSpec52(t *testing.T) {
	supportAll := []string{
		ActionTenantsList, ActionTenantsRead, ActionWorkspacesRead, ActionNotesWrite,
		ActionWorkspacesLock, ActionOverviewRead, ActionUsersRead, ActionAccountsControl, ActionAuditRead,
		ActionConsoleSignIn, ActionWorkspacesList, ActionTenantUsersRead, ActionRuntimesRead, ActionTenantPolicyRead, ActionTenantStatus,
	}
	billingAll := []string{
		ActionPriceChangeRead, ActionTenantsList, ActionTenantsRead, ActionWorkspacesRead, ActionNotesWrite,
		ActionSubscriptionsManage, ActionOrdersRead, ActionOrdersDecide, ActionPaymentsManage,
		ActionOverviewRead, ActionUsersRead, ActionAuditRead, ActionConsoleSignIn, ActionPlanHistoryRead, ActionWorkspacesList, ActionTenantUsersRead, ActionTenantLimitOverride, ActionQuotaRequestsRead,
	}
	for role, all := range map[Role][]string{RoleSupport: supportAll, RoleBilling: billingAll} {
		for _, a := range Actions() {
			want := None
			if slices.Contains(all, a) {
				want = All
			}
			if got := Allowed(role, a); got != want {
				t.Errorf("%s on %s = %d, want %d", role, a, got, want)
			}
		}
	}
}

func TestRolesWithListsRolesThatReachEveryObject(t *testing.T) {
	cases := map[string][]Role{
		ActionStaffManage:    {RoleSystemAdmin},
		ActionWorkspacesLock: {RoleSystemAdmin, RoleSupport}, // admin has Own only, which waits for scoping
		ActionOrdersRead:     {RoleSystemAdmin, RoleBilling},
		ActionConsoleSignIn:  {RoleSystemAdmin, RoleAdmin, RoleSupport, RoleBilling},
	}
	for action, want := range cases {
		if got := RolesWith(action); !slices.Equal(got, want) {
			t.Errorf("RolesWith(%s) = %v, want %v", action, got, want)
		}
	}
	if got := RolesWith("no.such.action"); len(got) != 0 {
		t.Errorf("RolesWith(unknown) = %v, want none", got)
	}
}

func TestRolesScopedAddsTheRolesWithOwnAccess(t *testing.T) {
	cases := map[string][]Role{
		ActionStaffManage:     {RoleSystemAdmin},
		ActionWorkspacesLock:  {RoleSystemAdmin, RoleAdmin, RoleSupport},
		ActionOrdersRead:      {RoleSystemAdmin, RoleAdmin, RoleBilling},
		ActionTenantsRead:     {RoleSystemAdmin, RoleAdmin, RoleSupport, RoleBilling},
		ActionTenantDeleteNow: {RoleSystemAdmin},
	}
	for action, want := range cases {
		if got := RolesScoped(action); !slices.Equal(got, want) {
			t.Errorf("RolesScoped(%s) = %v, want %v", action, got, want)
		}
	}
	if got := RolesScoped("no.such.action"); len(got) != 0 {
		t.Errorf("RolesScoped(unknown) = %v, want none", got)
	}
}
