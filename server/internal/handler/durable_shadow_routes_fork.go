package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The shadow run over HTTP (durable spec 9.4 and 9.5). A person opens a run on a seat, after
// seeing its estimated cost. Every route answers 404 while durable_seats is off.

// Defaults of a shadow run (spec 9.4): a 30 percent sample, 20 compared pairs.
const (
	shadowDefaultSampleRate = 0.3
	shadowDefaultTarget     = 20
)

// ShadowRunRequest is what a person sends to open a shadow run.
type ShadowRunRequest struct {
	CandidateRuntimeID string  `json:"candidate_runtime_id"`
	CandidateModel     string  `json:"candidate_model"`
	SampleRate         float64 `json:"sample_rate"`
	TargetPairs        int     `json:"target_pairs"`
	WorkClassPrefix    string  `json:"work_class_prefix"`
	AutoPromote        bool    `json:"auto_promote"`
	// ConfirmCost is the person's yes to a run whose estimate exceeds the budget the seat has
	// left (spec 9.5). Without it such a run is refused with shadow_cost_confirmation_required.
	ConfirmCost bool `json:"confirm_cost"`
}

// ShadowEstimateResponse is a run's cost before it starts. RemainingMicros is -1 when no active
// budget covers the seat.
type ShadowEstimateResponse struct {
	EstimateMicros  int64 `json:"estimate_micros"`
	RemainingMicros int64 `json:"remaining_micros"`
	Warn            bool  `json:"warn"`
}

// ShadowRunResponse is a shadow run as the person sees it.
type ShadowRunResponse struct {
	ID                 string  `json:"id"`
	AgentID            string  `json:"agent_id"`
	CandidateRuntimeID string  `json:"candidate_runtime_id"`
	CandidateModel     string  `json:"candidate_model"`
	SampleRate         float64 `json:"sample_rate"`
	TargetPairs        int32   `json:"target_pairs"`
	AutoPromote        bool    `json:"auto_promote"`
	State              string  `json:"state"`
}

// shadowRunParams reads a run's sample rate and target from the query, or the defaults.
func shadowRunParams(rawRate, rawTarget string) (float64, int, bool) {
	rate, target := float64(shadowDefaultSampleRate), shadowDefaultTarget
	if rawRate != "" {
		v, err := strconv.ParseFloat(rawRate, 64)
		if err != nil || v <= 0 || v > 1 {
			return 0, 0, false
		}
		rate = v
	}
	if rawTarget != "" {
		v, err := strconv.Atoi(rawTarget)
		if err != nil || v < 5 || v > 200 {
			return 0, 0, false
		}
		target = v
	}
	return rate, target, true
}

// GetShadowEstimate prices a shadow run for a seat (GET /api/agents/{id}/shadow/estimate).
func (h *Handler) GetShadowEstimate(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.seatDossierCaller(w, r)
	if !ok {
		return
	}
	seat, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "agent id")
	if !ok {
		return
	}
	rate, target, ok := shadowRunParams(r.URL.Query().Get("sample_rate"), r.URL.Query().Get("target_pairs"))
	if !ok {
		writeError(w, http.StatusBadRequest, "sample_rate must be in (0,1] and target_pairs 5 to 200")
		return
	}
	est, err := h.shadowService().Estimate(r.Context(), ws, seat, rate, target)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to estimate the shadow run")
		return
	}
	writeJSON(w, http.StatusOK, ShadowEstimateResponse{EstimateMicros: est.Micros, RemainingMicros: est.RemainingMicros, Warn: est.Warn})
}

// StartShadowRun opens a shadow run on a seat (POST /api/agents/{id}/shadow). Only a person
// starts one; auto-promotion is for an owner or an admin (checked by the service).
func (h *Handler) StartShadowRun(w http.ResponseWriter, r *http.Request) {
	ws, actor, ok := h.seatDossierCaller(w, r)
	if !ok {
		return
	}
	seat, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "agent id")
	if !ok {
		return
	}
	var req ShadowRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	candidate, ok := parseUUIDOrBadRequest(w, req.CandidateRuntimeID, "candidate_runtime_id")
	if !ok {
		return
	}
	if req.SampleRate == 0 {
		req.SampleRate = shadowDefaultSampleRate
	}
	if req.TargetPairs == 0 {
		req.TargetPairs = shadowDefaultTarget
	}
	svc := h.shadowService()
	est, err := svc.Estimate(r.Context(), ws, seat, req.SampleRate, req.TargetPairs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to estimate the shadow run")
		return
	}
	if est.Warn && !req.ConfirmCost {
		writeErrorCode(w, http.StatusConflict, "shadow_cost_confirmation_required",
			"the estimated cost of this shadow run is more than the budget the seat has left; confirm it to start")
		return
	}
	cfg, err := svc.Start(r.Context(), ws, actor, service.ShadowStartInput{
		AgentID: seat, CandidateRuntimeID: candidate, CandidateModel: req.CandidateModel,
		Purpose: service.ShadowPurposeModelTrial, WorkClassPrefix: req.WorkClassPrefix,
		SampleRate: req.SampleRate, TargetPairs: req.TargetPairs, AutoPromote: req.AutoPromote,
	})
	if err != nil {
		writeShadowStartError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, shadowRunResponse(cfg))
}

// writeShadowStartError maps a refused start to its status and code.
func writeShadowStartError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrShadowAlreadyRunning):
		writeErrorCode(w, http.StatusConflict, "shadow_already_running", err.Error())
	case errors.Is(err, service.ErrShadowInvalid):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, service.ErrGovernanceActorNotBoss):
		writeErrorCode(w, http.StatusForbidden, "auto_promote_owner_only", "only an owner or an admin may turn auto-promotion on")
	default:
		writeError(w, http.StatusInternalServerError, "failed to start the shadow run")
	}
}

func shadowRunResponse(c db.ShadowConfig) ShadowRunResponse {
	rate, _ := c.SampleRate.Float64Value()
	return ShadowRunResponse{
		ID: uuidToString(c.ID), AgentID: uuidToString(c.AgentID), CandidateRuntimeID: uuidToString(c.CandidateRuntimeID),
		CandidateModel: c.CandidateModel, SampleRate: rate.Float64, TargetPairs: c.TargetPairs,
		AutoPromote: c.AutoPromote, State: c.State,
	}
}
