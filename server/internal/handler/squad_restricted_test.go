package handler

// A squad's active-workflow switch and an edit of that workflow census and
// move the squad's routed issues (spec 7.1 rules 2, 3). A member who manages
// the squad but cannot see a restricted project sees no count from it, and
// cannot move the issues in it.

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// squadHiddenFixture is a squad managed by a plain member (who also owns the
// squad's active workflow), with one routed issue in a workspace project and
// one in a restricted project the member is not granted. Both issues sit on
// in_review.
type squadHiddenFixture struct {
	fx            iniWorkspace
	squadID       string
	activeID      string // the squad's active workflow; lists todo and in_review
	visibleIssue  string
	hiddenIssue   string
	hiddenProject string
}

func newSquadHiddenFixture(t *testing.T) squadHiddenFixture {
	t.Helper()
	fx := newIniWorkspace(t)
	f := squadHiddenFixture{fx: fx, squadID: sdlcSquad(t, fx, "Hidden mix")}
	dbfx.Exec(t, `UPDATE squad SET creator_id = $2 WHERE id = $1`, f.squadID, fx.MemberID)

	f.activeID = insertSquadWorkflow(t, fx.ID, f.squadID, "Hidden mix active "+fx.ID[:8], "todo", "in_review")
	dbfx.Exec(t, `UPDATE issue_workflow SET owner_id = $2 WHERE id = $1`, f.activeID, fx.MemberID)
	dbfx.Exec(t, `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, f.squadID, f.activeID)

	openProject := dbfxProject(t, fx, "Open")
	f.hiddenProject = dbfxProject(t, fx, "Hidden")
	dbfx.Exec(t, `UPDATE project SET access = 'restricted' WHERE id = $1`, f.hiddenProject)
	routed := func(title, projectID string) string {
		return dbfx.Issue(t, title, testutil.Cols{
			"workspace_id": fx.ID, "project_id": projectID, "status": "in_review",
			"assignee_type": "squad", "assignee_id": f.squadID,
		})
	}
	f.visibleIssue = routed("visible routed", openProject)
	f.hiddenIssue = routed("hidden routed", f.hiddenProject)
	return f
}

// switchTo posts the squad's active-workflow switch to a new workflow that
// lists todo and in_progress, as userID.
func (f squadHiddenFixture) switchTo(t *testing.T, userID, workflowID string, mapping map[string]string, dryRun bool) *testutil.Response {
	t.Helper()
	body := map[string]any{"workflow_id": workflowID, "dry_run": dryRun}
	if mapping != nil {
		body["status_mapping"] = mapping
	}
	req := newRequestAs(userID, "POST", "/api/squads/"+f.squadID+"/workflow", body)
	req = withURLParams(req, "workspaceId", f.fx.ID, "id", f.squadID)
	return testutil.Call(t, testHandler.SetSquadActiveWorkflow, req)
}

// editActive patches the squad's active workflow as userID.
func (f squadHiddenFixture) editActive(t *testing.T, userID string, body map[string]any) *testutil.Response {
	t.Helper()
	req := workspaceRequest(userID, f.fx.ID, "PATCH", "/api/issue-workflows/"+f.activeID, body)
	return testutil.Call(t, testHandler.UpdateIssueWorkflow, withURLParam(req, "id", f.activeID))
}

type squadPlanResponse struct {
	Plan struct {
		TotalIssues int64 `json:"total_issues"`
	} `json:"plan"`
}

// TestRestrictedSquadWorkflowCountsOmitHidden: the census a dry run reports
// (CountSquadIssuesByStatus) leaves out the issues of a project the viewer
// cannot see. The owner and admin count every routed issue.
func TestRestrictedSquadWorkflowCountsOmitHidden(t *testing.T) {
	f := newSquadHiddenFixture(t)
	// The target keeps in_review, so no issue needs a move and the member's
	// dry run is not refused; only the count differs.
	target := insertSquadWorkflow(t, f.fx.ID, f.squadID, "Hidden mix target "+f.fx.ID[:8], "todo", "in_review", "in_progress")

	t.Run("SetSquadActiveWorkflow", func(t *testing.T) {
		var member, admin squadPlanResponse
		f.switchTo(t, f.fx.MemberID, target, nil, true).Want(http.StatusOK).JSON(&member)
		f.switchTo(t, f.fx.AdminID, target, nil, true).Want(http.StatusOK).JSON(&admin)
		if member.Plan.TotalIssues != 1 {
			t.Errorf("member's census counts %d issues, want 1: the restricted project's issue is hidden", member.Plan.TotalIssues)
		}
		if admin.Plan.TotalIssues != 2 {
			t.Errorf("admin's census counts %d issues, want 2", admin.Plan.TotalIssues)
		}
	})

	t.Run("UpdateIssueWorkflow", func(t *testing.T) {
		dryRun := map[string]any{"dry_run": true}
		var member, admin squadPlanResponse
		f.editActive(t, f.fx.MemberID, dryRun).Want(http.StatusOK).JSON(&member)
		f.editActive(t, f.fx.AdminID, dryRun).Want(http.StatusOK).JSON(&admin)
		if member.Plan.TotalIssues != 1 {
			t.Errorf("member's census counts %d issues, want 1: the restricted project's issue is hidden", member.Plan.TotalIssues)
		}
		if admin.Plan.TotalIssues != 2 {
			t.Errorf("admin's census counts %d issues, want 2", admin.Plan.TotalIssues)
		}
	})
}

// TestRestrictedSquadRemapNeedsManage: a squad manager or workflow owner who
// is not an admin cannot remap issues of a project they cannot see, so a
// switch or an edit that would touch one is refused and changes nothing. An
// admin's remap moves every routed issue.
func TestRestrictedSquadRemapNeedsManage(t *testing.T) {
	statusOf := func(t *testing.T, f squadHiddenFixture) (visible, hidden string) {
		t.Helper()
		return issueStatusOf(t, f.visibleIssue), issueStatusOf(t, f.hiddenIssue)
	}
	mapping := map[string]string{"in_review": "in_progress"}

	t.Run("SetSquadActiveWorkflow", func(t *testing.T) {
		f := newSquadHiddenFixture(t)
		target := insertSquadWorkflow(t, f.fx.ID, f.squadID, "Hidden mix target "+f.fx.ID[:8], "todo", "in_progress")

		f.switchTo(t, f.fx.MemberID, target, mapping, false).Want(http.StatusForbidden)
		if visible, hidden := statusOf(t, f); visible != "in_review" || hidden != "in_review" {
			t.Errorf("refused switch moved issues: visible=%q hidden=%q, want both in_review", visible, hidden)
		}
		if got := squadActiveWorkflowID(t, f.squadID); got != f.activeID {
			t.Errorf("refused switch changed the active workflow to %s", got)
		}

		f.switchTo(t, f.fx.AdminID, target, mapping, false).Want(http.StatusOK)
		if visible, hidden := statusOf(t, f); visible != "in_progress" || hidden != "in_progress" {
			t.Errorf("admin switch: visible=%q hidden=%q, want both in_progress", visible, hidden)
		}
		if got := squadActiveWorkflowID(t, f.squadID); got != target {
			t.Errorf("admin switch: active workflow = %s, want %s", got, target)
		}
	})

	t.Run("UpdateIssueWorkflow", func(t *testing.T) {
		f := newSquadHiddenFixture(t)
		drop := map[string]any{"steps": statusSteps("backlog", "in_progress", "blocked"), "status_mapping": mapping}

		f.editActive(t, f.fx.MemberID, drop).Want(http.StatusNotFound)
		if visible, hidden := statusOf(t, f); visible != "in_review" || hidden != "in_review" {
			t.Errorf("refused edit moved issues: visible=%q hidden=%q, want both in_review", visible, hidden)
		}

		f.editActive(t, f.fx.AdminID, drop).Want(http.StatusOK)
		if visible, hidden := statusOf(t, f); visible != "in_progress" || hidden != "in_progress" {
			t.Errorf("admin edit: visible=%q hidden=%q, want both in_progress", visible, hidden)
		}
	})
}

// TestRestrictedAssignSquadProjectRemapNeedsManage: the first assignment gives
// a squad its workflow and moves its issues onto it. A squad manager who
// cannot see a restricted project holding some of those issues is refused, and
// the assignment changes nothing. An admin's assignment moves them all.
func TestRestrictedAssignSquadProjectRemapNeedsManage(t *testing.T) {
	fx := newIniWorkspace(t)
	// The squad's new workflow, SDLC Auto, has no legacy_stage step, so those issues move.
	squadID := sdlcSquad(t, fx, "Assign mix")
	dbfx.Exec(t, `UPDATE squad SET creator_id = $2 WHERE id = $1`, squadID, fx.MemberID)
	target := dbfxProject(t, fx, "Target")
	openProject := dbfxProject(t, fx, "Open")
	hiddenProject := dbfxProject(t, fx, "Hidden")
	dbfx.Exec(t, `UPDATE project SET access = 'restricted' WHERE id = $1`, hiddenProject)
	routed := func(title, projectID string) string {
		return dbfx.Issue(t, title, testutil.Cols{
			"workspace_id": fx.ID, "project_id": projectID, "status": "legacy_stage",
			"assignee_type": "squad", "assignee_id": squadID,
		})
	}
	visibleIssue, hiddenIssue := routed("visible routed", openProject), routed("hidden routed", hiddenProject)
	assign := func(userID string) *testutil.Response {
		req := newRequestAs(userID, "POST", "/api/squads/"+squadID+"/project", map[string]any{"project_id": target})
		return testutil.Call(t, testHandler.AssignSquadProject, withURLParams(req, "workspaceId", fx.ID, "id", squadID))
	}

	assign(fx.MemberID).Want(http.StatusForbidden)
	if got := projectSquadIDs(t, target); len(got) != 0 {
		t.Errorf("refused assignment added the squad to the project: %v", got)
	}
	if got := squadActiveWorkflowID(t, squadID); got != "" {
		t.Errorf("refused assignment gave the squad workflow %s", got)
	}
	for name, id := range map[string]string{"visible": visibleIssue, "hidden": hiddenIssue} {
		if got := issueStatusOf(t, id); got != "legacy_stage" {
			t.Errorf("refused assignment moved the %s issue to %q", name, got)
		}
	}

	var out struct {
		UpdatedIssues int `json:"updated_issues"`
	}
	assign(fx.AdminID).Want(http.StatusOK).JSON(&out)
	if out.UpdatedIssues != 2 {
		t.Errorf("admin assignment moved %d issues, want 2", out.UpdatedIssues)
	}
}
