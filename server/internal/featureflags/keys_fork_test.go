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

// A flag cannot be on while a flag it depends on is off.
func TestCheckDurableDependencies(t *testing.T) {
	on := func(keys ...string) func(string) bool {
		set := map[string]bool{}
		for _, k := range keys {
			set[k] = true
		}
		return func(k string) bool { return set[k] }
	}
	cases := []struct {
		name    string
		enabled func(string) bool
		wantErr bool
	}{
		{"all off", on(), false},
		{"outcomes and effects on enable seats", on(DurableOutcomes, DurableEffects, DurableSeats), false},
		{"seats on without effects refused", on(DurableOutcomes, DurableSeats), true},
		{"seats on without outcomes refused", on(DurableEffects, DurableSeats), true},
		{"open needs both", on(DurableEffects, DurableOpen), true},
		{"principal needs nothing", on(DurablePrincipal), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckDurableDependencies(c.enabled)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

// The flag-service form reads the same rule from the flag provider.
func TestValidateDurableFlagsFromService(t *testing.T) {
	sp := featureflag.NewStaticProvider()
	sp.Set(DurableSeats, featureflag.Rule{Default: true})
	flags := featureflag.NewService(sp)
	if err := ValidateDurableFlags(context.Background(), flags); err == nil {
		t.Fatal("durable_seats on with both dependencies off must be refused")
	}
}

// The clients read the durable flags from the published set, off by default.
func TestDurableFlagsPublishedOffByDefault(t *testing.T) {
	out := EvaluateFrontendPublicFlags(context.Background(), featureflag.NewService(featureflag.NewStaticProvider()))
	for _, k := range []string{DurableOutcomes, DurableEffects, DurablePrincipal, DurableSeats, DurableOpen} {
		v, ok := out[k]
		if !ok {
			t.Fatalf("%s not published", k)
		}
		if v {
			t.Fatalf("%s published on by default", k)
		}
	}
}
