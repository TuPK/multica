package handler

// The request usage sum and the squad handoff response leave out issues of
// restricted projects for a member who has no project_member row (spec 7.1
// rules 4 and 5).

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestRequestUsageVisibility_HiddenChildNotSummed(t *testing.T) {
	e := newInquiryEnv(t)
	memberID := dbfx.User(t, "Usage member", testutil.UniqueEmail("usage-member"))
	dbfx.Member(t, e.WorkspaceID, memberID, "member")
	hidden := dbfx.Project(t, "Hidden usage project", testutil.Cols{"workspace_id": e.WorkspaceID, "visibility": "private"})
	ownerJoinsProject(t, e.WorkspaceID, hidden)

	parent := e.issue(t, e.SquadID, "Usage request", "todo")
	open := e.issue(t, e.SquadID, "Open child", "in_progress")
	secret := e.issue(t, e.SquadID, "Hidden child", "in_progress", testutil.Cols{"project_id": hidden})
	dbfx.Exec(t, `UPDATE issue SET parent_issue_id = $1 WHERE id IN ($2, $3)`, parent, open, secret)

	addRun := func(issueID string, input, cost int64) {
		taskID := dbfx.Task(t, e.Secretary, testutil.Cols{"status": "completed", "runtime_id": e.RuntimeID, "issue_id": issueID})
		dbfx.Insert(t, "task_usage", testutil.Cols{
			"task_id": taskID, "provider": "claude", "model": "m",
			"input_tokens": input, "output_tokens": 0, "cache_read_tokens": 0, "cache_write_tokens": 0,
			"cost_usd_ticks": cost,
		})
	}
	addRun(parent, 10, 100)
	addRun(open, 20, 300)
	addRun(secret, 1000, 9000)

	type usage struct {
		RunCount     int64 `json:"run_count"`
		InputTokens  int64 `json:"input_tokens"`
		CostUsdTicks int64 `json:"cost_usd_ticks"`
	}
	read := func(userID string) usage {
		req := requestAs(userID, "GET", "/api/company/requests/"+parent+"/usage", nil)
		req.Header.Set("X-Workspace-ID", e.WorkspaceID)
		req = withURLParam(req, "id", parent)
		var out usage
		testutil.Call(t, testHandler.GetCompanyRequestUsage, req).Want(http.StatusOK).JSON(&out)
		return out
	}
	if got := read(memberID); got != (usage{RunCount: 2, InputTokens: 30, CostUsdTicks: 400}) {
		t.Errorf("member usage = %+v, want 2 runs, 30 input, 400 cost (hidden child left out)", got)
	}
	if got := read(testUserID); got != (usage{RunCount: 3, InputTokens: 1030, CostUsdTicks: 9400}) {
		t.Errorf("owner usage = %+v, want all three runs", got)
	}
}

// A handoff whose target request sits in a restricted project does not give the
// target id to a member who cannot see it; the owner-side reader still gets it.
func TestHandoffVisibility_HiddenTargetIDOmitted(t *testing.T) {
	fx := newHandoffFx(t)
	n := fx.handOff(t, "restricted")

	read := func(userID string) handoffOut {
		req := fx.as(newRequest("GET", "/api/issues/"+n.source+"/handoff", nil), userID)
		var out handoffOut
		testutil.Call(t, testHandler.GetIssueHandoff, withURLParams(req, "id", n.source)).Want(http.StatusOK).JSON(&out)
		return out
	}
	if out := read(fx.MemberID); out.Handoff == nil || out.Handoff.TargetIssueID != nil {
		t.Errorf("member handoff = %+v, want a handoff without target_issue_id", out.Handoff)
	}
	if out := read(fx.AdminID); out.Handoff == nil || out.Handoff.TargetIssueID == nil || *out.Handoff.TargetIssueID != n.target {
		t.Errorf("admin handoff = %+v, want target_issue_id %s", out.Handoff, n.target)
	}
}
