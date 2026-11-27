package service

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// policyStoreFixture returns a store on the fixture's pool, with the workspace's
// override rows removed when the test ends. Other packages write to the same
// database, so every assertion filters by this test's workspace.
func policyStoreFixture(t *testing.T) (*EffectPolicyStore, principalFixture) {
	t.Helper()
	f, _ := companyWorkspace(t)
	f.Cleanup(t, `DELETE FROM effect_policy_override WHERE workspace_id = $1`, f.WorkspaceID)
	f.Cleanup(t, `DELETE FROM governance_event WHERE workspace_id = $1`, f.WorkspaceID)
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableEffects, featureflag.Rule{Default: true})
	return &EffectPolicyStore{Queries: db.New(f.Pool), TxStarter: f.Pool, Flags: featureflag.NewService(provider)}, f
}

// TestEffectPolicyOverrideSavedAndRead: a stricter override is accepted, stored
// once, and read back with the same cell and decision.
func TestEffectPolicyOverrideSavedAndRead(t *testing.T) {
	store, f := policyStoreFixture(t)
	ctx := context.Background()
	ws := util.MustParseUUID(f.WorkspaceID)
	actor := util.MustParseUUID(f.UserID)

	// L1 write/compensable defaults to confirm (spec 4.3); block is stricter.
	want := Override{
		AutonomyLevel: 1,
		Effect:        connectedapps.EffectWrite,
		Reversibility: connectedapps.ReversibilityCompensable,
		Decision:      connectedapps.PolicyBlock,
	}
	if err := store.Save(ctx, ws, actor, want); err != nil {
		t.Fatalf("Save stricter override: %v", err)
	}
	got, err := store.Overrides(ctx, ws)
	if err != nil {
		t.Fatalf("Overrides: %v", err)
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("Overrides = %+v, want exactly %+v", got, want)
	}
}

// TestEffectPolicyOverrideRejectsLooserBelowL3: spec 4.3 allows loosening only at
// L3. A looser decision at L2 is refused and nothing is stored.
func TestEffectPolicyOverrideRejectsLooserBelowL3(t *testing.T) {
	store, f := policyStoreFixture(t)
	ctx := context.Background()
	ws := util.MustParseUUID(f.WorkspaceID)

	err := store.Save(ctx, ws, util.MustParseUUID(f.UserID), Override{
		AutonomyLevel: 2,
		Effect:        connectedapps.EffectSend,
		Reversibility: connectedapps.ReversibilityIrreversible,
		Decision:      connectedapps.PolicyAllow,
	})
	if err == nil {
		t.Fatal("Save looser override at L2: want error, got nil")
	}
	if n := f.Count(t, `SELECT count(*) FROM effect_policy_override WHERE workspace_id = $1`, f.WorkspaceID); n != 0 {
		t.Fatalf("rejected override was stored: %d rows", n)
	}
}

// TestEffectPolicyOverrideReplacesCell: a second save on the same cell replaces the
// decision; the table keeps one row per cell.
func TestEffectPolicyOverrideReplacesCell(t *testing.T) {
	store, f := policyStoreFixture(t)
	ctx := context.Background()
	ws := util.MustParseUUID(f.WorkspaceID)
	actor := util.MustParseUUID(f.UserID)
	cell := Override{AutonomyLevel: 1, Effect: connectedapps.EffectWrite, Reversibility: connectedapps.ReversibilityCompensable}

	cell.Decision = connectedapps.PolicyRehearse
	if err := store.Save(ctx, ws, actor, cell); err != nil {
		t.Fatalf("first save: %v", err)
	}
	cell.Decision = connectedapps.PolicyBlock
	if err := store.Save(ctx, ws, actor, cell); err != nil {
		t.Fatalf("second save: %v", err)
	}
	got, err := store.Overrides(ctx, ws)
	if err != nil {
		t.Fatalf("Overrides: %v", err)
	}
	if len(got) != 1 || got[0].Decision != connectedapps.PolicyBlock {
		t.Fatalf("Overrides after replace = %+v, want one block row", got)
	}
}

// TestEffectPolicyLoosenedOverrideWritesGovernanceEvent: loosening at L3 is a
// decision of the people, so it lands in the governance log. A stricter save
// does not.
func TestEffectPolicyLoosenedOverrideWritesGovernanceEvent(t *testing.T) {
	store, f := policyStoreFixture(t)
	ctx := context.Background()
	ws := util.MustParseUUID(f.WorkspaceID)
	actor := util.MustParseUUID(f.UserID)

	// L1 delete defaults to confirm; block is stricter, so no event.
	if err := store.Save(ctx, ws, actor, Override{
		AutonomyLevel: 1, Effect: connectedapps.EffectDelete,
		Reversibility: connectedapps.ReversibilityIrreversible, Decision: connectedapps.PolicyBlock,
	}); err != nil {
		t.Fatalf("stricter save: %v", err)
	}
	if n := f.Count(t, `SELECT count(*) FROM governance_event WHERE workspace_id = $1 AND kind = $2`,
		f.WorkspaceID, GovernanceKindEffectPolicy); n != 0 {
		t.Fatalf("stricter save wrote %d governance events, want 0", n)
	}

	// L3 delete defaults to rehearse; confirm is looser and allowed at L3.
	if err := store.Save(ctx, ws, actor, Override{
		AutonomyLevel: 3, Effect: connectedapps.EffectDelete,
		Reversibility: connectedapps.ReversibilityIrreversible, Decision: connectedapps.PolicyConfirm,
	}); err != nil {
		t.Fatalf("loosening save at L3: %v", err)
	}
	if n := f.Count(t, `SELECT count(*) FROM governance_event WHERE workspace_id = $1 AND kind = $2`,
		f.WorkspaceID, GovernanceKindEffectPolicy); n != 1 {
		t.Fatalf("loosening save wrote %d governance events, want 1", n)
	}
}

// TestEffectPolicyOverridesFeedDecideEffect: the saved overrides are the input
// DecideEffect applies, so a stored block changes the decision.
func TestEffectPolicyOverridesFeedDecideEffect(t *testing.T) {
	store, f := policyStoreFixture(t)
	ctx := context.Background()
	ws := util.MustParseUUID(f.WorkspaceID)

	if err := store.Save(ctx, ws, util.MustParseUUID(f.UserID), Override{
		AutonomyLevel: 1, Effect: connectedapps.EffectWrite,
		Reversibility: connectedapps.ReversibilityCompensable, Decision: connectedapps.PolicyBlock,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	overrides, err := store.Overrides(ctx, ws)
	if err != nil {
		t.Fatalf("Overrides: %v", err)
	}
	got := DecideEffect(1, connectedapps.EffectWrite, connectedapps.ReversibilityCompensable, overrides, runClassHuman, "", 0)
	if got != connectedapps.PolicyBlock {
		t.Fatalf("DecideEffect with stored override = %q, want block", got)
	}
}
