package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/attribution"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

// EnqueueIntentDecisionRun wakes an agent with the decision on one of its intents
// (durable spec 5.3). It is EnqueueAppDecisionRun for intents: the same guards and
// the same run, with the intent as the trigger evidence.
func (s *TaskService) EnqueueIntentDecisionRun(ctx context.Context, issueID, agentID, actorID, intentID pgtype.UUID, note string) error {
	issue, err := s.Queries.GetIssue(ctx, issueID)
	if err != nil {
		return fmt.Errorf("load issue: %w", err)
	}
	agent, err := s.Queries.GetAgent(ctx, agentID)
	if err != nil {
		return fmt.Errorf("load agent: %w", err)
	}
	if agent.ArchivedAt.Valid {
		return ErrChatTaskAgentArchived
	}
	if !agent.RuntimeID.Valid {
		return ErrChatTaskAgentNoRuntime
	}
	if err := s.guardRunProjectAccess(ctx, issue, agent, actorID, actorID); err != nil {
		return err
	}
	if err := guardIssueNotInTriage(ctx, s.Queries, issue.ID, OriginDerived); err != nil {
		return err
	}
	overlay := s.buildRuntimeMCPOverlay(ctx, actorID, agent)
	_, err = s.Queries.CreateWakeupTask(ctx, db.CreateWakeupTaskParams{
		ID:                   dbid.NewV7(),
		AgentID:              agent.ID,
		RuntimeID:            agent.RuntimeID,
		IssueID:              issue.ID,
		Priority:             priorityToInt(issue.Priority),
		TriggerSummary:       pgtype.Text{String: "Action intent decided", Valid: true},
		HandoffNote:          pgtype.Text{String: note, Valid: true},
		OriginatorUserID:     actorID,
		AccountableUserID:    actorID,
		OriginatorSource:     pgtype.Text{String: string(attribution.SourceDirectHuman), Valid: true},
		TriggerEvidenceKind:  pgtype.Text{String: "effect_intent", Valid: true},
		TriggerEvidenceRefID: intentID,
		RuntimeMcpOverlay:    overlay.Overlay,
		RuntimeConnectedApps: overlay.ConnectedApps,
	})
	if err != nil {
		return fmt.Errorf("create intent decision run: %w", err)
	}
	return nil
}
