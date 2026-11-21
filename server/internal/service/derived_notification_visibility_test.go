package service

// Notifications written by the services follow issue visibility (spec 7.1 rule
// 4): a recipient who cannot see the issue gets no inbox row, and the caller's
// operation still succeeds. The fixture is one workspace per test with a
// restricted project, an outsider (plain member, not granted) and a grantee.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type hiddenIssueFixture struct {
	principalFixture
	outsider, grantee string
	restricted        string // the restricted project
	hiddenIssue       string // in the restricted project
	openIssue         string // in no project
}

func newHiddenIssueFixture(t *testing.T) hiddenIssueFixture {
	t.Helper()
	f, _ := newPrincipalFixture(t)
	h := hiddenIssueFixture{principalFixture: f, outsider: f.member(t, "outsider"), grantee: f.member(t, "grantee")}
	restricted := f.Project(t, "Restricted notifications", testutil.Cols{"visibility": "private"})
	h.restricted = restricted
	f.Exec(t, `INSERT INTO project_member (project_id, workspace_id, user_id) VALUES ($1, $2, $3)`, restricted, f.WorkspaceID, h.grantee)
	f.Cleanup(t, `DELETE FROM project_member WHERE project_id = $1`, restricted)
	h.hiddenIssue = f.Issue(t, "Hidden", testutil.Cols{"project_id": restricted})
	h.openIssue = f.Issue(t, "Open")
	for _, id := range []string{h.hiddenIssue, h.openIssue} {
		f.Cleanup(t, `DELETE FROM inbox_item WHERE issue_id = $1`, id)
	}
	return h
}

func projectOrNil(id string) any {
	if id == "" {
		return nil
	}
	return id
}

func (h hiddenIssueFixture) inboxRows(t *testing.T, userID, itemType string) int {
	t.Helper()
	return h.Count(t, `SELECT count(*) FROM inbox_item WHERE workspace_id = $1 AND recipient_id = $2 AND type = $3`, h.WorkspaceID, userID, itemType)
}

func (h hiddenIssueFixture) issue(t *testing.T, id string) db.Issue {
	t.Helper()
	issue, err := h.q.GetIssue(context.Background(), parseTestUUID(t, id))
	if err != nil {
		t.Fatal(err)
	}
	return issue
}

// TestAutopilotNotificationSkipsHiddenRecipient: the subscriber notice of an
// autopilot-created issue skips a subscriber who cannot see the issue.
func TestAutopilotNotificationSkipsHiddenRecipient(t *testing.T) {
	h := newHiddenIssueFixture(t)
	ap := db.Autopilot{ID: parseTestUUID(t, uuid.NewString()), WorkspaceID: parseTestUUID(t, h.WorkspaceID), Title: "Nightly"}
	leader := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	subs := []db.AutopilotSubscriber{
		{UserType: "member", UserID: parseTestUUID(t, h.outsider)},
		{UserType: "member", UserID: parseTestUUID(t, h.grantee)},
	}

	h.svc.notifyAutopilotSubscribersOnCreate(context.Background(), ap, h.issue(t, h.hiddenIssue), leader, subs)
	if n := h.inboxRows(t, h.outsider, "issue_subscribed"); n != 0 {
		t.Errorf("a subscriber who cannot see the issue got %d inbox rows", n)
	}
	if n := h.inboxRows(t, h.grantee, "issue_subscribed"); n != 1 {
		t.Errorf("a subscriber who sees the issue got %d inbox rows, want 1", n)
	}

	h.svc.notifyAutopilotSubscribersOnCreate(context.Background(), ap, h.issue(t, h.openIssue), leader, subs)
	if n := h.inboxRows(t, h.outsider, "issue_subscribed"); n != 1 {
		t.Errorf("the same subscriber got %d inbox rows for an issue they see, want 1", n)
	}
}

// TestTaskNotificationSkipsHiddenRecipient: the "issue created" notice of a
// finished quick-create task is not written for a requester who cannot see the
// issue the agent filed.
func TestTaskNotificationSkipsHiddenRecipient(t *testing.T) {
	h := newHiddenIssueFixture(t)
	restricted := h.restricted
	runQuickCreate := func(requester, projectID string) int {
		taskID := uuid.NewString()
		issue := h.Issue(t, "Quick created "+taskID[:8], testutil.Cols{
			"project_id": projectOrNil(projectID), "origin_type": "quick_create", "origin_id": taskID,
		})
		h.Cleanup(t, `DELETE FROM inbox_item WHERE issue_id = $1`, issue)
		qc := QuickCreateContext{Type: QuickCreateContextType, Prompt: "p", RequesterID: requester, WorkspaceID: h.WorkspaceID}
		task := db.AgentTaskQueue{ID: parseTestUUID(t, taskID), AgentID: parseTestUUID(t, uuid.NewString())}
		h.svc.TaskSvc.notifyQuickCreateCompleted(context.Background(), task, qc, nil)
		return h.Count(t, `SELECT count(*) FROM inbox_item WHERE issue_id = $1 AND recipient_id = $2`, issue, requester)
	}
	if n := runQuickCreate(h.outsider, restricted); n != 0 {
		t.Errorf("a requester who cannot see the issue got %d inbox rows", n)
	}
	if n := runQuickCreate(h.grantee, restricted); n != 1 {
		t.Errorf("a requester who sees the issue got %d inbox rows, want 1", n)
	}
	if n := runQuickCreate(h.outsider, ""); n != 1 {
		t.Errorf("the outsider got %d inbox rows for an issue outside any project, want 1", n)
	}
}

// TestWakeupNotificationSkipsHiddenRecipient: the "children done" notice goes to
// the member assignee only when that member sees the parent issue; the rule's
// receipts are consumed either way.
func TestWakeupNotificationSkipsHiddenRecipient(t *testing.T) {
	ctx := context.Background()
	run := func(t *testing.T, restricted bool) (string, int, int) {
		f, s, issue, _ := wakeFixture(t)
		assignee := f.member(t, "assignee")
		project := f.Project(t, "Wakeup project", testutil.Cols{"visibility": map[bool]string{true: "private", false: "workspace"}[restricted]})
		f.Cleanup(t, `DELETE FROM project_member WHERE project_id = $1`, project)
		f.Exec(t, `UPDATE issue SET project_id = $1, assignee_type = 'member', assignee_id = $2 WHERE id = $3`, project, assignee, issue)
		f.Cleanup(t, `DELETE FROM inbox_item WHERE issue_id = $1`, issue)
		parent, err := f.q.GetIssue(ctx, issue)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.ensureRule(ctx, parent); err != nil {
			t.Fatal(err)
		}
		rule, err := f.q.GetSystemWakeup(ctx, db.GetSystemWakeupParams{IssueID: issue, SystemRule: systemRuleText(SystemRuleChildDone)})
		if err != nil {
			t.Fatal(err)
		}
		// A sub-issue that closes after the rule was made meets its condition.
		child := f.Issue(t, "Closed child", testutil.Cols{"parent_issue_id": issue, "status": "done"})
		f.Cleanup(t, `DELETE FROM inbox_item WHERE issue_id = $1`, child)
		if err = s.dispatchSystem(ctx, rule); err != nil {
			t.Fatalf("dispatchSystem must not fail because a notice was skipped: %v", err)
		}
		return assignee, f.Count(t, `SELECT count(*) FROM inbox_item WHERE issue_id = $1 AND recipient_id = $2 AND type = 'children_done'`, issue, assignee),
			f.Count(t, `SELECT count(*) FROM issue_wakeup_receipt WHERE wakeup_id = $1 AND processed_at IS NULL`, rule.ID)
	}
	t.Run("assignee sees the issue", func(t *testing.T) {
		if _, notices, _ := run(t, false); notices != 1 {
			t.Fatalf("a visible assignee got %d notices, want 1 (the control case must reach the notice)", notices)
		}
	})
	t.Run("assignee cannot see the issue", func(t *testing.T) {
		if _, notices, _ := run(t, true); notices != 0 {
			t.Errorf("an assignee who cannot see the issue got %d notices", notices)
		}
	})
}
