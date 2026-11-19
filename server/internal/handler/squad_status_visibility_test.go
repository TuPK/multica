package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The squad member status view keeps the member's working status for a viewer
// who cannot see the task's issue, but drops the issue link: no id, number,
// title or issue status (spec 7.1 rule 5, hidden = missing).

type squadStatusVisibilityFixture struct {
	squadID    string
	issueID    string
	title      string
	identifier string
}

// newSquadStatusVisibilityFixture builds a squad whose only agent member has a
// dispatched task on an issue in a project of the given access.
func newSquadStatusVisibilityFixture(t *testing.T, access string) squadStatusVisibilityFixture {
	t.Helper()
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	tag := uuid.NewString()[:8]
	agentID := createHandlerTestAgent(t, "squad-status-vis-"+tag, []byte(`{}`))
	squadID := createCommentTriggerPreviewSquad(t, "Squad Status Visibility "+tag, agentID)
	projectID := createRestrictedFixtureProject(t, "Status project "+tag, access)
	title := "STATUS-LEAK-TITLE-" + tag
	issueID := createIssueInProject(t, title, projectID)

	if _, err := testPool.Exec(ctx,
		`UPDATE issue SET status = 'in_review' WHERE id = $1`, issueID); err != nil {
		t.Fatalf("set issue status: %v", err)
	}
	if _, err := testPool.Exec(ctx,
		`INSERT INTO squad_member (squad_id, member_type, member_id) VALUES ($1, 'agent', $2)`,
		squadID, agentID); err != nil {
		t.Fatalf("insert squad member: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM squad_member WHERE squad_id = $1`, squadID)
	})

	var taskID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, priority, dispatched_at)
		VALUES ($1, $2, $3, 'dispatched', 0, now())
		RETURNING id
	`, agentID, handlerTestRuntimeID(t), issueID).Scan(&taskID); err != nil {
		t.Fatalf("insert dispatched task: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE id = $1`, taskID)
	})

	var number int
	var prefix string
	if err := testPool.QueryRow(ctx, `
		SELECT i.number, w.issue_prefix FROM issue i JOIN workspace w ON w.id = i.workspace_id WHERE i.id = $1
	`, issueID).Scan(&number, &prefix); err != nil {
		t.Fatalf("read issue number: %v", err)
	}
	return squadStatusVisibilityFixture{
		squadID:    squadID,
		issueID:    issueID,
		title:      title,
		identifier: prefix + "-" + strconv.Itoa(number),
	}
}

// statusAs reads the squad member status as the user and returns the raw body
// and the decoded response.
func (f squadStatusVisibilityFixture) statusAs(t *testing.T, userID string) (string, SquadMemberStatusListResponse) {
	t.Helper()
	w := httptest.NewRecorder()
	testHandler.ListSquadMemberStatus(w, squadScopeReq(
		userID, http.MethodGet, "/api/squads/"+f.squadID+"/members/status", nil,
		map[string]string{"id": f.squadID},
	))
	if w.Code != http.StatusOK {
		t.Fatalf("ListSquadMemberStatus as %s: %d %s", userID, w.Code, w.Body.String())
	}
	raw := w.Body.String()
	var resp SquadMemberStatusListResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("decode squad member status: %v", err)
	}
	if len(resp.Members) != 1 || resp.Members[0].Status == nil {
		t.Fatalf("members = %+v, want one agent member with a status", resp.Members)
	}
	return raw, resp
}

func TestSquadStatusVisibility_HiddenIssueOmitted(t *testing.T) {
	f := newSquadStatusVisibilityFixture(t, "restricted")
	outsider := addPlainMember(t, "status-outsider")

	raw, resp := f.statusAs(t, outsider)
	for _, leak := range []string{f.title, f.identifier, f.issueID, "in_review", "STATUS-LEAK"} {
		if strings.Contains(raw, leak) {
			t.Errorf("member status leaks %q: %s", leak, raw)
		}
	}
	member := resp.Members[0]
	if *member.Status != "working" {
		t.Errorf("member status = %q, want working", *member.Status)
	}
	if len(member.ActiveIssues) != 0 {
		t.Errorf("active issues = %+v, want none", member.ActiveIssues)
	}
	if member.LastActiveAt == nil {
		t.Errorf("last_active_at missing, want the task dispatch time")
	}

	// The owner still sees the issue.
	raw, resp = f.statusAs(t, testUserID)
	if len(resp.Members[0].ActiveIssues) != 1 || resp.Members[0].ActiveIssues[0].IssueID != f.issueID {
		t.Errorf("owner active issues = %+v, want %s", resp.Members[0].ActiveIssues, f.issueID)
	}
	if !strings.Contains(raw, f.title) {
		t.Errorf("owner status misses title %q: %s", f.title, raw)
	}
}

func TestSquadStatusVisibility_VisibleIssueShown(t *testing.T) {
	f := newSquadStatusVisibilityFixture(t, "workspace")
	outsider := addPlainMember(t, "status-open-member")

	_, resp := f.statusAs(t, outsider)
	member := resp.Members[0]
	if *member.Status != "working" {
		t.Errorf("member status = %q, want working", *member.Status)
	}
	if len(member.ActiveIssues) != 1 {
		t.Fatalf("active issues = %+v, want one", member.ActiveIssues)
	}
	got := member.ActiveIssues[0]
	if got.IssueID != f.issueID || got.Identifier != f.identifier || got.Title != f.title || got.IssueStatus != "in_review" {
		t.Errorf("active issue = %+v, want id %s, %s, %q, in_review", got, f.issueID, f.identifier, f.title)
	}
}
