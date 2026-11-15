package planlimit

import (
	"reflect"
	"testing"
)

func TestParseBlockedDomains(t *testing.T) {
	cases := map[string][]string{
		"":                           nil,
		"  ":                         nil,
		"example.com":                {"example.com"},
		" Example.COM , spam.io ,, ": {"example.com", "spam.io"},
		"a.com,b.com":                {"a.com", "b.com"},
		",,":                         nil,
	}
	for raw, want := range cases {
		if got := ParseBlockedDomains(raw); !reflect.DeepEqual(got, want) {
			t.Errorf("ParseBlockedDomains(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestEmailDomainBlocked(t *testing.T) {
	blocked := []string{"example.com", "spam.io"}
	cases := []struct {
		email string
		want  bool
	}{
		{"user@example.com", true},           // exact
		{"user@mail.example.com", true},      // subdomain
		{"user@a.b.example.com", true},       // deeper subdomain
		{"user@spam.io", true},               // second entry
		{"user@notexample.com", false},       // look-alike: suffix without the dot
		{"user@example.com.evil.org", false}, // entry in the middle
		{"user@examplexcom", false},
		{"user@example.org", false},
		{"user@com", false},
		{"USER@EXAMPLE.COM", true}, // case-insensitive
		{"user@Mail.Example.Com", true},
		{"user@example.com.", true},      // trailing dot is the same host
		{"user@@example.com", true},      // the domain is after the last @
		{"example.com@gmail.com", false}, // entry in the local part
		{"user.example.com", false},      // no @: no domain
		{"", false},
		{"@", false},
		{"user@", false},
	}
	for _, tc := range cases {
		if got := EmailDomainBlocked(tc.email, blocked); got != tc.want {
			t.Errorf("EmailDomainBlocked(%q) = %v, want %v", tc.email, got, tc.want)
		}
	}
}

func TestEmailDomainBlockedIsCaseInsensitiveOnTheList(t *testing.T) {
	// The list is normalized by ParseBlockedDomains, but a caller that builds
	// it by hand gets the same match.
	if !EmailDomainBlocked("a@example.com", []string{"Example.COM"}) {
		t.Error("an upper-case entry did not match")
	}
}

func TestEmptyBlockListBlocksNothing(t *testing.T) {
	if EmailDomainBlocked("a@example.com", nil) {
		t.Error("nil list blocked an address")
	}
	if EmailDomainBlocked("a@example.com", []string{"", " "}) {
		t.Error("blank entries blocked an address")
	}
}
