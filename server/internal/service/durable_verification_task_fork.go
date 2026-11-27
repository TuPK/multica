package service

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// EnqueueVerificationTask gives verifier a check on issue (durable spec 2.5). It takes the
// path a mention of an agent takes, so the pending slot, the attribution and the lock
// order are the existing ones. The caller marks the task as a verification task.
func (s *TaskService) EnqueueVerificationTask(ctx context.Context, issue db.Issue, verifierID pgtype.UUID) (db.AgentTaskQueue, error) {
	return s.enqueueMentionTask(ctx, issue, verifierID, pgtype.UUID{}, false, pgtype.UUID{}, false, "", pgtype.UUID{}, pgtype.UUID{}, OriginDerived)
}

func (s *TaskService) enqueueVerificationTaskTx(ctx context.Context, q *db.Queries, issue db.Issue, verifierID pgtype.UUID) (db.AgentTaskQueue, error) {
	return s.enqueueMentionTaskWithQueries(ctx, issue, verifierID, pgtype.UUID{}, nil, false, pgtype.UUID{}, false, "", pgtype.UUID{}, pgtype.UUID{}, OriginDerived, false, false, true, q)
}
