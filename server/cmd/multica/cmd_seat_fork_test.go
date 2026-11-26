package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestParseSeatEvidence(t *testing.T) {
	if _, err := parseSeatEvidence(nil); err == nil {
		t.Error("no --evidence: want an error")
	}
	for _, bad := range []string{"outcome", "outcome:", ":abc", "  "} {
		if _, err := parseSeatEvidence([]string{bad}); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
	got, err := parseSeatEvidence([]string{" outcome : abc-1 ", "retro:r2"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []map[string]string{{"type": "outcome", "ref": "abc-1"}, {"type": "retro", "ref": "r2"}}
	if len(got) != len(want) {
		t.Fatalf("got %d items, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i]["type"] != want[i]["type"] || got[i]["ref"] != want[i]["ref"] {
			t.Errorf("item %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestSeatFromFlagsPrefersTheFlagThenTheTaskAgent(t *testing.T) {
	const flagSeat = "11111111-1111-4111-8111-111111111111"
	const envSeat = "22222222-2222-4222-8222-222222222222"
	newCmd := func(agent string) *cobra.Command {
		cmd := &cobra.Command{}
		cmd.Flags().String("agent", agent, "")
		return cmd
	}

	t.Setenv("MULTICA_AGENT_ID", envSeat)
	if got, err := seatFromFlags(newCmd(flagSeat)); err != nil || got != flagSeat {
		t.Errorf("flag set: got %q, %v; want the flag seat", got, err)
	}
	if got, err := seatFromFlags(newCmd("")); err != nil || got != envSeat {
		t.Errorf("flag empty: got %q, %v; want the task's agent", got, err)
	}

	t.Setenv("MULTICA_AGENT_ID", "")
	if _, err := seatFromFlags(newCmd("")); err == nil {
		t.Error("no seat named and no task agent: want an error")
	}
}
