package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/multica-ai/multica/server/internal/featureflags"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// attentionJuryAsker puts a split or escalated jury panel to the decider through
// the attention ledger (durable spec 7.3, 8.3). It reads the jury service's flags
// and clock on each call so callers can wire them after NewJuryService.
type attentionJuryAsker struct {
	flags func() *featureflag.Service
	now   func() time.Time
}

func (a *attentionJuryAsker) Ask(ctx context.Context, q *db.Queries, panel db.JuryPanel, reason string) error {
	var flags *featureflag.Service
	if a.flags != nil {
		flags = a.flags()
	}
	if flags == nil || !featureflags.DurableSeatsEnabled(ctx, flags) || !featureflags.DurablePrincipalEnabled(ctx, flags) {
		return nil
	}
	outcome, err := q.GetWorkOutcomeInWorkspace(ctx, db.GetWorkOutcomeInWorkspaceParams{
		ID: panel.OutcomeID, WorkspaceID: panel.WorkspaceID,
	})
	if err != nil {
		return fmt.Errorf("load outcome: %w", err)
	}
	urgent := false
	issue, err := q.GetIssue(ctx, outcome.IssueID)
	if err == nil {
		urgent = issue.Priority == "urgent"
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("load issue: %w", err)
	}
	if reason == "" {
		reason = juryAskReason
	}
	att := &AttentionService{Queries: q, Flags: flags, Now: a.now}
	_, err = att.Admit(ctx, AskInput{
		WorkspaceID: panel.WorkspaceID,
		SquadID:     outcome.SquadID,
		SubjectType: "outcome_check",
		SubjectID:   panel.OutcomeID,
		Reason:      reason,
		Urgent:      urgent,
	})
	return err
}
