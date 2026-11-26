package handler

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Moving a squad member to another runtime is a seat change (durable spec 9.3): with
// durable_seats on it writes seat_model_changed for that agent.
func TestSetSquadRuntime_LogsSeatChangeWhenFlagOn(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.DurableSeats, true)
	e := newDigestEnv(t)
	squad := e.squad(t, "Seat batch")
	target := dbfx.Runtime(t, "Seat target "+uuid.NewString(), testutil.Cols{
		"workspace_id": e.WorkspaceID, "provider": "codex", "visibility": "public", "owner_id": testUserID,
	})
	dbfx.SquadMember(t, squad, "agent", e.AgentID)

	var out SetSquadRuntimeResponse
	testutil.Call(t, testHandler.SetSquadRuntime, asUser(setSquadRuntimeRequest(e.WorkspaceID, squad,
		map[string]any{"runtime_id": target}), testUserID)).Want(http.StatusOK).JSON(&out)
	if _, ok := resultIDs(out.Updated)[e.AgentID]; !ok {
		t.Fatalf("agent should move: %+v", out.Updated)
	}

	var n int
	dbfx.QueryRow(t, `SELECT COUNT(*) FROM governance_event
		WHERE workspace_id = $1 AND kind = 'seat_model_changed' AND subject_id = $2`, e.WorkspaceID, e.AgentID).Scan(&n)
	if n != 1 {
		t.Errorf("seat_model_changed events for the moved agent = %d, want 1", n)
	}
}
