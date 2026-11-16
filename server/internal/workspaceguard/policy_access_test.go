package workspaceguard

import (
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/workspacepolicy"
)

func TestCheckWorkspaceAccess(t *testing.T) {
	now := time.Unix(2_000_000, 0)
	fresh := now.Unix() - 3600
	stale := now.Unix() - 48*3600
	otp := []string{"otp"}
	mfa := []string{"otp", "mfa"}

	base := workspacepolicy.Default()
	mfaOn := base
	mfaOn.RequireMFA = true
	ageOn := base
	ageOn.SessionMaxAgeHours = 24
	both := mfaOn
	both.SessionMaxAgeHours = 24
	ssoOn := base
	ssoOn.RequireSSO = true

	cases := []struct {
		name string
		p    workspacepolicy.Policy
		s    Session
		want string
	}{
		{"defaults pass", base, Session{AMR: otp, AuthTime: stale, AuthTimeKnown: true}, ""},
		{"mfa required, session has no mfa", mfaOn, Session{AMR: otp, AuthTime: fresh, AuthTimeKnown: true}, CodeMFARequired},
		{"mfa required, session has mfa", mfaOn, Session{AMR: mfa, AuthTime: fresh, AuthTimeKnown: true}, ""},
		{"age limit, session too old", ageOn, Session{AMR: otp, AuthTime: stale, AuthTimeKnown: true}, CodeReauthRequired},
		{"age limit, session fresh", ageOn, Session{AMR: otp, AuthTime: fresh, AuthTimeKnown: true}, ""},
		{"age limit, auth time unknown", ageOn, Session{AMR: otp}, CodeReauthRequired},
		{"both, mfa is reported first", both, Session{AMR: otp, AuthTime: stale, AuthTimeKnown: true}, CodeMFARequired},
		{"both, mfa present but old", both, Session{AMR: mfa, AuthTime: stale, AuthTimeKnown: true}, CodeReauthRequired},
		{"require_sso with no connection facts refuses nothing", ssoOn, Session{AMR: otp, AuthTime: fresh, AuthTimeKnown: true}, ""},
		{"machine credential skips mfa", mfaOn, Session{Machine: true}, ""},
		{"machine credential skips the age limit", ageOn, Session{Machine: true}, ""},
		{"machine credential skips both", both, Session{Machine: true, AMR: otp, AuthTime: stale, AuthTimeKnown: true}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CheckWorkspaceAccess(c.p, c.s, now); got != c.want {
				t.Errorf("CheckWorkspaceAccess = %q, want %q", got, c.want)
			}
		})
	}
}

func TestPolicyCodesAndMessages(t *testing.T) {
	if CodeMFARequired != "mfa_required" || CodeReauthRequired != "reauth_required" {
		t.Fatalf("codes = %q, %q", CodeMFARequired, CodeReauthRequired)
	}
	for _, c := range []string{CodeMFARequired, CodeReauthRequired} {
		if Message(c) == Message(CodeSuspended) {
			t.Errorf("Message(%s) falls through to the suspended text", c)
		}
	}
}
