package service

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/connectedapps"
)

// TestOverrideLock pins the reason the policy grid gives for each decision a cell
// may not take. "" means the decision can be saved.
func TestOverrideLock(t *testing.T) {
	irreversible := connectedapps.ReversibilityIrreversible
	reversible := connectedapps.ReversibilityReversible
	cases := []struct {
		name           string
		level          int
		effect, rev    string
		decision, want string
	}{
		{"stricter than default is always saved", 0, connectedapps.EffectRead, reversible, connectedapps.PolicyConfirm, ""},
		{"equal to default is saved", 2, connectedapps.EffectWrite, reversible, connectedapps.PolicyAllow, ""},
		{"loosening below L3 is locked", 0, connectedapps.EffectSend, irreversible, connectedapps.PolicyAllow, LockLooserAtL3},
		{"loosening at L2 is locked", 2, connectedapps.EffectDelete, irreversible, connectedapps.PolicyConfirm, LockLooserAtL3},
		{"loosening at L3 is saved", 3, connectedapps.EffectSend, irreversible, connectedapps.PolicyAllow, ""},
		{"spend never becomes allow", 3, connectedapps.EffectSpend, irreversible, connectedapps.PolicyAllow, LockNeverAllow},
		{"share never becomes allow", 3, connectedapps.EffectShare, irreversible, connectedapps.PolicyAllow, LockNeverAllow},
		{"a missing cell is named", 1, connectedapps.EffectWrite, connectedapps.ReversibilityIrreversible, connectedapps.PolicyAllow, LockNoCell},
		{"an unknown decision is named", 1, connectedapps.EffectRead, reversible, "maybe", LockInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := OverrideLock(tc.level, tc.effect, tc.rev, tc.decision); got != tc.want {
				t.Fatalf("OverrideLock(L%d, %s, %s, %s) = %q, want %q", tc.level, tc.effect, tc.rev, tc.decision, got, tc.want)
			}
		})
	}
}

// TestOverrideLockAgreesWithValidateOverride keeps the grid's locks and the saving
// rule in step: a decision is locked exactly when ValidateOverride refuses it.
func TestOverrideLockAgreesWithValidateOverride(t *testing.T) {
	decisions := []string{connectedapps.PolicyAllow, connectedapps.PolicyConfirm, connectedapps.PolicyRehearse, connectedapps.PolicyBlock}
	for _, row := range PolicyRows() {
		for level := 0; level <= 3; level++ {
			for _, d := range decisions {
				locked := OverrideLock(level, row.Effect, row.Reversibility, d) != ""
				refused := ValidateOverride(level, row.Effect, row.Reversibility, d) != nil
				if locked != refused {
					t.Fatalf("L%d %s/%s %s: lock=%v but ValidateOverride refused=%v", level, row.Effect, row.Reversibility, d, locked, refused)
				}
			}
		}
	}
}
