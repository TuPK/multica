package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/multica-ai/multica/server/internal/planlimit"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The matching rules are tested in package planlimit, which needs no database.
// These tests cover the wiring in checkSignupAllowed. They run with the rest of
// this package, whose TestMain needs a test database.
func TestSignupBlockedDomains(t *testing.T) {
	t.Setenv(planlimit.BlockedDomainsEnv, "Example.com, spam.io")
	cases := []struct {
		email string
		isNew bool
		want  error
	}{
		{"user@example.com", true, ErrEmailNotAllowed},      // exact
		{"user@mail.example.com", true, ErrEmailNotAllowed}, // subdomain
		{"USER@EXAMPLE.COM", true, ErrEmailNotAllowed},      // case-insensitive
		{"user@spam.io", true, ErrEmailNotAllowed},          // second entry
		{"user@notexample.com", true, nil},                  // look-alike
		{"user@example.org", true, nil},
		{"user@example.com", false, nil}, // an existing account still signs in
	}
	for _, tc := range cases {
		h := newTestHandler(Config{AllowSignup: true})
		h.Queries = db.New(&mockDB{})
		if err := h.checkSignupAllowed(context.Background(), tc.email, tc.isNew); !errors.Is(err, tc.want) {
			t.Errorf("checkSignupAllowed(%q, new=%v) = %v, want %v", tc.email, tc.isNew, err, tc.want)
		}
	}
}

// The list is an abuse control, so no allowance overrides it: not the email
// allowlist, not the domain allowlist, not a pending invitation.
func TestSignupBlockedDomainBeatsEveryAllowance(t *testing.T) {
	t.Setenv(planlimit.BlockedDomainsEnv, "example.com")
	cfgs := map[string]Config{
		"email allowlist":  {AllowSignup: false, AllowedEmails: []string{"user@example.com"}},
		"domain allowlist": {AllowSignup: false, AllowedEmailDomains: []string{"example.com"}},
		"open signup":      {AllowSignup: true},
	}
	for name, cfg := range cfgs {
		h := newTestHandler(cfg)
		h.Queries = db.New(&mockDB{pendingInvitation: true})
		if err := h.checkSignupAllowed(context.Background(), "user@example.com", true); !errors.Is(err, ErrEmailNotAllowed) {
			t.Errorf("%s: err = %v, want ErrEmailNotAllowed", name, err)
		}
	}
}

// With the variable unset nothing changes.
func TestSignupWithoutABlockListIsUnchanged(t *testing.T) {
	t.Setenv(planlimit.BlockedDomainsEnv, "")
	h := newTestHandler(Config{AllowSignup: true})
	h.Queries = db.New(&mockDB{})
	if err := h.checkSignupAllowed(context.Background(), "user@example.com", true); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}
