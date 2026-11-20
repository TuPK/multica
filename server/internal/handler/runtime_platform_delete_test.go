package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Deleting a runtime removes the platform_runtime row staff wrote for it, in the
// same transaction, on both the owner delete and the staff unregister (spec R6
// section 3b.5 and AC128, D98). Otherwise the row outlives its runtime.
func TestDeleteRuntimeRemovesPlatformRuntimeRows(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	t.Run("owner delete", func(t *testing.T) {
		runtimeID := createClaimReclaimRuntime(t, ctx, "owner delete platform row runtime")
		disableRuntimeForTest(t, runtimeID)

		req := withURLParams(newRequestAs(testUserID, "DELETE", "/api/runtimes/"+runtimeID, nil), "runtimeId", runtimeID)
		testutil.Call(t, testHandler.DeleteAgentRuntime, req).Want(http.StatusOK)

		if n := runtimeRowCount(t, `SELECT count(*) FROM agent_runtime WHERE id = $1::uuid`, runtimeID); n != 0 {
			t.Fatalf("runtime rows left = %d, want 0", n)
		}
		if n := runtimeRowCount(t, `SELECT count(*) FROM platform_runtime WHERE runtime_id = $1::uuid`, runtimeID); n != 0 {
			t.Fatalf("platform_runtime rows left after owner delete = %d, want 0", n)
		}
	})

	t.Run("staff unregister", func(t *testing.T) {
		runtimeID := createClaimReclaimRuntime(t, ctx, "staff delete platform row runtime")
		disableRuntimeForTest(t, runtimeID)

		done, err := testHandler.DeleteRuntimeForStaff(ctx, runtimeID, staffRuntimeOrigin(uuid.NewString()))
		if err != nil || !done {
			t.Fatalf("DeleteRuntimeForStaff = %v, %v; want true, nil", done, err)
		}
		if n := runtimeRowCount(t, `SELECT count(*) FROM platform_runtime WHERE runtime_id = $1::uuid`, runtimeID); n != 0 {
			t.Fatalf("platform_runtime rows left after staff unregister = %d, want 0", n)
		}
	})
}
