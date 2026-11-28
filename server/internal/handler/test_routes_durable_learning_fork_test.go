package handler

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

func TestDurableJobsForTestRunsPrincipalLearning(t *testing.T) {
	e := newDigestEnv(t)
	e.setSecretary(t, e.AgentID)
	memberID := testUserID
	dbfx.InsertNoID(t, "principal_profile", testutil.Cols{"workspace_id": e.WorkspaceID, "member_id": memberID, "status": "active", "consent_at": testutil.Raw("now()")}, "workspace_id = $1 AND member_id = $2", e.WorkspaceID, memberID)
	subject := uuid.NewString()
	dbfx.Insert(t, "decision_prediction", testutil.Cols{
		"workspace_id": e.WorkspaceID, "member_id": memberID, "subject_type": "inquiry", "subject_id": subject,
		"decision_kind": "inquiry", "predicted": "yes", "confidence": 0.9, "actual": "yes", "matched": true, "resolved_at": testutil.Raw("now()"),
	})
	t.Cleanup(func() {
		dbfx.Exec(t, `DELETE FROM agent_task_queue WHERE agent_id = $1 AND context->>'kind' = 'principal_learning'`, e.AgentID)
	})
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: true})
	h := *testHandler
	h.FeatureFlags = featureflag.NewService(provider)
	req := newRequest("POST", "/api/test/durable/jobs", nil)
	req.Header.Set("X-Workspace-ID", e.WorkspaceID)
	testutil.Call(t, h.RunDurableJobsForTest, req).Want(http.StatusOK)
	if n := dbfx.Count(t, `SELECT count(*) FROM decision_prediction p JOIN agent_task_queue q ON q.id = p.learning_task_id WHERE p.workspace_id = $1 AND p.subject_id = $2 AND q.agent_id = $3`, e.WorkspaceID, subject, e.AgentID); n != 1 {
		t.Fatalf("learning tasks for the decision = %d, want 1", n)
	}
}
