package service

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// TestHoldFlowTaskCreatesDeclaredIntent: an external flow task gets a pending declared
// intent that names its step and task before it runs. Approving it runs nothing: the
// platform does not run declared work.
func TestHoldFlowTaskCreatesDeclaredIntent(t *testing.T) {
	fx := newEffectFixture(t, "")
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableEffects, featureflag.Rule{Default: true})
	intents := &IntentService{Queries: fx.q, TxStarter: fx.Pool, Flags: featureflag.NewService(provider)}
	ctx := context.Background()

	id, err := intents.HoldFlowTask(ctx, fx.request(connectedapps.SourceDeclared, "", ""), "draft", "send_quote")
	if err != nil {
		t.Fatalf("HoldFlowTask: %v", err)
	}
	var state, kind, target string
	fx.Pool.QueryRow(ctx, `SELECT state, preview->>'kind', target FROM effect_intent WHERE id = $1`, util.UUIDToString(id)).Scan(&state, &kind, &target)
	if state != "pending" || kind != "declared" || target != "flow/draft.send_quote" {
		t.Fatalf("intent state %q kind %q target %q, want pending declared flow/draft.send_quote", state, kind, target)
	}
	t.Cleanup(func() {
		fx.Cleanup(t, `DELETE FROM effect_intent WHERE id = $1`, util.UUIDToString(id))
	})

	if _, err := intents.Approve(ctx, util.MustParseUUID(fx.WorkspaceID), util.MustParseUUID(fx.UserID), id, nil, "go"); err != nil {
		t.Fatalf("Approve declared: %v", err)
	}
	fx.Pool.QueryRow(ctx, `SELECT state FROM effect_intent WHERE id = $1`, util.UUIDToString(id)).Scan(&state)
	if state != "approved" {
		t.Fatalf("declared intent after approval = %q, want approved", state)
	}
}
