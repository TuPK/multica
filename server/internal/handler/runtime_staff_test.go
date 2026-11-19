package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func runtimeRowCount(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func staffRuntimeOrigin(requestID string) DeletionOrigin {
	return DeletionOrigin{Kind: DeletionOriginStaff, RequestID: requestID, StaffID: uuid.NewString(), StaffLabel: "staff@example.test"}
}

// A staff unregister removes the runtime through the owner's teardown and writes
// runtime.deleted with the staff actor, the tenant and the request (spec R6
// section 3b.5, D98).
func TestStaffRuntimeUnregisterRemovesRuntimeWithStaffAudit(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID := createClaimReclaimRuntime(t, ctx, "staff unregister runtime")
	requestID := uuid.NewString()

	done, err := testHandler.DeleteRuntimeForStaff(ctx, runtimeID, staffRuntimeOrigin(requestID))
	if err != nil || !done {
		t.Fatalf("DeleteRuntimeForStaff = %v, %v; want true, nil", done, err)
	}
	if n := runtimeRowCount(t, `SELECT count(*) FROM agent_runtime WHERE id = $1::uuid`, runtimeID); n != 0 {
		t.Fatalf("runtime rows left = %d, want 0", n)
	}
	if n := runtimeRowCount(t, `SELECT count(*) FROM audit_event WHERE action = 'runtime.deleted' AND target_id = $1 AND actor_type = 'staff' AND detail->>'request_id' = $2`, runtimeID, requestID); n != 1 {
		t.Fatalf("runtime.deleted rows by staff for the request = %d, want 1", n)
	}
}

// Staff cannot confirm a cascade, so a runtime with active agents is refused and
// left as it is; the owner's delete refuses the same runtime with 409.
func TestStaffRuntimeUnregisterRefusesActiveAgents(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID := createClaimReclaimRuntime(t, ctx, "staff unregister busy runtime")
	dbfx.Agent(t, "staff unregister busy agent", runtimeID)

	done, err := testHandler.DeleteRuntimeForStaff(ctx, runtimeID, staffRuntimeOrigin(uuid.NewString()))
	var refused *StaffRuntimeError
	if done || !errors.As(err, &refused) || refused.Code != StaffRuntimeErrHasActiveAgents {
		t.Fatalf("DeleteRuntimeForStaff = %v, %v; want runtime_has_active_agents", done, err)
	}
	if n := runtimeRowCount(t, `SELECT count(*) FROM agent_runtime WHERE id = $1::uuid`, runtimeID); n != 1 {
		t.Fatalf("refused unregister removed the runtime: rows = %d", n)
	}
}

// A runtime that is already gone is done.
func TestStaffRuntimeUnregisterMissingRuntimeIsDone(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	done, err := testHandler.DeleteRuntimeForStaff(context.Background(), uuid.NewString(), staffRuntimeOrigin(uuid.NewString()))
	if err != nil || done {
		t.Fatalf("DeleteRuntimeForStaff = %v, %v; want false, nil", done, err)
	}
}
