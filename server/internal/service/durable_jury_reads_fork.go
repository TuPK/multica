package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// JuryPanelView is a panel of one outcome with its juror checks. Votes are empty while the
// panel is open: jurors vote blind to each other (spec 8.3), so a vote is shown only once the
// panel has left the open state.
type JuryPanelView struct {
	Panel db.JuryPanel
	Votes []db.OutcomeVerification
}

// ListOutcomeJuryPanels returns the jury panels of an outcome, oldest first. It reads only.
func ListOutcomeJuryPanels(ctx context.Context, q *db.Queries, ws, outcome pgtype.UUID) ([]JuryPanelView, error) {
	panels, err := q.ListJuryPanelsForOutcome(ctx, db.ListJuryPanelsForOutcomeParams{WorkspaceID: ws, OutcomeID: outcome})
	if err != nil {
		return nil, fmt.Errorf("list jury panels: %w", err)
	}
	out := make([]JuryPanelView, 0, len(panels))
	for _, p := range panels {
		view := JuryPanelView{Panel: p}
		if p.State != "open" && p.CriterionID.Valid {
			votes, err := q.ListJuryVotesForPanel(ctx, db.ListJuryVotesForPanelParams{
				WorkspaceID: ws, OutcomeID: outcome, CriterionID: p.CriterionID,
			})
			if err != nil {
				return nil, fmt.Errorf("list jury votes: %w", err)
			}
			view.Votes = votes
		}
		out = append(out, view)
	}
	return out, nil
}
