package handler

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type flowActorCtxKey struct{}

type flowIssueActor struct {
	actor  service.GovernanceActor
	taskID pgtype.UUID
}

func withFlowActorContext(r *http.Request, actor flowActor) context.Context {
	ctx := r.Context()
	if actor.Type != service.GovernanceActorMember && actor.Type != service.GovernanceActorAgent {
		return ctx
	}
	id, err := util.ParseUUID(actor.ID)
	if err != nil {
		return ctx
	}
	var taskID pgtype.UUID
	if actor.Type == service.GovernanceActorAgent {
		if raw := r.Header.Get("X-Task-ID"); raw != "" {
			if parsed, err := util.ParseUUID(raw); err == nil {
				taskID = parsed
			}
		}
	}
	return context.WithValue(ctx, flowActorCtxKey{}, flowIssueActor{
		actor:  service.GovernanceActor{Type: actor.Type, ID: id},
		taskID: taskID,
	})
}

func withFlowActorValue(ctx context.Context, actor flowActor) context.Context {
	if _, ok := ctx.Value(flowActorCtxKey{}).(flowIssueActor); ok {
		return ctx
	}
	if actor.Type != service.GovernanceActorMember && actor.Type != service.GovernanceActorAgent {
		return ctx
	}
	id, err := util.ParseUUID(actor.ID)
	if err != nil {
		return ctx
	}
	return context.WithValue(ctx, flowActorCtxKey{}, flowIssueActor{
		actor: service.GovernanceActor{Type: actor.Type, ID: id},
	})
}

func withFlowMemberContext(ctx context.Context, memberID string) context.Context {
	if _, ok := ctx.Value(flowActorCtxKey{}).(flowIssueActor); ok || memberID == "" {
		return ctx
	}
	id, err := util.ParseUUID(memberID)
	if err != nil {
		return ctx
	}
	return context.WithValue(ctx, flowActorCtxKey{}, flowIssueActor{
		actor: service.GovernanceActor{Type: service.GovernanceActorMember, ID: id},
	})
}

func withFlowTaskActorContext(ctx context.Context, task *db.AgentTaskQueue) context.Context {
	if task == nil || !task.AgentID.Valid {
		return ctx
	}
	return context.WithValue(ctx, flowActorCtxKey{}, flowIssueActor{
		actor:  service.GovernanceActor{Type: service.GovernanceActorAgent, ID: task.AgentID},
		taskID: task.ID,
	})
}

func applyFlowActorSession(ctx context.Context, qtx *db.Queries) error {
	if fa, ok := ctx.Value(flowActorCtxKey{}).(flowIssueActor); ok {
		return service.WithActorSession(ctx, qtx, fa.actor, fa.taskID)
	}
	if wa, ok := ctx.Value(wakeupActorKey{}).(wakeupActor); ok && (wa.kind == service.GovernanceActorMember || wa.kind == service.GovernanceActorAgent) {
		id, err := util.ParseUUID(wa.id)
		if err != nil {
			return nil
		}
		var taskID pgtype.UUID
		if wa.task != "" {
			if parsed, err := util.ParseUUID(wa.task); err == nil {
				taskID = parsed
			}
		}
		return service.WithActorSession(ctx, qtx, service.GovernanceActor{Type: wa.kind, ID: id}, taskID)
	}
	return nil
}
