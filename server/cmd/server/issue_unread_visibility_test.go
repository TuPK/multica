package main

// The "unread" mark an agent run leaves for the subscribers of an issue follows
// issue visibility (spec 7.1 rule 4): a subscriber who can no longer see the
// issue gets no mark and no event about it.

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func TestIssueUnreadSkipsHiddenSubscriber(t *testing.T) {
	f := newListenerVisibilityFixture(t)
	f.fx.Cleanup(t, `DELETE FROM issue_unread WHERE issue_id = ANY($1::uuid[])`, []string{f.hiddenIssueID, f.openIssueID})
	bus := events.New()
	registerIssueUnreadListeners(bus, f.queries)
	var unreadEvents []map[string]any
	bus.Subscribe(protocol.EventIssueUnreadChanged, func(e events.Event) {
		unreadEvents = append(unreadEvents, e.Payload.(map[string]any))
	})

	// Subscriptions written while the project was open: the outsider lost access
	// since, so the rows are still there.
	for _, issueID := range []string{f.hiddenIssueID, f.openIssueID} {
		addTestSubscriber(t, issueID, "member", f.outsiderID, "manual")
		addTestSubscriber(t, issueID, "member", f.granteeID, "manual")
	}
	runEnded := func(issueID string) {
		bus.Publish(events.Event{
			Type: protocol.EventTaskCompleted, WorkspaceID: testWorkspaceID,
			Payload: map[string]any{"issue_id": issueID, "agent_id": "", "task_id": ""},
		})
	}
	unreadRows := func(issueID, userID string) int {
		return f.fx.Count(t, `SELECT count(*) FROM issue_unread WHERE issue_id = $1 AND user_id = $2`, issueID, userID)
	}

	runEnded(f.hiddenIssueID)
	if n := unreadRows(f.hiddenIssueID, f.outsiderID); n != 0 {
		t.Errorf("a subscriber who cannot see the issue got %d unread rows", n)
	}
	if n := unreadRows(f.hiddenIssueID, f.granteeID); n != 1 {
		t.Errorf("a subscriber who sees the issue got %d unread rows, want 1", n)
	}
	for _, p := range unreadEvents {
		if p["recipient_id"] == f.outsiderID {
			t.Errorf("issue:unread_changed published for the subscriber who cannot see the issue: %v", p)
		}
	}

	// Regression: on an issue the outsider sees, the mark is written.
	runEnded(f.openIssueID)
	if n := unreadRows(f.openIssueID, f.outsiderID); n != 1 {
		t.Errorf("the outsider got %d unread rows for an issue they see, want 1", n)
	}
}
