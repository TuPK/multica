package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/connectedapps"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// GovernanceKindEffectPolicy records a workspace loosening one cell of the
// effect policy (spec 4.3). Stricter saves are not logged.
const GovernanceKindEffectPolicy = "effect_policy_changed"

// EffectPolicyStore reads and saves a workspace's effect policy overrides.
type EffectPolicyStore struct {
	Queries   *db.Queries
	TxStarter TxStarter
}

// Overrides returns the workspace's saved overrides in cell order. DecideEffect
// takes them as they are; it ignores any that ValidateOverride rejects.
func (s *EffectPolicyStore) Overrides(ctx context.Context, workspaceID pgtype.UUID) ([]Override, error) {
	rows, err := s.Queries.ListEffectPolicyOverrides(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]Override, 0, len(rows))
	for _, r := range rows {
		out = append(out, Override{
			AutonomyLevel: int(r.AutonomyLevel),
			Effect:        r.EffectClass,
			Reversibility: r.Reversibility,
			Decision:      r.Decision,
		})
	}
	return out, nil
}

// Save stores one cell's decision. A looser decision is accepted only where
// ValidateOverride allows it, and it is recorded in the governance log in the same
// transaction as the save.
func (s *EffectPolicyStore) Save(ctx context.Context, workspaceID, actorID pgtype.UUID, o Override) error {
	if err := ValidateOverride(o.AutonomyLevel, o.Effect, o.Reversibility, o.Decision); err != nil {
		return err
	}
	if !validEffectReversibility(o.Reversibility) {
		return fmt.Errorf("reversibility %q is not one of reversible, compensable, irreversible", o.Reversibility)
	}
	base, _ := baseDecision(o.AutonomyLevel, o.Effect, o.Reversibility)
	loosened := isLooser(o.Decision, base)

	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)
	if err := q.UpsertEffectPolicyOverride(ctx, db.UpsertEffectPolicyOverrideParams{
		WorkspaceID:   workspaceID,
		AutonomyLevel: int32(o.AutonomyLevel),
		EffectClass:   o.Effect,
		Reversibility: o.Reversibility,
		Decision:      o.Decision,
		SetBy:         actorID,
	}); err != nil {
		return err
	}
	if loosened {
		if err := NewGovernanceLog().Record(ctx, q, GovernanceEvent{
			WorkspaceID: workspaceID,
			Kind:        GovernanceKindEffectPolicy,
			ActorType:   GovernanceActorMember,
			ActorID:     actorID,
			SubjectType: "effect_policy",
			Summary: fmt.Sprintf("L%d %s/%s loosened from %s to %s",
				o.AutonomyLevel, o.Effect, o.Reversibility, base, o.Decision),
			Details: map[string]any{
				"autonomy_level": o.AutonomyLevel,
				"effect_class":   o.Effect,
				"reversibility":  o.Reversibility,
				"default":        base,
				"decision":       o.Decision,
			},
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// validEffectReversibility reports whether r is a value the override table stores.
func validEffectReversibility(r string) bool {
	switch r {
	case connectedapps.ReversibilityReversible, connectedapps.ReversibilityCompensable,
		connectedapps.ReversibilityIrreversible:
		return true
	}
	return false
}
