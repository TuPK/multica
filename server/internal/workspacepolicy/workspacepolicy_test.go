package workspacepolicy

import (
	"reflect"
	"testing"
	"time"
)

func TestDefaultsAreOpen(t *testing.T) {
	d := Default()
	if len(d.AllowedInviteDomains) != 0 || !d.ShareLinksEnabled || d.RequireMFA || d.RequireSSO || d.SessionMaxAgeHours != 0 {
		t.Fatalf("Default() = %+v, want open domains, share links on, no MFA, no SSO, no age limit", d)
	}
}

func TestDomainAllowed(t *testing.T) {
	cases := []struct {
		name    string
		allowed []string
		email   string
		want    bool
	}{
		{"empty list allows any", nil, "a@anything.io", true},
		{"empty list allows a malformed address too", []string{}, "not-an-email", true},
		{"exact", []string{"example.com"}, "a@example.com", true},
		{"exact, other case in email", []string{"example.com"}, "A@Example.COM", true},
		{"exact, other case in list", []string{"Example.Com"}, "a@example.com", true},
		{"subdomain", []string{"example.com"}, "a@dev.example.com", true},
		{"deep subdomain", []string{"example.com"}, "a@x.y.example.com", true},
		{"second entry matches", []string{"other.org", "example.com"}, "a@example.com", true},
		{"look-alike suffix", []string{"example.com"}, "a@evilexample.com", false},
		{"look-alike prefix", []string{"example.com"}, "a@example.com.evil.io", false},
		{"parent domain is not allowed by a subdomain entry", []string{"dev.example.com"}, "a@example.com", false},
		{"unrelated", []string{"example.com"}, "a@other.org", false},
		{"local part names the domain", []string{"example.com"}, "a@example.com@evil.io", false},
		{"local part with the domain after a quoted at", []string{"example.com"}, "evil@example.com@other.org", false},
		{"no at sign", []string{"example.com"}, "example.com", false},
		{"empty email", []string{"example.com"}, "", false},
		{"empty domain", []string{"example.com"}, "a@", false},
		{"trailing dot", []string{"example.com"}, "a@example.com.", true},
		{"surrounding space", []string{"example.com"}, "  a@example.com ", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DomainAllowed(c.allowed, c.email); got != c.want {
				t.Errorf("DomainAllowed(%v, %q) = %v, want %v", c.allowed, c.email, got, c.want)
			}
		})
	}
}

func TestNormalizeDomains(t *testing.T) {
	got, err := NormalizeDomains([]string{" Example.COM ", "@dev.example.org", "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"example.com", "dev.example.org"}; !reflect.DeepEqual(got, want) {
		t.Errorf("NormalizeDomains = %v, want %v", got, want)
	}
	if got, err := NormalizeDomains(nil); err != nil || got == nil || len(got) != 0 {
		t.Errorf("NormalizeDomains(nil) = %v, %v; want an empty non-nil list", got, err)
	}
	for _, bad := range []string{"", "  ", "a b.com", "a@b.com", "-x.com", "x..com", "nodot", "exa/mple.com", ".example.com", "*.example.com"} {
		if _, err := NormalizeDomains([]string{bad}); err == nil {
			t.Errorf("NormalizeDomains(%q) accepted", bad)
		}
	}
	many := make([]string, MaxDomains+1)
	for i := range many {
		many[i] = "d" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".com"
	}
	if _, err := NormalizeDomains(many); err == nil {
		t.Errorf("NormalizeDomains accepted %d domains", len(many))
	}
}

func TestHasMFA(t *testing.T) {
	if !HasMFA([]string{"otp", "mfa"}) {
		t.Error("otp+mfa has no mfa")
	}
	if HasMFA([]string{"otp"}) || HasMFA([]string{"google"}) || HasMFA(nil) {
		t.Error("a session without the mfa method passed")
	}
	if HasMFA([]string{"mfa_pending", "sso:mfa"}) {
		t.Error("a look-alike method passed")
	}
}

func TestSessionTooOld(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	hour := int64(3600)
	cases := []struct {
		name     string
		authTime int64
		known    bool
		hours    int32
		want     bool
	}{
		{"no limit", now.Unix() - 1000*hour, true, 0, false},
		{"within the limit", now.Unix() - 5*hour, true, 8, false},
		{"exactly at the limit", now.Unix() - 8*hour, true, 8, false},
		{"one second past the limit", now.Unix() - 8*hour - 1, true, 8, true},
		{"auth time in the future", now.Unix() + hour, true, 8, false},
		{"limit set, auth time unknown", 0, false, 8, true},
		{"no limit, auth time unknown", 0, false, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SessionTooOld(c.authTime, c.known, now, c.hours); got != c.want {
				t.Errorf("SessionTooOld = %v, want %v", got, c.want)
			}
		})
	}
}

func TestChangesNamesOldAndNewOfChangedFieldsOnly(t *testing.T) {
	old := Default()
	next := old
	next.RequireMFA = true
	next.SessionMaxAgeHours = 12
	next.AllowedInviteDomains = []string{"example.com"}
	got := Changes(old, next)
	want := map[string]any{
		"require_mfa":            map[string]any{"old": false, "new": true},
		"session_max_age_hours":  map[string]any{"old": int32(0), "new": int32(12)},
		"allowed_invite_domains": map[string]any{"old": []string{}, "new": []string{"example.com"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Changes = %#v, want %#v", got, want)
	}
	if c := Changes(old, old); len(c) != 0 {
		t.Errorf("Changes of equal policies = %v, want none", c)
	}
	off := old
	off.ShareLinksEnabled = false
	off.RequireSSO = true
	c := Changes(old, off)
	if len(c) != 2 || c["share_links_enabled"] == nil || c["require_sso"] == nil {
		t.Errorf("Changes = %v, want share_links_enabled and require_sso", c)
	}
}
