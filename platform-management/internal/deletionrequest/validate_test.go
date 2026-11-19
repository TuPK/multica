package deletionrequest

import (
	"context"
	"strings"
	"testing"
)

func TestConfirmMatching(t *testing.T) {
	cases := []struct {
		name     string
		typed    string
		expected string
		want     bool
	}{
		{"exact slug", "acme-team", "acme-team", true},
		{"surrounding spaces are trimmed", "  acme-team \n", "acme-team", true},
		{"slug is case-sensitive", "ACME-team", "acme-team", false},
		{"tenant name is case-sensitive", "acme corp", "Acme Corp", false},
		{"tenant name keeps inner spaces", "Acme  Corp", "Acme Corp", false},
		{"empty typed never matches", "", "acme-team", false},
		{"different text", "acme", "acme-team", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ConfirmMatches(c.typed, c.expected); got != c.want {
				t.Fatalf("ConfirmMatches(%q, %q) = %v, want %v", c.typed, c.expected, got, c.want)
			}
		})
	}
}

func TestDeleteNowReasonRequired(t *testing.T) {
	for _, reason := range []string{"", "   ", "\t\n"} {
		_, err := CleanReason(reason)
		if err == nil || err.Code != CodeReasonRequired || err.Status != 400 {
			t.Fatalf("CleanReason(%q) = %v, want reason_required 400", reason, err)
		}
	}
	if _, err := CleanReason(strings.Repeat("a", MaxReasonChars+1)); err == nil || err.Code != CodeReasonTooLong {
		t.Fatalf("501 characters: got %v, want reason_too_long", err)
	}
	if got, err := CleanReason("  customer request  "); err != nil || got != "customer request" {
		t.Fatalf("CleanReason trim = %q, %v", got, err)
	}
}

// A rejected request writes nothing: the runner must not be reached.
type noRunner struct{ t *testing.T }

func (n noRunner) InTx(context.Context, func(Tx) error) error {
	n.t.Fatal("the runner was reached for a rejected request")
	return nil
}

func TestRequestRejectsBeforeAnyWrite(t *testing.T) {
	s := New(Deps{Runner: noRunner{t}})
	_, _, err := s.RequestWorkspace(context.Background(), Actor{ID: "x"}, "0f0f0f0f-0000-4000-8000-000000000001", Input{Reason: " ", Confirm: "a"})
	if e, ok := err.(*Error); !ok || e.Code != CodeReasonRequired {
		t.Fatalf("RequestWorkspace with blank reason: %v", err)
	}
	_, _, err = s.RequestTenant(context.Background(), Actor{ID: "x"}, "0f0f0f0f-0000-4000-8000-000000000001", Input{Reason: "", Confirm: "a"})
	if e, ok := err.(*Error); !ok || e.Code != CodeReasonRequired {
		t.Fatalf("RequestTenant with blank reason: %v", err)
	}
}
