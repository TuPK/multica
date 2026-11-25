package featureflags

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// The five durable flags are off until an operator turns them on, and each
// key is the name the spec gives it.
func TestDurableFlagsDefaultOff(t *testing.T) {
	ctx := context.Background()
	flags := featureflag.NewService(featureflag.NewStaticProvider())

	cases := []struct {
		key     string
		want    string
		enabled func(context.Context, *featureflag.Service) bool
	}{
		{DurableOutcomes, "durable_outcomes", DurableOutcomesEnabled},
		{DurableEffects, "durable_effects", DurableEffectsEnabled},
		{DurablePrincipal, "durable_principal", DurablePrincipalEnabled},
		{DurableSeats, "durable_seats", DurableSeatsEnabled},
		{DurableOpen, "durable_open", DurableOpenEnabled},
	}
	for _, c := range cases {
		if c.key != c.want {
			t.Errorf("key = %q, want %q", c.key, c.want)
		}
		if c.enabled(ctx, flags) {
			t.Errorf("%s is on by default", c.want)
		}
	}
}

// The dependency table mirrors spec 14.1. Order within a dependency list does
// not matter, so both sides are sorted before comparing.
func TestDurableFlagDependencies(t *testing.T) {
	want := map[string][]string{
		DurableOutcomes:  {},
		DurableEffects:   {},
		DurablePrincipal: {},
		DurableSeats:     {DurableOutcomes, DurableEffects},
		DurableOpen:      {DurableEffects, DurableOutcomes},
	}

	got := DurableDependencies()
	if len(got) != len(want) {
		t.Errorf("table has %d flags, want %d: %v", len(got), len(want), got)
	}
	for key, wantDeps := range want {
		gotDeps, ok := got[key]
		if !ok {
			t.Errorf("%s missing from the dependency table", key)
			continue
		}
		g := append([]string{}, gotDeps...)
		w := append([]string{}, wantDeps...)
		sort.Strings(g)
		sort.Strings(w)
		if !reflect.DeepEqual(g, w) {
			t.Errorf("%s depends on %v, want %v", key, g, w)
		}
	}
}
