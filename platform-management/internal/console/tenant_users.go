package console

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// Users of a tenant and member removal or role change through staff requests
// (spec R6 section 3b.3, QT3). A user belongs to a tenant when it is a member
// of one of the tenant's workspaces (D92). Removal and role change are requests
// on platform_member_request; Multica's member_provisioning job runs them (D94).

// MemberChange is a remove or change_role request to insert.
type MemberChange struct {
	WorkspaceID  string
	Kind         string
	TargetUserID string
	Email        string
	PreviousRole string
	Role         string
}

// Member kinds of platform_member_request (spec R6 section 3b.3, rule 4).
const (
	memberKindRemove     = "remove"
	memberKindChangeRole = "change_role"
)

// validMemberRoles are the roles a change_role request may give (spec rule 5).
var validMemberRoles = []string{"admin", "member", "viewer"}

// TenantUser is a user of a tenant with its memberships in the tenant's workspaces.
type TenantUser struct {
	ID            string
	Email         string
	Name          string
	AccountStatus string
	IsStaff       bool
	LastLoginAt   *time.Time
	Workspaces    []TenantUserWorkspace
}

// TenantUserWorkspace is one membership of a tenant user.
type TenantUserWorkspace struct {
	ID   string
	Name string
	Role string
}

// TenantUserSearch is one page of a tenant's users, keyset-paged by user id.
type TenantUserSearch struct {
	TenantID     string
	Pattern      string
	CursorUserID string
	Limit        int
	ViewerAll    bool
}

// checkRoleChange applies the rules of spec R6 section 3b.3, rule 5 to a change
// of a member's role: owner is immutable on either side, and an admin may not
// change its own role. The role must be one of validMemberRoles.
func checkRoleChange(viewerRole staff.Role, viewerID, targetID, current, next string) *apiError {
	if current == "owner" || next == "owner" {
		return &apiError{Status: http.StatusConflict, Code: "owner_role_immutable", Message: "the owner role cannot be changed by staff"}
	}
	if !containsString(validMemberRoles, next) {
		return badRequest("member_role_invalid", "role must be admin, member or viewer")
	}
	if viewerRole == staff.RoleAdmin && viewerID == targetID {
		return &apiError{Status: http.StatusConflict, Code: "cannot_change_own_role", Message: "an admin cannot change its own role"}
	}
	return nil
}

// refuseStaffTarget answers a target that is an active staff member: 404 for
// admin, which does not see staff, and 409 user_is_staff for system_admin
// (spec rule 5). It reports whether it refused.
func refuseStaffTarget(w http.ResponseWriter, viewer staff.Role, isStaff bool) bool {
	if !isStaff {
		return false
	}
	if viewer == staff.RoleSystemAdmin {
		writeError(w, &apiError{Status: http.StatusConflict, Code: "user_is_staff", Message: "an active staff member cannot be changed here"})
	} else {
		writeError(w, notFound())
	}
	return true
}

// tenantVisible reports whether the scope may see the tenant.
func tenantVisible(scope Scope, tenantID string) bool {
	return scope.All || (scope.TenantID != "" && scope.TenantID == tenantID)
}

func tenantUserJSON(u TenantUser) map[string]any {
	ws := make([]map[string]any, len(u.Workspaces))
	for i, x := range u.Workspaces {
		ws[i] = map[string]any{"workspace_id": x.ID, "name": x.Name, "member_role": x.Role}
	}
	out := map[string]any{
		"user_id": u.ID, "email": u.Email, "name": u.Name, "account_status": u.AccountStatus,
		"is_staff": u.IsStaff, "workspaces": ws, "seats": len(u.Workspaces),
	}
	if u.LastLoginAt != nil {
		out["last_login_at"] = ts(*u.LastLoginAt)
	} else {
		out["last_login_at"] = nil
	}
	return out
}

// listTenantUsers returns one page of a tenant's users (M35). Staff users are
// hidden from admin; the is_staff flag is real only for system_admin.
func (h *Handler) listTenantUsers(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionTenantUsersRead)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if !tenantVisible(scope, id) {
		writeError(w, notFound())
		return
	}
	q := r.URL.Query()
	limit := 50
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			writeError(w, badRequest("invalid_limit", "limit must be 1 to 200"))
			return
		}
		limit = n
	}
	pattern := ""
	if s := q.Get("q"); s != "" {
		p, perr := likePattern(s)
		if perr != nil {
			writeError(w, perr)
			return
		}
		pattern = *p
	}
	users, err := h.Reader.TenantUsers(r.Context(), TenantUserSearch{
		TenantID: id, Pattern: pattern, CursorUserID: q.Get("cursor"), Limit: limit, ViewerAll: scope.All,
	})
	if err != nil {
		h.fail(w, r, "list tenant users", err)
		return
	}
	users, next := nextCursor(users, limit, func(u TenantUser) (time.Time, string) { return time.Time{}, u.ID })
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		if !scope.All && u.IsStaff {
			continue
		}
		if !scope.All {
			u.IsStaff = false
		}
		out = append(out, tenantUserJSON(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out, "next_cursor": next})
}

// getTenantUser returns one user of a tenant with its memberships there.
func (h *Handler) getTenantUser(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionTenantUsersRead)
	if !ok {
		return
	}
	tenantID, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	userID, perr := pathID(r, "user_id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if !tenantVisible(scope, tenantID) {
		writeError(w, notFound())
		return
	}
	u, found, err := h.Reader.TenantUser(r.Context(), tenantID, userID)
	if err != nil {
		h.fail(w, r, "get tenant user", err)
		return
	}
	if !found || (!scope.All && u.IsStaff) {
		writeError(w, notFound())
		return
	}
	if !scope.All {
		u.IsStaff = false
	}
	body := tenantUserJSON(u)
	allocation, err := h.tenantUserAllocation(r.Context(), scope, tenantID, u)
	if err != nil {
		h.fail(w, r, "read tenant user allocation", err)
		return
	}
	body["allocation"] = map[string]any{"workspaces": allocation}
	writeJSON(w, http.StatusOK, body)
}

// tenantUserAllocation is what a user holds in each workspace of the tenant: the
// seat it takes there, and the workspace's effective limits and usage (spec
// R6 section 3b.3 rule 2, D95). Limits belong to workspaces, so the user has none
// of its own. Each membership takes one seat, as the list's seats count does.
func (h *Handler) tenantUserAllocation(ctx context.Context, scope Scope, tenantID string, u TenantUser) ([]map[string]any, error) {
	d, found, err := h.Reader.GetTenant(ctx, tenantID, scope)
	if err != nil {
		return nil, err
	}
	if !found || len(u.Workspaces) == 0 {
		return []map[string]any{}, nil
	}
	roles := make(map[string]string, len(u.Workspaces))
	for _, m := range u.Workspaces {
		roles[m.ID] = m.Role
	}
	var mine []TenantWorkspace
	for _, ws := range d.Workspaces {
		if _, ok := roles[ws.ID]; ok {
			mine = append(mine, ws)
		}
	}
	usage, err := h.tenantUsage(ctx, mine, h.Now())
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(usage.workspaces))
	for _, ws := range usage.workspaces {
		out = append(out, map[string]any{
			"workspace_id": ws.WorkspaceID, "slug": ws.Slug, "plan_key": ws.PlanKey,
			"member_role": roles[ws.WorkspaceID], "occupies_seat": true, "resources": ws.Resources,
		})
	}
	return out, nil
}

// requestMemberChange inserts one change request and its audit row in one
// transaction. A live request for the same workspace, kind and email is kept
// and returned, not duplicated.
func (h *Handler) requestMemberChange(ctx context.Context, a actor, tenantID string, c MemberChange) (string, error) {
	var id string
	err := h.Runner.InTx(ctx, func(tx Tx) error {
		row, inserted, err := tx.InsertMemberChange(ctx, c, a.ID, a.Email)
		if err != nil {
			return err
		}
		id = row
		if !inserted {
			return nil
		}
		action := audit.ActionMemberStaffRemoveRequested
		if c.Kind == memberKindChangeRole {
			action = audit.ActionMemberStaffRoleChangeRequested
		}
		return audit.Record(ctx, tx, a.staffEntry(audit.Entry{
			WorkspaceID: c.WorkspaceID, TenantID: tenantID, Action: action,
			TargetType: targetUser, TargetID: c.TargetUserID, TargetLabel: c.Email,
			Detail: map[string]any{"request_id": row, "previous_role": c.PreviousRole, "role": c.Role},
		}))
	})
	return id, err
}

// removeWorkspaceMember asks for a member to be removed from one workspace (M36).
func (h *Handler) removeWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionMembersRemove)
	if !ok {
		return
	}
	wsID, userID, ok := h.memberPath(w, r)
	if !ok {
		return
	}
	if !h.visibleWorkspace(w, r, scope, wsID) {
		return
	}
	role, email, found, err := h.Reader.WorkspaceMember(r.Context(), wsID, userID)
	if err != nil {
		h.fail(w, r, "read member", err)
		return
	}
	if !found {
		writeError(w, notFound())
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	isStaff, err := h.Reader.IsActiveStaff(r.Context(), userID)
	if err != nil {
		h.fail(w, r, "check staff", err)
		return
	}
	if refuseStaffTarget(w, a.Role, isStaff) {
		return
	}
	if role == "owner" {
		writeError(w, &apiError{Status: http.StatusConflict, Code: "owner_role_immutable", Message: "an owner cannot be removed by staff"})
		return
	}
	h.answerMemberChange(w, r, wsID, MemberChange{WorkspaceID: wsID, Kind: memberKindRemove, TargetUserID: userID, Email: email, PreviousRole: role, Role: role})
}

// changeWorkspaceMemberRole asks for a member's role to change (M37).
func (h *Handler) changeWorkspaceMemberRole(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionMembersChangeRole)
	if !ok {
		return
	}
	wsID, userID, ok := h.memberPath(w, r)
	if !ok {
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if perr := decodeBody(w, r, &body); perr != nil {
		writeError(w, perr)
		return
	}
	if !h.visibleWorkspace(w, r, scope, wsID) {
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	role, email, found, err := h.Reader.WorkspaceMember(r.Context(), wsID, userID)
	if err != nil {
		h.fail(w, r, "read member", err)
		return
	}
	if !found {
		writeError(w, notFound())
		return
	}
	isStaff, err := h.Reader.IsActiveStaff(r.Context(), userID)
	if err != nil {
		h.fail(w, r, "check staff", err)
		return
	}
	if refuseStaffTarget(w, a.Role, isStaff) {
		return
	}
	if perr := checkRoleChange(a.Role, a.ID, userID, role, body.Role); perr != nil {
		writeError(w, perr)
		return
	}
	if role == body.Role {
		writeJSON(w, http.StatusOK, map[string]any{"changed": false})
		return
	}
	h.answerMemberChange(w, r, wsID, MemberChange{WorkspaceID: wsID, Kind: memberKindChangeRole, TargetUserID: userID, Email: email, PreviousRole: role, Role: body.Role})
}

// deactivateTenantUser removes a user from every workspace of the tenant it is
// a member of, one remove request each (M36, D93 scope a). It refuses an owner.
func (h *Handler) deactivateTenantUser(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionMembersRemove)
	if !ok {
		return
	}
	tenantID, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	userID, perr := pathID(r, "user_id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if !tenantVisible(scope, tenantID) {
		writeError(w, notFound())
		return
	}
	u, found, err := h.Reader.TenantUser(r.Context(), tenantID, userID)
	if err != nil {
		h.fail(w, r, "get tenant user", err)
		return
	}
	if !found {
		writeError(w, notFound())
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	if refuseStaffTarget(w, a.Role, u.IsStaff) {
		return
	}
	for _, m := range u.Workspaces {
		if m.Role == "owner" {
			writeError(w, &apiError{Status: http.StatusConflict, Code: "owner_role_immutable", Message: "an owner cannot be removed by staff; remove the owner role first"})
			return
		}
	}
	ids := make([]string, 0, len(u.Workspaces))
	for _, m := range u.Workspaces {
		id, err := h.requestMemberChange(r.Context(), a, tenantID, MemberChange{
			WorkspaceID: m.ID, Kind: memberKindRemove, TargetUserID: userID, Email: u.Email, PreviousRole: m.Role, Role: m.Role,
		})
		if err != nil {
			h.fail(w, r, "request member removal", err)
			return
		}
		ids = append(ids, id)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"request_ids": ids})
}

// memberPath reads the workspace and user ids of a member route.
func (h *Handler) memberPath(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	wsID, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return "", "", false
	}
	userID, perr := pathID(r, "user_id")
	if perr != nil {
		writeError(w, perr)
		return "", "", false
	}
	return wsID, userID, true
}

// answerMemberChange writes the request and answers 202 with its id.
func (h *Handler) answerMemberChange(w http.ResponseWriter, r *http.Request, wsID string, c MemberChange) {
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	tenantID, err := h.Reader.WorkspaceTenant(r.Context(), wsID)
	if err != nil {
		h.fail(w, r, "workspace tenant", err)
		return
	}
	id, err := h.requestMemberChange(r.Context(), a, tenantID, c)
	if err != nil {
		h.fail(w, r, "request member change", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"request_id": id, "kind": c.Kind})
}
