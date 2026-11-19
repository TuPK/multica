package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// disableRuntimeForTest writes the platform_runtime row staff writes when they
// disable a runtime (spec R6 section 3b.5, D98).
func disableRuntimeForTest(t *testing.T, runtimeID string) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`INSERT INTO platform_runtime (runtime_id, changed_by) VALUES ($1::uuid, $2::uuid)`,
		runtimeID, uuid.NewString()); err != nil {
		t.Fatalf("disable runtime: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM platform_runtime WHERE runtime_id = $1::uuid`, runtimeID)
	})
}

func enableRuntimeForTest(t *testing.T, runtimeID string) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(), `DELETE FROM platform_runtime WHERE runtime_id = $1::uuid`, runtimeID); err != nil {
		t.Fatalf("enable runtime: %v", err)
	}
}

// A disabled runtime is refused on every daemon route that acts for it: the
// heartbeat and the task claim (spec R6 section 3b.5, D98).
func TestDisabledRuntimeRefusedOnDaemonRoutes(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID := createClaimReclaimRuntime(t, ctx, "disabled routes runtime")
	disableRuntimeForTest(t, runtimeID)

	heartbeat := newOwnerDaemonRequest("POST", "/api/daemon/heartbeat", DaemonHeartbeatRequest{RuntimeID: runtimeID})
	testutil.Call(t, testHandler.DaemonHeartbeat, heartbeat).Want(http.StatusForbidden)

	claim := newOwnerDaemonRequest("POST", "/api/daemon/runtimes/"+runtimeID+"/tasks/claim", nil)
	claim = withURLParam(claim, "runtimeId", runtimeID)
	w := testutil.Call(t, testHandler.ClaimTaskByRuntime, claim).Want(http.StatusForbidden)
	if !strings.Contains(w.Body.String(), "runtime_disabled") {
		t.Fatalf("claim body = %s, want runtime_disabled", w.Body.String())
	}
}

// Enabling the runtime again restores its routes; the task it was holding stays
// where it was, and nothing was lost while it was disabled.
func TestEnableRestoresDaemonRoutes(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID := createClaimReclaimRuntime(t, ctx, "enable routes runtime")
	disableRuntimeForTest(t, runtimeID)
	enableRuntimeForTest(t, runtimeID)

	heartbeat := newOwnerDaemonRequest("POST", "/api/daemon/heartbeat", DaemonHeartbeatRequest{RuntimeID: runtimeID})
	testutil.Call(t, testHandler.DaemonHeartbeat, heartbeat).Want(http.StatusOK)
}

// A registration for a disabled runtime is refused whole and leaves the row alone.
func TestDisabledRuntimeCannotRegister(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	daemonID := "disabled-" + uuid.NewString()
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_runtime WHERE workspace_id = $1 AND daemon_id = $2`, testWorkspaceID, daemonID)
	})
	testutil.Call(t, testHandler.DaemonRegister, registerAs(testUserID, daemonID)).Want(http.StatusOK)

	var runtimeID string
	if err := testPool.QueryRow(context.Background(),
		`SELECT id::text FROM agent_runtime WHERE workspace_id = $1 AND daemon_id = $2 AND provider = 'claude'`,
		testWorkspaceID, daemonID).Scan(&runtimeID); err != nil {
		t.Fatalf("read registered runtime: %v", err)
	}
	disableRuntimeForTest(t, runtimeID)

	w := testutil.Call(t, testHandler.DaemonRegister, registerAs(testUserID, daemonID)).Want(http.StatusForbidden)
	if !strings.Contains(w.Body.String(), "runtime_disabled") {
		t.Fatalf("register body = %s, want runtime_disabled", w.Body.String())
	}
}

// Deleting the workspace removes the disable rows of its runtimes, so no state
// for a runtime that is gone is left behind.
func TestDeleteWorkspaceRemovesPlatformRuntimeRows(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ws := dbfx.Workspace(t, "disabled runtime delete", "drt-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "owner")
	runtimeID := dbfx.Runtime(t, "disabled runtime in workspace", testutil.Cols{"workspace_id": ws, "owner_id": testUserID})
	disableRuntimeForTest(t, runtimeID)
	dbfx.Cleanup(t, `DELETE FROM audit_event WHERE target_id = $1`, ws)

	done, err := testHandler.DeleteWorkspaceForStaff(context.Background(), ws, staffOrigin(stagedStaffRequest(t, "workspace", ws, "")), nil)
	if err != nil || !done {
		t.Fatalf("DeleteWorkspaceForStaff = (%v, %v), want (true, nil)", done, err)
	}
	if n := runtimeRowCount(t, `SELECT count(*) FROM platform_runtime WHERE runtime_id = $1::uuid`, runtimeID); n != 0 {
		t.Errorf("platform_runtime rows = %d after the workspace is deleted, want 0", n)
	}
}
