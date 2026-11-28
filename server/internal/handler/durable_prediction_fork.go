package handler

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
)

// The Secretary's predictions over HTTP (durable spec 6.4). Only the workspace Secretary records a
// prediction; the service refuses anyone else.

// RecordPrincipalPrediction stores the Secretary's prediction for a pending question (POST
// /api/principal/predictions).
//
//	{member_id, subject_type, subject_id, decision_kind, predicted, confidence, why, trait_ids, task_id?}
func (h *Handler) RecordPrincipalPrediction(w http.ResponseWriter, r *http.Request) {
	c, ok := h.principalCallerFor(w, r, true)
	if !ok {
		return
	}
	var req struct {
		MemberID     string   `json:"member_id"`
		SubjectType  string   `json:"subject_type"`
		SubjectID    string   `json:"subject_id"`
		DecisionKind string   `json:"decision_kind"`
		Predicted    string   `json:"predicted"`
		Confidence   float64  `json:"confidence"`
		Why          string   `json:"why"`
		TraitIDs     []string `json:"trait_ids"`
		TaskID       string   `json:"task_id"`
	}
	if !decodeInitiativeBody(w, r, &req) {
		return
	}
	member, ok := parseUUIDOrBadRequest(w, req.MemberID, "member_id")
	if !ok {
		return
	}
	subject, ok := parseUUIDOrBadRequest(w, req.SubjectID, "subject_id")
	if !ok {
		return
	}
	traits := make([]pgtype.UUID, 0, len(req.TraitIDs))
	for _, raw := range req.TraitIDs {
		id, ok := parseUUIDOrBadRequest(w, raw, "trait_ids")
		if !ok {
			return
		}
		traits = append(traits, id)
	}
	task := pgtype.UUID{}
	if req.TaskID != "" {
		id, ok := parseUUIDOrBadRequest(w, req.TaskID, "task_id")
		if !ok {
			return
		}
		task = id
	}
	row, err := (&service.PredictionService{
		Queries:   h.Queries,
		TxStarter: h.TxStarter,
		Tasks:     h.TaskService,
		Flags:     h.FeatureFlags,
		Proposals: h.proposalService(),
		Intents:   h.durable.Intents,
		Checks:    h.outcomeService(),
	}).Record(r.Context(), c.actor, service.PredictionInput{
		WorkspaceID: c.ws, MemberID: member, SubjectType: req.SubjectType, SubjectID: subject,
		DecisionKind: req.DecisionKind, Predicted: req.Predicted, Confidence: req.Confidence,
		Why: req.Why, TraitIDs: traits, TaskID: task,
	})
	switch {
	case errors.Is(err, service.ErrPredictionLate):
		writeErrorCode(w, http.StatusConflict, "prediction_late", err.Error())
		return
	case errors.Is(err, service.ErrPredictionInvalid):
		writeErrorCode(w, http.StatusUnprocessableEntity, "prediction_invalid", err.Error())
		return
	case err != nil:
		writePrincipalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":           util.UUIDToString(row.ID),
		"subject_type": row.SubjectType,
		"subject_id":   util.UUIDToString(row.SubjectID),
		"predicted":    row.Predicted,
	})
}

// PrincipalPredictionResponse is the person's view of one prediction: what the Secretary said, why,
// and what the person decided once they did.
type PrincipalPredictionResponse struct {
	Predicted  string  `json:"predicted"`
	Confidence float64 `json:"confidence"`
	Why        string  `json:"why"`
	Actual     *string `json:"actual"`
	Matched    *bool   `json:"matched"`
}

// GetPrincipalPrediction reads the person's own prediction of one question (GET
// /api/principal/predictions?subject_type=&subject_id=). Only the person sees it; a question with no
// prediction answers 404.
func (h *Handler) GetPrincipalPrediction(w http.ResponseWriter, r *http.Request) {
	c, ok := h.principalCallerFor(w, r, true)
	if !ok {
		return
	}
	if !requirePrincipalPerson(w, c) {
		return
	}
	subject, ok := parseUUIDOrBadRequest(w, r.URL.Query().Get("subject_id"), "subject_id")
	if !ok {
		return
	}
	pred, err := (&service.PredictionService{Queries: h.Queries, Flags: h.FeatureFlags}).Prediction(
		r.Context(), c.ws, c.actor.ID, r.URL.Query().Get("subject_type"), subject)
	switch {
	case errors.Is(err, service.ErrPredictionInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, service.ErrPredictionNone):
		writeError(w, http.StatusNotFound, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "failed to read the prediction")
		return
	}
	confidence, _ := pred.Confidence.Float64Value()
	out := PrincipalPredictionResponse{Predicted: pred.Predicted, Confidence: confidence.Float64, Why: pred.Why}
	if pred.Actual.Valid {
		actual := pred.Actual.String
		out.Actual = &actual
	}
	if pred.Matched.Valid {
		matched := pred.Matched.Bool
		out.Matched = &matched
	}
	writeJSON(w, http.StatusOK, out)
}
