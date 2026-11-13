package handler

import (
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// The cleanup of a flow test shares a database with real runs (a drill, another suite). It may remove
// the runs of the test workspace and nothing else.
func TestCleanupFlowRows_LeavesOtherWorkspacesAlone(t *testing.T) {
	other := dbfx.Workspace(t, "Cleanup other "+uuid.NewString(), "cleanup-other-"+uuid.NewString())
	otherRuntime := dbfx.Runtime(t, "cleanup runtime "+uuid.NewString(), testutil.Cols{"workspace_id": other})
	otherAgent := dbfx.Agent(t, "cleanup agent "+uuid.NewString(), otherRuntime, testutil.Cols{"workspace_id": other})
	foreign := dbfx.Task(t, otherAgent, testutil.Cols{"runtime_id": otherRuntime})

	ownAgent := seededReadyAgentID(t)
	var ownRuntime string
	dbfx.QueryRow(t, `SELECT runtime_id FROM agent WHERE id = $1`, ownAgent).Scan(&ownRuntime)
	own := dbfx.Task(t, ownAgent, testutil.Cols{"runtime_id": ownRuntime})

	t.Run("cleanup", func(t *testing.T) { cleanupFlowRows(t) })

	if n := dbfx.Count(t, `SELECT count(*) FROM agent_task_queue WHERE id = $1`, foreign); n != 1 {
		t.Fatalf("the run of another workspace was removed (found %d)", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM agent_task_queue WHERE id = $1`, own); n != 0 {
		t.Fatalf("the run of the test workspace was kept (found %d)", n)
	}
}
