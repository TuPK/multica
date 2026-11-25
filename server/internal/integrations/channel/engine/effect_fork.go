package engine

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/util"
)

// replyTicket is one outbound reply the effect log recorded (durable spec 4.5, source
// channel). The zero ticket is a reply the log did not record.
type replyTicket struct {
	gate  connectedapps.EffectGate
	ws    pgtype.UUID
	event pgtype.UUID
}

// gateReply logs one outbound reply and reports whether it may be sent. A reply carries
// no task, so the shadow and paused rules do not reach it. The policy can only block it,
// and holding a reply would silence the agent, so every other decision is sent and
// logged. With no gate configured every reply is sent. A gate error holds the reply
// (fail closed).
func (r *Router) gateReply(ctx context.Context, inst ResolvedInstallation) (replyTicket, bool) {
	if r.effects == nil {
		return replyTicket{}, true
	}
	rec, err := r.effects.Gate(ctx, connectedapps.EffectRequest{
		WS:            inst.WorkspaceID,
		AgentID:       inst.AgentID,
		Source:        connectedapps.SourceChannel,
		Target:        "channel/" + util.UUIDToString(inst.ID),
		Class:         connectedapps.EffectSend,
		Reversibility: connectedapps.ReversibilityIrreversible,
		Summary:       "reply in a chat channel",
		Prior:         connectedapps.PolicyAllow,
	})
	if err != nil {
		r.logger.Error("channel router: effect gate failed; holding the reply", "error", err)
		return replyTicket{}, false
	}
	if rec.Decision == connectedapps.PolicyBlock {
		return replyTicket{}, false
	}
	return replyTicket{gate: r.effects, ws: inst.WorkspaceID, event: rec.EventID}, true
}

// settleReply records that the reply went out. The adapter reports no send error
// through the replier, so the event is executed. It uses a context that a closing
// router does not cancel, so the outcome is still written.
func (r *Router) settleReply(ctx context.Context, t replyTicket) {
	if t.gate == nil || !t.event.Valid {
		return
	}
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := t.gate.Finish(settleCtx, t.ws, t.event, "executed", nil); err != nil {
		r.logger.Warn("channel router: settle effect", "error", err)
	}
}
