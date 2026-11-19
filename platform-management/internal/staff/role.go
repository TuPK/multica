// Package staff holds the platform's staff roles and the seeding of the first
// administrators.
package staff

// Role is a staff member's role. The database stores it as text; application
// code checks it with ParseRole.
type Role string

const (
	// RoleSystemAdmin may do everything, on every tenant.
	RoleSystemAdmin Role = "system_admin"
	// RoleAdmin works inside one tenant (platform_staff.tenant_id).
	RoleAdmin   Role = "admin"
	RoleSupport Role = "support"
	RoleBilling Role = "billing"
)

// ParseRole returns the role named s, and false for any other text.
func ParseRole(s string) (Role, bool) {
	r := Role(s)
	for _, known := range roleOrder {
		if r == known {
			return r, true
		}
	}
	return "", false
}

// Permits reports whether the role has any access to the action, on all
// objects or only on its own tenant. An unknown action or role is refused.
func (r Role) Permits(action string) bool { return Allowed(r, action) != None }
