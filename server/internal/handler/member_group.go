package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/membergroup"
	"github.com/multica-ai/multica/server/internal/planlimit"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Account groups (platform management spec 7b.2). The routes sit behind
// RequireHumanActor; the group routes are owner and admin only, and
// permissions/me is open to every member.

const (
	codeGroupNameTaken    = "group_name_taken"
	codeGroupNotFound     = "group_not_found"
	codeNotAMember        = "not_a_member"
	codeGroupMemberLimit  = "group_member_limit"
	codePermissionUnknown = "permission_unknown"
	codeTooManyUserIDs    = "too_many_user_ids"
)

// MemberGroupResponse is the JSON shape of a group.
type MemberGroupResponse struct {
	ID          string   `json:"id"`
	WorkspaceID string   `json:"workspace_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	MemberCount int64    `json:"member_count"`
	CreatedBy   *string  `json:"created_by"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// MemberGroupMemberResponse is one person in a group.
type MemberGroupMemberResponse struct {
	UserID  string  `json:"user_id"`
	AddedBy *string `json:"added_by"`
	AddedAt string  `json:"added_at"`
}

// MemberGroupDetailResponse is a group with the people in it.
type MemberGroupDetailResponse struct {
	MemberGroupResponse
	Members []MemberGroupMemberResponse `json:"members"`
}

// CreateMemberGroupRequest is the body of POST /groups.
type CreateMemberGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UpdateMemberGroupRequest is the body of PATCH /groups/{groupId}. A field
// that is absent keeps its value.
type UpdateMemberGroupRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// SetMemberGroupPermissionsRequest is the body of PUT /groups/{groupId}/permissions.
type SetMemberGroupPermissionsRequest struct {
	Permissions *[]string `json:"permissions"`
}

// AddMemberGroupMembersRequest is the body of POST /groups/{groupId}/members.
type AddMemberGroupMembersRequest struct {
	UserIDs []string `json:"user_ids"`
}

// MyPermissionsResponse is the body of GET /permissions/me: the caller's role
// and effective permissions, sorted.
type MyPermissionsResponse struct {
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
}

func memberGroupToResponse(g db.MemberGroup, memberCount int64) MemberGroupResponse {
	resp := MemberGroupResponse{
		ID:          uuidToString(g.ID),
		WorkspaceID: uuidToString(g.WorkspaceID),
		Name:        g.Name,
		Description: g.Description,
		Permissions: membergroup.StoredPermissions(g),
		MemberCount: memberCount,
		CreatedAt:   timestampToString(g.CreatedAt),
		UpdatedAt:   timestampToString(g.UpdatedAt),
	}
	if g.CreatedBy.Valid {
		by := uuidToString(g.CreatedBy)
		resp.CreatedBy = &by
	}
	return resp
}

func memberGroupAudit(action string, g db.MemberGroup, detail map[string]any) audit.Entry {
	return audit.Entry{
		WorkspaceID: uuidToString(g.WorkspaceID),
		Action:      action,
		TargetType:  "group",
		TargetID:    uuidToString(g.ID),
		Detail:      detail,
	}
}

// memberGroupScope reads the caller and the workspace of a group route.
func memberGroupScope(w http.ResponseWriter, r *http.Request) (user, workspace pgtype.UUID, ok bool) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return user, workspace, false
	}
	if user, ok = parseUUIDOrBadRequest(w, userID, "user id"); !ok {
		return user, workspace, false
	}
	workspace, ok = parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
	return user, workspace, ok
}

// memberGroupID reads {groupId}, a pure UUID.
func memberGroupID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return parseUUIDOrBadRequest(w, chi.URLParam(r, "groupId"), "group id")
}

// writeMemberGroupError answers err with the status and code of spec 7b.2. A
// failed audit write and any error it does not know are a 500.
func writeMemberGroupError(w http.ResponseWriter, r *http.Request, err error) {
	var unknown *membergroup.UnknownPermissionError
	var limit *membergroup.MemberLimitError
	switch {
	case isAuditWriteError(err):
		slog.Warn("member group audit write failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to record the change")
	case errors.Is(err, membergroup.ErrGroupNotFound):
		writeErrorCode(w, http.StatusNotFound, codeGroupNotFound, "group not found")
	case errors.Is(err, membergroup.ErrNameTaken):
		writeErrorCode(w, http.StatusConflict, codeGroupNameTaken, "a group with this name already exists")
	case errors.Is(err, membergroup.ErrNotAMember):
		writeErrorCode(w, http.StatusConflict, codeNotAMember, "only members of the workspace can join a group")
	case errors.Is(err, membergroup.ErrNameInvalid), errors.Is(err, membergroup.ErrDescriptionInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.As(err, &unknown):
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": err.Error(), "code": codePermissionUnknown, "permission": unknown.Key,
		})
	case errors.As(err, &limit):
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": err.Error(), "code": codeGroupMemberLimit, "limit": limit.Limit,
		})
	default:
		slog.Warn("member group request failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to process the request")
	}
}

// memberGroupGateRefused answers a creation that the member_groups gate
// refuses: limit 0 is a plan without groups (402 plan_feature_unavailable),
// anything else is the plan limit body of spec 10. It reports false when err is
// not a gate refusal.
func memberGroupGateRefused(w http.ResponseWriter, err error) bool {
	var reached *planlimit.ReachedError
	if !errors.As(err, &reached) {
		return false
	}
	if reached.Limit == 0 {
		writeJSON(w, http.StatusPaymentRequired, map[string]any{
			"error":   "account groups are not included in the plan of this workspace",
			"code":    codePlanFeatureUnavailable,
			"feature": string(entitlement.GateMemberGroups),
			// Older clients read the name of the gate from "gate".
			"gate": string(entitlement.GateMemberGroups),
		})
		return true
	}
	return writePlanLimitReached(w, err)
}

// GetPermissionCatalogue returns the fixed list of permission keys.
// GET /api/workspaces/{id}/permission-catalogue
func (h *Handler) GetPermissionCatalogue(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]string{"permissions": membergroup.Catalogue()})
}

// GetMyPermissions returns the caller's role and effective permissions, read
// from the database on every request.
// GET /api/workspaces/{id}/permissions/me
func (h *Handler) GetMyPermissions(w http.ResponseWriter, r *http.Request) {
	user, workspace, ok := memberGroupScope(w, r)
	if !ok {
		return
	}
	access, err := membergroup.New(h.Queries).Access(r.Context(), workspace, user)
	if errors.Is(err, membergroup.ErrNotAMember) {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	if err != nil {
		writeMemberGroupError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, MyPermissionsResponse{Role: access.Role, Permissions: access.Permissions})
}

// ListMemberGroups lists the groups of the workspace with their member counts.
// GET /api/workspaces/{id}/groups
func (h *Handler) ListMemberGroups(w http.ResponseWriter, r *http.Request) {
	_, workspace, ok := memberGroupScope(w, r)
	if !ok {
		return
	}
	rows, err := membergroup.New(h.Queries).List(r.Context(), workspace)
	if err != nil {
		writeMemberGroupError(w, r, err)
		return
	}
	resp := make([]MemberGroupResponse, len(rows))
	for i, row := range rows {
		resp[i] = memberGroupToResponse(db.MemberGroup{
			ID: row.ID, WorkspaceID: row.WorkspaceID, Name: row.Name, Description: row.Description,
			Permissions: row.Permissions, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}, row.MemberCount)
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetMemberGroup returns one group with its members.
// GET /api/workspaces/{id}/groups/{groupId}
func (h *Handler) GetMemberGroup(w http.ResponseWriter, r *http.Request) {
	_, workspace, ok := memberGroupScope(w, r)
	if !ok {
		return
	}
	id, ok := memberGroupID(w, r)
	if !ok {
		return
	}
	svc := membergroup.New(h.Queries)
	g, err := svc.Get(r.Context(), workspace, id)
	if err != nil {
		writeMemberGroupError(w, r, err)
		return
	}
	members, err := svc.ListMembers(r.Context(), workspace, id)
	if err != nil {
		writeMemberGroupError(w, r, err)
		return
	}
	detail := MemberGroupDetailResponse{
		MemberGroupResponse: memberGroupToResponse(g, int64(len(members))),
		Members:             make([]MemberGroupMemberResponse, len(members)),
	}
	for i, m := range members {
		detail.Members[i] = MemberGroupMemberResponse{UserID: uuidToString(m.UserID), AddedAt: timestampToString(m.AddedAt)}
		if m.AddedBy.Valid {
			by := uuidToString(m.AddedBy)
			detail.Members[i].AddedBy = &by
		}
	}
	writeJSON(w, http.StatusOK, detail)
}

// CreateMemberGroup creates a group. The member_groups gate counts it: the
// count runs in the creating transaction after the advisory lock.
// POST /api/workspaces/{id}/groups
func (h *Handler) CreateMemberGroup(w http.ResponseWriter, r *http.Request) {
	user, workspace, ok := memberGroupScope(w, r)
	if !ok {
		return
	}
	var req CreateMemberGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Bad input is a 400 whatever the plan says.
	if _, err := membergroup.NormalizeName(req.Name); err != nil {
		writeMemberGroupError(w, r, err)
		return
	}
	if _, err := membergroup.NormalizeDescription(req.Description); err != nil {
		writeMemberGroupError(w, r, err)
		return
	}

	// The plan is read before the transaction: a policy fetch is a network call.
	gate := h.resolvePlanGate(r.Context(), workspace, entitlement.GateMemberGroups)

	var created db.MemberGroup
	err := h.auditTx(r, func(qtx *db.Queries) (audit.Entry, error) {
		svc := membergroup.New(qtx)
		if err := gate.Admit(r.Context(), qtx, workspace, 1, func(ctx context.Context) (int64, error) {
			return svc.CountGroups(ctx, workspace)
		}); err != nil {
			return audit.Entry{}, err
		}
		g, err := svc.Create(r.Context(), workspace, user, req.Name, req.Description)
		if err != nil {
			return audit.Entry{}, err
		}
		created = g
		return memberGroupAudit(audit.ActionGroupCreated, g, nil), nil
	})
	if err != nil {
		if memberGroupGateRefused(w, err) {
			return
		}
		writeMemberGroupError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, memberGroupToResponse(created, 0))
}

// UpdateMemberGroup changes the name and description. group.updated records
// the names of the fields that changed, and nothing is written when none did.
// PATCH /api/workspaces/{id}/groups/{groupId}
func (h *Handler) UpdateMemberGroup(w http.ResponseWriter, r *http.Request) {
	_, workspace, ok := memberGroupScope(w, r)
	if !ok {
		return
	}
	id, ok := memberGroupID(w, r)
	if !ok {
		return
	}
	var req UpdateMemberGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var updated db.MemberGroup
	err := h.auditTx(r, func(qtx *db.Queries) (audit.Entry, error) {
		g, changed, err := membergroup.New(qtx).Update(r.Context(), workspace, id, membergroup.UpdateInput{
			Name: req.Name, Description: req.Description,
		})
		if err != nil {
			return audit.Entry{}, err
		}
		updated = g
		if len(changed) == 0 {
			return audit.Entry{}, nil
		}
		return memberGroupAudit(audit.ActionGroupUpdated, g, map[string]any{"fields": changed}), nil
	})
	if err != nil {
		writeMemberGroupError(w, r, err)
		return
	}
	h.writeMemberGroup(w, r, http.StatusOK, updated)
}

// SetMemberGroupPermissions replaces the keys of a group. group.permissions_changed
// records the old and the new array; writing the keys already stored records nothing.
// PUT /api/workspaces/{id}/groups/{groupId}/permissions
func (h *Handler) SetMemberGroupPermissions(w http.ResponseWriter, r *http.Request) {
	_, workspace, ok := memberGroupScope(w, r)
	if !ok {
		return
	}
	id, ok := memberGroupID(w, r)
	if !ok {
		return
	}
	var req SetMemberGroupPermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Permissions == nil {
		writeError(w, http.StatusBadRequest, "permissions is required")
		return
	}

	var updated db.MemberGroup
	err := h.auditTx(r, func(qtx *db.Queries) (audit.Entry, error) {
		g, old, err := membergroup.New(qtx).SetPermissions(r.Context(), workspace, id, *req.Permissions)
		if err != nil {
			return audit.Entry{}, err
		}
		updated = g
		next := membergroup.StoredPermissions(g)
		if slices.Equal(old, next) {
			return audit.Entry{}, nil
		}
		return memberGroupAudit(audit.ActionGroupPermissionsChanged, g, map[string]any{"old": old, "new": next}), nil
	})
	if err != nil {
		writeMemberGroupError(w, r, err)
		return
	}
	h.writeMemberGroup(w, r, http.StatusOK, updated)
}

// DeleteMemberGroup deletes a group and its member rows in one transaction.
// DELETE /api/workspaces/{id}/groups/{groupId}
func (h *Handler) DeleteMemberGroup(w http.ResponseWriter, r *http.Request) {
	_, workspace, ok := memberGroupScope(w, r)
	if !ok {
		return
	}
	id, ok := memberGroupID(w, r)
	if !ok {
		return
	}
	err := h.auditTx(r, func(qtx *db.Queries) (audit.Entry, error) {
		g, members, err := membergroup.New(qtx).Delete(r.Context(), workspace, id)
		if err != nil {
			return audit.Entry{}, err
		}
		return memberGroupAudit(audit.ActionGroupDeleted, g, map[string]any{"member_count": members}), nil
	})
	if err != nil {
		writeMemberGroupError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AddMemberGroupMembers puts up to 100 workspace members in a group. It writes
// one group.member_added row for each person actually added.
// POST /api/workspaces/{id}/groups/{groupId}/members
func (h *Handler) AddMemberGroupMembers(w http.ResponseWriter, r *http.Request) {
	user, workspace, ok := memberGroupScope(w, r)
	if !ok {
		return
	}
	id, ok := memberGroupID(w, r)
	if !ok {
		return
	}
	var req AddMemberGroupMembersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch {
	case len(req.UserIDs) == 0:
		writeError(w, http.StatusBadRequest, "user_ids is required")
		return
	case len(req.UserIDs) > membergroup.MaxAddPerRequest:
		writeErrorCode(w, http.StatusBadRequest, codeTooManyUserIDs, "a request adds at most 100 people")
		return
	}
	users, ok := parseUUIDSliceOrBadRequest(w, req.UserIDs, "user id")
	if !ok {
		return
	}

	added := []string{}
	err := h.auditTx(r, func(qtx *db.Queries) (audit.Entry, error) {
		svc := membergroup.New(qtx)
		people, err := svc.AddMembers(r.Context(), workspace, id, user, users)
		if err != nil {
			return audit.Entry{}, err
		}
		g, err := svc.Get(r.Context(), workspace, id)
		if err != nil {
			return audit.Entry{}, err
		}
		for _, person := range people {
			added = append(added, uuidToString(person))
			if err := h.recordAuditTx(r, qtx, memberGroupAudit(audit.ActionGroupMemberAdded, g, map[string]any{"user_id": uuidToString(person)})); err != nil {
				return audit.Entry{}, err
			}
		}
		return audit.Entry{}, nil
	})
	if err != nil {
		writeMemberGroupError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"added": added})
}

// RemoveMemberGroupMember takes one person out of a group. Removing someone who
// is not in it changes nothing and writes no audit row.
// DELETE /api/workspaces/{id}/groups/{groupId}/members/{userId}
func (h *Handler) RemoveMemberGroupMember(w http.ResponseWriter, r *http.Request) {
	_, workspace, ok := memberGroupScope(w, r)
	if !ok {
		return
	}
	id, ok := memberGroupID(w, r)
	if !ok {
		return
	}
	person, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "userId"), "user id")
	if !ok {
		return
	}
	err := h.auditTx(r, func(qtx *db.Queries) (audit.Entry, error) {
		removed, g, err := membergroup.New(qtx).RemoveMember(r.Context(), workspace, id, person)
		if err != nil || !removed {
			return audit.Entry{}, err
		}
		return memberGroupAudit(audit.ActionGroupMemberRemoved, g, map[string]any{"user_id": uuidToString(person)}), nil
	})
	if err != nil {
		writeMemberGroupError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeMemberGroup answers with g and its current member count.
func (h *Handler) writeMemberGroup(w http.ResponseWriter, r *http.Request, status int, g db.MemberGroup) {
	count, err := h.Queries.CountMemberGroupMembers(r.Context(), db.CountMemberGroupMembersParams{WorkspaceID: g.WorkspaceID, GroupID: g.ID})
	if err != nil {
		writeMemberGroupError(w, r, err)
		return
	}
	writeJSON(w, status, memberGroupToResponse(g, count))
}
