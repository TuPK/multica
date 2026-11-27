package handler

import (
	"encoding/json"
	"net/http"

	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/service"
)

// EffectPolicyOptionResponse is one decision a cell may take. Allowed is false when the
// workspace may not save that decision there; Reason then names why (durable spec 4.3).
type EffectPolicyOptionResponse struct {
	Decision string `json:"decision"`
	Allowed  bool   `json:"allowed"`
	Reason   string `json:"reason"`
}

// EffectPolicyCellResponse is one cell of the policy grid: the table's default for it,
// the workspace's saved decision (null when none), and the decisions it may take.
type EffectPolicyCellResponse struct {
	AutonomyLevel int                          `json:"autonomy_level"`
	EffectClass   string                       `json:"effect_class"`
	Reversibility string                       `json:"reversibility"`
	Default       string                       `json:"default"`
	Override      *string                      `json:"override"`
	Options       []EffectPolicyOptionResponse `json:"options"`
}

var effectPolicyDecisions = []string{
	connectedapps.PolicyAllow, connectedapps.PolicyConfirm, connectedapps.PolicyRehearse, connectedapps.PolicyBlock,
}

// effectPolicyCell builds one cell of the grid. The default is the table's decision with
// no run context, so the grid shows the policy itself, not one run's outcome.
func effectPolicyCell(level int, effect, rev string, overrides []service.Override) EffectPolicyCellResponse {
	out := EffectPolicyCellResponse{
		AutonomyLevel: level,
		EffectClass:   effect,
		Reversibility: rev,
		Default:       service.DecideEffect(level, effect, rev, nil, "", "", 0),
		Options:       make([]EffectPolicyOptionResponse, 0, len(effectPolicyDecisions)),
	}
	if d, ok := service.CellOverride(overrides, level, effect, rev); ok {
		out.Override = &d
	}
	for _, d := range effectPolicyDecisions {
		lock := service.OverrideLock(level, effect, rev, d)
		out.Options = append(out.Options, EffectPolicyOptionResponse{Decision: d, Allowed: lock == service.LockNone, Reason: lock})
	}
	return out
}

// GetEffectPolicy returns the policy grid for the workspace (GET /api/effects/policy).
// Cells come row by row, each row's four autonomy levels in order.
func (h *Handler) GetEffectPolicy(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	if h.durable.EffectPolicy == nil {
		writeError(w, http.StatusServiceUnavailable, "the effect policy is not configured")
		return
	}
	overrides, err := h.durable.EffectPolicy.Overrides(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load the effect policy")
		return
	}
	cells := make([]EffectPolicyCellResponse, 0, len(service.PolicyRows())*4)
	for _, row := range service.PolicyRows() {
		for level := 0; level <= 3; level++ {
			cells = append(cells, effectPolicyCell(level, row.Effect, row.Reversibility, overrides))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"cells": cells})
}

// SaveEffectPolicy saves one cell's decision (PUT /api/effects/policy). Only the owner or
// an admin may change the policy. A loosening the grid locks is refused with its reason as
// the code; a loosening that is allowed is recorded in the governance log by the store.
func (h *Handler) SaveEffectPolicy(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	if member.Role != "owner" && member.Role != "admin" {
		writeError(w, http.StatusForbidden, "only the owner or an admin can change the effect policy")
		return
	}
	var req struct {
		AutonomyLevel int    `json:"autonomy_level"`
		EffectClass   string `json:"effect_class"`
		Reversibility string `json:"reversibility"`
		Decision      string `json:"decision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if h.durable.EffectPolicy == nil {
		writeError(w, http.StatusServiceUnavailable, "the effect policy is not configured")
		return
	}
	if lock := service.OverrideLock(req.AutonomyLevel, req.EffectClass, req.Reversibility, req.Decision); lock != service.LockNone {
		writeErrorCode(w, http.StatusUnprocessableEntity, lock, "this decision is not allowed for that cell")
		return
	}
	override := service.Override{
		AutonomyLevel: req.AutonomyLevel,
		Effect:        req.EffectClass,
		Reversibility: req.Reversibility,
		Decision:      req.Decision,
	}
	if err := h.durable.EffectPolicy.Save(r.Context(), wsUUID, member.UserID, override); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save the effect policy")
		return
	}
	overrides, err := h.durable.EffectPolicy.Overrides(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load the effect policy")
		return
	}
	writeJSON(w, http.StatusOK, effectPolicyCell(req.AutonomyLevel, req.EffectClass, req.Reversibility, overrides))
}
