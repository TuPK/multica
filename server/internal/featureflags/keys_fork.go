package featureflags

import (
	"context"

	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// Feature flags of the durable platform program (docs/specs/durable-platform,
// spec 14.1). Each one gates one phase and is off until an operator turns it on.
const (
	DurableOutcomes  = "durable_outcomes"
	DurableEffects   = "durable_effects"
	DurablePrincipal = "durable_principal"
	DurableSeats     = "durable_seats"
	DurableOpen      = "durable_open"
)

// DurableDependencies maps each durable flag to the flags it needs on. The
// server refuses to turn a flag on while one of its dependencies is off; that
// check belongs to the write path, not to this table.
//
// durable_principal depends on nothing: F6 works without F5, and only its
// prediction part needs F5.
func DurableDependencies() map[string][]string {
	return map[string][]string{
		DurableOutcomes:  nil,
		DurableEffects:   nil,
		DurablePrincipal: nil,
		DurableSeats:     {DurableOutcomes, DurableEffects},
		DurableOpen:      {DurableEffects, DurableOutcomes},
	}
}

func DurableOutcomesEnabled(ctx context.Context, flags *featureflag.Service) bool {
	return flags.IsEnabled(ctx, DurableOutcomes, false)
}

func DurableEffectsEnabled(ctx context.Context, flags *featureflag.Service) bool {
	return flags.IsEnabled(ctx, DurableEffects, false)
}

func DurablePrincipalEnabled(ctx context.Context, flags *featureflag.Service) bool {
	return flags.IsEnabled(ctx, DurablePrincipal, false)
}

func DurableSeatsEnabled(ctx context.Context, flags *featureflag.Service) bool {
	return flags.IsEnabled(ctx, DurableSeats, false)
}

func DurableOpenEnabled(ctx context.Context, flags *featureflag.Service) bool {
	return flags.IsEnabled(ctx, DurableOpen, false)
}
