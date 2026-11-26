package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// TestPredictionResolvedOnDecision: a person's decision on a proposal, made through the decide
// endpoint, resolves the prediction made for it (durable spec 6.4). The prediction is set up in the
// database; the decision goes through DecideProposal, the path the app and the chat buttons share.
func TestPredictionResolvedOnDecision(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	previous := testHandler.FeatureFlags
	t.Cleanup(func() {
		testHandler.FeatureFlags = previous
		testPool.Exec(context.Background(), `DELETE FROM decision_prediction WHERE workspace_id = $1`, testWorkspaceID)
	})
	p := featureflag.NewStaticProvider()
	p.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: true})
	testHandler.FeatureFlags = featureflag.NewService(p)

	fx := newProposalTestFixture(t)
	// With durable_principal on, an agent's proposal says why the boss decides it (spec 7.4).
	proposal := fx.fileProposal(t, "predicted", map[string]any{"reason": "judgment", "why_not_decide": "the boss owns this call"})
	if _, err := testPool.Exec(context.Background(), `INSERT INTO decision_prediction
		(workspace_id, member_id, subject_type, subject_id, decision_kind, predicted, confidence)
		VALUES ($1, $2, 'proposal', $3, 'proposal:general', 'approve', 0.9)`,
		testWorkspaceID, testUserID, proposal.ID); err != nil {
		t.Fatalf("setup prediction: %v", err)
	}

	testutil.Call(t, testHandler.DecideProposal, decideRequest(proposal.ID, map[string]any{"decision": "approve", "note": "Go ahead."})).
		Want(http.StatusOK)

	var matched bool
	var resolved bool
	testPool.QueryRow(context.Background(), `SELECT matched, resolved_at IS NOT NULL FROM decision_prediction WHERE subject_id = $1`, proposal.ID).
		Scan(&matched, &resolved)
	if !resolved || !matched {
		t.Fatalf("prediction after the decision: resolved=%v matched=%v, want both true", resolved, matched)
	}
}
