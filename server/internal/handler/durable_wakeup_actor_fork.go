package handler

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
)

func (h *Handler) setWakeupActorSession(ctx context.Context, tx pgx.Tx, actor wakeupActor) error {
	if _, err := tx.Exec(ctx, `SELECT set_config('multica.source_task_id',$1,true)`, actor.task); err != nil {
		return err
	}
	var id, task pgtype.UUID
	var err error
	if actor.id != "" {
		if id, err = util.ParseUUID(actor.id); err != nil {
			return err
		}
	}
	if actor.task != "" {
		if task, err = util.ParseUUID(actor.task); err != nil {
			return err
		}
	}
	return service.WithActorSession(ctx, h.Queries.WithTx(tx), service.GovernanceActor{Type: actor.kind, ID: id}, task)
}
