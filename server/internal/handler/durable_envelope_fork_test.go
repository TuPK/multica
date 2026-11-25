package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// TestEnvelopeAgentForbidden: an agent acting through its own task may not set a squad's
// autonomy envelope (durable spec 3.4 and 3.5). The refusal is 403 and writes nothing.
func TestEnvelopeAgentForbidden(t *testing.T) {
	o := newOutcomeFixture(t)
	task := o.wsfx.Task(t, o.doer, testutil.Cols{"issue_id": o.issue, "status": "running", "runtime_id": o.runtime})
	squad := o.wsfx.Squad(t, "envelope squad", o.doer)
	o.wsfx.Cleanup(t, `DELETE FROM squad_autonomy_envelope WHERE squad_id = $1`, squad)

	body := map[string]any{"floor_level": 0, "ceiling_level": 2, "reason": "an agent sets its own room"}
	req := agentReq(t, o.fx.ID, o.fx.Owner, o.doer, task, http.MethodPut, "/api/squads/"+squad+"/autonomy/envelope", body)
	testutil.Call(t, testHandler.SetSquadEnvelope, testutil.WithURLParams(req, "id", squad)).Want(http.StatusForbidden)

	if n := o.wsfx.Count(t, `SELECT count(*) FROM squad_autonomy_envelope WHERE squad_id = $1`, squad); n != 0 {
		t.Fatalf("envelope rows after an agent's refused write = %d, want none", n)
	}
}
