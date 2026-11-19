package plan

import (
	"context"
	"testing"
)

// TestSeedRefusesCurrencyOutsideEnabledSet: a catalogue plan in a currency that
// is not enabled makes Seed fail and writes no row.
func TestSeedRefusesCurrencyOutsideEnabledSet(t *testing.T) {
	s := newFakeStore()
	n, err := SeedEnabled(context.Background(), s, discard(), []string{"usd"})
	if err == nil {
		t.Fatal("Seed accepted a catalogue currency outside the enabled set")
	}
	if n != 0 || len(s.rows) != 0 || s.inserts != 0 {
		t.Fatalf("inserted %d, rows %d, insert calls %d, want none", n, len(s.rows), s.inserts)
	}
	if _, err := SeedEnabled(context.Background(), s, discard(), []string{"vnd"}); err != nil {
		t.Fatalf("enabled currency refused: %v", err)
	}
}
