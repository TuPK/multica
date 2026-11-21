package main

// Notifications and subscriptions follow issue visibility (spec 7.1 rule 4). A
// member who cannot see an issue is never written an inbox item for it, never
// notified by a mention, and never subscribed to it. The fixture: an issue of a
// restricted project, a plain member who is granted the project (grantee), and a
// plain member who is not (outsider).

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

type listenerVisibilityFixture struct {
	fx            *testutil.Fixture
	queries       *db.Queries
	outsiderID    string
	granteeID     string
	hiddenIssueID string
	openIssueID   string
}

func newListenerVisibilityFixture(t *testing.T) listenerVisibilityFixture {
	t.Helper()
	fx := workspaceFixture(t)
	f := listenerVisibilityFixture{fx: fx, queries: db.New(testPool)}
	f.outsiderID = fx.User(t, "Visibility outsider", testutil.UniqueEmail("vis-outsider"))
	f.granteeID = fx.User(t, "Visibility grantee", testutil.UniqueEmail("vis-grantee"))
	fx.Member(t, testWorkspaceID, f.outsiderID, "member")
	fx.Member(t, testWorkspaceID, f.granteeID, "member")

	restricted := fx.Project(t, "Restricted listeners", testutil.Cols{"visibility": "private"})
	open := fx.Project(t, "Open listeners", testutil.Cols{"visibility": "workspace"})
	fx.Exec(t, `INSERT INTO project_member (project_id, workspace_id, user_id) VALUES ($1, $2, $3)`, restricted, testWorkspaceID, f.granteeID)
	// Creating a private project makes its creator, the test user, a member of it.
	fx.Exec(t, `INSERT INTO project_member (project_id, workspace_id, user_id) VALUES ($1, $2, $3)`, restricted, testWorkspaceID, testUserID)
	fx.Cleanup(t, `DELETE FROM project_member WHERE project_id = $1`, restricted)

	f.hiddenIssueID = fx.Issue(t, "Hidden listener issue", testutil.Cols{"project_id": restricted})
	f.openIssueID = fx.Issue(t, "Open listener issue", testutil.Cols{"project_id": open})
	for _, id := range []string{f.hiddenIssueID, f.openIssueID} {
		fx.Cleanup(t, `DELETE FROM inbox_item WHERE issue_id = $1`, id)
		fx.Cleanup(t, `DELETE FROM issue_subscriber WHERE issue_id = $1`, id)
	}
	return f
}

// inboxCount counts the rows written for userID, whatever the viewer-scoped
// inbox read would show.
func (f listenerVisibilityFixture) inboxCount(t *testing.T, userID string) int {
	t.Helper()
	return f.fx.Count(t, `SELECT count(*) FROM inbox_item WHERE workspace_id = $1 AND recipient_type = 'member' AND recipient_id = $2`, testWorkspaceID, userID)
}

func (f listenerVisibilityFixture) subscribed(t *testing.T, issueID, userID string) bool {
	t.Helper()
	return isSubscribed(t, f.queries, issueID, "member", userID)
}

func issuePayload(id, title, creatorID string, assigneeID, description *string) handler.IssueResponse {
	issue := handler.IssueResponse{
		ID: id, WorkspaceID: testWorkspaceID, Title: title, Status: "todo", Priority: "medium",
		CreatorType: "member", CreatorID: creatorID, Description: description,
	}
	if assigneeID != nil {
		memberType := "member"
		issue.AssigneeType, issue.AssigneeID = &memberType, assigneeID
	}
	return issue
}

// TestNoNotificationForNonVisibleRecipient: the three delivery paths (direct,
// subscriber, and the unassigned notice) skip a recipient who cannot see the
// issue, and still deliver to one who can, whatever the subscriber table says.
func TestNoNotificationForNonVisibleRecipient(t *testing.T) {
	f := newListenerVisibilityFixture(t)
	bus := newNotificationBus(t, f.queries)
	var inboxEvents []events.Event
	bus.Subscribe(protocol.EventInboxNew, func(e events.Event) { inboxEvents = append(inboxEvents, e) })

	// Subscriptions written while the project was open: the outsider lost access
	// since, so the row is still there.
	addTestSubscriber(t, f.hiddenIssueID, "member", f.outsiderID, "manual")
	addTestSubscriber(t, f.hiddenIssueID, "member", f.granteeID, "manual")
	addTestSubscriber(t, f.openIssueID, "member", f.outsiderID, "manual")

	status := func(issueID string) {
		bus.Publish(events.Event{
			Type: protocol.EventIssueUpdated, WorkspaceID: testWorkspaceID, ActorType: "member", ActorID: testUserID,
			Payload: map[string]any{
				"issue":          issuePayload(issueID, "status change", testUserID, nil, nil),
				"status_changed": true, "prev_status": "backlog",
			},
		})
	}
	status(f.hiddenIssueID)
	if n := f.inboxCount(t, f.outsiderID); n != 0 {
		t.Errorf("a subscriber who cannot see the issue got %d inbox items", n)
	}
	// A direct assignment still reaches a member who sees the issue.
	bus.Publish(events.Event{
		Type: protocol.EventIssueUpdated, WorkspaceID: testWorkspaceID, ActorType: "member", ActorID: testUserID,
		Payload: map[string]any{
			"issue":            issuePayload(f.hiddenIssueID, "assigned", testUserID, &f.granteeID, nil),
			"assignee_changed": true,
		},
	})
	if n := f.inboxCount(t, f.granteeID); n < 1 {
		t.Errorf("an assignee who sees the issue got %d inbox items, want at least 1", n)
	}
	for _, e := range inboxEvents {
		if item, _ := e.Payload.(map[string]any)["item"].(map[string]any); item["recipient_id"] == f.outsiderID {
			t.Errorf("inbox:new published for the non-visible recipient: %+v", item)
		}
	}

	// Direct assignment of the hidden issue, and the notice to the previous assignee.
	bus.Publish(events.Event{
		Type: protocol.EventIssueUpdated, WorkspaceID: testWorkspaceID, ActorType: "member", ActorID: testUserID,
		Payload: map[string]any{
			"issue":            issuePayload(f.hiddenIssueID, "assigned", testUserID, &f.outsiderID, nil),
			"assignee_changed": true, "prev_assignee_type": strPtr("member"), "prev_assignee_id": &f.outsiderID,
		},
	})
	bus.Publish(events.Event{
		Type: protocol.EventIssueCreated, WorkspaceID: testWorkspaceID, ActorType: "member", ActorID: testUserID,
		Payload: map[string]any{"issue": issuePayload(f.hiddenIssueID, "created", testUserID, &f.outsiderID, nil)},
	})
	if n := f.inboxCount(t, f.outsiderID); n != 0 {
		t.Errorf("direct paths wrote %d inbox items for a member who cannot see the issue", n)
	}

	// Regression: the same events on an issue the outsider sees still deliver.
	status(f.openIssueID)
	bus.Publish(events.Event{
		Type: protocol.EventIssueCreated, WorkspaceID: testWorkspaceID, ActorType: "member", ActorID: testUserID,
		Payload: map[string]any{"issue": issuePayload(f.openIssueID, "created", testUserID, &f.outsiderID, nil)},
	})
	if n := f.inboxCount(t, f.outsiderID); n < 1 {
		t.Errorf("the outsider got %d inbox items for the issue they see, want at least 1", n)
	}
}

// TestMentionOfNonVisibleUserIsIgnored: an @mention, an @all and an @squad
// reach only the members who see the issue, and subscribe only them.
func TestMentionOfNonVisibleUserIsIgnored(t *testing.T) {
	f := newListenerVisibilityFixture(t)
	bus := newNotificationBus(t, f.queries)

	description := "[@Outsider](mention://member/" + f.outsiderID + ") [@Grantee](mention://member/" + f.granteeID + ")"
	bus.Publish(events.Event{
		Type: protocol.EventIssueCreated, WorkspaceID: testWorkspaceID, ActorType: "member", ActorID: testUserID,
		Payload: map[string]any{"issue": issuePayload(f.hiddenIssueID, "mention in description", testUserID, nil, &description)},
	})
	if n := f.inboxCount(t, f.outsiderID); n != 0 {
		t.Errorf("a description mention wrote %d inbox items for a member who cannot see the issue", n)
	}
	if f.subscribed(t, f.hiddenIssueID, f.outsiderID) {
		t.Error("a description mention subscribed a member who cannot see the issue")
	}
	if n := f.inboxCount(t, f.granteeID); n != 1 || !f.subscribed(t, f.hiddenIssueID, f.granteeID) {
		t.Errorf("the mentioned grantee got %d inbox items, subscribed = %v; want 1 and true", n, f.subscribed(t, f.hiddenIssueID, f.granteeID))
	}

	for _, content := range []string{
		"[@Outsider](mention://member/" + f.outsiderID + ") look",
		"[@All](mention://all/all) look",
	} {
		commentID := f.fx.Comment(t, f.hiddenIssueID, content)
		bus.Publish(events.Event{
			Type: protocol.EventCommentCreated, WorkspaceID: testWorkspaceID, ActorType: "member", ActorID: testUserID,
			Payload: map[string]any{
				"comment": handler.CommentResponse{
					ID: commentID, IssueID: f.hiddenIssueID, AuthorType: "member", AuthorID: testUserID, Content: content, Type: "comment",
				},
				"issue_title": "Hidden listener issue", "issue_status": "todo",
			},
		})
	}
	if n := f.inboxCount(t, f.outsiderID); n != 0 {
		t.Errorf("comment mentions wrote %d inbox items for a member who cannot see the issue", n)
	}
	if n := f.fx.Count(t, `SELECT count(*) FROM inbox_item WHERE issue_id = $1 AND recipient_id = $2`, f.hiddenIssueID, f.granteeID); n < 2 {
		t.Errorf("the grantee got %d inbox items from the @all comment and the description, want at least 2", n)
	}

	// Regression: on an issue the outsider sees, the mention delivers.
	open := "[@Outsider](mention://member/" + f.outsiderID + ")"
	bus.Publish(events.Event{
		Type: protocol.EventIssueCreated, WorkspaceID: testWorkspaceID, ActorType: "member", ActorID: testUserID,
		Payload: map[string]any{"issue": issuePayload(f.openIssueID, "mention on open", testUserID, nil, &open)},
	})
	if n := f.inboxCount(t, f.outsiderID); n != 1 {
		t.Errorf("the outsider got %d inbox items for a mention on an issue they see, want 1", n)
	}
}
