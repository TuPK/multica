package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/service"
)

// JuryVoteResponse is one juror's check on a criterion (durable spec 8.3).
type JuryVoteResponse struct {
	ID         string `json:"id"`
	VerifierID string `json:"verifier_id"`
	Result     string `json:"result"`
	Note       string `json:"note"`
	CreatedAt  string `json:"created_at"`
}

// JuryPanelResponse is a panel of one outcome. Votes stay empty until the panel has left the
// open state.
type JuryPanelResponse struct {
	ID             string             `json:"id"`
	CriterionID    string             `json:"criterion_id"`
	JurorAgentIDs  []string           `json:"juror_agent_ids"`
	Diversity      string             `json:"diversity"`
	State          string             `json:"state"`
	Verdict        *string            `json:"verdict"`
	ResolvedByType *string            `json:"resolved_by_type"`
	CreatedAt      string             `json:"created_at"`
	ClosedAt       *string            `json:"closed_at"`
	Votes          []JuryVoteResponse `json:"votes"`
}

func juryTextPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

// ListOutcomeJuryPanels returns the jury panels of an outcome (GET /api/outcomes/{id}/jury).
// It reads the same caller scope as the outcome itself.
func (h *Handler) ListOutcomeJuryPanels(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.outcomeCallerFor(w, r, workspaceID); !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "outcome id")
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	views, err := service.ListOutcomeJuryPanels(r.Context(), h.Queries, wsUUID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list jury panels")
		return
	}
	out := make([]JuryPanelResponse, 0, len(views))
	for _, v := range views {
		p := v.Panel
		jurors := make([]string, 0, len(p.JurorAgentIds))
		for _, j := range p.JurorAgentIds {
			jurors = append(jurors, uuidToString(j))
		}
		votes := make([]JuryVoteResponse, 0, len(v.Votes))
		for _, vote := range v.Votes {
			votes = append(votes, JuryVoteResponse{
				ID:         uuidToString(vote.ID),
				VerifierID: uuidToString(vote.VerifierID),
				Result:     vote.Result,
				Note:       vote.Note,
				CreatedAt:  timestampToString(vote.CreatedAt),
			})
		}
		out = append(out, JuryPanelResponse{
			ID:             uuidToString(p.ID),
			CriterionID:    uuidToString(p.CriterionID),
			JurorAgentIDs:  jurors,
			Diversity:      p.Diversity,
			State:          p.State,
			Verdict:        juryTextPtr(p.Verdict),
			ResolvedByType: juryTextPtr(p.ResolvedByType),
			CreatedAt:      timestampToString(p.CreatedAt),
			ClosedAt:       timestampPtr(p.ClosedAt),
			Votes:          votes,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"panels": out})
}
