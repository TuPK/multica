package handler

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// GetAssigneeFrequency counts only issues of projects the caller sees
// (spec 7.1 rule 5), for both the activity and the created-issue source.

func TestAssigneeFrequencyOmitsHiddenIssues(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	// A target no other test uses, so the assertions read only this fixture.
	target := uuid.NewString()

	for _, actor := range []string{f.outsiderID, f.granteeID} {
		for _, project := range []string{f.restrictedProjectID, f.workspaceProjectID} {
			dbfx.Issue(t, "freq created "+uuid.NewString()[:8], testutil.Cols{
				"project_id": project, "creator_id": actor,
				"assignee_type": "member", "assignee_id": target,
			})
			assigned := dbfx.Issue(t, "freq assigned "+uuid.NewString()[:8], testutil.Cols{"project_id": project})
			dbfx.Insert(t, "activity_log", testutil.Cols{
				"workspace_id": testWorkspaceID, "issue_id": assigned,
				"actor_type": "member", "actor_id": actor, "action": "assignee_changed",
				"details": testutil.Raw(`'{"to_type":"member","to_id":"` + target + `"}'::jsonb`),
			})
		}
	}

	frequency := func(userID string) int64 {
		var out []AssigneeFrequencyEntry
		testutil.Call(t, testHandler.GetAssigneeFrequency, requestAs(userID, "GET", "/api/assignee-frequency", nil)).
			Want(http.StatusOK).JSON(&out)
		for _, e := range out {
			if e.AssigneeType == "member" && e.AssigneeID == target {
				return e.Frequency
			}
		}
		return 0
	}
	// Each actor made two created issues and two assignments, one pair per project.
	if got := frequency(f.outsiderID); got != 2 {
		t.Fatalf("outsider frequency = %d, want 2 (restricted project hidden)", got)
	}
	if got := frequency(f.granteeID); got != 4 {
		t.Fatalf("grantee frequency = %d, want 4", got)
	}
}
