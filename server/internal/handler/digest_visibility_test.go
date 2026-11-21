package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// The company digest omits issues of restricted projects, and the counts and
// flags built from them, for a member who has no project_member row (spec 7.1
// rule 5). The owner still sees everything.

type digestVisibilityFixture struct {
	digestEnv
	squadID    string
	hiddenProj string
	openProj   string
	memberID   string
}

func newDigestVisibilityFixture(t *testing.T) digestVisibilityFixture {
	t.Helper()
	e := newDigestEnv(t)
	f := digestVisibilityFixture{digestEnv: e, squadID: e.squad(t, "Visibility squad")}
	f.hiddenProj = dbfx.Project(t, "Hidden "+uuid.NewString()[:8], testutil.Cols{"workspace_id": e.WorkspaceID, "visibility": "private"})
	ownerJoinsProject(t, e.WorkspaceID, f.hiddenProj)
	f.openProj = dbfx.Project(t, "Open "+uuid.NewString()[:8], testutil.Cols{"workspace_id": e.WorkspaceID, "visibility": "workspace"})
	f.memberID = dbfx.User(t, "Digest member", testutil.UniqueEmail("digest-member"))
	dbfx.Member(t, e.WorkspaceID, f.memberID, "member")
	return f
}

// digestAs reads the digest as the given user, as raw JSON text and decoded.
func (f digestVisibilityFixture) digestAs(t *testing.T, userID string) (string, service.Digest) {
	t.Helper()
	req := requestAs(userID, "GET", "/api/company/digest", nil)
	req.Header.Set("X-Workspace-ID", f.WorkspaceID)
	res := testutil.Call(t, testHandler.GetCompanyDigest, req).Want(http.StatusOK)
	var out service.Digest
	res.JSON(&out)
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), out
}

func (f digestVisibilityFixture) issueNumber(t *testing.T, issueID string) int {
	t.Helper()
	var n int
	dbfx.QueryRow(t, `SELECT number FROM issue WHERE id = $1`, issueID).Scan(&n)
	return n
}

// ident is the identifier the digest prints for the issue.
func (f digestVisibilityFixture) ident(t *testing.T, issueID string) string {
	return fmt.Sprintf("DIG-%d", f.issueNumber(t, issueID))
}

func requireHidden(t *testing.T, raw string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if strings.Contains(raw, n) {
			t.Errorf("member digest leaks %q: %s", n, raw)
		}
	}
}

func requireShown(t *testing.T, who, raw string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(raw, n) {
			t.Errorf("%s digest misses %q", who, n)
		}
	}
}

func TestDigestVisibility_PendingProposals(t *testing.T) {
	f := newDigestVisibilityFixture(t)
	hiddenIssue := dbfx.Issue(t, "HIDDEN-PROPOSAL-TITLE", testutil.Cols{"workspace_id": f.WorkspaceID, "project_id": f.hiddenProj})
	openIssue := dbfx.Issue(t, "OPEN-PROPOSAL-TITLE", testutil.Cols{"workspace_id": f.WorkspaceID, "project_id": f.openProj})
	for _, issueID := range []string{hiddenIssue, openIssue} {
		dbfx.Insert(t, "proposal", testutil.Cols{
			"workspace_id": f.WorkspaceID, "squad_id": f.squadID, "issue_id": issueID,
			"created_by_type": "agent", "created_by_id": f.AgentID,
		})
	}

	raw, d := f.digestAs(t, f.memberID)
	requireHidden(t, raw, "HIDDEN-PROPOSAL-TITLE")
	requireShown(t, "member", raw, "OPEN-PROPOSAL-TITLE")
	if got := findDigestSquad(t, d, f.squadID).PendingProposalCount; got != 1 {
		t.Errorf("member pending_proposal_count = %d, want 1", got)
	}

	raw, d = f.digestAs(t, testUserID)
	requireShown(t, "owner", raw, "HIDDEN-PROPOSAL-TITLE", "OPEN-PROPOSAL-TITLE")
	if got := findDigestSquad(t, d, f.squadID).PendingProposalCount; got != 2 {
		t.Errorf("owner pending_proposal_count = %d, want 2", got)
	}
}

func TestDigestVisibility_DoneIssuesAndSquadCounts(t *testing.T) {
	f := newDigestVisibilityFixture(t)
	hidden := f.issue(t, f.squadID, "HIDDEN-DONE-TITLE", "done", testutil.Cols{"project_id": f.hiddenProj})
	f.issue(t, f.squadID, "OPEN-DONE-TITLE", "done", testutil.Cols{"project_id": f.openProj})
	f.issue(t, f.squadID, "hidden in progress", "in_progress", testutil.Cols{"project_id": f.hiddenProj})
	f.issue(t, f.squadID, "hidden blocked", "blocked", testutil.Cols{"project_id": f.hiddenProj})

	raw, d := f.digestAs(t, f.memberID)
	requireHidden(t, raw, "HIDDEN-DONE-TITLE", f.ident(t, hidden))
	requireShown(t, "member", raw, "OPEN-DONE-TITLE")
	sq := findDigestSquad(t, d, f.squadID)
	if sq.Done != 1 || sq.InProgress != 0 || sq.Blocked != 0 {
		t.Errorf("member counts done/in_progress/blocked = %d/%d/%d, want 1/0/0", sq.Done, sq.InProgress, sq.Blocked)
	}

	raw, d = f.digestAs(t, testUserID)
	requireShown(t, "owner", raw, "HIDDEN-DONE-TITLE", "OPEN-DONE-TITLE", f.ident(t, hidden))
	sq = findDigestSquad(t, d, f.squadID)
	if sq.Done != 2 || sq.InProgress != 1 || sq.Blocked != 1 {
		t.Errorf("owner counts done/in_progress/blocked = %d/%d/%d, want 2/1/1", sq.Done, sq.InProgress, sq.Blocked)
	}
}

func TestDigestVisibility_CharterSilentIgnoresHiddenActivity(t *testing.T) {
	f := newDigestVisibilityFixture(t)
	f.digestCharter(t, f.squadID, "10 days")
	// Keeps the squad in the full list without recent activity of its own.
	f.digestPendingProposalAt(t, f.squadID, "10 days")
	f.issue(t, f.squadID, "hidden recent work", "todo", testutil.Cols{"project_id": f.hiddenProj})

	_, d := f.digestAs(t, f.memberID)
	if c := findDigestSquad(t, d, f.squadID).Charter; c == nil || !c.Silent {
		t.Errorf("member sees the squad as active because of a hidden issue: charter=%+v", c)
	}
	_, d = f.digestAs(t, testUserID)
	if c := findDigestSquad(t, d, f.squadID).Charter; c == nil || c.Silent {
		t.Errorf("owner should see the squad as active: charter=%+v", c)
	}
}

func TestDigestVisibility_BlockedDirectives(t *testing.T) {
	f := newDigestVisibilityFixture(t)
	past := testutil.Raw("now() - interval '3 days'")
	hidden := dbfx.Issue(t, "HIDDEN-DIRECTIVE-TITLE", testutil.Cols{
		"workspace_id": f.WorkspaceID, "project_id": f.hiddenProj, "origin_type": "directive", "status": "blocked", "due_date": past,
	})
	dbfx.Issue(t, "OPEN-DIRECTIVE-TITLE", testutil.Cols{
		"workspace_id": f.WorkspaceID, "project_id": f.openProj, "origin_type": "directive", "status": "blocked", "due_date": past,
	})

	raw, d := f.digestAs(t, f.memberID)
	requireHidden(t, raw, "HIDDEN-DIRECTIVE-TITLE", f.ident(t, hidden))
	requireShown(t, "member", raw, "OPEN-DIRECTIVE-TITLE")
	if len(d.BlockedDirectives) != 1 {
		t.Errorf("member blocked directives = %d, want 1", len(d.BlockedDirectives))
	}
	raw, d = f.digestAs(t, testUserID)
	requireShown(t, "owner", raw, "HIDDEN-DIRECTIVE-TITLE", "OPEN-DIRECTIVE-TITLE")
	if len(d.BlockedDirectives) != 2 {
		t.Errorf("owner blocked directives = %d, want 2", len(d.BlockedDirectives))
	}
}

func TestDigestVisibility_OverdueSquadRequests(t *testing.T) {
	f := newDigestVisibilityFixture(t)
	old := testutil.Raw("now() - interval '30 days'")
	mk := func(title, project string) string {
		return dbfx.Issue(t, title, testutil.Cols{
			"workspace_id": f.WorkspaceID, "project_id": project, "origin_type": "squad_request",
			"assignee_type": "squad", "assignee_id": f.squadID, "status": "todo", "created_at": old,
		})
	}
	hidden := mk("HIDDEN-REQUEST-TITLE", f.hiddenProj)
	mk("OPEN-REQUEST-TITLE", f.openProj)

	raw, d := f.digestAs(t, f.memberID)
	requireHidden(t, raw, "HIDDEN-REQUEST-TITLE", f.ident(t, hidden))
	requireShown(t, "member", raw, "OPEN-REQUEST-TITLE")
	if len(d.OverdueRequests) != 1 {
		t.Errorf("member overdue requests = %d, want 1", len(d.OverdueRequests))
	}
	raw, d = f.digestAs(t, testUserID)
	requireShown(t, "owner", raw, "HIDDEN-REQUEST-TITLE", "OPEN-REQUEST-TITLE")
	if len(d.OverdueRequests) != 2 {
		t.Errorf("owner overdue requests = %d, want 2", len(d.OverdueRequests))
	}
}

func TestDigestVisibility_SecretaryRequestsPastDeadline(t *testing.T) {
	f := newDigestVisibilityFixture(t)
	// The Secretary desk is found by title; a restricted desk hides its requests.
	desk := dbfx.Project(t, service.SecretaryDeskProjectTitle, testutil.Cols{"workspace_id": f.WorkspaceID, "visibility": "private"})
	ownerJoinsProject(t, f.WorkspaceID, desk)
	hidden := dbfx.Issue(t, "HIDDEN-SECRETARY-TITLE", testutil.Cols{
		"workspace_id": f.WorkspaceID, "project_id": desk, "assignee_type": "agent", "assignee_id": f.AgentID,
		"status": "todo", "due_date": testutil.Raw("now() - interval '3 days'"),
	})

	raw, d := f.digestAs(t, f.memberID)
	requireHidden(t, raw, "HIDDEN-SECRETARY-TITLE", f.ident(t, hidden))
	if len(d.RequestsPastDeadline) != 0 {
		t.Errorf("member requests past deadline = %d, want 0", len(d.RequestsPastDeadline))
	}
	raw, d = f.digestAs(t, testUserID)
	requireShown(t, "owner", raw, "HIDDEN-SECRETARY-TITLE")
	if len(d.RequestsPastDeadline) != 1 {
		t.Errorf("owner requests past deadline = %d, want 1", len(d.RequestsPastDeadline))
	}
}
