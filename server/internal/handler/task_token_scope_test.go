package handler

// A run reads with the visibility of the person who started it, not with the
// visibility of the runtime owner whose id the mat_ token carries (spec 7.1
// rule 12, D55). The requests here go through middleware.Auth with a real mat_
// token, so the headers the handler sees are the ones the middleware wrote.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// matRun is a running task of the test workspace and its mat_ token. The token's
// user is the workspace owner (the runtime owner), who sees every project, so
// any narrower answer comes from the originator.
type matRun struct {
	token  string
	taskID string
}

// startMatRun queues a running task with the given originator ("" for NULL) and
// mints its mat_ token the way the claim does.
func startMatRun(t *testing.T, originatorID string) matRun {
	t.Helper()
	agentID := createHandlerTestAgent(t, "ScopeRunAgent-"+uuid.NewString()[:8], []byte("[]"))
	cols := testutil.Cols{"status": "running", "runtime_id": handlerTestRuntimeID(t), "started_at": testutil.Raw("now()")}
	if originatorID != "" {
		cols["originator_user_id"] = originatorID
		cols["accountable_user_id"] = originatorID
		cols["originator_source"] = "direct_human"
	}
	taskID := dbfx.Task(t, agentID, cols)

	tokenStr, err := auth.GenerateAgentTaskToken()
	if err != nil {
		t.Fatalf("generate task token: %v", err)
	}
	if _, err := testHandler.Queries.CreateTaskToken(context.Background(), db.CreateTaskTokenParams{
		ID:          parseUUID(uuid.NewString()),
		TokenHash:   auth.HashToken(tokenStr),
		TaskID:      parseUUID(taskID),
		AgentID:     parseUUID(agentID),
		WorkspaceID: parseUUID(testWorkspaceID),
		UserID:      parseUUID(testUserID),
		ExpiresAt:   pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("create task token: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM task_token WHERE task_id = $1`, taskID)
	})
	return matRun{token: tokenStr, taskID: taskID}
}

func (m matRun) serve(h http.HandlerFunc, method, path string, params map[string]string) *httptest.ResponseRecorder {
	req := newRequest(method, path, nil)
	req.Header.Del("X-User-ID")
	req.Header.Set("Authorization", "Bearer "+m.token)
	for k, v := range params {
		req = withURLParam(req, k, v)
	}
	w := httptest.NewRecorder()
	middleware.Auth(testHandler.Queries, nil, nil, nil, nil)(h).ServeHTTP(w, req)
	return w
}

func (m matRun) getIssue(issueID string) *httptest.ResponseRecorder {
	return m.serve(testHandler.GetIssue, "GET", "/api/issues/"+issueID, map[string]string{"id": issueID})
}

func (m matRun) listBody(t *testing.T) string {
	t.Helper()
	w := m.serve(testHandler.ListIssues, "GET", "/api/issues?workspace_id="+testWorkspaceID+"&limit=500", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("ListIssues with a mat_ token: %d %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

func TestMatTokenReadsWithOriginatorScope(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	hiddenTitle := "mat hidden " + uuid.NewString()[:8]
	openTitle := "mat open " + uuid.NewString()[:8]
	hiddenID := createIssueInProject(t, hiddenTitle, f.restrictedProjectID)
	openID := createIssueInProject(t, openTitle, f.workspaceProjectID)

	cases := []struct {
		name       string
		originator string
		seesHidden bool
	}{
		{"originator outside the project", f.outsiderID, false},
		{"originator granted", f.granteeID, true},
		{"originator is the owner", testUserID, true},
		{"NULL originator", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run := startMatRun(t, c.originator)

			if w := run.getIssue(openID); w.Code != http.StatusOK {
				t.Fatalf("open issue: %d %s", w.Code, w.Body.String())
			}
			wantHidden := http.StatusNotFound
			if c.seesHidden {
				wantHidden = http.StatusOK
			}
			if w := run.getIssue(hiddenID); w.Code != wantHidden {
				t.Errorf("restricted issue: %d %s, want %d", w.Code, w.Body.String(), wantHidden)
			}
			list := run.listBody(t)
			if !strings.Contains(list, openTitle) {
				t.Errorf("list lacks the open issue")
			}
			if got := strings.Contains(list, hiddenTitle); got != c.seesHidden {
				t.Errorf("list contains the restricted issue = %v, want %v", got, c.seesHidden)
			}
		})
	}
}

func TestAccessLostMidRun(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	title := "mat lost " + uuid.NewString()[:8]
	issueID := createIssueInProject(t, title, f.restrictedProjectID)
	run := startMatRun(t, f.granteeID)

	if w := run.getIssue(issueID); w.Code != http.StatusOK {
		t.Fatalf("before the revoke: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(run.listBody(t), title) {
		t.Fatal("before the revoke the list lacks the issue")
	}

	if _, err := testPool.Exec(context.Background(),
		`DELETE FROM project_member WHERE project_id = $1 AND user_id = $2`, f.restrictedProjectID, f.granteeID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if w := run.getIssue(issueID); w.Code != http.StatusNotFound {
		t.Errorf("after the revoke: %d %s, want 404", w.Code, w.Body.String())
	}
	if strings.Contains(run.listBody(t), title) {
		t.Error("after the revoke the list still has the issue")
	}

	// Leaving the workspace ends the originator's visibility altogether.
	if _, err := testPool.Exec(context.Background(),
		`DELETE FROM member WHERE workspace_id = $1 AND user_id = $2`, testWorkspaceID, f.granteeID); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	if w := run.getIssue(issueID); w.Code != http.StatusNotFound {
		t.Errorf("after leaving the workspace: %d %s, want 404", w.Code, w.Body.String())
	}
}
