package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// NecessityTestRequest starts a necessity test on one skill the seat holds.
type NecessityTestRequest struct {
	SkillID string `json:"skill_id"`
}

// NecessityTestResponse is one necessity test of a seat (durable spec 10.2).
type NecessityTestResponse struct {
	ID          string          `json:"id"`
	SubjectType string          `json:"subject_type"`
	SubjectRef  string          `json:"subject_ref"`
	AgentID     string          `json:"agent_id"`
	ShadowRunID string          `json:"shadow_run_id,omitempty"`
	State       string          `json:"state"`
	Result      json.RawMessage `json:"result"`
	ProposalID  string          `json:"proposal_id,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

func necessityTestResponseOf(t db.NecessityTest) NecessityTestResponse {
	out := NecessityTestResponse{
		ID: uuidToString(t.ID), SubjectType: t.SubjectType, SubjectRef: t.SubjectRef,
		AgentID: uuidToString(t.AgentID), State: t.State, Result: json.RawMessage(t.Result),
		CreatedAt: t.CreatedAt.Time,
	}
	if t.ShadowConfigID.Valid {
		out.ShadowRunID = uuidToString(t.ShadowConfigID)
	}
	if t.ProposalID.Valid {
		out.ProposalID = uuidToString(t.ProposalID)
	}
	if t.CompletedAt.Valid {
		at := t.CompletedAt.Time
		out.CompletedAt = &at
	}
	return out
}

// ListNecessityTests returns a seat's recent necessity tests (GET /agents/{id}/necessity). Members read them.
func (h *Handler) ListNecessityTests(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.seatDossierCaller(w, r)
	if !ok {
		return
	}
	seat, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "agent id")
	if !ok {
		return
	}
	tests, err := h.NecessityService().ListForSeat(r.Context(), ws, seat)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list necessity tests")
		return
	}
	out := make([]NecessityTestResponse, 0, len(tests))
	for _, t := range tests {
		out = append(out, necessityTestResponseOf(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tests": out})
}

// StartNecessityTest opens a necessity test on one skill of the seat (POST /agents/{id}/necessity).
// It is a person's decision (durable spec 10.3); the weekly job opens the same tests on its schedule.
func (h *Handler) StartNecessityTest(w http.ResponseWriter, r *http.Request) {
	ws, actor, ok := h.seatDossierCaller(w, r)
	if !ok {
		return
	}
	seat, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "agent id")
	if !ok {
		return
	}
	var req NecessityTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	skill, ok := parseUUIDOrBadRequest(w, req.SkillID, "skill_id")
	if !ok {
		return
	}
	test, err := h.NecessityService().Start(r.Context(), ws, actor, seat, skill)
	if err != nil {
		writeNecessityStartError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, necessityTestResponseOf(test))
}

// writeNecessityStartError maps a refused start to its status and code.
func writeNecessityStartError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrNecessityRunning):
		writeErrorCode(w, http.StatusConflict, "necessity_test_running", err.Error())
	case errors.Is(err, service.ErrShadowAlreadyRunning):
		writeErrorCode(w, http.StatusConflict, "shadow_already_running", err.Error())
	case errors.Is(err, service.ErrNecessityInvalid):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "failed to start the necessity test")
	}
}
