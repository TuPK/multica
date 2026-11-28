package service

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

func TestResolvingPredictionReviewsDelegationAccuracy(t *testing.T) {
	dt := newDelegationTest(t)
	ctx := context.Background()
	const kind = "proposal:hire"
	dt.seedResolved(t, kind, 100, trueN(20))
	if err := dt.del().Enable(ctx, dt.owner, dt.ws, dt.owner.ID, kind, 0.9); err != nil {
		t.Fatalf("enable delegation: %v", err)
	}
	dt.seedResolved(t, kind, 1, falseN(10))
	subject := dbid.NewV7()
	dt.Insert(t, "decision_prediction", testutil.Cols{
		"workspace_id": dt.WorkspaceID, "member_id": util.UUIDToString(dt.owner.ID),
		"subject_type": "proposal", "subject_id": util.UUIDToString(subject),
		"decision_kind": kind, "predicted": "approve", "confidence": 0.9,
	})
	if err := dt.pred().Resolve(ctx, dt.ws, "proposal", subject, "reject"); err != nil {
		t.Fatalf("resolve prediction: %v", err)
	}
	if n := dt.Count(t, `SELECT COUNT(*) FROM principal_delegation WHERE workspace_id = $1 AND member_id = $2 AND decision_kind = $3 AND disabled_at IS NULL`, dt.WorkspaceID, dt.owner.ID, kind); n != 0 {
		t.Fatalf("delegation remains active after accuracy fell below 80%%")
	}
}
