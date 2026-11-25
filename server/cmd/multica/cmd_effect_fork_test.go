package main

import "testing"

// TestDeclareExitCodes: `multica effect declare` exits 0 when the action may run, 3
// when it waits for the person, and 4 when the policy blocks it (durable spec 4.5).
func TestDeclareExitCodes(t *testing.T) {
	cases := map[string]int{
		"allow":    0,
		"confirm":  3,
		"rehearse": 3,
		"block":    4,
	}
	for decision, want := range cases {
		if got := effectExitCode(decision); got != want {
			t.Errorf("effectExitCode(%q) = %d, want %d", decision, got, want)
		}
	}
}

// TestEffectCommandSubcommands: the command offers declare, list and undo.
func TestEffectCommandSubcommands(t *testing.T) {
	have := map[string]bool{}
	for _, c := range effectCmd.Commands() {
		have[c.Name()] = true
	}
	for _, name := range []string{"declare", "list", "undo"} {
		if !have[name] {
			t.Errorf("effect is missing the %s subcommand", name)
		}
	}
}
