package handler

import (
	"net/http"
	"testing"
	"time"

	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// open_requests of the squad directory and the insights of a flow count only
// requests of projects the viewer sees (spec 7.1 rule 5).

func TestOpenRequestsOmitHiddenIssues(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	rig := newDraftRig(t, 1)
	for _, project := range []string{f.restrictedProjectID, f.workspaceProjectID, ""} {
		cols := testutil.Cols{
			"origin_type": "squad_request", "assignee_type": "squad", "assignee_id": rig.squadID, "status": "todo",
		}
		if project != "" {
			cols["project_id"] = project
		}
		dbfx.Issue(t, "directory request "+uuid.NewString()[:8], cols)
	}
	open := func(userID string) int64 {
		var out struct {
			Squads []SquadDirectoryEntryResponse `json:"squads"`
		}
		testutil.Call(t, testHandler.ListSquadDirectory, requestAs(userID, "GET", "/api/squads/directory", nil)).
			Want(http.StatusOK).JSON(&out)
		for _, s := range out.Squads {
			if s.ID == rig.squadID {
				return s.OpenRequests
			}
		}
		t.Fatalf("squad %s missing from the directory", rig.squadID)
		return -1
	}
	if got := open(f.outsiderID); got != 2 {
		t.Fatalf("outsider open_requests = %d, want 2 (restricted project hidden)", got)
	}
	if got := open(f.granteeID); got != 3 {
		t.Fatalf("grantee open_requests = %d, want 3", got)
	}
	if got := open(testUserID); got != 3 {
		t.Fatalf("owner open_requests = %d, want 3", got)
	}
}

func TestFlowInsightsOmitHiddenRequests(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	rig := newDraftRig(t, 1)
	v1 := rig.publishOK(rig.draftWith("Insight visibility", rig.publishableDoc(rig.agents[0])), nil)
	// A member who created the squad manages it, so the outsider may ask for the insights.
	dbfx.Exec(t, `UPDATE squad SET creator_id = $2 WHERE id = $1`, rig.squadID, f.outsiderID)

	state := closedState(leftAt("write", "w_ok", 0))
	state.WorkflowID = v1.ID
	for _, project := range []string{f.restrictedProjectID, f.workspaceProjectID} {
		rig.requestInProject(v1.ID, state, project, time.Hour)
	}

	insightsAs := func(userID string) int {
		var report issueworkflow.Report
		rig.insights(func(r *http.Request) *http.Request { r.Header.Set("X-User-ID", userID); return r }, v1.ID, "").
			Want(http.StatusOK).JSON(&report)
		return report.Requests.Total
	}
	if got := insightsAs(f.outsiderID); got != 1 {
		t.Fatalf("outsider requests total = %d, want 1 (restricted project hidden)", got)
	}
	dbfx.Exec(t, `UPDATE squad SET creator_id = $2 WHERE id = $1`, rig.squadID, f.granteeID)
	if got := insightsAs(f.granteeID); got != 2 {
		t.Fatalf("grantee requests total = %d, want 2", got)
	}
	if got := insightsAs(testUserID); got != 2 {
		t.Fatalf("owner requests total = %d, want 2", got)
	}
}

// requestInProject makes a flow request like request, inside a project.
func (rig *draftRig) requestInProject(workflowID string, state issueworkflow.State, projectID string, age time.Duration) string {
	rig.t.Helper()
	state.WorkflowID = workflowID
	state.Version = issueworkflow.FlowDocVersion
	raw, err := json.Marshal(state)
	if err != nil {
		rig.t.Fatal(err)
	}
	return dbfx.Issue(rig.t, "insight request "+workflowTestSuffix(), testutil.Cols{
		"status": "todo", "project_id": projectID, "flow": testutil.Raw(fmt.Sprintf(`'%s'::jsonb`, string(raw))), "created_at": time.Now().Add(-age),
	})
}
