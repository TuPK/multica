package handler

import (
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// An issue assigned to an agent that leads a live squad is held by that squad: the assignment is stored as
// the squad, so the squad's handoff and leader runs follow it.

func TestAssignIssueToLeaderAgentStoresSquad(t *testing.T) {
	fx := newIniWorkspace(t)
	squad, lead := planningSquad(t, fx, "Leader squad")
	issue := dbfx.Issue(t, "Assign to the leader", testutil.Cols{"workspace_id": fx.ID})

	req := fx.as(newRequest("PUT", "/api/issues/"+issue, map[string]any{
		"assignee_type": "agent", "assignee_id": lead,
	}), fx.AdminID)
	resp := testutil.Call(t, testHandler.UpdateIssue, withURLParams(req, "id", issue)).Want(200)

	got := resp.Map()
	if got["assignee_type"] != "squad" || got["assignee_id"] != squad {
		t.Fatalf("assignee = %v/%v, want squad/%s", got["assignee_type"], got["assignee_id"], squad)
	}
}

func TestCreateIssueAssignedToLeaderAgentStoresSquad(t *testing.T) {
	fx := newIniWorkspace(t)
	squad, lead := planningSquad(t, fx, "Leader squad")

	req := fx.as(newRequest("POST", "/api/issues", map[string]any{
		"title": "Created for the leader", "assignee_type": "agent", "assignee_id": lead,
	}), fx.AdminID)
	got := testutil.Call(t, testHandler.CreateIssue, req).Want(201).Map()
	if got["assignee_type"] != "squad" || got["assignee_id"] != squad {
		t.Fatalf("assignee = %v/%v, want squad/%s", got["assignee_type"], got["assignee_id"], squad)
	}
}

func TestAssignIssueToAgentWithoutSquadStaysAgent(t *testing.T) {
	fx := newIniWorkspace(t)
	plain := iniAgent(t, fx.ID, "Plain agent")
	issue := dbfx.Issue(t, "Personal work", testutil.Cols{"workspace_id": fx.ID})

	req := fx.as(newRequest("PUT", "/api/issues/"+issue, map[string]any{
		"assignee_type": "agent", "assignee_id": plain,
	}), fx.AdminID)
	got := testutil.Call(t, testHandler.UpdateIssue, withURLParams(req, "id", issue)).Want(200).Map()
	if got["assignee_type"] != "agent" || got["assignee_id"] != plain {
		t.Fatalf("assignee = %v/%v, want agent/%s", got["assignee_type"], got["assignee_id"], plain)
	}
}

func TestAssignIssueToLeaderOfArchivedSquadStaysAgent(t *testing.T) {
	fx := newIniWorkspace(t)
	squad, lead := planningSquad(t, fx, "Archived squad")
	dbfx.Exec(t, `UPDATE squad SET archived_at = now() WHERE id = $1`, squad)
	issue := dbfx.Issue(t, "Leader of an archived squad", testutil.Cols{"workspace_id": fx.ID})

	req := fx.as(newRequest("PUT", "/api/issues/"+issue, map[string]any{
		"assignee_type": "agent", "assignee_id": lead,
	}), fx.AdminID)
	got := testutil.Call(t, testHandler.UpdateIssue, withURLParams(req, "id", issue)).Want(200).Map()
	if got["assignee_type"] != "agent" || got["assignee_id"] != lead {
		t.Fatalf("assignee = %v/%v, want agent/%s", got["assignee_type"], got["assignee_id"], lead)
	}
}

// An agent leads at most one live squad. The partial unique index is the backstop; the handlers return 409 first.
func TestAgentCannotLeadTwoLiveSquads(t *testing.T) {
	fx := newIniWorkspace(t)
	_, lead := planningSquad(t, fx, "First squad")

	_, err := testPool.Exec(t.Context(), `
		INSERT INTO squad (workspace_id, name, leader_id, creator_id) VALUES ($1, 'Second squad', $2, $3)`,
		fx.ID, lead, fx.AdminID)
	if err == nil {
		t.Fatal("a second live squad for the same leader was accepted")
	}
	if !isUniqueViolationOn(err, squadLiveLeaderIndex) {
		t.Fatalf("insert error = %v, want a violation of %s", err, squadLiveLeaderIndex)
	}
}

func TestCreateSquadRejectsLeaderOfLiveSquad(t *testing.T) {
	fx := newIniWorkspace(t)
	_, lead := planningSquad(t, fx, "First squad")

	req := fx.as(newRequest("POST", "/api/squads", map[string]any{
		"name": "Second squad", "leader_id": lead,
	}), fx.AdminID)
	req = withURLParams(req, "workspaceId", fx.ID)
	testutil.Call(t, testHandler.CreateSquad, req).Want(409)
}

func TestBatchAssignIssuesToLeaderAgentStoresSquad(t *testing.T) {
	fx := newIniWorkspace(t)
	squad, lead := planningSquad(t, fx, "Leader squad")
	issue := dbfx.Issue(t, "Batch to the leader", testutil.Cols{"workspace_id": fx.ID})

	req := fx.as(newRequest("POST", "/api/issues/batch-update", map[string]any{
		"issue_ids": []string{issue},
		"updates":   map[string]any{"assignee_type": "agent", "assignee_id": lead},
	}), fx.AdminID)
	testutil.Call(t, testHandler.BatchUpdateIssues, req).Want(200)

	var assigneeType, assigneeID string
	dbfx.QueryRow(t, `SELECT assignee_type, assignee_id::text FROM issue WHERE id = $1`, issue).Scan(&assigneeType, &assigneeID)
	if assigneeType != "squad" || assigneeID != squad {
		t.Fatalf("assignee = %s/%s, want squad/%s", assigneeType, assigneeID, squad)
	}
}

// freshSquadLeader makes an agent that leads a squad of its own. An agent leads one live squad at a time, so
// each squad in a fixture needs a leader that no other squad has.
func freshSquadLeader(t *testing.T, label string) string {
	t.Helper()
	return createHandlerTestAgent(t, label+" leader "+uuid.NewString(), nil)
}

// freshLeaderIn makes an agent in ws that leads a squad of its own (see freshSquadLeader).
func freshLeaderIn(t *testing.T, ws, label string) string {
	t.Helper()
	runtime := dbfx.Runtime(t, label+" runtime "+uuid.NewString(), testutil.Cols{"workspace_id": ws})
	return dbfx.Agent(t, label+" leader "+uuid.NewString(), runtime, testutil.Cols{"workspace_id": ws})
}
