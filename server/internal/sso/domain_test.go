package sso

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

type fakeLookup struct {
	records map[string][]string
	err     error
	asked   []string
}

func (f *fakeLookup) LookupTXT(_ context.Context, name string) ([]string, error) {
	f.asked = append(f.asked, name)
	if f.err != nil {
		return nil, f.err
	}
	recs, ok := f.records[name]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
	return recs, nil
}

func TestDomainPublicEmailDomainsAreRefused(t *testing.T) {
	for _, d := range []string{
		"gmail.com", "googlemail.com", "outlook.com", "hotmail.com", "live.com",
		"yahoo.com", "icloud.com", "aol.com", "proton.me", "protonmail.com",
		"gmx.com", "mail.com", "yandex.com", "qq.com", "163.com",
		// Any subdomain of a public domain is public too.
		"mail.gmail.com",
		// The check ignores case and a trailing dot.
		"GMAIL.com", "gmail.com.",
	} {
		if !IsPublicEmailDomain(d) {
			t.Errorf("IsPublicEmailDomain(%q) = false, want true", d)
		}
	}
	for _, d := range []string{"example.com", "notgmail.com", "gmail.com.evil.io", "gmail.company", "acme.io"} {
		if IsPublicEmailDomain(d) {
			t.Errorf("IsPublicEmailDomain(%q) = true, want false", d)
		}
	}
}

func TestDomainPublicEmailListIsWellFormed(t *testing.T) {
	if len(publicEmailDomains) < 20 {
		t.Fatalf("the public list has %d entries; it should cover the common providers", len(publicEmailDomains))
	}
	seen := map[string]bool{}
	for _, d := range publicEmailDomains {
		norm, err := NormalizeDomain(d)
		if err != nil || norm != d {
			t.Errorf("entry %q is not a normalized domain (got %q, %v)", d, norm, err)
		}
		if seen[d] {
			t.Errorf("entry %q is listed twice", d)
		}
		seen[d] = true
	}
}

func TestDomainNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"example.com":       "example.com",
		"  Example.COM  ":   "example.com",
		"example.com.":      "example.com",
		"sub.example.co.uk": "sub.example.co.uk",
	} {
		got, err := NormalizeDomain(in)
		if err != nil || got != want {
			t.Errorf("NormalizeDomain(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "localhost", "a b.com", "a@example.com", "-x.example.com", "example..com", "http://example.com", "example.com/path"} {
		if got, err := NormalizeDomain(in); err == nil {
			t.Errorf("NormalizeDomain(%q) = %q, want an error", in, got)
		}
	}
}

func TestDomainTokensAreRandomAndUnguessable(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewToken()
	if a == b {
		t.Fatal("two tokens are equal")
	}
	if len(a) < 32 {
		t.Fatalf("token %q has %d characters; want at least 32", a, len(a))
	}
}

func TestDomainChallengeRecord(t *testing.T) {
	if got := ChallengeName("example.com"); got != "_multica-challenge.example.com" {
		t.Errorf("ChallengeName = %q", got)
	}
	if got := ChallengeValue("abc"); got != "multica-verification=abc" {
		t.Errorf("ChallengeValue = %q", got)
	}
}

func TestDomainTXTVerification(t *testing.T) {
	const domain, token = "example.com", "tok123"
	name, value := ChallengeName(domain), ChallengeValue(token)

	cases := map[string]struct {
		lookup *fakeLookup
		want   bool
	}{
		"the right record":              {&fakeLookup{records: map[string][]string{name: {value}}}, true},
		"the right record among others": {&fakeLookup{records: map[string][]string{name: {"v=spf1 -all", value}}}, true},
		"no record":                     {&fakeLookup{records: map[string][]string{}}, false},
		"a wrong token":                 {&fakeLookup{records: map[string][]string{name: {ChallengeValue("other")}}}, false},
		"the token without its prefix":  {&fakeLookup{records: map[string][]string{name: {token}}}, false},
		"the token as a substring":      {&fakeLookup{records: map[string][]string{name: {value + "x"}}}, false},
		"the record at the wrong name":  {&fakeLookup{records: map[string][]string{domain: {value}}}, false},
		"the record of another domain":  {&fakeLookup{records: map[string][]string{ChallengeName("other.com"): {value}}}, false},
		"an empty answer":               {&fakeLookup{records: map[string][]string{name: {}}}, false},
	}
	for label, c := range cases {
		got, err := CheckTXT(context.Background(), c.lookup, domain, token)
		if err != nil || got != c.want {
			t.Errorf("%s: CheckTXT = %v, %v; want %v, nil", label, got, err, c.want)
		}
	}

	lookup := &fakeLookup{records: map[string][]string{name: {value}}}
	CheckTXT(context.Background(), lookup, domain, token)
	if len(lookup.asked) != 1 || lookup.asked[0] != name {
		t.Errorf("asked %v, want only %q", lookup.asked, name)
	}

	// An empty token never verifies, even against an empty-looking record.
	if ok, _ := CheckTXT(context.Background(), &fakeLookup{records: map[string][]string{name: {ChallengeValue("")}}}, domain, ""); ok {
		t.Error("an empty token verified")
	}
}

func TestDomainTXTLookupFailureIsAnErrorNotAMismatch(t *testing.T) {
	boom := errors.New("resolver down")
	ok, err := CheckTXT(context.Background(), &fakeLookup{err: boom}, "example.com", "tok")
	if ok || !errors.Is(err, boom) {
		t.Fatalf("CheckTXT = %v, %v; want false and the resolver error", ok, err)
	}
	if strings.Contains(err.Error(), "tok") {
		t.Errorf("error %q repeats the token", err)
	}
}
