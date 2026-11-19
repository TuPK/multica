package handler

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// The org map and the issue-limit usage omit issues of restricted projects for a
// member who has no project_member row (spec 7.1 rule 5). Quota enforcement
// still counts every issue.

type orgMapVisibilityFixture struct {
	digestEnv
	squadID  string
	memberID string
}

func newOrgMapVisibilityFixture(t *testing.T) orgMapVisibilityFixture {
	t.Helper()
	e := newDigestEnv(t)
	f := orgMapVisibilityFixture{digestEnv: e, squadID: e.squad(t, "Org map visibility squad")}
	f.memberID = dbfx.User(t, "Org map member", testutil.UniqueEmail("org-map-member"))
	dbfx.Member(t, e.WorkspaceID, f.memberID, "member")
	return f
}

// projectOfSquad inserts a project the squad holds.
func (f orgMapVisibilityFixture) projectOfSquad(t *testing.T, title, access string) string {
	t.Helper()
	id := dbfx.Project(t, title, testutil.Cols{"workspace_id": f.WorkspaceID, "access": access})
	dbfx.Exec(t, `UPDATE project SET squad_ids = ARRAY[$2::uuid] WHERE id = $1`, id, f.squadID)
	return id
}

func (f orgMapVisibilityFixture) squadAs(t *testing.T, userID string) service.OrgMapSquad {
	t.Helper()
	req := requestAs(userID, "GET", "/api/company/org-map", nil)
	req.Header.Set("X-Workspace-ID", f.WorkspaceID)
	var out service.OrgMap
	testutil.Call(t, testHandler.GetCompanyOrgMap, req).Want(http.StatusOK).JSON(&out)
	for _, s := range out.Squads {
		if s.ID == f.squadID {
			return s
		}
	}
	t.Fatalf("squad %s missing from the org map of user %s", f.squadID, userID)
	return service.OrgMapSquad{}
}

func TestOrgMapVisibility_RestrictedProjectIsHidden(t *testing.T) {
	f := newOrgMapVisibilityFixture(t)
	hidden := f.projectOfSquad(t, "HIDDEN-ORGMAP-PROJECT", "restricted")
	// One issue held by the squad outside any project, one held by the squad in
	// the restricted project, and one in the restricted project with no assignee.
	f.issue(t, f.squadID, "open squad issue", "todo")
	f.issue(t, f.squadID, "hidden squad issue", "todo", testutil.Cols{"project_id": hidden})
	dbfx.Issue(t, "hidden project issue", testutil.Cols{"workspace_id": f.WorkspaceID, "project_id": hidden})

	m := f.squadAs(t, f.memberID)
	if m.Project != nil {
		t.Errorf("member sees the restricted project %+v", m.Project)
	}
	if m.OpenIssues != 1 {
		t.Errorf("member open_issues = %d, want 1", m.OpenIssues)
	}

	o := f.squadAs(t, testUserID)
	if o.Project == nil || o.Project.Title != "HIDDEN-ORGMAP-PROJECT" {
		t.Errorf("owner project = %+v, want the restricted project", o.Project)
	}
	if o.OpenIssues != 3 {
		t.Errorf("owner open_issues = %d, want 3", o.OpenIssues)
	}
}

func TestOrgMapVisibility_WorkspaceProjectStaysVisible(t *testing.T) {
	f := newOrgMapVisibilityFixture(t)
	open := f.projectOfSquad(t, "OPEN-ORGMAP-PROJECT", "workspace")
	f.issue(t, f.squadID, "open project issue", "todo", testutil.Cols{"project_id": open})

	m := f.squadAs(t, f.memberID)
	if m.Project == nil || m.Project.Title != "OPEN-ORGMAP-PROJECT" || m.OpenIssues != 1 {
		t.Errorf("member sees project %+v open_issues %d, want the workspace project and 1", m.Project, m.OpenIssues)
	}
}

// The GET shows a member only the issues the member sees; the enforcement count
// that gates a create keeps counting every issue.
func TestIssueLimitUsageVisibility_DisplayHidesEnforcementCountsAll(t *testing.T) {
	f := newOrgMapVisibilityFixture(t)
	hidden := dbfx.Project(t, "Hidden usage "+uuid.NewString()[:8], testutil.Cols{"workspace_id": f.WorkspaceID, "access": "restricted"})
	dbfx.Issue(t, "hidden usage issue", testutil.Cols{"workspace_id": f.WorkspaceID, "project_id": hidden})
	dbfx.Issue(t, "open usage issue", testutil.Cols{"workspace_id": f.WorkspaceID})
	setPlanLimitGates(t, f.WorkspaceID, map[entitlement.GateName]int{entitlement.GateIssueCount: 100})

	usedBy := func(userID string) int64 {
		req := requestAs(userID, "GET", "/api/issue-limit-usage", nil)
		req.Header.Set("X-Workspace-ID", f.WorkspaceID)
		var out IssueLimitUsageResponse
		testutil.Call(t, testHandler.GetIssueLimitUsage, req).Want(http.StatusOK).JSON(&out)
		return out.Used
	}
	if got := usedBy(f.memberID); got != 1 {
		t.Errorf("member used = %d, want 1", got)
	}
	if got := usedBy(testUserID); got != 2 {
		t.Errorf("owner used = %d, want 2", got)
	}

	policy := service.ResolveIssueCountPolicy(t.Context(), testHandler.Entitlements, parseUUID(f.WorkspaceID))
	got, err := service.CountIssueUsage(t.Context(), testHandler.Queries, parseUUID(f.WorkspaceID), policy)
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Errorf("enforcement count = %d, want 2 (hidden issues still count toward the quota)", got)
	}
}
