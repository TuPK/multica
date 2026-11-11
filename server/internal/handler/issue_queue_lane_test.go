package handler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// TestPromoteQueuedIssuesRoomsRunSideBySideOnOneProject: an in-place project keeps
// one board, but a queued issue waits only for a running issue of its own room.
// Runs of another room, or of no room, do not hold it the same way: a room's
// queued issue is released while another room's issue runs, and a no-room
// queued issue still waits for every running issue of the project.
func TestPromoteQueuedIssuesRoomsRunSideBySideOnOneProject(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	ws := testWorkspaceID
	leader := seededReadyAgentID(t)
	var runtimeID string
	dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, leader).Scan(&runtimeID)

	project := dbfx.Project(t, "lane project "+suffix, testutil.Cols{"workspace_id": ws})
	roomA := dbfx.Squad(t, "room a "+suffix, leader, testutil.Cols{"workspace_id": ws})
	roomB := dbfx.Squad(t, "room b "+suffix, leader, testutil.Cols{"workspace_id": ws})
	agentInB := dbfx.Agent(t, "lane agent "+suffix, runtimeID, testutil.Cols{
		"workspace_id": ws, "home_squad_id": roomB,
	})

	running := dbfx.Issue(t, "lane running in A", testutil.Cols{
		"status": "in_progress", "project_id": project,
		"assignee_type": "squad", "assignee_id": roomA,
	})
	dbfx.Task(t, leader, testutil.Cols{
		"issue_id": running, "runtime_id": runtimeID, "status": "running", "started_at": time.Now(),
	})

	sameRoom := queueIssueIn(t, "queued in A", project, "squad", roomA)
	otherRoom := queueIssueIn(t, "queued in B", project, "squad", roomB)
	agentRoom := queueIssueIn(t, "queued by agent of B", project, "agent", agentInB)
	noRoom := queueIssueIn(t, "queued with no room", project, "", "")

	if _, err := testHandler.PromoteQueuedIssues(context.Background(), parseUUID(ws), false); err != nil {
		t.Fatalf("PromoteQueuedIssues: %v", err)
	}

	if got := queuedIssueStatus(t, sameRoom); got != "backlog" {
		t.Fatalf("queued issue of the running room = %q, want backlog until its room's run ends", got)
	}
	if got := queuedIssueStatus(t, otherRoom); got != "todo" {
		t.Fatalf("queued issue of another room = %q, want todo beside the running room", got)
	}
	if got := queuedIssueStatus(t, agentRoom); got != "todo" {
		t.Fatalf("queued issue of an agent in another room = %q, want todo", got)
	}
	if got := queuedIssueStatus(t, noRoom); got != "backlog" {
		t.Fatalf("queued issue with no room = %q, want backlog while the project runs", got)
	}
}

// queueIssueIn makes a backlog issue in the project, queued with no waits and
// assigned to the given kind of assignee ("" for none).
func queueIssueIn(t *testing.T, title, projectID, assigneeType, assigneeID string) string {
	t.Helper()
	cols := testutil.Cols{"status": "backlog", "project_id": projectID}
	if assigneeType != "" {
		cols["assignee_type"] = assigneeType
		cols["assignee_id"] = assigneeID
	}
	id := dbfx.Issue(t, title, cols)
	setRunQueue(t, id)
	return id
}
