package service

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestWorkClassPriority pins the order in spec 1.2: flow, then a "wc:" label,
// then the squad, then "general". The cap is spec 1.2's char_length <= 200,
// counted in characters, the way Postgres counts char_length.
func TestWorkClassPriority(t *testing.T) {
	cases := []struct {
		name string
		in   WorkClassInput
		want string
	}{
		{
			name: "flow with stage key beats label and squad",
			in: WorkClassInput{
				FlowID:       "f1",
				FlowStageKey: "review",
				LabelNames:   []string{"wc:billing"},
				SquadID:      "s1",
			},
			want: "flow:f1:review",
		},
		{
			name: "flow without stage key falls through to label",
			in: WorkClassInput{
				FlowID:     "f1",
				LabelNames: []string{"wc:billing"},
				SquadID:    "s1",
			},
			want: "label:billing",
		},
		{
			name: "label beats squad",
			in: WorkClassInput{
				LabelNames: []string{"bug", "wc:billing"},
				SquadID:    "s1",
			},
			want: "label:billing",
		},
		{
			name: "squad when no flow and no wc label",
			in: WorkClassInput{
				LabelNames: []string{"bug"},
				SquadID:    "s1",
			},
			want: "squad:s1",
		},
		{
			name: "general when nothing matches",
			in:   WorkClassInput{LabelNames: []string{"bug"}},
			want: "general",
		},
		{
			name: "first wc label by name, not by input order",
			in:   WorkClassInput{LabelNames: []string{"wc:zeta", "wc:alpha"}},
			want: "label:alpha",
		},
		{
			// Assumption: an empty tail ("wc:" alone) names no class, so it is
			// skipped rather than producing "label:".
			name: "empty wc tail is skipped",
			in:   WorkClassInput{LabelNames: []string{"wc:"}, SquadID: "s1"},
			want: "squad:s1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WorkClassFor(tc.in); got != tc.want {
				t.Fatalf("WorkClassFor(%+v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	t.Run("exactly 200 characters is kept", func(t *testing.T) {
		// "squad:" is 6 characters, so 194 more make 200.
		squad := strings.Repeat("s", 194)
		got := WorkClassFor(WorkClassInput{SquadID: squad})
		if want := "squad:" + squad; got != want {
			t.Fatalf("got %d chars, want the full 200-char value", utf8.RuneCountInString(got))
		}
	})

	t.Run("over 200 characters is cut to exactly 200", func(t *testing.T) {
		// Assumption: the spec only states the limit. The value is cut at 200
		// characters, not bytes, so a multi-byte squad id is not split mid-rune
		// and the result still matches Postgres char_length.
		squad := strings.Repeat("é", 300)
		got := WorkClassFor(WorkClassInput{SquadID: squad})
		if n := utf8.RuneCountInString(got); n != 200 {
			t.Fatalf("rune count = %d, want 200", n)
		}
		if want := "squad:" + strings.Repeat("é", 194); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}
