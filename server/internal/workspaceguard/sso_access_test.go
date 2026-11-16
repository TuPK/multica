package workspaceguard

import (
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/workspacepolicy"
)

func TestSSORequiredDecision(t *testing.T) {
	now := time.Unix(2_000_000, 0)
	on := workspacepolicy.Default()
	on.RequireSSO = true
	off := workspacepolicy.Default()

	const conn = "c0nn-1"
	base := Session{AMR: []string{"otp"}, AuthTime: now.Unix(), AuthTimeKnown: true, SSOConnectionID: conn, SSOCovered: true}
	with := func(f func(*Session)) Session { s := base; f(&s); return s }

	cases := []struct {
		name string
		p    workspacepolicy.Policy
		s    Session
		want string
	}{
		{"member signed in by email code is refused", on, base, CodeSSORequired},
		{"member signed in by google is refused", on, with(func(s *Session) { s.AMR = []string{"google"} }), CodeSSORequired},
		{"member signed in through this connection passes", on, with(func(s *Session) { s.AMR = []string{"sso:" + conn} }), ""},
		{"member signed in through another connection is refused", on, with(func(s *Session) { s.AMR = []string{"sso:other"} }), CodeSSORequired},
		{"sso plus mfa passes", on, with(func(s *Session) { s.AMR = []string{"sso:" + conn, "mfa"} }), ""},
		{"the owner is not refused", on, with(func(s *Session) { s.IsOwner = true }), ""},
		{"a member outside the verified domains is not refused", on, with(func(s *Session) { s.SSOCovered = false }), ""},
		{"no active connection refuses nobody", on, with(func(s *Session) { s.SSOConnectionID = "" }), ""},
		{"policy off refuses nobody", off, base, ""},
		{"machine credential is exempt", on, Session{Machine: true}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CheckWorkspaceAccess(c.p, c.s, now); got != c.want {
				t.Errorf("CheckWorkspaceAccess = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSSORequiredHasAMessage(t *testing.T) {
	if CodeSSORequired != "sso_required" {
		t.Fatalf("CodeSSORequired = %q, want sso_required", CodeSSORequired)
	}
	if got := Message(CodeSSORequired); got == "" || got == Message("something-else") {
		t.Fatalf("Message(sso_required) = %q, want its own text", got)
	}
}
