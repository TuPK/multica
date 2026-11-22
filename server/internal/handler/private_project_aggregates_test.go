package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Aggregates over many issues (MUL-8472) must count a private project only for
// viewers who can see it, and must answer a hidden project_id exactly like a
// project that does not exist. Dashboard totals are the exception: they are
// the workspace's spend and failure volume, so they stay complete for every
// member, as they do for restricted agents (foldRestrictedAgents), and only
// the project_id filter refuses a hidden project.

func TestPrivateProjectDashboardsKeepWorkspaceTotals(t *testing.T) {
	f := newPrivateProjectFixture(t)
	agentID := createHandlerTestAgent(t, f.marker+"-dash", []byte("[]"))
	runtimeID := handlerTestRuntimeID(t)
	for _, status := range []string{"completed", "failed"} {
		dbfx.Task(t, agentID, testutil.Cols{
			"runtime_id": runtimeID, "issue_id": f.secretIssue, "status": status,
			"failure_reason": "agent_error.process_failure",
			"started_at":     testutil.Raw("now() - interval '2 minutes'"),
			"completed_at":   testutil.Raw("now() - interval '1 minute'"),
		})
	}
	dbfx.InsertNoID(t, "task_usage_hourly", testutil.Cols{
		"bucket_hour": testutil.Raw("date_trunc('hour', now())"), "workspace_id": testWorkspaceID,
		"runtime_id": runtimeID, "agent_id": agentID, "project_id": f.projectID,
		"provider": "anthropic", "model": f.marker, "input_tokens": 10, "task_count": 1,
	}, "agent_id = $1", agentID)

	get := func(h http.HandlerFunc, path, userID, query string) []map[string]any {
		t.Helper()
		rec := httptest.NewRecorder()
		h(rec, asUser(newRequest(http.MethodGet, path+"?days=7"+query, nil), userID))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s%s as %s = %d: %s", path, query, userID, rec.Code, rec.Body.String())
		}
		var rows []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		return rows
	}
	sum := func(rows []map[string]any, key string) float64 {
		var n float64
		for _, row := range rows {
			v, _ := row[key].(float64)
			n += v
		}
		return n
	}
	hasAgent := func(rows []map[string]any) bool {
		for _, row := range rows {
			if row["agent_id"] == agentID {
				return true
			}
		}
		return false
	}
	missing := "&project_id=00000000-0000-0000-0000-0000000000ff"
	secret := "&project_id=" + f.projectID
	for _, ep := range []struct {
		path    string
		h       http.HandlerFunc
		byAgent bool
		count   string
	}{
		{"/api/dashboard/usage/daily", testHandler.GetDashboardUsageDaily, false, "input_tokens"},
		{"/api/dashboard/usage/by-agent", testHandler.GetDashboardUsageByAgent, true, "input_tokens"},
		{"/api/dashboard/agent-runtime", testHandler.GetDashboardAgentRunTime, true, "task_count"},
		{"/api/dashboard/runtime/daily", testHandler.GetDashboardRunTimeDaily, false, "task_count"},
		{"/api/dashboard/failures/daily", testHandler.GetDashboardFailuresDaily, false, "task_count"},
		{"/api/dashboard/failures/by-agent", testHandler.GetDashboardFailuresByAgent, true, "task_count"},
	} {
		t.Run(ep.path, func(t *testing.T) {
			share := sum(get(ep.h, ep.path, f.insider, secret), ep.count)
			if share == 0 {
				t.Fatal("project member: the project rollup is empty")
			}
			if rows := get(ep.h, ep.path, f.outsider, secret); len(rows) != len(get(ep.h, ep.path, f.outsider, missing)) || len(rows) != 0 {
				t.Fatalf("outsider: hidden project_id returned %d rows, want the missing-project answer (0)", len(rows))
			}
			outside := get(ep.h, ep.path, f.outsider, "")
			if rows := get(ep.h, ep.path, f.outsideAdmin, secret); len(rows) != 0 {
				t.Fatalf("workspace admin outside the project: hidden project_id returned %d rows, want 0", len(rows))
			}
			for name, userID := range map[string]string{"project member": f.insider, "workspace admin outside the project": f.outsideAdmin} {
				if got, want := sum(outside, ep.count), sum(get(ep.h, ep.path, userID, ""), ep.count); got != want || got < share {
					t.Fatalf("workspace %s: outsider = %v, %s = %v, want equal totals that include the private project's %v", ep.count, got, name, want, share)
				}
			}
			if ep.byAgent && !hasAgent(outside) {
				t.Fatal("outsider: the agent's row is missing from the workspace rollup")
			}
		})
	}
}
func TestPrivateProjectSearchIndexManifestLeavesOutHiddenRows(t *testing.T) {
	f := newPrivateProjectFixture(t)
	manifest := func(userID string) SearchIndexManifestResponse {
		return testutil.Decode[SearchIndexManifestResponse](t, testHandler.GetSearchIndexManifest,
			asUser(searchIndexRequest(http.MethodGet, "/api/search-index/manifest", testWorkspaceID, nil), userID), http.StatusOK)
	}
	inside, outside, owner := manifest(f.insider), manifest(f.outsider), manifest(testUserID)
	if d := inside.IssueCount - outside.IssueCount; d != 1 {
		t.Errorf("issue_count member - outsider = %d, want 1", d)
	}
	if d := inside.CommentCount - outside.CommentCount; d != 1 {
		t.Errorf("comment_count member - outsider = %d, want 1", d)
	}
	if d := inside.ProjectCount - outside.ProjectCount; d != 1 {
		t.Errorf("project_count member - outsider = %d, want 1", d)
	}
	if inside.TextBytes <= outside.TextBytes {
		t.Errorf("text_bytes member %d <= outsider %d", inside.TextBytes, outside.TextBytes)
	}
	if owner.IssueCount != inside.IssueCount || owner.ProjectCount != inside.ProjectCount {
		t.Errorf("owner counts %d/%d, want the member's %d/%d", owner.IssueCount, owner.ProjectCount, inside.IssueCount, inside.ProjectCount)
	}
}
