package connectedapps

import "testing"

func TestEffectMapAppClass(t *testing.T) {
	cases := map[string]string{
		ClassRead:           EffectRead,
		ClassWrite:          EffectWrite,
		ClassSend:           EffectSend,
		ClassDelete:         EffectDelete,
		ClassShare:          EffectShare,
		EffectWriteInternal: "", // no app class maps to write_internal
		EffectSpend:         "", // spend comes only from the curated list or a declaration
		"":                  "",
		"frobnicate":        "",
	}
	for class, want := range cases {
		if got := MapAppClass(class); got != want {
			t.Errorf("MapAppClass(%q) = %q, want %q", class, got, want)
		}
	}
}

func TestEffectDefaultReversibility(t *testing.T) {
	cases := map[string]string{
		EffectWriteInternal: ReversibilityReversible,
		EffectWrite:         ReversibilityCompensable,
		EffectSend:          ReversibilityIrreversible,
		EffectShare:         ReversibilityIrreversible,
		EffectDelete:        ReversibilityIrreversible,
		EffectSpend:         ReversibilityIrreversible,
		EffectRead:          "", // read has nothing to reverse
		"frobnicate":        "",
	}
	for effect, want := range cases {
		if got := DefaultReversibility(effect); got != want {
			t.Errorf("DefaultReversibility(%q) = %q, want %q", effect, got, want)
		}
	}
}

func TestStricterWithRehearse(t *testing.T) {
	// Strictness rises left to right, so the stricter of two is the later one.
	order := []string{PolicyAllow, PolicyConfirm, PolicyRehearse, PolicyBlock}
	for i, a := range order {
		for j, b := range order {
			want := order[max(i, j)]
			if got := Stricter(a, b); got != want {
				t.Errorf("Stricter(%s, %s) = %s, want %s", a, b, got, want)
			}
		}
	}
}
