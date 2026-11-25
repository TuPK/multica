package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/integrations/channel"
)

// recordingGate is an EffectGate that answers one decision and records what it was asked.
type recordingGate struct {
	decision string
	err      error
	gated    []connectedapps.EffectRequest
	finished []string
}

func (g *recordingGate) Gate(_ context.Context, in connectedapps.EffectRequest) (connectedapps.EffectRecord, error) {
	g.gated = append(g.gated, in)
	if g.err != nil {
		return connectedapps.EffectRecord{}, g.err
	}
	return connectedapps.EffectRecord{Decision: g.decision, EventID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}, nil
}

func (g *recordingGate) Finish(_ context.Context, _, _ pgtype.UUID, outcome string, _ *connectedapps.UndoSpec) error {
	g.finished = append(g.finished, outcome)
	return nil
}

// countingReplier counts the replies the adapter was asked to send.
type countingReplier struct{ replies int }

func (c *countingReplier) Reply(context.Context, ResolvedInstallation, channel.InboundMessage, Result) {
	c.replies++
}

func replyFixture(gate connectedapps.EffectGate) (*Router, ResolverSet, *countingReplier) {
	replier := &countingReplier{}
	r := NewRouter(nil, nil, nil, RouterConfig{Effects: gate})
	return r, ResolverSet{Replier: replier}, replier
}

// TestChannelReplyLogsOneEventAndSends: a reply the policy allows is logged once as a
// channel effect for the installation's agent, sent once, and settled executed.
func TestChannelReplyLogsOneEventAndSends(t *testing.T) {
	gate := &recordingGate{decision: connectedapps.PolicyAllow}
	r, set, replier := replyFixture(gate)
	inst := ResolvedInstallation{ID: pgtype.UUID{Bytes: [16]byte{7}, Valid: true}, WorkspaceID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}, AgentID: pgtype.UUID{Bytes: [16]byte{3}, Valid: true}}

	r.emitFlushReply(context.Background(), set, inst, channel.InboundMessage{}, pgtype.UUID{}, pgtype.UUID{}, 0, "")

	if len(gate.gated) != 1 || gate.gated[0].Source != connectedapps.SourceChannel || gate.gated[0].AgentID != inst.AgentID {
		t.Fatalf("gated %+v, want one channel request for the installation's agent", gate.gated)
	}
	if replier.replies != 1 {
		t.Fatalf("replies sent = %d, want 1", replier.replies)
	}
	if len(gate.finished) != 1 || gate.finished[0] != "executed" {
		t.Fatalf("settled %v, want executed once", gate.finished)
	}
}

// TestChannelReplyBlockedIsNotSent: a reply the policy blocks is not sent, and nothing
// is settled because the gate already wrote the event as blocked.
func TestChannelReplyBlockedIsNotSent(t *testing.T) {
	gate := &recordingGate{decision: connectedapps.PolicyBlock}
	r, set, replier := replyFixture(gate)

	r.emitFlushReply(context.Background(), set, ResolvedInstallation{}, channel.InboundMessage{}, pgtype.UUID{}, pgtype.UUID{}, 0, "")

	if replier.replies != 0 || len(gate.finished) != 0 {
		t.Fatalf("blocked reply: sent %d, settled %v; want nothing", replier.replies, gate.finished)
	}
}

// TestChannelReplyGateErrorHoldsReply: a gate failure holds the reply (fail closed).
func TestChannelReplyGateErrorHoldsReply(t *testing.T) {
	gate := &recordingGate{err: errors.New("db down")}
	r, set, replier := replyFixture(gate)

	r.emitFlushReply(context.Background(), set, ResolvedInstallation{}, channel.InboundMessage{}, pgtype.UUID{}, pgtype.UUID{}, 0, "")

	if replier.replies != 0 {
		t.Fatalf("reply sent after a gate error: %d", replier.replies)
	}
}

// TestChannelReplyWithoutGateSends: with no gate configured every reply is sent.
func TestChannelReplyWithoutGateSends(t *testing.T) {
	r, set, replier := replyFixture(nil)
	r.emitFlushReply(context.Background(), set, ResolvedInstallation{}, channel.InboundMessage{}, pgtype.UUID{}, pgtype.UUID{}, 0, "")
	if replier.replies != 1 {
		t.Fatalf("replies = %d, want 1", replier.replies)
	}
}
