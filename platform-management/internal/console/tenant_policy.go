package console

import (
	"errors"
	"net/http"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/internal/tenantpolicy"
)

// The tenant policy floor (spec R6 section 3c.4, QT6): read by admin of the
// tenant and support, written by system_admin and admin of the tenant.

// policyFeature is the plan feature a tenant's policy write needs (rule 4).
const policyFeature = "workspace_policies"

// tenantPolicyBody is a policy as a client sends it, with the version it was read at.
type tenantPolicyBody struct {
	ExpectedVersion      *int64   `json:"expected_version"`
	AllowedInviteDomains []string `json:"allowed_invite_domains"`
	ShareLinksEnabled    *bool    `json:"share_links_enabled"`
	RequireSSO           bool     `json:"require_sso"`
	RequireMFA           bool     `json:"require_mfa"`
	SessionMaxAgeHours   int32    `json:"session_max_age_hours"`
}

// getTenantPolicy returns the tenant's floor. A tenant with no row has the
// default policy and version 0 (rule 4).
func (h *Handler) getTenantPolicy(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionTenantPolicyRead)
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
	stored, found, err := h.Reader.TenantPolicy(r.Context(), id)
	if err != nil {
		h.fail(w, r, "read tenant policy", err)
		return
	}
	if !found {
		stored = tenantpolicy.Stored{Policy: tenantpolicy.Default()}
	}
	writeJSON(w, http.StatusOK, map[string]any{"policy": stored.Policy, "version": stored.Version})
}

// putTenantPolicy writes the tenant's floor (M45). A write needs a workspace of
// the tenant on a plan with workspace_policies (402 otherwise). The expected
// version, when sent, must match the stored one (409 otherwise). A write that
// changes nothing keeps the version.
func (h *Handler) putTenantPolicy(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionTenantPolicyWrite)
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
	var body tenantPolicyBody
	if perr := decodeBody(w, r, &body); perr != nil {
		writeError(w, perr)
		return
	}
	domains, err := tenantpolicy.NormalizeDomains(body.AllowedInviteDomains)
	if err != nil {
		writeError(w, badRequest("invalid_policy", err.Error()))
		return
	}
	next := tenantpolicy.Policy{
		AllowedInviteDomains: domains, ShareLinksEnabled: true,
		RequireSSO: body.RequireSSO, RequireMFA: body.RequireMFA, SessionMaxAgeHours: body.SessionMaxAgeHours,
	}
	if body.ShareLinksEnabled != nil {
		next.ShareLinksEnabled = *body.ShareLinksEnabled
	}
	if err := tenantpolicy.Validate(next); err != nil {
		writeError(w, badRequest("invalid_policy", err.Error()))
		return
	}
	hasFeature, err := h.Reader.TenantHasPolicyFeature(r.Context(), id)
	if err != nil {
		h.fail(w, r, "check policy feature", err)
		return
	}
	if !hasFeature {
		writeJSON(w, http.StatusPaymentRequired, map[string]any{
			"error": "no workspace of the tenant has the plan feature", "code": "plan_feature_unavailable", "feature": policyFeature,
		})
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	var version int64
	err = h.Runner.InTx(r.Context(), func(tx Tx) error {
		cur, found, err := tx.TenantPolicyForUpdate(r.Context(), id)
		if err != nil {
			return err
		}
		before := tenantpolicy.Stored{Policy: tenantpolicy.Default()}
		if found {
			before = cur
		}
		if body.ExpectedVersion != nil && *body.ExpectedVersion != before.Version {
			return &apiError{Status: http.StatusConflict, Code: "policy_version_conflict", Message: "the policy changed since it was read; reload it and retry"}
		}
		version = before.Version
		changed := tenantpolicy.Changed(before.Policy, next)
		if len(changed) == 0 && found {
			return nil
		}
		version = before.Version + 1
		if err := tx.SaveTenantPolicy(r.Context(), id, tenantpolicy.Stored{Policy: next, Version: version}, a.ID); err != nil {
			return err
		}
		return audit.Record(r.Context(), tx, a.staffEntry(audit.Entry{
			TenantID: id, Action: audit.ActionTenantPolicyChanged, TargetType: "tenant", TargetID: id,
			Detail: map[string]any{"changes": changed, "version": version, "from": before.Policy, "to": next},
		}))
	})
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		writeError(w, apiErr)
		return
	}
	if err != nil {
		h.fail(w, r, "write tenant policy", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"policy": next, "version": version})
}
