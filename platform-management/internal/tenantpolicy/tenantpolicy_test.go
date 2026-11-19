package tenantpolicy

import (
	"slices"
	"strings"
	"testing"
)

// Spec R6 section 3c.4: the floor uses the workspace policy's bounds and records changed fields.

func TestNormalizeDomainsTrimsLowersAndDedupes(t *testing.T) {
	got, err := NormalizeDomains([]string{" Example.COM ", "example.com", "corp.test"})
	if err != nil {
		t.Fatalf("NormalizeDomains: %v", err)
	}
	if !slices.Equal(got, []string{"example.com", "corp.test"}) {
		t.Fatalf("domains = %v", got)
	}
	for _, bad := range []string{"", "a b.com", "user@example.com"} {
		if _, err := NormalizeDomains([]string{bad}); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	many := make([]string, MaxDomains+1)
	for i := range many {
		many[i] = strings.Repeat("x", i+1) + ".test"
	}
	if _, err := NormalizeDomains(many); err == nil {
		t.Error("more than MaxDomains must be refused")
	}
}

func TestValidateSessionBounds(t *testing.T) {
	if err := Validate(Policy{SessionMaxAgeHours: MaxSessionMaxAgeHours}); err != nil {
		t.Fatalf("the upper bound is allowed: %v", err)
	}
	if err := Validate(Policy{SessionMaxAgeHours: MaxSessionMaxAgeHours + 1}); err == nil {
		t.Fatal("above the bound must be refused")
	}
	if err := Validate(Policy{SessionMaxAgeHours: -1}); err == nil {
		t.Fatal("a negative age must be refused")
	}
}

func TestChangedNamesOnlyDifferingFields(t *testing.T) {
	a := Default()
	b := a
	b.RequireSSO = true
	b.SessionMaxAgeHours = 12
	got := Changed(a, b)
	if !slices.Equal(got, []string{"require_sso", "session_max_age_hours"}) {
		t.Fatalf("changed = %v", got)
	}
	if len(Changed(a, a)) != 0 {
		t.Fatal("equal policies changed fields")
	}
}
