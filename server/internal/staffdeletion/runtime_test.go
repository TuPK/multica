package staffdeletion

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// A runtime request unregisters the runtime and completes (spec R6 section 3b.5, D98).
func TestRuntimeRequestRunsToCompletion(t *testing.T) {
	ws := workspaceWithOwner(t, "staff job runtime completes")
	owner := fx.User(t, "runtime owner", testutil.UniqueEmail("rt-owner"))
	runtimeID := fx.Runtime(t, "staff job runtime", testutil.Cols{"workspace_id": ws, "owner_id": owner})
	reqID := stageRequest(t, "runtime", runtimeID, "", "pending")

	if n, err := newRunner(nil).RunOnce(context.Background()); err != nil || n != 1 {
		t.Fatalf("RunOnce = %d, %v; want 1, nil", n, err)
	}
	if state, _, code := requestState(t, reqID); state != "completed" || code != "" {
		t.Fatalf("request = %q code %q, want completed", state, code)
	}
	if n := count(t, `SELECT count(*) FROM agent_runtime WHERE id = $1::uuid`, runtimeID); n != 0 {
		t.Errorf("runtime rows = %d, want 0", n)
	}
	if n := count(t, `SELECT count(*) FROM workspace WHERE id = $1::uuid`, ws); n != 1 {
		t.Errorf("the runtime's workspace was touched: rows = %d", n)
	}
}

// Active agents on the runtime fail the request at once, with its code, and the
// runtime stays: staff must reassign the agents first.
func TestRuntimeRequestWithActiveAgentFailsAtOnce(t *testing.T) {
	ws := workspaceWithOwner(t, "staff job runtime busy")
	owner := fx.User(t, "busy runtime owner", testutil.UniqueEmail("rt-busy"))
	runtimeID := fx.Runtime(t, "staff job busy runtime", testutil.Cols{"workspace_id": ws, "owner_id": owner})
	fx.Agent(t, "staff job busy agent", runtimeID, testutil.Cols{"workspace_id": ws, "owner_id": owner})
	reqID := stageRequest(t, "runtime", runtimeID, "", "pending")

	newRunner(nil).RunOnce(context.Background())
	state, attempts, code := requestState(t, reqID)
	if state != "failed" || code != "runtime_has_active_agents" || attempts != 1 {
		t.Fatalf("request = %q attempts %d code %q, want failed/1/runtime_has_active_agents", state, attempts, code)
	}
	if n := count(t, `SELECT count(*) FROM agent_runtime WHERE id = $1::uuid`, runtimeID); n != 1 {
		t.Errorf("runtime rows = %d, want the busy runtime kept", n)
	}
}
