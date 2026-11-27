package service

import (
	"context"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func updateQoderIssueStatus(ctx context.Context, q *db.Queries, task db.AgentTaskQueue, params db.UpdateIssueStatusParams) (db.Issue, error) {
	if err := WithActorSession(ctx, q, GovernanceActor{Type: GovernanceActorAgent, ID: task.AgentID}, task.ID); err != nil {
		return db.Issue{}, err
	}
	return q.UpdateIssueStatus(ctx, params)
}
