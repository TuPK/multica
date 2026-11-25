package connectedapps

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
)

// effectTicket is one gated call: the decision to enforce, and the event and undo
// to settle once the call has run.
type effectTicket struct {
	policy string
	ws     pgtype.UUID
	event  pgtype.UUID
	undo   *UndoSpec
	// req is the request the gate decided on, kept for an intent a rehearsal makes.
	req EffectRequest
}

// gateEffect logs one tool call with the effect gate and returns the decision to
// enforce (durable spec 4.4). prior is the account and agent decision the gateway
// already reached. With no gate wired, prior stands. A gate error fails closed: the
// call is blocked and no event is written.
func (s *Service) gateEffect(ctx context.Context, rn *run, toolkit, tool, class, summary, prior string) effectTicket {
	if s.effects == nil {
		return effectTicket{policy: prior, ws: rn.workspaceID}
	}
	effect := MapAppClass(class)
	rev, undo := ToolEffect(toolkit, tool, effect)
	req := EffectRequest{
		WS:            rn.workspaceID,
		TaskID:        rn.task.ID,
		AgentID:       rn.agent.ID,
		MemberID:      rn.triggeredBy,
		IssueID:       rn.task.IssueID,
		Source:        SourceConnectedApp,
		Target:        toolkit + "/" + tool,
		Class:         effect,
		Reversibility: rev,
		Summary:       summary,
		Undo:          undo,
		Prior:         prior,
	}
	rec, err := s.effects.Gate(ctx, req)
	if err != nil {
		slog.Error("effect gate failed; blocking the app call", "error", err, "toolkit", toolkit, "tool", tool)
		return effectTicket{policy: PolicyBlock, ws: rn.workspaceID}
	}
	return effectTicket{policy: rec.Decision, ws: rn.workspaceID, event: rec.EventID, undo: undo, req: req}
}

// settleEffect records the outcome of a gated call: executed (with its undo, resolved
// from the result), failed, or blocked when a later refusal stopped it. It does
// nothing for a ticket with no event.
func (s *Service) settleEffect(ctx context.Context, t effectTicket, outcome string, result map[string]any) {
	if s.effects == nil || !t.event.Valid {
		return
	}
	var undo *UndoSpec
	if outcome == "executed" {
		undo = ResolveUndo(t.undo, result)
	}
	if err := s.effects.Finish(ctx, t.ws, t.event, outcome, undo); err != nil {
		slog.Error("effect settle failed", "error", err, "outcome", outcome)
	}
}

// settleRefusal records a gated call that an later check stopped. Only an allowed
// call still awaits an outcome here: a blocked one is already settled, and a
// rehearsed or confirmed one waits for the person.
func (s *Service) settleRefusal(ctx context.Context, t effectTicket) {
	if t.policy == PolicyAllow {
		s.settleEffect(ctx, t, "blocked", nil)
	}
}
