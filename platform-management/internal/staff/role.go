// Package staff holds the platform's staff roles and the seeding of the first
// administrators.
package staff

// Role is a staff member's role. The database stores it as text; application
// code checks it with ParseRole.
type Role string

const (
	RoleSupport Role = "support"
	RoleBilling Role = "billing"
	RoleAdmin   Role = "admin"
)

// rank orders the roles: each role has every permission of the roles below it.
var rank = map[Role]int{RoleSupport: 1, RoleBilling: 2, RoleAdmin: 3}

// routeRole names the lowest role that may use each route group.
var routeRole = map[string]Role{
	// Reads.
	"tenants.read":  RoleSupport,
	"users.read":    RoleSupport,
	"audit.read":    RoleSupport,
	"notes.read":    RoleSupport,
	"overview.read": RoleSupport,

	// Support actions.
	"accounts.lock":      RoleSupport,
	"workspaces.suspend": RoleSupport,
	"sessions.revoke":    RoleSupport,
	"notes.write":        RoleSupport,

	// Billing actions.
	"orders.read":          RoleBilling,
	"orders.confirm":       RoleBilling,
	"orders.cancel":        RoleBilling,
	"subscriptions.change": RoleBilling,
	"subscriptions.renew":  RoleBilling,

	// Administration.
	"staff.manage":                 RoleAdmin,
	"plans.manage":                 RoleAdmin,
	"workspaces.schedule_deletion": RoleAdmin,
	"workspaces.cancel_deletion":   RoleAdmin,
}

// ParseRole returns the role named s, and false for any other text.
func ParseRole(s string) (Role, bool) {
	r := Role(s)
	_, ok := rank[r]
	return r, ok
}

// Permits reports whether the role may use the named route group. An unknown
// route or an unknown role is refused.
func (r Role) Permits(route string) bool {
	have, ok := rank[r]
	if !ok {
		return false
	}
	need, ok := routeRole[route]
	return ok && have >= rank[need]
}
