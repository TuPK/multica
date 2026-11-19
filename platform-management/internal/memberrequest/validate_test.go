package memberrequest

import (
	"slices"
	"strings"
	"testing"
)

func TestMemberRequestValidate(t *testing.T) {
	longEmail := strings.Repeat("a", 250) + "@x.io" // 255 bytes
	cases := []struct {
		name       string
		in         Input
		wantEmail  string
		wantRole   string
		wantCode   string
		wantFields []string
	}{
		{"valid member", Input{Email: "Ann@Example.com", Role: "member"}, "ann@example.com", "member", "", nil},
		{"trims and lower-cases", Input{Email: "  Bob@Example.COM ", Role: " viewer "}, "bob@example.com", "viewer", "", nil},
		{"admin role", Input{Email: "c@example.com", Role: "admin"}, "c@example.com", "admin", "", nil},
		{"no email", Input{Email: "", Role: "member"}, "", "", CodeInvalid, []string{"email"}},
		{"not an address", Input{Email: "not-an-email", Role: "member"}, "", "", CodeInvalid, []string{"email"}},
		{"display name form", Input{Email: "Ann <ann@example.com>", Role: "member"}, "", "", CodeInvalid, []string{"email"}},
		{"domain without a dot", Input{Email: "ann@localhost", Role: "member"}, "", "", CodeInvalid, []string{"email"}},
		{"over 254 bytes", Input{Email: longEmail, Role: "member"}, "", "", CodeInvalid, []string{"email"}},
		{"exactly 254 bytes", Input{Email: strings.Repeat("a", 249) + "@x.io", Role: "member"}, strings.Repeat("a", 249) + "@x.io", "member", "", nil},
		{"owner is refused", Input{Email: "a@example.com", Role: "owner"}, "", "", CodeRoleInvalid, nil},
		{"unknown role", Input{Email: "a@example.com", Role: "superuser"}, "", "", CodeRoleInvalid, nil},
		{"staff role is not a member role", Input{Email: "a@example.com", Role: "system_admin"}, "", "", CodeRoleInvalid, nil},
		{"missing role", Input{Email: "a@example.com"}, "", "", CodeRoleInvalid, nil},
		{"role is case sensitive", Input{Email: "a@example.com", Role: "Admin"}, "", "", CodeRoleInvalid, nil},
		{"a bad email wins over a bad role", Input{Email: "x", Role: "owner"}, "", "", CodeInvalid, []string{"email"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateInput(tc.in)
			if tc.wantCode == "" {
				if err != nil || got.Email != tc.wantEmail || got.Role != tc.wantRole {
					t.Fatalf("ValidateInput = %+v, %v; want %q %q", got, err, tc.wantEmail, tc.wantRole)
				}
				return
			}
			if err == nil {
				t.Fatalf("no error, want %s", tc.wantCode)
			}
			if err.Status != 400 || err.Code != tc.wantCode || !slices.Equal(err.Fields, tc.wantFields) {
				t.Fatalf("error = %d %s %v, want 400 %s %v", err.Status, err.Code, err.Fields, tc.wantCode, tc.wantFields)
			}
		})
	}
}
