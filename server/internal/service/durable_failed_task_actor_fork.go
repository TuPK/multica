package service

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func (s *TaskService) updateFailedTaskIssueStatus(ctx context.Context, agentID pgtype.UUID, params db.UpdateIssueStatusParams) (db.Issue, error) {
	actor := GovernanceActor{Type: GovernanceActorSystem}
	if agentID.Valid {
		actor = GovernanceActor{Type: GovernanceActorAgent, ID: agentID}
	}
	var issue db.Issue
	err := s.runInTx(ctx, func(q *db.Queries) error {
		if err := WithActorSession(ctx, q, actor, params.SourceTaskID); err != nil {
			return err
		}
		var err error
		issue, err = q.UpdateIssueStatus(ctx, params)
		return err
	})
	return issue, err
}
