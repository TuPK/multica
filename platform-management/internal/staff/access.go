package staff

// Access is how far a staff role may use an action.
type Access int

const (
	// None: the route does not exist for the role (a flat 404).
	None Access = iota
	// Own: only on objects inside the staff member's own tenant.
	Own
	// All: on every object of the platform.
	All
)

// The actions of the matrix in spec R6 section 3.6, one per row M1 to M29.
const (
	ActionStaffManage          = "staff.manage"             // M1
	ActionPlansManage          = "plans.manage"             // M2
	ActionPriceChangeManage    = "plans.price_change"       // M3
	ActionPriceChangeRead      = "plans.price_change.read"  // M4
	ActionAccountRequests      = "accounts.request"         // M5
	ActionTenantsList          = "tenants.list"             // M6
	ActionTenantsRead          = "tenants.read"             // M7
	ActionWorkspacesRead       = "workspaces.read"          // M8
	ActionNotesWrite           = "notes.write"              // M9
	ActionWorkspacesLock       = "workspaces.lock"          // M10
	ActionWorkspacesDeletion   = "workspaces.deletion"      // M11
	ActionSubscriptionsManage  = "subscriptions.manage"     // M12
	ActionOrdersRead           = "orders.read"              // M13
	ActionOrdersDecide         = "orders.decide"            // M14
	ActionUsageWriteOff        = "usage.write_off"          // M15
	ActionPaymentsManage       = "payments.manage"          // M16
	ActionOverviewRead         = "overview.read"            // M17
	ActionUsersRead            = "users.read"               // M18
	ActionAccountsControl      = "accounts.control"         // M19
	ActionAuditRead            = "audit.read"               // M20
	ActionMembersAdd           = "members.add"              // M21
	ActionMemberRequests       = "member_requests.manage"   // M22
	ActionWorkspaceDeleteNow   = "workspaces.delete_now"    // M23
	ActionTenantDeleteNow      = "tenants.delete_now"       // M24
	ActionAccountDeleteNow     = "accounts.delete_now"      // M25
	ActionAccountDeleteChoice  = "accounts.delete_choice"   // M26
	ActionDeletionRequestsRead = "deletion_requests.read"   // M27
	ActionDeletionRequestsAct  = "deletion_requests.manage" // M28
	ActionConsoleSignIn        = "console.sign_in"          // M29
	ActionPlansRead            = "plans.read"               // M30
	ActionPlanHistoryRead      = "plans.history"            // M32
	ActionWorkspacesList       = "workspaces.list"          // M33
	ActionWorkspaceExport      = "workspaces.export"        // M34
	ActionTenantUsersRead      = "tenant_users.read"        // M35
	ActionMembersRemove        = "members.remove"           // M36
	ActionMembersChangeRole    = "members.change_role"      // M37
	ActionTenantLimitOverride  = "tenants.limit_override"   // M40
	ActionRuntimesRead         = "runtimes.read"            // M41
	ActionRuntimeStatus        = "runtimes.status"          // M42
	ActionRuntimeUnregister    = "runtimes.unregister"      // M43
	ActionTenantPolicyRead     = "tenant_policy.read"       // M44
	ActionTenantPolicyWrite    = "tenant_policy.write"      // M45
)

// roleOrder is the order of RolesWith and of the columns of matrix.
var roleOrder = []Role{RoleSystemAdmin, RoleAdmin, RoleSupport, RoleBilling}

// matrix is spec R6 section 3.6: what each role may do, one row per action, in
// the order system_admin, admin, support, billing. It replaces the additive
// rank of the first spec: a role has exactly the access written here.
var matrix = map[string][4]Access{
	ActionStaffManage:          {All, None, None, None},
	ActionPlansManage:          {All, None, None, None},
	ActionPlansRead:            {All, All, None, None},
	ActionPlanHistoryRead:      {All, None, None, All},
	ActionWorkspacesList:       {All, Own, All, All},
	ActionWorkspaceExport:      {All, Own, None, None},
	ActionTenantUsersRead:      {All, Own, All, All},
	ActionMembersRemove:        {All, Own, None, None},
	ActionMembersChangeRole:    {All, Own, None, None},
	ActionTenantLimitOverride:  {All, None, None, All},
	ActionRuntimesRead:         {All, Own, All, None},
	ActionRuntimeStatus:        {All, Own, None, None},
	ActionRuntimeUnregister:    {All, Own, None, None},
	ActionTenantPolicyRead:     {All, Own, All, None},
	ActionTenantPolicyWrite:    {All, Own, None, None},
	ActionPriceChangeManage:    {All, None, None, None},
	ActionPriceChangeRead:      {All, None, None, All},
	ActionAccountRequests:      {All, None, None, None},
	ActionTenantsList:          {All, Own, All, All},
	ActionTenantsRead:          {All, Own, All, All},
	ActionWorkspacesRead:       {All, Own, All, All},
	ActionNotesWrite:           {All, Own, All, All},
	ActionWorkspacesLock:       {All, Own, All, None},
	ActionWorkspacesDeletion:   {All, Own, None, None},
	ActionSubscriptionsManage:  {All, None, None, All},
	ActionOrdersRead:           {All, Own, None, All},
	ActionOrdersDecide:         {All, None, None, All},
	ActionUsageWriteOff:        {All, None, None, None},
	ActionPaymentsManage:       {All, None, None, All},
	ActionOverviewRead:         {All, None, All, All},
	ActionUsersRead:            {All, Own, All, All},
	ActionAccountsControl:      {All, None, All, None},
	ActionAuditRead:            {All, Own, All, All},
	ActionMembersAdd:           {All, Own, None, None},
	ActionMemberRequests:       {All, Own, None, None},
	ActionWorkspaceDeleteNow:   {All, Own, None, None},
	ActionTenantDeleteNow:      {All, None, None, None},
	ActionAccountDeleteNow:     {All, None, None, None},
	ActionAccountDeleteChoice:  {All, None, None, None},
	ActionDeletionRequestsRead: {All, Own, None, None},
	ActionDeletionRequestsAct:  {All, Own, None, None},
	ActionConsoleSignIn:        {All, All, All, All},
}

// Actions returns every action of the matrix.
func Actions() []string {
	out := make([]string, 0, len(matrix))
	for a := range matrix {
		out = append(out, a)
	}
	return out
}

// Allowed returns how far role may use action. An unknown role or action is
// None.
func Allowed(role Role, action string) Access {
	row, ok := matrix[action]
	if !ok {
		return None
	}
	for i, r := range roleOrder {
		if r == role {
			return row[i]
		}
	}
	return None
}

// RolesWith returns the roles that may use action on every object: the role
// list of a route whose handler does not scope its reads and writes. A role
// with Own access is left out.
func RolesWith(action string) []Role {
	var out []Role
	for _, r := range roleOrder {
		if Allowed(r, action) == All {
			out = append(out, r)
		}
	}
	return out
}

// RolesScoped returns the roles that may use action on every object or, with
// Own access, on the objects of their own tenant. A route that applies the
// staff member's Scope lists these roles.
func RolesScoped(action string) []Role {
	var out []Role
	for _, r := range roleOrder {
		if Allowed(r, action) != None {
			out = append(out, r)
		}
	}
	return out
}
