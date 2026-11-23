package main

import (
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// A duplicate in the open names its original in the issue events it publishes.
// When the original is in a private project, the workspace gets the event
// without that reference and only the original's members get it whole.
func TestRegisterListeners_DuplicateOfPrivateOriginalIsHiddenFromWorkspace(t *testing.T) {
	const secret = "private original title"
	issue := func(original string) map[string]any {
		return map[string]any{
			"id": ppPublicIssue, "project_id": nil, "title": "the duplicate", "status": "cancelled",
			"duplicate_of": map[string]any{"id": original, "identifier": "SEC-1", "title": secret, "status": "todo"},
		}
	}

	t.Run("private original", func(t *testing.T) {
		bus := events.New()
		fb := &fakeBroadcaster{}
		registerListeners(bus, fb, newPrivateProjectAudience(fakeAudienceQuerier{}))
		bus.Publish(events.Event{
			Type: protocol.EventIssueUpdated, WorkspaceID: ppWorkspace,
			Payload: map[string]any{"issue": issue(ppSecretIssue)},
		})

		fb.mu.Lock()
		defer fb.mu.Unlock()
		if len(fb.workspaceCalls) != 1 {
			t.Fatalf("workspace broadcasts = %d, want 1", len(fb.workspaceCalls))
		}
		wide := string(fb.workspaceCalls[0].msg)
		if strings.Contains(wide, secret) || strings.Contains(wide, "duplicate_of") {
			t.Errorf("workspace frame carries the private original: %s", wide)
		}
		if !strings.Contains(wide, "the duplicate") {
			t.Errorf("workspace frame lost the duplicate itself: %s", wide)
		}
		if len(fb.scopeCalls) != 2 {
			t.Fatalf("member frames = %d, want the 2 members of the private project", len(fb.scopeCalls))
		}
		for _, c := range fb.scopeCalls {
			if !strings.Contains(string(c.msg), secret) {
				t.Errorf("member frame lost duplicate_of: %s", c.msg)
			}
		}
	})

	t.Run("open original", func(t *testing.T) {
		bus := events.New()
		fb := &fakeBroadcaster{}
		registerListeners(bus, fb, newPrivateProjectAudience(fakeAudienceQuerier{}))
		bus.Publish(events.Event{
			Type: protocol.EventIssueUpdated, WorkspaceID: ppWorkspace,
			Payload: map[string]any{"issue": issue(ppPublicIssue)},
		})

		fb.mu.Lock()
		defer fb.mu.Unlock()
		if len(fb.workspaceCalls) != 1 || len(fb.scopeCalls) != 0 {
			t.Fatalf("workspace=%d scope=%d, want one workspace frame only", len(fb.workspaceCalls), len(fb.scopeCalls))
		}
		if !strings.Contains(string(fb.workspaceCalls[0].msg), secret) {
			t.Errorf("an open original must keep its reference: %s", fb.workspaceCalls[0].msg)
		}
	})
}
