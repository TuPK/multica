package handler

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// A member credential asking the daemon GC check about an issue of a
// restricted project it cannot see gets the answer for a missing issue (spec
// 7.1 rules 5 and 9). A daemon token keeps its access.

type gcVisibilityFixture struct {
	hiddenIssue string
	openIssue   string
	memberID    string
}

func newGCVisibilityFixture(t *testing.T) gcVisibilityFixture {
	t.Helper()
	hiddenProj := dbfx.Project(t, "Hidden "+uuid.NewString()[:8], testutil.Cols{"workspace_id": testWorkspaceID, "visibility": "private"})
	ownerJoinsProject(t, testWorkspaceID, hiddenProj)
	openProj := dbfx.Project(t, "Open "+uuid.NewString()[:8], testutil.Cols{"workspace_id": testWorkspaceID, "visibility": "workspace"})
	f := gcVisibilityFixture{
		hiddenIssue: dbfx.Issue(t, "gc hidden", testutil.Cols{"workspace_id": testWorkspaceID, "project_id": hiddenProj, "status": "done"}),
		openIssue:   dbfx.Issue(t, "gc open", testutil.Cols{"workspace_id": testWorkspaceID, "project_id": openProj, "status": "done"}),
		memberID:    dbfx.User(t, "GC member", testutil.UniqueEmail("gc-member")),
	}
	dbfx.Member(t, testWorkspaceID, f.memberID, "member")
	return f
}

func (f gcVisibilityFixture) single(issueID, as string) *http.Request {
	var req *http.Request
	if as == "daemon" {
		req = newDaemonTokenRequest("GET", "/api/daemon/issues/"+issueID+"/gc-check", nil, testWorkspaceID, "gc-daemon")
	} else {
		req = requestAs(as, "GET", "/api/daemon/issues/"+issueID+"/gc-check", nil)
	}
	return withURLParam(req, "issueId", issueID)
}

func (f gcVisibilityFixture) batch(as string, ids ...string) *http.Request {
	body := map[string]any{"issue_ids": ids}
	path := "/api/daemon/workspaces/" + testWorkspaceID + "/issues/gc-check"
	var req *http.Request
	if as == "daemon" {
		req = newDaemonTokenRequest("POST", path, body, testWorkspaceID, "gc-daemon")
	} else {
		req = requestAs(as, "POST", path, body)
	}
	return withURLParam(req, "workspaceId", testWorkspaceID)
}

type gcBatchResult struct {
	Issues []struct {
		ID    string `json:"id"`
		Found bool   `json:"found"`
	} `json:"issues"`
}

func (r gcBatchResult) found(id string) bool {
	for _, i := range r.Issues {
		if i.ID == id {
			return i.Found
		}
	}
	return false
}

func TestGetIssueGCCheck_HiddenIssueIsMissingForMember(t *testing.T) {
	f := newGCVisibilityFixture(t)
	testutil.Call(t, testHandler.GetIssueGCCheck, f.single(f.hiddenIssue, f.memberID)).Want(http.StatusNotFound)
	testutil.Call(t, testHandler.GetIssueGCCheck, f.single(f.openIssue, f.memberID)).Want(http.StatusOK)
	testutil.Call(t, testHandler.GetIssueGCCheck, f.single(f.hiddenIssue, testUserID)).Want(http.StatusOK)
	testutil.Call(t, testHandler.GetIssueGCCheck, f.single(f.hiddenIssue, "daemon")).Want(http.StatusOK)
}

func TestBatchIssueGCCheck_HiddenIssueIsMissingForMember(t *testing.T) {
	f := newGCVisibilityFixture(t)
	missing := uuid.NewString()
	var out gcBatchResult
	testutil.Call(t, testHandler.BatchIssueGCCheck, f.batch(f.memberID, f.hiddenIssue, f.openIssue, missing)).Want(http.StatusOK).JSON(&out)
	if out.found(f.hiddenIssue) {
		t.Errorf("member sees hidden issue as found: %+v", out)
	}
	if !out.found(f.openIssue) {
		t.Errorf("member does not see open issue: %+v", out)
	}

	out = gcBatchResult{}
	testutil.Call(t, testHandler.BatchIssueGCCheck, f.batch(testUserID, f.hiddenIssue)).Want(http.StatusOK).JSON(&out)
	if !out.found(f.hiddenIssue) {
		t.Errorf("owner misses hidden issue: %+v", out)
	}
	out = gcBatchResult{}
	testutil.Call(t, testHandler.BatchIssueGCCheck, f.batch("daemon", f.hiddenIssue)).Want(http.StatusOK).JSON(&out)
	if !out.found(f.hiddenIssue) {
		t.Errorf("daemon token misses hidden issue: %+v", out)
	}
}
