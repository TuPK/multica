package service

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/connectedapps"
)

// specTable is spec 4.3 written out by hand, one row per effect/reversibility
// pair the table names. Columns are autonomy levels L0..L3.
var specTable = []struct {
	name, effect, rev string
	want              [4]string
}{
	{"read", connectedapps.EffectRead, "", [4]string{"allow", "allow", "allow", "allow"}},
	{"write_internal", connectedapps.EffectWriteInternal, connectedapps.ReversibilityReversible, [4]string{"allow", "allow", "allow", "allow"}},
	{"write/reversible", connectedapps.EffectWrite, connectedapps.ReversibilityReversible, [4]string{"confirm", "allow", "allow", "allow"}},
	{"write/compensable", connectedapps.EffectWrite, connectedapps.ReversibilityCompensable, [4]string{"confirm", "confirm", "allow", "allow"}},
	{"send", connectedapps.EffectSend, connectedapps.ReversibilityIrreversible, [4]string{"block", "rehearse", "rehearse", "allow"}},
	{"share", connectedapps.EffectShare, connectedapps.ReversibilityIrreversible, [4]string{"block", "rehearse", "rehearse", "rehearse"}},
	{"delete", connectedapps.EffectDelete, connectedapps.ReversibilityIrreversible, [4]string{"block", "confirm", "rehearse", "rehearse"}},
	{"spend", connectedapps.EffectSpend, connectedapps.ReversibilityIrreversible, [4]string{"block", "rehearse", "rehearse", "rehearse"}},
}

// decide runs DecideEffect for an ordinary run with no overrides, a live
// company and no sends yet today, so only the table decides.
func decide(level int, effect, rev string) string {
	return DecideEffect(level, effect, rev, nil, "agent", "active", 0)
}

func TestDecideEffectFullTable(t *testing.T) {
	for _, row := range specTable {
		for level := 0; level <= 3; level++ {
			if got := decide(level, row.effect, row.rev); got != row.want[level] {
				t.Errorf("%s at L%d = %s, want %s", row.name, level, got, row.want[level])
			}
		}
	}
}

func TestDecideEffectUnknownFailsClosed(t *testing.T) {
	cases := []struct {
		name        string
		level       int
		effect, rev string
	}{
		{"write without a reversibility the table knows", 3, connectedapps.EffectWrite, connectedapps.ReversibilityIrreversible},
		{"unknown effect", 3, "frobnicate", ""},
		{"level above L3", 4, connectedapps.EffectRead, ""},
		{"level below L0", -1, connectedapps.EffectRead, ""},
	}
	for _, c := range cases {
		if got := decide(c.level, c.effect, c.rev); got != "block" {
			t.Errorf("%s = %s, want block", c.name, got)
		}
	}
}

func TestDecideEffectTaskAndCompanyRules(t *testing.T) {
	cases := []struct {
		name                      string
		level                     int
		effect, rev, runClass, co string
		want                      string
	}{
		// shadow: everything but read is blocked, even at L3 where the table allows it.
		{"shadow read", 3, connectedapps.EffectRead, "", "shadow", "active", "allow"},
		{"shadow write_internal", 3, connectedapps.EffectWriteInternal, connectedapps.ReversibilityReversible, "shadow", "active", "block"},
		{"shadow write", 3, connectedapps.EffectWrite, connectedapps.ReversibilityCompensable, "shadow", "active", "block"},
		{"shadow send", 3, connectedapps.EffectSend, connectedapps.ReversibilityIrreversible, "shadow", "active", "block"},
		// verification: only read and write_internal.
		{"verification read", 0, connectedapps.EffectRead, "", "verification", "active", "allow"},
		{"verification write_internal", 3, connectedapps.EffectWriteInternal, connectedapps.ReversibilityReversible, "verification", "active", "allow"},
		{"verification write", 3, connectedapps.EffectWrite, connectedapps.ReversibilityReversible, "verification", "active", "block"},
		{"verification send", 3, connectedapps.EffectSend, connectedapps.ReversibilityIrreversible, "verification", "active", "block"},
		// paused company: non-human runs may only read.
		{"paused agent read", 3, connectedapps.EffectRead, "", "agent", "paused", "allow"},
		{"paused agent write_internal", 3, connectedapps.EffectWriteInternal, connectedapps.ReversibilityReversible, "agent", "paused", "block"},
		{"paused agent write/reversible", 3, connectedapps.EffectWrite, connectedapps.ReversibilityReversible, "agent", "paused", "block"},
		{"paused human write_internal", 3, connectedapps.EffectWriteInternal, connectedapps.ReversibilityReversible, "human", "paused", "allow"},
		{"paused human read", 3, connectedapps.EffectRead, "", "human", "paused", "allow"},
	}
	for _, c := range cases {
		got := DecideEffect(c.level, c.effect, c.rev, nil, c.runClass, c.co, 0)
		if got != c.want {
			t.Errorf("%s = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestDecideEffectSendCap(t *testing.T) {
	send := connectedapps.EffectSend
	irr := connectedapps.ReversibilityIrreversible
	cases := []struct {
		name  string
		level int
		count int
		want  string
	}{
		{"L3 twentieth send is within the cap", 3, 19, "allow"},
		{"L3 twenty-first send goes to rehearse", 3, 20, "rehearse"},
		{"L3 send far over the cap still rehearses", 3, 500, "rehearse"},
		{"L2 send is rehearse regardless of count", 2, 0, "rehearse"},
	}
	for _, c := range cases {
		got := DecideEffect(c.level, send, irr, nil, "agent", "active", c.count)
		if got != c.want {
			t.Errorf("%s = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestDecideEffectAppliesOnlyValidOverrides(t *testing.T) {
	overrides := []Override{
		{AutonomyLevel: 3, Effect: connectedapps.EffectShare, Reversibility: connectedapps.ReversibilityIrreversible, Decision: "confirm"},
		{AutonomyLevel: 2, Effect: connectedapps.EffectDelete, Reversibility: connectedapps.ReversibilityIrreversible, Decision: "confirm"},
		{AutonomyLevel: 1, Effect: connectedapps.EffectSend, Reversibility: connectedapps.ReversibilityIrreversible, Decision: "block"},
	}
	cases := []struct {
		name        string
		level       int
		effect, rev string
		want        string
	}{
		// A valid loosening at L3 takes effect.
		{"L3 share loosened to confirm", 3, connectedapps.EffectShare, connectedapps.ReversibilityIrreversible, "confirm"},
		// A loosening at L2 is not allowed, so the default stands.
		{"L2 delete stays rehearse", 2, connectedapps.EffectDelete, connectedapps.ReversibilityIrreversible, "rehearse"},
		// A stricter override applies.
		{"L1 send tightened to block", 1, connectedapps.EffectSend, connectedapps.ReversibilityIrreversible, "block"},
		// An override only applies at its own level.
		{"L2 send is not affected by the L1 override", 2, connectedapps.EffectSend, connectedapps.ReversibilityIrreversible, "rehearse"},
	}
	for _, c := range cases {
		got := DecideEffect(c.level, c.effect, c.rev, overrides, "agent", "active", 0)
		if got != c.want {
			t.Errorf("%s = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestOverrideRules(t *testing.T) {
	irr := connectedapps.ReversibilityIrreversible
	cases := []struct {
		name        string
		level       int
		effect, rev string
		decision    string
		wantErr     bool
	}{
		// Loosening is refused below L3.
		{"L2 delete rehearse to confirm", 2, connectedapps.EffectDelete, irr, "confirm", true},
		{"L2 send rehearse to allow", 2, connectedapps.EffectSend, irr, "allow", true},
		{"L0 write/compensable confirm to allow", 0, connectedapps.EffectWrite, connectedapps.ReversibilityCompensable, "allow", true},
		{"L1 spend rehearse to confirm", 1, connectedapps.EffectSpend, irr, "confirm", true},
		// Loosening is allowed at L3, except spend and share becoming allow.
		{"L3 delete rehearse to confirm", 3, connectedapps.EffectDelete, irr, "confirm", false},
		{"L3 share rehearse to confirm", 3, connectedapps.EffectShare, irr, "confirm", false},
		// spend and share never become allow, at any level.
		{"spend to allow at L0", 0, connectedapps.EffectSpend, irr, "allow", true},
		{"spend to allow at L1", 1, connectedapps.EffectSpend, irr, "allow", true},
		{"spend to allow at L2", 2, connectedapps.EffectSpend, irr, "allow", true},
		{"spend to allow at L3", 3, connectedapps.EffectSpend, irr, "allow", true},
		{"share to allow at L0", 0, connectedapps.EffectShare, irr, "allow", true},
		{"share to allow at L1", 1, connectedapps.EffectShare, irr, "allow", true},
		{"share to allow at L2", 2, connectedapps.EffectShare, irr, "allow", true},
		{"share to allow at L3", 3, connectedapps.EffectShare, irr, "allow", true},
		// Tightening is always allowed, and so is keeping the default.
		{"L0 write_internal to confirm", 0, connectedapps.EffectWriteInternal, connectedapps.ReversibilityReversible, "confirm", false},
		{"L2 send rehearse to block", 2, connectedapps.EffectSend, irr, "block", false},
		{"L0 spend block to block", 0, connectedapps.EffectSpend, irr, "block", false},
		{"L1 spend rehearse to rehearse", 1, connectedapps.EffectSpend, irr, "rehearse", false},
		// Malformed overrides are refused.
		{"unknown decision", 3, connectedapps.EffectDelete, irr, "maybe", true},
		{"level 4", 4, connectedapps.EffectDelete, irr, "confirm", true},
		{"write with irreversible has no row", 3, connectedapps.EffectWrite, irr, "allow", true},
	}
	for _, c := range cases {
		err := ValidateOverride(c.level, c.effect, c.rev, c.decision)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: ValidateOverride err = %v, wantErr %v", c.name, err, c.wantErr)
		}
	}
}
