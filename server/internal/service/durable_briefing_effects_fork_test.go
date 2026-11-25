package service

import (
	"context"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/featureflags"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

func effectsBriefingFlags(effects, outcomes bool) *featureflag.Service {
	p := featureflag.NewStaticProvider()
	p.Set(featureflags.DurableEffects, featureflag.Rule{Default: effects})
	p.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: outcomes})
	return featureflag.NewService(p)
}

// TestBriefingDeclarationBlockFollowsEffectsFlag: the declaration block is in every
// agent's briefing while durable_effects is on, and nowhere while it is off (spec 4.5).
// A task with no issue gets it alone, so this test needs no database.
func TestBriefingDeclarationBlockFollowsEffectsFlag(t *testing.T) {
	task := db.AgentTaskQueue{} // no issue: the outcome blocks never apply

	on := DurableBriefingBlocks(context.Background(), nil, effectsBriefingFlags(true, false), task)
	if len(on) != 1 || on[0].Title != briefingEffectsTitle {
		t.Fatalf("effects on: blocks = %+v, want one declaration block", on)
	}
	if !strings.Contains(on[0].Body, "multica effect declare") || !strings.Contains(on[0].Body, "exit 3") {
		t.Fatalf("declaration block does not name the command and the exit codes: %s", on[0].Body)
	}

	if off := DurableBriefingBlocks(context.Background(), nil, effectsBriefingFlags(false, true), task); len(off) != 0 {
		t.Fatalf("effects off: blocks = %+v, want none", off)
	}
}
