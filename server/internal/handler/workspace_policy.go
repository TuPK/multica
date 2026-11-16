package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/auditaction"
	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/workspaceguard"
	"github.com/multica-ai/multica/server/internal/workspacepolicy"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const codePlanFeatureUnavailable = "plan_feature_unavailable"

// WorkspacePolicyResponse is the JSON shape of a workspace policy. A workspace
// with no stored policy reads as the defaults, with no updated_by or
// updated_at.
type WorkspacePolicyResponse struct {
	AllowedInviteDomains []string `json:"allowed_invite_domains"`
	ShareLinksEnabled    bool     `json:"share_links_enabled"`
	RequireSSO           bool     `json:"require_sso"`
	RequireMFA           bool     `json:"require_mfa"`
	SessionMaxAgeHours   int32    `json:"session_max_age_hours"`
	UpdatedBy            *string  `json:"updated_by"`
	UpdatedAt            *string  `json:"updated_at"`
}

// UpdateWorkspacePolicyRequest carries the fields to change. A field that is
// absent keeps its value.
type UpdateWorkspacePolicyRequest struct {
	AllowedInviteDomains *[]string `json:"allowed_invite_domains"`
	ShareLinksEnabled    *bool     `json:"share_links_enabled"`
	RequireSSO           *bool     `json:"require_sso"`
	RequireMFA           *bool     `json:"require_mfa"`
	SessionMaxAgeHours   *int      `json:"session_max_age_hours"`
}

func policyToResponse(p workspacepolicy.Policy, row *db.WorkspacePolicy) WorkspacePolicyResponse {
	resp := WorkspacePolicyResponse{
		AllowedInviteDomains: p.AllowedInviteDomains,
		ShareLinksEnabled:    p.ShareLinksEnabled,
		RequireSSO:           p.RequireSSO,
		RequireMFA:           p.RequireMFA,
		SessionMaxAgeHours:   p.SessionMaxAgeHours,
	}
	if resp.AllowedInviteDomains == nil {
		resp.AllowedInviteDomains = []string{}
	}
	if row != nil {
		if row.UpdatedBy.Valid {
			s := uuidToString(row.UpdatedBy)
			resp.UpdatedBy = &s
		}
		if row.UpdatedAt.Valid {
			s := row.UpdatedAt.Time.UTC().Format(time.RFC3339)
			resp.UpdatedAt = &s
		}
	}
	return resp
}

// workspacePolicy returns the policy in force for a workspace: the defaults
// when it has no row or its plan lacks workspace_policies.
func (h *Handler) workspacePolicy(r *http.Request, workspaceID pgtype.UUID) (workspacepolicy.Policy, error) {
	return workspaceguard.EffectivePolicy(r.Context(), h.Queries, h.Entitlements, workspaceID)
}

// refuseByInvitePolicy answers 403 invite_domain_not_allowed and returns false
// when the policy does not allow email.
func refuseByInvitePolicy(w http.ResponseWriter, p workspacepolicy.Policy, email string) bool {
	if workspacepolicy.DomainAllowed(p.AllowedInviteDomains, email) {
		return true
	}
	writeErrorCode(w, http.StatusForbidden, workspacepolicy.CodeInviteDomainNotAllowed, "this workspace does not accept invitations for that email domain")
	return false
}

// GetWorkspacePolicy returns the workspace policy to owners and admins.
// GET /api/workspaces/{id}/policy
func (h *Handler) GetWorkspacePolicy(w http.ResponseWriter, r *http.Request) {
	workspaceUUID, ok := parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
	if !ok {
		return
	}
	row, err := h.Queries.GetWorkspacePolicy(r.Context(), workspaceUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, policyToResponse(workspacepolicy.Default(), nil))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load policy")
		return
	}
	// Without the plan feature the stored policy is not in force, and the
	// read shows what is.
	if !workspaceguard.PolicyFeatureAvailable(r.Context(), h.Entitlements, workspaceUUID) {
		writeJSON(w, http.StatusOK, policyToResponse(workspacepolicy.Default(), nil))
		return
	}
	writeJSON(w, http.StatusOK, policyToResponse(workspaceguard.PolicyFromRow(row), &row))
}

// UpdateWorkspacePolicy changes the workspace policy; owner only. It needs the
// workspace_policies plan feature. Turning share links off deactivates the
// workspace's active links in the same transaction, and a change writes
// policy.updated with the old and new value of each changed field.
// PUT /api/workspaces/{id}/policy
func (h *Handler) UpdateWorkspacePolicy(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	userUUID, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}
	workspaceUUID, ok := parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
	if !ok {
		return
	}

	// The plan is read before the transaction: a policy fetch is a network call.
	if !workspaceguard.PolicyFeatureAvailable(r.Context(), h.Entitlements, workspaceUUID) {
		writeJSON(w, http.StatusPaymentRequired, map[string]any{
			"error": "workspace policies are not included in the plan of this workspace",
			"code":  codePlanFeatureUnavailable,
			"gate":  string(entitlement.GateWorkspacePolicies),
		})
		return
	}

	var req UpdateWorkspacePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var domains []string
	if req.AllowedInviteDomains != nil {
		var err error
		if domains, err = workspacepolicy.NormalizeDomains(*req.AllowedInviteDomains); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.SessionMaxAgeHours != nil && (*req.SessionMaxAgeHours < 0 || *req.SessionMaxAgeHours > workspacepolicy.MaxSessionMaxAgeHours) {
		writeError(w, http.StatusBadRequest, "session_max_age_hours must be between 0 and 8760")
		return
	}

	var result WorkspacePolicyResponse
	err := h.auditTx(r, func(qtx *db.Queries) (audit.Entry, error) {
		old := workspacepolicy.Default()
		stored, err := qtx.GetWorkspacePolicyForUpdate(r.Context(), workspaceUUID)
		var storedRow *db.WorkspacePolicy
		switch {
		case err == nil:
			old = workspaceguard.PolicyFromRow(stored)
			storedRow = &stored
		case !errors.Is(err, pgx.ErrNoRows):
			return audit.Entry{}, err
		}

		next := old
		if req.AllowedInviteDomains != nil {
			next.AllowedInviteDomains = domains
		}
		if req.ShareLinksEnabled != nil {
			next.ShareLinksEnabled = *req.ShareLinksEnabled
		}
		if req.RequireSSO != nil {
			next.RequireSSO = *req.RequireSSO
		}
		if req.RequireMFA != nil {
			next.RequireMFA = *req.RequireMFA
		}
		if req.SessionMaxAgeHours != nil {
			next.SessionMaxAgeHours = int32(*req.SessionMaxAgeHours)
		}

		changes := workspacepolicy.Changes(old, next)
		if len(changes) == 0 {
			result = policyToResponse(old, storedRow)
			return audit.Entry{}, nil
		}

		row, err := qtx.UpsertWorkspacePolicy(r.Context(), db.UpsertWorkspacePolicyParams{
			WorkspaceID:          workspaceUUID,
			AllowedInviteDomains: next.AllowedInviteDomains,
			ShareLinksEnabled:    next.ShareLinksEnabled,
			RequireSso:           next.RequireSSO,
			RequireMfa:           next.RequireMFA,
			SessionMaxAgeHours:   next.SessionMaxAgeHours,
			UpdatedBy:            userUUID,
		})
		if err != nil {
			return audit.Entry{}, err
		}
		if old.ShareLinksEnabled && !next.ShareLinksEnabled {
			if err := qtx.DeactivateWorkspaceShareLinks(r.Context(), workspaceUUID); err != nil {
				return audit.Entry{}, err
			}
		}
		result = policyToResponse(next, &row)
		return audit.Entry{
			WorkspaceID: uuidToString(workspaceUUID),
			Action:      auditaction.PolicyUpdated,
			TargetType:  "workspace_policy",
			TargetID:    uuidToString(workspaceUUID),
			Detail:      map[string]any{"changes": changes},
		}, nil
	})
	if err != nil {
		slog.Warn("update workspace policy failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to update policy")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
