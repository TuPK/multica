package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// TestClaimCarriesTheIssueRoom: a claimed issue run names the room it belongs to,
// so an in-place daemon can run it beside other rooms. The room is the squad the
// issue is assigned to, or the home squad of an agent assignee; a run with no room
// names none, and the daemon keeps the whole directory for it.
func TestClaimCarriesTheIssueRoom(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var leaderID, runtimeID string
	dbfx.QueryRow(t, `SELECT id, runtime_id FROM agent WHERE workspace_id = $1 LIMIT 1`, testWorkspaceID).Scan(&leaderID, &runtimeID)
	room := dbfx.Squad(t, "claim room "+suffix, leaderID, testutil.Cols{"workspace_id": testWorkspaceID})
	agentInRoom := dbfx.Agent(t, "claim agent "+suffix, runtimeID, testutil.Cols{
		"workspace_id": testWorkspaceID, "home_squad_id": room,
	})
	agentLoose := dbfx.Agent(t, "claim loose agent "+suffix, runtimeID, testutil.Cols{"workspace_id": testWorkspaceID})

	for _, tc := range []struct {
		name         string
		assigneeType string
		assigneeID   string
		agentID      string
		want         string
	}{
		{name: "squad assignee", assigneeType: "squad", assigneeID: room, agentID: leaderID, want: room},
		{name: "agent with a home room", assigneeType: "agent", assigneeID: agentInRoom, agentID: agentInRoom, want: room},
		{name: "agent with no home room", assigneeType: "agent", assigneeID: agentLoose, agentID: agentLoose, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issueID := dbfx.Issue(t, "claim lane "+tc.name+" "+suffix, testutil.Cols{
				"assignee_type": tc.assigneeType, "assignee_id": tc.assigneeID,
			})
			taskID := dbfx.Task(t, tc.agentID, testutil.Cols{
				"issue_id": issueID, "runtime_id": runtimeID, "status": "dispatched", "started_at": time.Now(),
			})
			task := db.AgentTaskQueue{
				ID: parseUUID(taskID), AgentID: parseUUID(tc.agentID), RuntimeID: parseUUID(runtimeID),
				IssueID: parseUUID(issueID), Status: "dispatched",
			}
			runtime := db.AgentRuntime{ID: parseUUID(runtimeID), WorkspaceID: parseUUID(testWorkspaceID)}
			req := httptest.NewRequest(http.MethodPost, "/api/daemon/runtimes/"+runtimeID+"/claim", nil)

			resp, _, _, _, _, failure := testHandler.buildClaimedTaskResponse(req, &task, runtime, runtimeID, testWorkspaceID)
			if failure != nil {
				t.Fatalf("claim failed: %s", failure.outcome)
			}
			if resp.LaneSquadID != tc.want {
				t.Fatalf("lane_squad_id = %q, want %q", resp.LaneSquadID, tc.want)
			}
		})
	}
}
