package console

import (
	"context"
	"net/http"
	"strings"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/platformauth"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// staffTarget is the role and tenant of a staff row: an admin has exactly one
// tenant, every other role has none (spec R6 section 3.7).
type staffTarget struct {
	Role     staff.Role
	TenantID string
}

// validateStaffTarget checks the role and the tenant of a request body together.
// The tenant is returned in its canonical text form, and empty for none. That the
// tenant exists is checked in the transaction, by tenantMustExist.
func validateStaffTarget(roleText, tenantText string) (staffTarget, *apiError) {
	role, perr := parseStaffRole(roleText)
	if perr != nil {
		return staffTarget{}, perr
	}
	tenantText = strings.TrimSpace(tenantText)
	if role != staff.RoleAdmin {
		if tenantText != "" {
			return staffTarget{}, badRequest("tenant_not_allowed", "only the admin role takes a tenant_id")
		}
		return staffTarget{Role: role}, nil
	}
	if tenantText == "" {
		return staffTarget{}, badRequest("tenant_required", "the admin role needs a tenant_id")
	}
	tenant, ok := canonicalUUID(tenantText)
	if !ok {
		return staffTarget{}, badRequest("tenant_invalid", "tenant_id is not a valid tenant id")
	}
	return staffTarget{Role: role, TenantID: tenant}, nil
}

// requireStaffManager answers the flat 404 of an unknown route, before anything
// is read or written, unless the signed-in role has full access to the action.
// The route table already admits only those roles; this keeps the handlers safe
// when they are reached another way (spec R6 section 3.5, rule 4).
func (h *Handler) requireStaffManager(w http.ResponseWriter, r *http.Request) bool {
	id, ok := platformauth.IdentityFrom(r.Context())
	if !ok || staff.Allowed(id.Role, staff.ActionStaffManage) != staff.All {
		http.NotFound(w, r)
		return false
	}
	return true
}

// tenantMustExist answers 400 tenant_invalid for a tenant that is not in the
// tenant table (spec R6 section 3.7, rule 1). An empty tenant needs no check.
func tenantMustExist(ctx context.Context, tx Tx, tenantID string) error {
	if tenantID == "" {
		return nil
	}
	ok, err := tx.TenantExists(ctx, tenantID)
	if err != nil {
		return err
	}
	if !ok {
		return badRequest("tenant_invalid", "tenant_id is not a valid tenant id")
	}
	return nil
}

func tenantOrNull(tenant string) any {
	if tenant == "" {
		return nil
	}
	return tenant
}

type updateStaffBody struct {
	Role     string `json:"role"`
	TenantID string `json:"tenant_id"`
}

// updateStaff changes the role and tenant of an active staff row. The change and
// its audit rows commit together.
func (h *Handler) updateStaff(w http.ResponseWriter, r *http.Request) {
	if !h.requireStaffManager(w, r) {
		return
	}
	id, perr := pathID(r, "user_id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	var body updateStaffBody
	if perr := decodeBody(w, r, &body); perr != nil {
		writeError(w, perr)
		return
	}
	target, perr := validateStaffTarget(body.Role, body.TenantID)
	if perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}

	err = h.Runner.InTx(r.Context(), func(tx Tx) error {
		if err := tenantMustExist(r.Context(), tx, target.TenantID); err != nil {
			return err
		}
		// Locking the system administrators first makes two demotions queue up,
		// so each one counts the system administrators that remain.
		admins, err := tx.ActiveSystemAdmins(r.Context())
		if err != nil {
			return err
		}
		oldRole, oldTenant, found, err := tx.ActiveStaffRole(r.Context(), id)
		if err != nil {
			return err
		}
		if !found {
			return notFound()
		}
		if oldRole == string(staff.RoleSystemAdmin) && target.Role != staff.RoleSystemAdmin && !otherThan(admins, id) {
			return lastSystemAdmin()
		}
		if oldRole == string(target.Role) && oldTenant == target.TenantID {
			return nil
		}
		email, _, err := tx.UserEmail(r.Context(), id)
		if err != nil {
			return err
		}
		updated, err := tx.UpdateStaff(r.Context(), id, string(target.Role), target.TenantID)
		if err != nil {
			return err
		}
		if !updated {
			return notFound()
		}
		// The rows are about the tenant the change touches: the new one, or the
		// one the staff member leaves.
		rowTenant := target.TenantID
		if rowTenant == "" {
			rowTenant = oldTenant
		}
		entry := func(action string) audit.Entry {
			return a.staffEntry(audit.Entry{
				Action:      action,
				TenantID:    rowTenant,
				TargetType:  targetUser,
				TargetID:    id,
				TargetLabel: email,
				Detail: map[string]any{
					"old_role": oldRole, "new_role": string(target.Role),
					"old_tenant_id": oldTenant, "new_tenant_id": target.TenantID,
				},
			})
		}
		if oldRole != string(target.Role) {
			if err := audit.Record(r.Context(), tx, entry(audit.ActionStaffRoleChanged)); err != nil {
				return err
			}
		}
		if target.TenantID != "" && target.TenantID != oldTenant {
			return audit.Record(r.Context(), tx, entry(audit.ActionStaffTenantAssigned))
		}
		return nil
	})
	if err != nil {
		h.fail(w, r, "update staff", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": id, "role": string(target.Role), "tenant_id": tenantOrNull(target.TenantID)})
}

// otherThan reports whether ids holds an id other than id.
func otherThan(ids []string, id string) bool {
	for _, other := range ids {
		if other != id {
			return true
		}
	}
	return false
}

func lastSystemAdmin() *apiError {
	return badRequest("last_system_admin", "the last active system administrator cannot be demoted or removed")
}
