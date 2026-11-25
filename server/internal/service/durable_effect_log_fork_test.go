package service

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// effectService returns an EffectService over the fixture and the provider that
// decides durable_effects, so a test can turn the flag on or off.
func effectService(t *testing.T, f principalFixture, flagOn bool) (*EffectService, *featureflag.StaticProvider) {
	t.Helper()
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableEffects, featureflag.Rule{Default: flagOn})
	svc := &EffectService{Queries: f.q, Flags: featureflag.NewService(provider)}
	return svc, provider
}

// effectFixture is a workspace with one agent, one task on it and the event rows
// the test wrote. The rows are removed when the test ends.
type effectFixture struct {
	principalFixture
	agentID string
	taskID  string
}

func newEffectFixture(t *testing.T, taskContext string) effectFixture {
	t.Helper()
	f, _ := companyWorkspace(t)
	f.Cleanup(t, `DELETE FROM effect_event WHERE workspace_id = $1`, f.WorkspaceID)
	runtimeID := f.Runtime(t, "effect runtime")
	agentID := f.Agent(t, "effect agent", runtimeID)
	cols := testutil.Cols{"runtime_id": runtimeID}
	if taskContext != "" {
		cols["context"] = testutil.Raw("'" + taskContext + "'::jsonb")
	}
	taskID := f.Task(t, agentID, cols)
	return effectFixture{principalFixture: f, agentID: agentID, taskID: taskID}
}

func (fx effectFixture) request(source, class, rev string) EffectInput {
	return EffectInput{
		WS:            util.MustParseUUID(fx.WorkspaceID),
		TaskID:        util.MustParseUUID(fx.taskID),
		AgentID:       util.MustParseUUID(fx.agentID),
		Source:        source,
		Target:        "test/target",
		Class:         class,
		Reversibility: rev,
		Summary:       "test action",
		Prior:         connectedapps.PolicyAllow,
	}
}

func (fx effectFixture) eventCount(t *testing.T, where string, args ...any) int {
	t.Helper()
	all := append([]any{fx.WorkspaceID}, args...)
	return fx.Count(t, `SELECT count(*) FROM effect_event WHERE workspace_id = $1 AND `+where, all...)
}

// TestEachSourceLogsOneEvent: every source in spec 4.5 except declared writes exactly
// one event per action, and a blocked action is logged too.
func TestEachSourceLogsOneEvent(t *testing.T) {
	sources := []string{
		connectedapps.SourceConnectedApp, connectedapps.SourcePluginAction, connectedapps.SourceCLI,
		connectedapps.SourceChannel, connectedapps.SourceWebhook, connectedapps.SourcePublishChannel,
	}
	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			fx := newEffectFixture(t, "")
			svc, _ := effectService(t, fx.principalFixture, true)
			ctx := context.Background()

			got, err := svc.Gate(ctx, fx.request(src, connectedapps.EffectWriteInternal, connectedapps.ReversibilityReversible))
			if err != nil {
				t.Fatalf("Gate allowed action: %v", err)
			}
			if got.Decision != connectedapps.PolicyAllow || !got.EventID.Valid {
				t.Fatalf("allowed action: decision %q, event valid %v", got.Decision, got.EventID.Valid)
			}

			// A shadow task may only read, so its send is blocked (not rehearsed).
			shadow := newEffectFixture(t, `{"durable_kind":"shadow"}`)
			svc2, _ := effectService(t, shadow.principalFixture, true)
			got2, err := svc2.Gate(ctx, shadow.request(src, connectedapps.EffectSend, connectedapps.ReversibilityIrreversible))
			if err != nil {
				t.Fatalf("Gate blocked action: %v", err)
			}
			if got2.Decision != connectedapps.PolicyBlock {
				t.Fatalf("shadow send decision %q, want block", got2.Decision)
			}

			if n := fx.eventCount(t, `source = $2`, src); n != 1 {
				t.Fatalf("allowed action logged %d events for %s, want 1", n, src)
			}
			if n := shadow.eventCount(t, `source = $2 AND status = 'blocked'`, src); n != 1 {
				t.Fatalf("blocked action logged %d blocked events for %s, want 1", n, src)
			}
		})
	}
}

// TestShadowTaskGatewayWriteBlocked: a shadow task is blocked from every write, even
// at a level that would allow it (spec 4.3, D18).
func TestShadowTaskGatewayWriteBlocked(t *testing.T) {
	fx := newEffectFixture(t, `{"durable_kind":"shadow"}`)
	svc, _ := effectService(t, fx.principalFixture, true)
	ctx := context.Background()

	for _, class := range []string{connectedapps.EffectWrite, connectedapps.EffectWriteInternal, connectedapps.EffectSend} {
		rev := connectedapps.DefaultReversibility(class)
		got, err := svc.Gate(ctx, fx.request(connectedapps.SourceConnectedApp, class, rev))
		if err != nil {
			t.Fatalf("Gate %s: %v", class, err)
		}
		if got.Decision != connectedapps.PolicyBlock {
			t.Fatalf("shadow %s decision %q, want block", class, got.Decision)
		}
	}
	got, err := svc.Gate(ctx, fx.request(connectedapps.SourceConnectedApp, connectedapps.EffectRead, ""))
	if err != nil {
		t.Fatalf("Gate read: %v", err)
	}
	if got.Decision != connectedapps.PolicyAllow {
		t.Fatalf("shadow read decision %q, want allow", got.Decision)
	}
}

// TestGateFlagOffWritesNothing: while durable_effects is off the gate returns the
// caller's own decision and writes no event.
func TestGateFlagOffWritesNothing(t *testing.T) {
	fx := newEffectFixture(t, "")
	svc, _ := effectService(t, fx.principalFixture, false)

	got, err := svc.Gate(context.Background(), fx.request(connectedapps.SourceCLI, connectedapps.EffectSend, connectedapps.ReversibilityIrreversible))
	if err != nil {
		t.Fatalf("Gate: %v", err)
	}
	if got.Decision != connectedapps.PolicyAllow || got.EventID.Valid {
		t.Fatalf("flag off: decision %q, event valid %v; want the prior allow and no event", got.Decision, got.EventID.Valid)
	}
	if n := fx.eventCount(t, `true`); n != 0 {
		t.Fatalf("flag off wrote %d events", n)
	}
}

// TestGateRehearseAndFinish: a rehearsed send waits as awaiting. An allowed write
// goes awaiting, then Finish settles it once, with its undo, and a second Finish is
// refused.
func TestGateRehearseAndFinish(t *testing.T) {
	fx := newEffectFixture(t, "")
	svc, _ := effectService(t, fx.principalFixture, true)
	ctx := context.Background()

	// At L1 the agent has no squad, so a send rehearses.
	got, err := svc.Gate(ctx, fx.request(connectedapps.SourceConnectedApp, connectedapps.EffectSend, connectedapps.ReversibilityIrreversible))
	if err != nil {
		t.Fatalf("Gate send: %v", err)
	}
	if got.Decision != connectedapps.PolicyRehearse {
		t.Fatalf("send at L1 decision %q, want rehearse", got.Decision)
	}
	if n := fx.eventCount(t, `status = 'awaiting' AND effect_class = 'send'`); n != 1 {
		t.Fatalf("rehearsed send: %d awaiting rows, want 1", n)
	}

	write, err := svc.Gate(ctx, fx.request(connectedapps.SourceConnectedApp, connectedapps.EffectWrite, connectedapps.ReversibilityReversible))
	if err != nil {
		t.Fatalf("Gate write: %v", err)
	}
	if write.Decision != connectedapps.PolicyAllow {
		t.Fatalf("reversible write at L1 decision %q, want allow", write.Decision)
	}
	undo := &connectedapps.UndoSpec{Tool: "GMAIL_DELETE_DRAFT", Args: map[string]any{"draft_id": "d-1"}}
	if err := svc.Finish(ctx, util.MustParseUUID(fx.WorkspaceID), write.EventID, "executed", undo); err != nil {
		t.Fatalf("Finish executed: %v", err)
	}
	if n := fx.eventCount(t, `id = $2 AND status = 'executed' AND undo IS NOT NULL AND undo_until > now()`, util.UUIDToString(write.EventID)); n != 1 {
		t.Fatalf("finished write: %d executed rows with undo, want 1", n)
	}
	if err := svc.Finish(ctx, util.MustParseUUID(fx.WorkspaceID), write.EventID, "failed", nil); err == nil {
		t.Fatal("second Finish: want an error for an event already settled")
	}
}
