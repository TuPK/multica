package main

import (
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// publishDuplicate publishes an issue event for a cancelled duplicate that
// lives in dupProject and points at original, the shape IssueToMapResolved
// produces.
func (w *restrictedWorld) publishDuplicate(eventType, dupID, dupProject, originalID, secret string) {
	issue := map[string]any{
		"id": dupID, "title": "the duplicate", "status": "cancelled", "project_id": dupProject,
		"duplicate_of": map[string]any{
			"id": originalID, "identifier": "SECRET-1", "title": secret, "status": "in_progress",
		},
	}
	w.bus.Publish(events.Event{
		Type: eventType, WorkspaceID: testWorkspaceID, ActorType: "member", ActorID: testUserID,
		Payload: map[string]any{"issue": issue},
	})
}

// A hidden original behaves as missing for every recipient who cannot see it
// (spec 7.1 rule 5 and 13): the duplicate's event reaches its own audience,
// but only people who see the original get the original's fields.
func TestDuplicateOfHiddenOriginalNotLeakedToEventRecipients(t *testing.T) {
	const secret = "hidden original title"
	for _, eventType := range []string{protocol.EventIssueCreated, protocol.EventIssueUpdated} {
		t.Run(eventType, func(t *testing.T) {
			w := newRestrictedWorld(t, "owner", "member", "other", "guest", "otherGuest")
			open := w.project("Duplicates", "workspace", "guest", "otherGuest")
			hidden := w.project("Hidden originals", "restricted", "member", "guest")
			original := w.issueIn(secret, hidden)
			dup := w.issueIn("the duplicate", open)

			w.publishDuplicate(eventType, dup, open, original, secret)

			for name, sees := range map[string]bool{
				"owner": true, "member": true, "guest": true, "other": false, "otherGuest": false,
			} {
				frames := ofType(w.received(name), eventType)
				if len(frames) != 1 {
					t.Fatalf("%s: got %d %s frames, want 1", name, len(frames), eventType)
				}
				raw := frames[0].raw
				if !strings.Contains(raw, "the duplicate") {
					t.Errorf("%s: duplicate itself missing from %s", name, raw)
				}
				has := strings.Contains(raw, secret) || strings.Contains(raw, "SECRET-1") || strings.Contains(raw, "duplicate_of")
				if has != sees {
					t.Errorf("%s: duplicate_of present = %v, want %v: %s", name, has, sees, raw)
				}
			}
		})
	}
}

// An original everyone sees keeps the shared workspace fanout and payload.
func TestDuplicateOfVisibleOriginalKeepsPayload(t *testing.T) {
	w := newRestrictedWorld(t, "owner", "member", "other")
	open := w.project("Duplicates", "workspace")
	original := w.fx.Issue(t, "visible original", testutil.Cols{"project_id": open})
	dup := w.issueIn("the duplicate", open)

	w.publishDuplicate(protocol.EventIssueUpdated, dup, open, original, "visible original")

	for _, name := range []string{"owner", "member", "other"} {
		frames := ofType(w.received(name), protocol.EventIssueUpdated)
		if len(frames) != 1 || !strings.Contains(frames[0].raw, "SECRET-1") {
			t.Errorf("%s: want the full duplicate_of, got %v", name, frames)
		}
	}
}

// A workspace-wide original without grants is visible to every non-guest
// member, so a member granted on the restricted duplicate project gets the
// full duplicate_of. A guest granted only on the duplicate project does not.
func TestDuplicateOfWorkspaceOriginalReachesGrantedMember(t *testing.T) {
	const secret = "workspace original title"
	for _, eventType := range []string{protocol.EventIssueCreated, protocol.EventIssueUpdated} {
		t.Run(eventType, func(t *testing.T) {
			w := newRestrictedWorld(t, "owner", "member", "other", "guest")
			restricted := w.project("Restricted duplicates", "restricted", "member", "guest")
			wide := w.project("Workspace originals", "workspace")
			original := w.issueIn(secret, wide)
			dup := w.issueIn("the duplicate", restricted)

			w.publishDuplicate(eventType, dup, restricted, original, secret)

			for name, sees := range map[string]bool{"member": true, "guest": false} {
				frames := ofType(w.received(name), eventType)
				if len(frames) != 1 {
					t.Fatalf("%s: got %d %s frames, want 1", name, len(frames), eventType)
				}
				raw := frames[0].raw
				if !strings.Contains(raw, "the duplicate") {
					t.Errorf("%s: duplicate itself missing from %s", name, raw)
				}
				has := strings.Contains(raw, secret) || strings.Contains(raw, "SECRET-1")
				if has != sees {
					t.Errorf("%s: duplicate_of present = %v, want %v: %s", name, has, sees, raw)
				}
			}
		})
	}
}
