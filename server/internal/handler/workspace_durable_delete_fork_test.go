package handler

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Deleting the workspace removes its durable rows, including the ones that carry no workspace_id
// of their own (durable_outcome_applied follows the workspace's status transitions).
func TestDeleteWorkspaceRemovesDurableRows(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ws := dbfx.Workspace(t, "durable delete", "durable-del-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "owner")
	issue := dbfx.Issue(t, "durable delete issue", map[string]any{"workspace_id": ws})
	transition := dbfx.Insert(t, "issue_status_transition", map[string]any{
		"workspace_id": ws, "issue_id": issue, "to_status_key": "done", "actor_type": "member", "actor_id": testUserID,
	})
	dbfx.InsertNoID(t, "durable_outcome_applied", map[string]any{"transition_id": transition, "status": "applied", "attempts": 1}, "transition_id = $1", transition)
	dbfx.Insert(t, "effect_event", map[string]any{
		"workspace_id": ws, "source": "connected_app", "target": "probe.target", "effect_class": "write",
		"reversibility": "compensable", "decision": "allow", "status": "executed", "summary": "durable delete",
	})

	done, err := testHandler.DeleteWorkspaceForStaff(context.Background(), ws, staffOrigin(stagedStaffRequest(t, "workspace", ws, "")), nil)
	if err != nil || !done {
		t.Fatalf("DeleteWorkspaceForStaff = (%v, %v), want (true, nil)", done, err)
	}
	for _, q := range []string{
		`SELECT count(*) FROM effect_event WHERE workspace_id = $1::uuid`,
		`SELECT count(*) FROM issue_status_transition WHERE workspace_id = $1::uuid`,
		`SELECT count(*) FROM durable_outcome_applied WHERE transition_id = $1::uuid`,
	} {
		arg := ws
		if q == `SELECT count(*) FROM durable_outcome_applied WHERE transition_id = $1::uuid` {
			arg = transition
		}
		if n := dbfx.Count(t, q, arg); n != 0 {
			t.Fatalf("%s = %d after the workspace is deleted, want 0", q, n)
		}
	}
}
