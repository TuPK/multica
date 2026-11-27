package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
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

// EnqueueJurorTask gives a juror its verification task and marks it with its panel in
// one transaction, so no daemon can claim the task before the panel context lands.
func (s *TaskService) EnqueueJurorTask(ctx context.Context, issue db.Issue, jurorID, outcomeID, panelID pgtype.UUID) (db.AgentTaskQueue, error) {
	var task db.AgentTaskQueue
	err := s.runInTx(ctx, func(q *db.Queries) error {
		var err error
		task, err = s.enqueueMentionTaskWithQueries(ctx, issue, jurorID, pgtype.UUID{}, nil, false, pgtype.UUID{}, false, "", pgtype.UUID{}, pgtype.UUID{}, OriginDerived, false, false, false, q)
		if err != nil {
			return err
		}
		n, err := q.SetJurorTaskContext(ctx, db.SetJurorTaskContextParams{
			ID: task.ID, OutcomeID: util.UUIDToString(outcomeID), JuryPanelID: util.UUIDToString(panelID),
		})
		if err != nil {
			return fmt.Errorf("mark juror task: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("juror task %s was claimed before it was marked", util.UUIDToString(task.ID))
		}
		task, err = q.GetAgentTask(ctx, task.ID)
		if err != nil {
			return fmt.Errorf("load juror task: %w", err)
		}
		return nil
	})
	if err != nil {
		return db.AgentTaskQueue{}, err
	}
	s.broadcastTaskEvent(ctx, protocol.EventTaskQueued, task)
	s.NotifyTaskEnqueued(ctx, task)
	return task, nil
}
