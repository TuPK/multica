package service

import (
	"context"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// holdFixture is an effect fixture with the intent service on, and an issue for the
// flow request. The intents of the issue are removed at cleanup.
func holdFixture(t *testing.T, on bool) (effectFixture, *IntentService, string) {
	t.Helper()
	fx := newEffectFixture(t, "")
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableEffects, featureflag.Rule{Default: on})
	intents := &IntentService{Queries: fx.q, TxStarter: fx.Pool, Flags: featureflag.NewService(provider)}
	issue := fx.Issue(t, "flow request")
	t.Cleanup(func() {
		fx.Cleanup(t, `DELETE FROM effect_intent WHERE issue_id = $1`, issue)
	})
	return fx, intents, issue
}

// pendingHeld counts the intents a flow task of the issue holds, by state.
func pendingHeld(t *testing.T, fx effectFixture, issue, target, state string) int {
	t.Helper()
	var n int
	if err := fx.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM effect_intent WHERE issue_id = $1 AND target = $2 AND state = $3`,
		issue, target, state).Scan(&n); err != nil {
		t.Fatalf("count intents: %v", err)
	}
	return n
}

// TestHoldFlowTaskOpensOnlyAfterApproval: an external flow task is held by one declared
// intent per round, naming its step and task. It opens only once that intent is approved,
// and a later round holds again (durable spec 5.4). Approval runs nothing itself.
func TestHoldFlowTaskOpensOnlyAfterApproval(t *testing.T) {
	fx, intents, issue := holdFixture(t, true)
	ctx := context.Background()
	ws := util.MustParseUUID(fx.WorkspaceID)
	const target = "flow/draft.send_quote#0"

	open, err := intents.HoldFlowTask(ctx, ws, util.MustParseUUID(issue), "draft", "send_quote", 0)
	if err != nil || open {
		t.Fatalf("first hold: open %v err %v, want held", open, err)
	}
	if n := pendingHeld(t, fx, issue, target, "pending"); n != 1 {
		t.Fatalf("pending intents for %s = %d, want 1", target, n)
	}

	open, err = intents.HoldFlowTask(ctx, ws, util.MustParseUUID(issue), "draft", "send_quote", 0)
	if err != nil || open {
		t.Fatalf("second hold before approval: open %v err %v, want still held", open, err)
	}
	if n := pendingHeld(t, fx, issue, target, "pending"); n != 1 {
		t.Fatalf("a repeated hold made %d pending intents, want 1", n)
	}

	var id string
	if err := fx.Pool.QueryRow(ctx, `SELECT id::text FROM effect_intent WHERE issue_id = $1 AND target = $2`, issue, target).Scan(&id); err != nil {
		t.Fatalf("load intent: %v", err)
	}
	if _, err := intents.Approve(ctx, ws, util.MustParseUUID(fx.UserID), util.MustParseUUID(id), nil, "go"); err != nil {
		t.Fatalf("Approve declared: %v", err)
	}
	open, err = intents.HoldFlowTask(ctx, ws, util.MustParseUUID(issue), "draft", "send_quote", 0)
	if err != nil || !open {
		t.Fatalf("hold after approval: open %v err %v, want open", open, err)
	}

	// A new round of the same task is a new piece of work: it holds again.
	open, err = intents.HoldFlowTask(ctx, ws, util.MustParseUUID(issue), "draft", "send_quote", 1)
	if err != nil || open {
		t.Fatalf("round 1 hold: open %v err %v, want held", open, err)
	}
	if n := pendingHeld(t, fx, issue, "flow/draft.send_quote#1", "pending"); n != 1 {
		t.Fatalf("round 1 pending intents = %d, want 1", n)
	}
}

// TestHoldFlowTaskFlagOffOpensAsOrdinaryTask: while durable_effects is off the mark does
// nothing: the task opens and no intent is written (Phase 4 behaviour is kept).
func TestHoldFlowTaskFlagOffOpensAsOrdinaryTask(t *testing.T) {
	fx, intents, issue := holdFixture(t, false)
	open, err := intents.HoldFlowTask(context.Background(), util.MustParseUUID(fx.WorkspaceID), util.MustParseUUID(issue), "draft", "send_quote", 0)
	if err != nil || !open {
		t.Fatalf("flag off: open %v err %v, want open", open, err)
	}
	var n int
	fx.Pool.QueryRow(context.Background(), `SELECT count(*) FROM effect_intent WHERE issue_id = $1`, issue).Scan(&n)
	if n != 0 {
		t.Fatalf("flag off wrote %d intents, want 0", n)
	}
}

func TestHoldFlowTaskTerminalIntent(t *testing.T) {
	for _, state := range []string{"rejected", "expired"} {
		t.Run(state, func(t *testing.T) {
			fx, intents, issue := holdFixture(t, true)
			ctx := context.Background()
			ws, issueID := util.MustParseUUID(fx.WorkspaceID), util.MustParseUUID(issue)
			if _, err := intents.HoldFlowTask(ctx, ws, issueID, "draft", "send_quote", 0); err != nil {
				t.Fatal(err)
			}
			if _, err := fx.Pool.Exec(ctx, `UPDATE effect_intent SET state = $1 WHERE workspace_id = $2 AND issue_id = $3`, state, ws, issueID); err != nil {
				t.Fatal(err)
			}
			open, err := intents.HoldFlowTask(ctx, ws, issueID, "draft", "send_quote", 0)
			if open || err == nil || !strings.Contains(err.Error(), state) {
				t.Fatalf("terminal intent: open %v, err %v; want closed and reason %q", open, err, state)
			}
			if n := pendingHeld(t, fx, issue, "flow/draft.send_quote#0", state); n != 1 {
				t.Fatalf("terminal intents = %d, want 1", n)
			}
		})
	}
}
