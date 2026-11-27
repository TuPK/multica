package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/service"
)

// Delegation by prediction over HTTP (durable spec 6.5). Only the person turns a delegation on, off,
// or reverts a delegated decision. Each route answers 404 while durable_principal is off.

// PrincipalDelegationResponse is one decision kind's delegation: on or off, and why it went off.
type PrincipalDelegationResponse struct {
	DecisionKind   string  `json:"decision_kind"`
	Enabled        bool    `json:"enabled"`
	MinConfidence  float64 `json:"min_confidence"`
	EnabledAt      string  `json:"enabled_at"`
	DisabledReason string  `json:"disabled_reason"`
}

// PrincipalKindAccuracyResponse is a decision kind's match record and whether it may be delegated.
type PrincipalKindAccuracyResponse struct {
	DecisionKind string  `json:"decision_kind"`
	Matched      int     `json:"matched"`
	N            int     `json:"n"`
	Lower        float64 `json:"lower"`
	Eligible     bool    `json:"eligible"`
}

// GetPrincipalDelegations reads the person's delegations and the accuracy of each kind they have
// predictions of, for the delegation panel (GET /api/principal/delegations). Only the person reads it.
func (h *Handler) GetPrincipalDelegations(w http.ResponseWriter, r *http.Request) {
	c, ok := h.principalCallerFor(w, r, true)
	if !ok {
		return
	}
	if !requirePrincipalPerson(w, c) {
		return
	}
	delegations, accuracy, err := h.delegationService().Status(r.Context(), c.ws, c.actor.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read the delegations")
		return
	}
	out := struct {
		Delegations []PrincipalDelegationResponse   `json:"delegations"`
		Kinds       []PrincipalKindAccuracyResponse `json:"kinds"`
	}{Delegations: make([]PrincipalDelegationResponse, 0, len(delegations)), Kinds: make([]PrincipalKindAccuracyResponse, 0, len(accuracy))}
	for _, d := range delegations {
		minConfidence, _ := d.MinConfidence.Float64Value()
		out.Delegations = append(out.Delegations, PrincipalDelegationResponse{
			DecisionKind: d.DecisionKind, Enabled: !d.DisabledAt.Valid, MinConfidence: minConfidence.Float64,
			EnabledAt: timestampToString(d.EnabledAt), DisabledReason: d.DisabledReason,
		})
	}
	for _, k := range accuracy {
		out.Kinds = append(out.Kinds, PrincipalKindAccuracyResponse{
			DecisionKind: k.DecisionKind, Matched: k.Matched, N: k.N, Lower: k.Lower, Eligible: k.Eligible,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// EnablePrincipalDelegation delegates one decision kind to the predictor (PUT
// /api/principal/delegations). The person is checked as the caller, and RequireHumanActor refuses agents.
//
//	{decision_kind, min_confidence}
func (h *Handler) EnablePrincipalDelegation(w http.ResponseWriter, r *http.Request) {
	c, ok := h.principalCallerFor(w, r, true)
	if !ok {
		return
	}
	var req struct {
		DecisionKind  string  `json:"decision_kind"`
		MinConfidence float64 `json:"min_confidence"`
	}
	if !decodeInitiativeBody(w, r, &req) {
		return
	}
	err := h.delegationService().Enable(r.Context(), c.actor, c.ws, c.actor.ID, req.DecisionKind, req.MinConfidence)
	switch {
	case errors.Is(err, service.ErrDelegationNotEligible):
		writeErrorCode(w, http.StatusUnprocessableEntity, "delegation_not_eligible", err.Error())
		return
	case errors.Is(err, service.ErrDelegationForbiddenKind):
		writeErrorCode(w, http.StatusUnprocessableEntity, "delegation_forbidden_kind", err.Error())
		return
	case err != nil:
		writePrincipalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"decision_kind": req.DecisionKind, "enabled": true})
}

// DisablePrincipalDelegation turns a delegation off at the person's word (DELETE
// /api/principal/delegations?decision_kind=).
func (h *Handler) DisablePrincipalDelegation(w http.ResponseWriter, r *http.Request) {
	c, ok := h.principalCallerFor(w, r, true)
	if !ok {
		return
	}
	if err := h.delegationService().Disable(r.Context(), c.actor, c.ws, c.actor.ID, r.URL.Query().Get("decision_kind")); err != nil {
		writePrincipalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RevertDelegatedDecision undoes a delegated decision within 72 hours (POST
// /api/principal/decisions/{id}/revert). Two reverts in the last twenty turn the delegation off.
func (h *Handler) RevertDelegatedDecision(w http.ResponseWriter, r *http.Request) {
	c, ok := h.principalCallerFor(w, r, true)
	if !ok {
		return
	}
	predID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "decision id")
	if !ok {
		return
	}
	err := h.delegationService().OnReverted(r.Context(), c.ws, c.actor, predID)
	switch {
	case errors.Is(err, service.ErrDelegationRevertWindow):
		writeErrorCode(w, http.StatusUnprocessableEntity, "delegation_revert_window", err.Error())
		return
	case err != nil:
		writePrincipalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) delegationService() *service.DelegationService {
	return &service.DelegationService{Queries: h.Queries, Flags: h.FeatureFlags, Predictions: &service.PredictionService{Queries: h.Queries, Flags: h.FeatureFlags}}
}
