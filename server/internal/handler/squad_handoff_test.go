package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// handoffFx is a planning squad with a request, and the SDLC build squad with an open project to hand it to.
type handoffFx struct {
	iniWorkspace
	Planning string
	Lead     string
	LeadTask string
	Project  string
}

func newHandoffFx(t *testing.T) handoffFx {
	t.Helper()
	fx := newIniWorkspace(t)
	// The fixtures insert issues with numbers of their own, so the counter starts above them.
	dbfx.Exec(t, `UPDATE workspace SET issue_counter = 1000 WHERE id = $1`, fx.ID)
	squad, lead := planningSquad(t, fx, "Planning")
	project := dbfxProject(t, fx, "Build repo")
	dbfx.Exec(t, `UPDATE project SET squad_id = $1 WHERE id = $2`, fx.Build, project)
	return handoffFx{iniWorkspace: fx, Planning: squad, Lead: lead, LeadTask: createHandlerTestTaskForAgent(t, lead), Project: project}
}

// request makes a request held by the planning squad.
func (fx handoffFx) request(t *testing.T, title string) string {
	t.Helper()
	return dbfx.Issue(t, title, testutil.Cols{
		"workspace_id": fx.ID, "assignee_type": "squad", "assignee_id": fx.Planning,
	})
}

const testHandoffPacket = "## Planning handoff\nRequest: X\nOutcome: planned\nReadiness verdict: PASS (review issue T-1)\nDelivered and untouched: none"

func (fx handoffFx) packet(t *testing.T, issueID string) {
	t.Helper()
	dbfx.Comment(t, issueID, testHandoffPacket, testutil.Cols{"workspace_id": fx.ID, "author_type": "agent", "author_id": fx.Lead})
}

func (fx handoffFx) setRule(t *testing.T, mode string) *testutil.Response {
	t.Helper()
	req := fx.as(newRequest("PUT", "/api/squads/"+fx.Planning+"/handoff-rule",
		map[string]any{"target_squad_id": fx.Build, "mode": mode}), fx.AdminID)
	return testutil.Call(t, testHandler.SetSquadHandoffRule, withURLParams(req, "id", fx.Planning))
}

// leadAsks is the planning lead asking for the handoff of a request.
func (fx handoffFx) leadAsks(t *testing.T, issueID string) *testutil.Response {
	t.Helper()
	req := fx.asAgent(newRequest("POST", "/api/issues/"+issueID+"/handoff", nil), fx.Lead, fx.LeadTask)
	return testutil.Call(t, testHandler.RequestIssueHandoff, withURLParams(req, "id", issueID))
}

func (fx handoffFx) bossHandsOff(t *testing.T, issueID string) *testutil.Response {
	t.Helper()
	req := fx.as(newRequest("POST", "/api/issues/"+issueID+"/handoff", nil), fx.AdminID)
	return testutil.Call(t, testHandler.RequestIssueHandoff, withURLParams(req, "id", issueID))
}

type handoffOut struct {
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Handoff *struct {
		Status        string  `json:"status"`
		TargetIssueID *string `json:"target_issue_id"`
	} `json:"handoff"`
}

func countBuildRequests(t *testing.T, fx handoffFx) int {
	t.Helper()
	var n int
	dbfx.QueryRow(t, `SELECT count(*) FROM issue WHERE workspace_id = $1 AND project_id = $2`, fx.ID, fx.Project).Scan(&n)
	return n
}

// TestSquadHandoffRuleIsBossOnlyAndRefusesLoops: owners and admins set the rule; a squad cannot hand
// to itself or to a squad that hands back.
func TestSquadHandoffRuleIsBossOnlyAndRefusesLoops(t *testing.T) {
	fx := newHandoffFx(t)

	req := fx.as(newRequest("PUT", "/api/squads/"+fx.Planning+"/handoff-rule",
		map[string]any{"target_squad_id": fx.Build, "mode": "auto"}), fx.MemberID)
	testutil.Call(t, testHandler.SetSquadHandoffRule, withURLParams(req, "id", fx.Planning)).Want(http.StatusForbidden)

	fx.setRule(t, "auto").Want(http.StatusOK)
	fx.setRule(t, "sometimes").Want(http.StatusBadRequest)

	self := fx.as(newRequest("PUT", "/api/squads/"+fx.Planning+"/handoff-rule",
		map[string]any{"target_squad_id": fx.Planning, "mode": "auto"}), fx.AdminID)
	testutil.Call(t, testHandler.SetSquadHandoffRule, withURLParams(self, "id", fx.Planning)).Want(http.StatusBadRequest)

	back := fx.as(newRequest("PUT", "/api/squads/"+fx.Build+"/handoff-rule",
		map[string]any{"target_squad_id": fx.Planning, "mode": "auto"}), fx.AdminID)
	testutil.Call(t, testHandler.SetSquadHandoffRule, withURLParams(back, "id", fx.Build)).Want(http.StatusBadRequest)

	get := fx.as(newRequest("GET", "/api/squads/"+fx.Planning+"/handoff-rule", nil), fx.MemberID)
	var rule struct {
		Mode          string  `json:"mode"`
		TargetSquadID *string `json:"target_squad_id"`
	}
	testutil.Call(t, testHandler.GetSquadHandoffRule, withURLParams(get, "id", fx.Planning)).Want(http.StatusOK).JSON(&rule)
	if rule.Mode != "auto" || rule.TargetSquadID == nil || *rule.TargetSquadID != fx.Build {
		t.Fatalf("rule = %+v, want auto to the build squad", rule)
	}
}

// TestSquadHandoffOffHandsNothingOver: with no rule or with mode off, the lead is told off and nothing is created.
func TestSquadHandoffOffHandsNothingOver(t *testing.T) {
	fx := newHandoffFx(t)
	issue := fx.request(t, "Add feature X")
	fx.packet(t, issue)

	var out handoffOut
	fx.leadAsks(t, issue).Want(http.StatusOK).JSON(&out)
	if out.Status != "off" {
		t.Fatalf("no rule: status = %q, want off", out.Status)
	}
	fx.setRule(t, "off").Want(http.StatusOK)
	fx.leadAsks(t, issue).Want(http.StatusOK).JSON(&out)
	if out.Status != "off" || countBuildRequests(t, fx) != 0 {
		t.Fatalf("mode off: status = %q, build requests = %d, want off and none", out.Status, countBuildRequests(t, fx))
	}
}

// TestSquadHandoffAutoNeedsThePacketThenHandsOverOnce: auto refuses a request with no packet, then
// creates one request in the build squad's project, and asking again returns the same one.
func TestSquadHandoffAutoNeedsThePacketThenHandsOverOnce(t *testing.T) {
	fx := newHandoffFx(t)
	fx.setRule(t, "auto").Want(http.StatusOK)
	issue := fx.request(t, "Add feature X")

	resp := fx.leadAsks(t, issue).Want(http.StatusUnprocessableEntity)
	if !strings.Contains(resp.Text(), "handoff_packet_missing") {
		t.Fatalf("body = %s, want handoff_packet_missing", resp.Text())
	}

	fx.packet(t, issue)
	var first, second handoffOut
	fx.leadAsks(t, issue).Want(http.StatusOK).JSON(&first)
	if first.Status != "handed_off" || first.Handoff == nil || first.Handoff.TargetIssueID == nil {
		t.Fatalf("first answer = %+v, want handed_off with a target issue", first)
	}
	fx.leadAsks(t, issue).Want(http.StatusOK).JSON(&second)
	if second.Handoff == nil || second.Handoff.TargetIssueID == nil || *second.Handoff.TargetIssueID != *first.Handoff.TargetIssueID {
		t.Fatalf("second answer = %+v, want the same target issue %v", second, *first.Handoff.TargetIssueID)
	}
	if n := countBuildRequests(t, fx); n != 1 {
		t.Fatalf("build requests = %d, want exactly 1", n)
	}

	var assigneeType, assigneeID, origin, originID, title string
	dbfx.QueryRow(t, `SELECT assignee_type, assignee_id::text, origin_type, origin_id::text, title FROM issue WHERE id = $1`,
		*first.Handoff.TargetIssueID).Scan(&assigneeType, &assigneeID, &origin, &originID, &title)
	if assigneeType != "squad" || assigneeID != fx.Build || origin != "squad_request" || originID != issue {
		t.Fatalf("target issue = %s %s origin %s %s, want the build squad and a squad_request from the source", assigneeType, assigneeID, origin, originID)
	}
	if !strings.HasPrefix(title, "[Planning handoff] ") {
		t.Fatalf("target title = %q", title)
	}
	var packetInBody int
	dbfx.QueryRow(t, `SELECT count(*) FROM issue WHERE id = $1 AND description LIKE '%Readiness verdict: PASS%'`, *first.Handoff.TargetIssueID).Scan(&packetInBody)
	if packetInBody != 1 {
		t.Fatalf("the packet is not in the target request's description")
	}
	// The source request keeps its status and its assignee.
	var status, assignee string
	dbfx.QueryRow(t, `SELECT status, assignee_id::text FROM issue WHERE id = $1`, issue).Scan(&status, &assignee)
	if status != "todo" || assignee != fx.Planning {
		t.Fatalf("source issue = %s %s, want it untouched", status, assignee)
	}
}

// TestSquadHandoffAskWaitsForTheBoss: ask files a pending handoff; the boss approves it, or declines it.
func TestSquadHandoffAskWaitsForTheBoss(t *testing.T) {
	fx := newHandoffFx(t)
	fx.setRule(t, "ask").Want(http.StatusOK)

	approved := fx.request(t, "Add feature X")
	fx.packet(t, approved)
	var out handoffOut
	fx.leadAsks(t, approved).Want(http.StatusOK).JSON(&out)
	if out.Status != "pending" || countBuildRequests(t, fx) != 0 {
		t.Fatalf("ask: status = %q, build requests = %d, want pending and none", out.Status, countBuildRequests(t, fx))
	}
	fx.bossHandsOff(t, approved).Want(http.StatusOK).JSON(&out)
	if out.Status != "handed_off" || countBuildRequests(t, fx) != 1 {
		t.Fatalf("approve: status = %q, build requests = %d, want handed_off and 1", out.Status, countBuildRequests(t, fx))
	}

	declined := fx.request(t, "Add feature Y")
	fx.packet(t, declined)
	fx.leadAsks(t, declined).Want(http.StatusOK)
	req := fx.as(newRequest("POST", "/api/issues/"+declined+"/handoff/decline", map[string]any{"note": "not now"}), fx.AdminID)
	testutil.Call(t, testHandler.DeclineIssueHandoff, withURLParams(req, "id", declined)).Want(http.StatusOK).JSON(&out)
	if out.Status != "declined" || countBuildRequests(t, fx) != 1 {
		t.Fatalf("decline: status = %q, build requests = %d, want declined and still 1", out.Status, countBuildRequests(t, fx))
	}
	// A declined request is not handed off again when the lead asks once more.
	fx.leadAsks(t, declined).Want(http.StatusOK).JSON(&out)
	if out.Status != "declined" {
		t.Fatalf("asking again after a decline: status = %q, want declined", out.Status)
	}
}

// TestSquadHandoffBossHandsOffByHandWhenOff: with the rule off the boss can still hand a request over.
func TestSquadHandoffBossHandsOffByHandWhenOff(t *testing.T) {
	fx := newHandoffFx(t)
	fx.setRule(t, "off").Want(http.StatusOK)
	issue := fx.request(t, "Add feature X")

	var out handoffOut
	fx.bossHandsOff(t, issue).Want(http.StatusOK).JSON(&out)
	if out.Status != "handed_off" || countBuildRequests(t, fx) != 1 {
		t.Fatalf("status = %q, build requests = %d, want handed_off and 1", out.Status, countBuildRequests(t, fx))
	}
}

// TestSquadHandoffRefusesWhoIsNotTheLeaderOrBoss: a plain member and an agent outside the squad cannot ask.
func TestSquadHandoffRefusesWhoIsNotTheLeaderOrBoss(t *testing.T) {
	fx := newHandoffFx(t)
	fx.setRule(t, "auto").Want(http.StatusOK)
	issue := fx.request(t, "Add feature X")
	fx.packet(t, issue)

	req := fx.as(newRequest("POST", "/api/issues/"+issue+"/handoff", nil), fx.MemberID)
	testutil.Call(t, testHandler.RequestIssueHandoff, withURLParams(req, "id", issue)).Want(http.StatusForbidden)

	other := fx.asAgent(newRequest("POST", "/api/issues/"+issue+"/handoff", nil), fx.Leader, fx.Task)
	testutil.Call(t, testHandler.RequestIssueHandoff, withURLParams(other, "id", issue)).Want(http.StatusForbidden)
	if n := countBuildRequests(t, fx); n != 0 {
		t.Fatalf("build requests = %d, want none", n)
	}
}

// TestSquadHandoffNeedsAnOpenProjectAndDoesNotChain: the target squad must have a project, and a request
// that came from a handoff is not handed on.
func TestSquadHandoffNeedsAnOpenProjectAndDoesNotChain(t *testing.T) {
	fx := newHandoffFx(t)
	fx.setRule(t, "auto").Want(http.StatusOK)
	issue := fx.request(t, "Add feature X")
	fx.packet(t, issue)

	dbfx.Exec(t, `UPDATE project SET squad_id = NULL WHERE id = $1`, fx.Project)
	resp := fx.leadAsks(t, issue).Want(http.StatusUnprocessableEntity)
	if !strings.Contains(resp.Text(), "handoff_target_unavailable") {
		t.Fatalf("body = %s, want handoff_target_unavailable", resp.Text())
	}

	dbfx.Exec(t, `UPDATE project SET squad_id = $1 WHERE id = $2`, fx.Build, fx.Project)
	var out handoffOut
	fx.leadAsks(t, issue).Want(http.StatusOK).JSON(&out)
	if out.Status != "handed_off" {
		t.Fatalf("after assigning the project: status = %q, want handed_off", out.Status)
	}

	// The request the build squad received is itself held by a squad with no rule, but a handoff
	// created it, so even a manual handoff from it is refused.
	dbfx.Exec(t, `UPDATE issue SET assignee_id = $1 WHERE id = $2`, fx.Planning, *out.Handoff.TargetIssueID)
	req := fx.as(newRequest("POST", "/api/issues/"+*out.Handoff.TargetIssueID+"/handoff", nil), fx.AdminID)
	testutil.Call(t, testHandler.RequestIssueHandoff, withURLParams(req, "id", *out.Handoff.TargetIssueID)).Want(http.StatusConflict)
}
