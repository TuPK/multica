package featureflags

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"

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

// durableFrontendFlags are published to the web and desktop clients so their
// views can gate the durable screens. A missing key reads as off on the client.
var durableFrontendFlags = []string{DurableOutcomes, DurableEffects, DurablePrincipal, DurableSeats, DurableOpen}

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

// ValidateDurableFlags refuses a configuration that turns a durable flag on
// while one of its dependencies is off. The server calls it once at startup:
// flags are read from configuration, not written by an API, so the refusal is
// a startup error like any other misconfigured flag file.
func ValidateDurableFlags(ctx context.Context, flags *featureflag.Service) error {
	return CheckDurableDependencies(func(key string) bool {
		return flags.IsEnabled(ctx, key, false)
	})
}

// CheckDurableDependencies is ValidateDurableFlags over a plain lookup, so the
// rule can be tested without a flag service. Keys are visited in sorted order so
// the first error is the same on every start.
func CheckDurableDependencies(enabled func(key string) bool) error {
	deps := DurableDependencies()
	keys := make([]string, 0, len(deps))
	for k := range deps {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !enabled(key) {
			continue
		}
		for _, dep := range deps[key] {
			if !enabled(dep) {
				return fmt.Errorf("feature flag %s is on but its dependency %s is off", key, dep)
			}
		}
	}
	return nil
}

// RequireDurableDependencies is the startup form of ValidateDurableFlags: a
// refused configuration logs the reason and exits, as a malformed flag file does.
func RequireDurableDependencies(ctx context.Context, flags *featureflag.Service) {
	if err := ValidateDurableFlags(ctx, flags); err != nil {
		slog.Error("durable feature flags refused", "error", err)
		os.Exit(1)
	}
}
