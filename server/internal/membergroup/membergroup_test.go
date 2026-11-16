package membergroup

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// The catalogue is exactly the six keys of spec 7b.2. Adding a key is a spec
// change, a constant, a route mapping and a test, so this list is pinned.
func TestMemberGroupCatalogueIsTheSixKeys(t *testing.T) {
	want := []string{
		"members.invite", "audit.read", "billing.view", "billing.manage",
		"integrations.manage", "runtime_profiles.manage",
	}
	got := Catalogue()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Catalogue() = %v, want %v", got, want)
	}
	for _, key := range want {
		if !IsKnown(key) {
			t.Errorf("IsKnown(%q) = false", key)
		}
	}
	for _, key := range []string{"", "members", "groups.manage", "Members.Invite", "members.invite ", "admin"} {
		if IsKnown(key) {
			t.Errorf("IsKnown(%q) = true, want false", key)
		}
	}
	// Mutating the returned slice must not change the catalogue.
	got[0] = "tampered"
	if Catalogue()[0] != "members.invite" {
		t.Fatal("Catalogue() returns its backing array")
	}
}

func TestMemberGroupLimitsAreTheSpecValues(t *testing.T) {
	if MaxMembers != 1000 || MaxAddPerRequest != 100 || MaxNameLength != 64 || MaxDescriptionLength != 280 {
		t.Fatalf("limits = %d/%d/%d/%d, want 1000/100/64/280",
			MaxMembers, MaxAddPerRequest, MaxNameLength, MaxDescriptionLength)
	}
}

func TestMemberGroupNormalizeName(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		invalid bool
	}{
		{"plain", "Finance", "Finance", false},
		{"trimmed", "  Finance \t", "Finance", false},
		{"empty", "", "", true},
		{"blank", "   ", "", true},
		{"64 characters", strings.Repeat("a", 64), strings.Repeat("a", 64), false},
		{"65 characters", strings.Repeat("a", 65), "", true},
		// The limit counts characters, not bytes.
		{"64 multibyte characters", strings.Repeat("é", 64), strings.Repeat("é", 64), false},
		{"65 multibyte characters", strings.Repeat("é", 65), "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NormalizeName(c.in)
			if c.invalid {
				if !errors.Is(err, ErrNameInvalid) {
					t.Fatalf("NormalizeName(%q) = %q, %v; want ErrNameInvalid", c.in, got, err)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("NormalizeName(%q) = %q, %v; want %q", c.in, got, err, c.want)
			}
		})
	}
}

func TestMemberGroupNormalizeDescription(t *testing.T) {
	if got, err := NormalizeDescription(""); err != nil || got != "" {
		t.Fatalf("empty description = %q, %v; want it accepted", got, err)
	}
	if got, err := NormalizeDescription("  the accountants  "); err != nil || got != "the accountants" {
		t.Fatalf("description = %q, %v; want it trimmed", got, err)
	}
	if _, err := NormalizeDescription(strings.Repeat("x", 280)); err != nil {
		t.Fatalf("280 characters: %v", err)
	}
	if _, err := NormalizeDescription(strings.Repeat("x", 281)); !errors.Is(err, ErrDescriptionInvalid) {
		t.Fatalf("281 characters: err = %v, want ErrDescriptionInvalid", err)
	}
	if _, err := NormalizeDescription(strings.Repeat("é", 281)); !errors.Is(err, ErrDescriptionInvalid) {
		t.Fatalf("281 multibyte characters: err = %v, want ErrDescriptionInvalid", err)
	}
}

func TestMemberGroupNormalizePermissionsRejectsAnUnknownKey(t *testing.T) {
	for _, bad := range []string{"groups.manage", "", "Billing.View", "billing.view "} {
		_, err := NormalizePermissions([]string{"billing.view", bad})
		var unknown *UnknownPermissionError
		if !errors.As(err, &unknown) {
			t.Fatalf("NormalizePermissions with %q: err = %v, want *UnknownPermissionError", bad, err)
		}
		if unknown.Key != bad {
			t.Errorf("UnknownPermissionError.Key = %q, want %q", unknown.Key, bad)
		}
	}
}

func TestMemberGroupNormalizePermissionsDedupesAndUsesCatalogueOrder(t *testing.T) {
	got, err := NormalizePermissions([]string{"runtime_profiles.manage", "billing.view", "members.invite", "billing.view"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"members.invite", "billing.view", "runtime_profiles.manage"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// An empty or nil set is an empty slice, never nil: it is stored as [].
	for _, in := range [][]string{nil, {}} {
		got, err := NormalizePermissions(in)
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("NormalizePermissions(%v) = %#v, %v; want a non-nil empty slice", in, got, err)
		}
	}
}

func TestMemberGroupEffectivePermissionsOfTheRoles(t *testing.T) {
	all := []string{"audit.read", "billing.manage", "billing.view", "integrations.manage", "members.invite", "runtime_profiles.manage"}
	for _, role := range []string{"owner", "admin"} {
		if got := Effective(role); !reflect.DeepEqual(got, all) {
			t.Errorf("Effective(%s) = %v, want every key sorted", role, got)
		}
	}
	for _, role := range []string{"member", "viewer", "", "guest"} {
		if got := Effective(role); got == nil || len(got) != 0 {
			t.Errorf("Effective(%q) = %#v, want a non-nil empty slice", role, got)
		}
	}
}

func TestMemberGroupEffectivePermissionsAreTheUnionOfTheGroups(t *testing.T) {
	got := Effective("member",
		[]string{"audit.read"},
		[]string{"members.invite", "audit.read"},
	)
	want := []string{"audit.read", "members.invite"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// A viewer takes the keys of its groups like a member: the write ban is
	// the viewer guard's, not the key set's.
	if got := Effective("viewer", []string{"billing.view"}); !reflect.DeepEqual(got, []string{"billing.view"}) {
		t.Fatalf("viewer = %v", got)
	}
}

func TestMemberGroupBillingManageImpliesBillingView(t *testing.T) {
	got := Effective("member", []string{"billing.manage"})
	want := []string{"billing.manage", "billing.view"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// billing.view does not imply billing.manage.
	if got := Effective("member", []string{"billing.view"}); !reflect.DeepEqual(got, []string{"billing.view"}) {
		t.Fatalf("billing.view alone = %v", got)
	}
}

// A key written by another build grants nothing, whatever it looks like.
func TestMemberGroupStoredUnknownKeysAreIgnored(t *testing.T) {
	got := Effective("member",
		[]string{"audit.read", "groups.manage", "future.key", "admin", ""},
		[]string{"nothing.at.all"},
	)
	if !reflect.DeepEqual(got, []string{"audit.read"}) {
		t.Fatalf("got %v, want only audit.read", got)
	}
	if got := Effective("member", []string{"groups.manage"}); len(got) != 0 {
		t.Fatalf("an unknown key granted %v", got)
	}
}

// Rule 6: no key opens the group routes, so no group can grant managing groups.
func TestMemberGroupNoKeyIsAGroupManagementKey(t *testing.T) {
	for _, key := range Catalogue() {
		if strings.HasPrefix(key, "groups.") || strings.Contains(key, "group") {
			t.Errorf("catalogue key %q concerns groups", key)
		}
	}
}

func TestMemberGroupHas(t *testing.T) {
	eff := Effective("member", []string{"billing.manage"})
	if !Has(eff, KeyBillingView) || !Has(eff, KeyBillingManage) || Has(eff, KeyAuditRead) {
		t.Fatalf("Has over %v is wrong", eff)
	}
	if Has(nil, KeyAuditRead) {
		t.Fatal("Has(nil) = true")
	}
}
