package service

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// setInitiativeActorSession attributes initiative issue status transitions to
// the acting member or agent. System sweeps leave the session unset (system).
func setInitiativeActorSession(ctx context.Context, q *db.Queries, actor InitiativeActor) error {
	if (actor.Type != GovernanceActorMember && actor.Type != GovernanceActorAgent) || !actor.ID.Valid {
		return nil
	}
	return WithActorSession(ctx, q, GovernanceActor{Type: actor.Type, ID: actor.ID}, pgtype.UUID{})
}
