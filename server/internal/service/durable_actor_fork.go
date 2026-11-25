package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// WithActorSession sets who changes issues for the rest of q's transaction. The
// issue status trigger reads these values when it writes a transition row
// (migration 1044), and records system when none are set.
//
// q must be bound to a transaction (db.New(tx) or q.WithTx(tx)). Outside one,
// SET LOCAL ends at once, so this reads the actor type back and returns an error
// if it did not stick. A missing check would record the change as system.
func WithActorSession(ctx context.Context, q *db.Queries, actor GovernanceActor, taskID pgtype.UUID) error {
	var actorID, task string
	if actor.ID.Valid {
		actorID = util.UUIDToString(actor.ID)
	}
	if taskID.Valid {
		task = util.UUIDToString(taskID)
	}
	if err := q.SetIssueActorSession(ctx, db.SetIssueActorSessionParams{
		ActorType: actor.Type,
		ActorID:   actorID,
		TaskID:    task,
	}); err != nil {
		return fmt.Errorf("set issue actor session: %w", err)
	}
	got, err := q.CurrentIssueActorType(ctx)
	if err != nil {
		return fmt.Errorf("read issue actor session: %w", err)
	}
	if got != actor.Type {
		return errors.New("WithActorSession: q is not bound to a transaction, so the actor session ended at once")
	}
	return nil
}
