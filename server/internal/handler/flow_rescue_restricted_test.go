package handler

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/projectaccess"
)

// An explicit advance rescues the agent tasks of the current step. A task in a
// restricted project the advancing viewer cannot see is left alone: a run queued
// on it would change, and the answer would name, a task the viewer cannot see.
func TestRescueSkipsTaskInProjectTheViewerCannotSee(t *testing.T) {
	f, requestID := manualFlowFixture(t)
	request := loadIssueRow(t, requestID)
	docs := tasksAt(t, request, "collect", 0)["docs"]
	failRunsLikeDaemonRestart(t, docs, f.Agents.Docs, 1)
	hidden := newRestrictedFixture(t)
	dbfx.Exec(t, `UPDATE issue SET project_id = $2 WHERE id = $1`, docs, hidden.restrictedProjectID)

	ctx := context.Background()
	scope, err := projectaccess.ForPrincipals(ctx, testHandler.Queries, parseUUID(testWorkspaceID), parseUUID(hidden.outsiderID))
	if err != nil {
		t.Fatalf("scope: %v", err)
	}
	rescued, err := testHandler.rescueFlowTasks(ctx, request, scope)
	if err != nil {
		t.Fatalf("rescueFlowTasks: %v", err)
	}
	if len(rescued) != 0 {
		t.Errorf("rescued %d tasks in a project the viewer cannot see, want none", len(rescued))
	}
	if n := queuedRuns(t, docs); n != 0 {
		t.Errorf("hidden task got %d live runs, want none", n)
	}
}
