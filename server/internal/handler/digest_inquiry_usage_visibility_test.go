package handler

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// The digest's secretary inquiries and token totals come from rows that follow
// an issue or project (brief_inquiry.issue_id, task_usage_hourly.project_id). A
// member who has no project_member row for a restricted project must not read
// the question asked about its issue or count its tokens (spec 7.1 rules 4, 5).

func TestDigestVisibility_InquiryOfHiddenIssueOmitted(t *testing.T) {
	f := newDigestVisibilityFixture(t)
	hiddenIssue := dbfx.Issue(t, "HIDDEN-INQUIRY-ISSUE", testutil.Cols{"workspace_id": f.WorkspaceID, "project_id": f.hiddenProj})
	openIssue := dbfx.Issue(t, "OPEN-INQUIRY-ISSUE", testutil.Cols{"workspace_id": f.WorkspaceID, "project_id": f.openProj})
	for question, issueID := range map[string]any{
		"HIDDEN-QUESTION-TEXT": hiddenIssue, "OPEN-QUESTION-TEXT": openIssue, "LOOSE-QUESTION-TEXT": nil,
	} {
		cols := testutil.Cols{
			"workspace_id": f.WorkspaceID, "asked_by_agent_id": f.AgentID, "squad_id": f.squadID,
			"question": question, "asked_on": testutil.Raw("(now() AT TIME ZONE 'UTC')::date"),
		}
		if issueID != nil {
			cols["issue_id"] = issueID
		}
		dbfx.Insert(t, "brief_inquiry", cols)
	}

	raw, _ := f.digestAs(t, f.memberID)
	requireHidden(t, raw, "HIDDEN-QUESTION-TEXT")
	requireShown(t, "member", raw, "OPEN-QUESTION-TEXT", "LOOSE-QUESTION-TEXT")
	raw, _ = f.digestAs(t, testUserID)
	requireShown(t, "owner", raw, "HIDDEN-QUESTION-TEXT", "OPEN-QUESTION-TEXT", "LOOSE-QUESTION-TEXT")
}

func TestDigestVisibility_TokenTotalsOmitHiddenProject(t *testing.T) {
	f := newDigestVisibilityFixture(t)
	runtimeID := dbfx.Runtime(t, "Digest usage runtime", testutil.Cols{"workspace_id": f.WorkspaceID})
	for projectID, tokens := range map[string]int{f.hiddenProj: 700, f.openProj: 30} {
		dbfx.Exec(t, `
			INSERT INTO task_usage_hourly (bucket_hour, workspace_id, runtime_id, agent_id, project_id, provider, model, input_tokens, task_count)
			VALUES (date_trunc('hour', now()), $1, $2, $3, $4, 'claude', 'digest-usage-model', $5, 1)`,
			f.WorkspaceID, runtimeID, f.AgentID, projectID, tokens)
	}
	t.Cleanup(func() {
		testPool.Exec(t.Context(), `DELETE FROM task_usage_hourly WHERE workspace_id = $1`, f.WorkspaceID)
	})

	_, d := f.digestAs(t, f.memberID)
	if got := d.Workspace.InputTokens; got != 30 {
		t.Errorf("member input_tokens = %d, want 30 (the hidden project's 700 are not counted)", got)
	}
	_, d = f.digestAs(t, testUserID)
	if got := d.Workspace.InputTokens; got != 730 {
		t.Errorf("owner input_tokens = %d, want 730", got)
	}
}
