package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// ListLabels reports usage_count per label. The count covers only the issues
// the viewer sees (spec 7.1 rules 4 and 5): a hidden issue is not counted.
func TestRestrictedLabelUsageCountOmitsHiddenIssues(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	hiddenID := createIssueInProject(t, "label hidden "+uuid.NewString()[:8], f.restrictedProjectID)
	openID := createIssueInProject(t, "label open "+uuid.NewString()[:8], f.workspaceProjectID)
	labelID := dbfx.Insert(t, "issue_label", testutil.Cols{
		"workspace_id": testWorkspaceID, "name": "vis-" + uuid.NewString()[:8], "color": "#112233",
	})
	for _, id := range []string{hiddenID, openID} {
		dbfx.Exec(t, `INSERT INTO issue_to_label (issue_id, label_id) VALUES ($1, $2)`, id, labelID)
	}

	usage := func(userID string) int64 {
		t.Helper()
		var out struct {
			Labels []struct {
				ID         string `json:"id"`
				UsageCount int64  `json:"usage_count"`
			} `json:"labels"`
		}
		testutil.Call(t, testHandler.ListLabels, requestAs(userID, "GET", "/api/labels", nil)).Want(http.StatusOK).JSON(&out)
		for _, l := range out.Labels {
			if l.ID == labelID {
				return l.UsageCount
			}
		}
		b, _ := json.Marshal(out)
		t.Fatalf("label %s missing from %s", labelID, b)
		return 0
	}
	if got := usage(f.outsiderID); got != 1 {
		t.Errorf("outsider usage_count = %d, want 1 (the hidden issue is not counted)", got)
	}
	if got := usage(f.granteeID); got != 2 {
		t.Errorf("grantee usage_count = %d, want 2", got)
	}
	if got := usage(testUserID); got != 2 {
		t.Errorf("owner usage_count = %d, want 2", got)
	}
}
