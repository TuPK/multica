package workspaceguard

import (
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/workspacepolicy"
)

func TestCheckWorkspaceAccess_TokenBinding(t *testing.T) {
	const wsA, wsB = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	now := time.Now()
	cases := []struct {
		name string
		s    Session
		want string
	}{
		{"unbound token, any workspace", Session{Machine: true, WorkspaceID: wsB}, ""},
		{"unbound token, no workspace", Session{Machine: true}, ""},
		{"bound token, same workspace", Session{Machine: true, TokenWorkspaceID: wsA, WorkspaceID: wsA}, ""},
		{"bound token, same workspace in upper case", Session{Machine: true, TokenWorkspaceID: wsA, WorkspaceID: "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA"}, ""},
		{"bound token, other workspace", Session{Machine: true, TokenWorkspaceID: wsA, WorkspaceID: wsB}, CodeTokenWorkspaceMismatch},
		{"bound token, no workspace context", Session{Machine: true, TokenWorkspaceID: wsA}, CodeTokenWorkspaceMismatch},
		{"bound token, no workspace context, allowed route", Session{Machine: true, TokenWorkspaceID: wsA, AllowNoWorkspace: true}, ""},
		{"bound token, other workspace, allowed route", Session{Machine: true, TokenWorkspaceID: wsA, WorkspaceID: wsB, AllowNoWorkspace: true}, CodeTokenWorkspaceMismatch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CheckWorkspaceAccess(workspacepolicy.Default(), c.s, now); got != c.want {
				t.Errorf("CheckWorkspaceAccess = %q, want %q", got, c.want)
			}
		})
	}
}

func TestTokenRefusalMessages(t *testing.T) {
	if CodeTokenReadOnly != "token_read_only" || CodeTokenWorkspaceMismatch != "token_workspace_mismatch" {
		t.Fatalf("codes are part of the API: %q %q", CodeTokenReadOnly, CodeTokenWorkspaceMismatch)
	}
	for _, code := range []string{CodeTokenReadOnly, CodeTokenWorkspaceMismatch} {
		if Message(code) == Message(CodeSuspended) {
			t.Errorf("%s falls through to the suspended message", code)
		}
	}
}
