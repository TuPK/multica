package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Engine tests (S6 of docs/engineering/kitviet-workflow-plan.md). The agents are
// fake: a test closes a task the way an agent's CLI call would, by writing the
// task's status through the API, and finishes the run that call belongs to.

// tradeAgents are the six roles of the Trade flow.
type tradeAgents struct {
	Docs, Customs, Drafter, DocsGate, CustomsGate, Handover string
}

func newTradeAgents(t *testing.T) tradeAgents {
	t.Helper()
	first := seededReadyAgentID(t)
	next := func() string { return goldenLeadAgent(t, first) }
	return tradeAgents{Docs: first, Customs: next(), Drafter: next(), DocsGate: next(), CustomsGate: next(), Handover: next()}
}

const tradeStop1Message = "BLOCKED · stop 1 · thiếu căn cứ\n{requester} {request} dừng ở {step}.\n{missing}\n{tasks}"

// tradeFlowBody is the Trade flow of spec section 3.6, on custom statuses.
func tradeFlowBody(name string, a tradeAgents) map[string]any {
	agent := func(id string) map[string]any { return map[string]any{"type": "agent", "id": id} }
	cond := func(field, op string, value any) map[string]any {
		return map[string]any{"field": field, "op": op, "value": value}
	}
	tr := func(id string, when map[string]any, to string) map[string]any {
		return map[string]any{"id": id, "label": id, "when": when, "to_status_key": to}
	}
	blocked := tr("blocked", cond("tasks_outcome", "eq", "any_blocked"), "blocked")
	return map[string]any{
		"name": name,
		"flow_config": map[string]any{
			"version": 1,
			"switches": []map[string]any{
				{"key": "route", "type": "enum", "values": []string{"shipment_dossier"}, "default": "shipment_dossier"},
				{"key": "customs_lookup", "type": "bool", "default": false},
				{"key": "export", "type": "bool", "default": false},
			},
			"messages": map[string]any{
				"stop1":   tradeStop1Message,
				"stop2":   "IN REVIEW · stop 2 · trước khi ra khỏi workspace\n{requester} {request} chờ bạn duyệt.\n{tasks}",
				"stop3":   "BLOCKED · stop 3 · hết 2 vòng sửa\n{requester} {request} đã dùng {rework} vòng sửa.\n{tasks}",
				"stalled": "BLOCKED · treo\n{requester} {request} dừng ở {step}.\n{reason}",
			},
		},
		"steps": []map[string]any{
			{"status_key": "todo", "transitions": []map[string]any{
				tr("blocked", cond("outcome", "eq", "blocked"), "blocked"), tr("default", nil, "collect"),
			}},
			{"status_key": "collect",
				"tasks": []map[string]any{
					{"key": "docs", "title": "Read the documents", "kind": "work", "instructions": "Read.", "handler": agent(a.Docs)},
					{"key": "customs", "title": "Customs lookup", "kind": "work", "instructions": "Look up.", "handler": agent(a.Customs),
						"when": cond("switch.customs_lookup", "eq", true)},
				},
				"transitions": []map[string]any{blocked, tr("default", nil, "draft")}},
			{"status_key": "draft",
				"tasks": []map[string]any{
					{"key": "crosscheck", "title": "Cross-check", "kind": "work", "instructions": "Draft.", "handler": agent(a.Drafter), "requires": []string{"attachment"}},
				},
				"transitions": []map[string]any{blocked, tr("default", nil, "gate")}},
			{"status_key": "gate",
				"tasks": []map[string]any{
					{"key": "docs_gate", "title": "Documents gate", "kind": "gate", "instructions": "Check.", "handler": agent(a.DocsGate), "requires": []string{"outcome"}},
					{"key": "customs_gate", "title": "Customs gate", "kind": "gate", "instructions": "Check.", "handler": agent(a.CustomsGate), "requires": []string{"outcome"}},
				},
				"transitions": []map[string]any{
					blocked,
					tr("pass", cond("tasks_outcome", "eq", "all_approved"), "handover"),
					{"id": "redo", "label": "redo", "rework": true, "to_status_key": "draft", "when": map[string]any{"all": []map[string]any{
						cond("tasks_outcome", "eq", "any_rejected"), cond("rework_count", "lt", 2),
					}}},
					tr("default", nil, "blocked"),
				}},
			{"status_key": "handover",
				"tasks": []map[string]any{
					{"key": "handover", "title": "Handover", "kind": "gate", "instructions": "Hand over.", "handler": agent(a.Handover), "requires": []string{"outcome"}},
				},
				"transitions": []map[string]any{
					tr("pass", cond("tasks_outcome", "eq", "all_approved"), "in_review"), tr("default", nil, "blocked"),
				}},
			{"status_key": "in_review", "human_only": true,
				"tasks": []map[string]any{
					{"key": "export", "title": "Export", "kind": "work", "instructions": "Export.", "handler": agent(a.Drafter),
						"when": cond("switch.export", "eq", true)},
				}},
			{"status_key": "blocked"},
		},
	}
}

// tradeFlow is a squad on the Trade flow with its agents.
type tradeFlowFixture struct {
	Agents   tradeAgents
	Workflow IssueWorkflowResponse
	SquadID  string
}

// flowSquad is a squad whose lead is an agent of its own, with the agents of the
// flow as members: the roster the engine checks when it opens a task (E14).
func flowSquad(t *testing.T, name string, agentIDs ...string) string {
	t.Helper()
	lead := goldenLeadAgent(t, agentIDs[0])
	squadID := dbfx.Squad(t, name+" "+workflowTestSuffix(), lead)
	dbfx.SquadMember(t, squadID, "agent", lead, testutil.Cols{"role": "leader"})
	for _, id := range agentIDs {
		dbfx.SquadMember(t, squadID, "agent", id)
	}
	return squadID
}

func (a tradeAgents) all() []string {
	return []string{a.Docs, a.Customs, a.Drafter, a.DocsGate, a.CustomsGate, a.Handover}
}

func newTradeFlowFixture(t *testing.T) tradeFlowFixture {
	t.Helper()
	cleanupFlowRows(t)
	seedTestCatalog(t)
	for _, key := range []string{"collect", "draft", "gate", "handover"} {
		ensureGoldenStatus(t, key, "started")
	}
	agents := newTradeAgents(t)
	wf := createFlowWorkflowForTest(t, tradeFlowBody("Trade flow "+workflowTestSuffix(), agents))
	squadID := flowSquad(t, "Trade squad", agents.all()...)
	dbfx.Exec(t, `UPDATE issue_workflow SET squad_id = $2 WHERE id = $1`, wf.ID, squadID)
	dbfx.Exec(t, `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, squadID, wf.ID)
	return tradeFlowFixture{Agents: agents, Workflow: wf, SquadID: squadID}
}

func loadIssueRow(t *testing.T, id string) db.Issue {
	t.Helper()
	row, err := testHandler.Queries.GetIssue(context.Background(), parseUUID(id))
	if err != nil {
		t.Fatalf("load issue %s: %v", id, err)
	}
	return row
}

// newRequest makes a request for the squad and starts it on the flow.
func (f tradeFlowFixture) newRequest(t *testing.T) db.Issue {
	t.Helper()
	id := dbfx.Issue(t, "Trade request "+workflowTestSuffix(), testutil.Cols{
		"status": "todo", "assignee_type": "squad", "assignee_id": f.SquadID,
	})
	started, err := testHandler.EnsureFlowStarted(context.Background(), loadIssueRow(t, id))
	if err != nil {
		t.Fatalf("EnsureFlowStarted: %v", err)
	}
	if len(started.Flow) == 0 {
		t.Fatal("the request did not start on the flow")
	}
	return started
}

func (f tradeFlowFixture) plan(t *testing.T, request db.Issue, switches map[string]any) IssueFlowStateResponse {
	t.Helper()
	state, err := testHandler.PlanFlow(context.Background(), loadIssueRow(t, uuidToString(request.ID)), FlowPlanInput{Switches: switches})
	if err != nil {
		t.Fatalf("PlanFlow: %v", err)
	}
	return state
}

func advance(t *testing.T, request db.Issue) FlowAdvanceResponse {
	t.Helper()
	resp, err := testHandler.AdvanceFlow(context.Background(), loadIssueRow(t, uuidToString(request.ID)))
	if err != nil {
		t.Fatalf("AdvanceFlow: %v", err)
	}
	return resp
}

// tasksAt lists the task issues of a request at a step and round.
func tasksAt(t *testing.T, request db.Issue, step string, round int) map[string]string {
	t.Helper()
	out := map[string]string{}
	rows, err := testPool.Query(context.Background(), `SELECT flow_task, id::text FROM issue
		WHERE parent_issue_id = $1 AND flow_step = $2 AND flow_round = $3`, request.ID, step, round)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, id string
		if err := rows.Scan(&key, &id); err != nil {
			t.Fatal(err)
		}
		out[key] = id
	}
	return out
}

func countTasks(t *testing.T, request db.Issue) int {
	t.Helper()
	return dbfx.Count(t, `SELECT count(*) FROM issue WHERE parent_issue_id = $1 AND flow_task IS NOT NULL`, request.ID)
}

// endRuns finishes the runs of an issue, as the CLI exiting does after the agent's
// last command.
func endRuns(t *testing.T, issueID string) {
	t.Helper()
	dbfx.Exec(t, `UPDATE agent_task_queue SET status = 'completed', completed_at = now()
		WHERE issue_id = $1 AND status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')`, issueID)
}

// finishTask closes a task with an outcome the way its agent would.
func finishTask(t *testing.T, issueID, agentID, outcome string) {
	t.Helper()
	attachAs(t, issueID, agentID)
	body := map[string]any{"status": "done"}
	if outcome != "" {
		body["outcome"] = outcome
	}
	updateIssueForTest(t, issueID, body).Want(http.StatusOK)
	endRuns(t, issueID)
}

// blockTask reports that a task cannot go on.
func blockTask(t *testing.T, issueID string) {
	t.Helper()
	updateIssueForTest(t, issueID, map[string]any{"status": "blocked", "outcome": "blocked"}).Want(http.StatusOK)
	endRuns(t, issueID)
}

func requestStatus(t *testing.T, request db.Issue) string {
	t.Helper()
	status, _, _ := issueStatusAndAssignee(t, uuidToString(request.ID))
	return status
}

func actionKinds(resp FlowAdvanceResponse) []string {
	out := make([]string, len(resp.Actions))
	for i, a := range resp.Actions {
		out[i] = a.Kind
	}
	return out
}

func TestAdvance_OpensParallelTasks(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)

	state := f.plan(t, request, map[string]any{"customs_lookup": true})
	if state.Step != "collect" || requestStatus(t, request) != "collect" {
		t.Fatalf("after the plan the request is at %q (status %q), want collect", state.Step, requestStatus(t, request))
	}
	collect := tasksAt(t, request, "collect", 0)
	if len(collect) != 2 || collect["docs"] == "" || collect["customs"] == "" {
		t.Fatalf("collect tasks = %v, want docs and customs", collect)
	}
	// A request waits while one task of the step is open.
	finishTask(t, collect["docs"], f.Agents.Docs, "")
	resp := advance(t, request)
	if resp.State.Step != "collect" || len(resp.Pending) != 1 || resp.Pending[0].Task != "customs" {
		t.Fatalf("with one task open: step %q, pending %+v; want collect waiting for customs", resp.State.Step, resp.Pending)
	}
	if kinds := actionKinds(resp); len(kinds) != 1 || kinds[0] != "noop" {
		t.Fatalf("actions = %v, want [noop]", kinds)
	}
	finishTask(t, collect["customs"], f.Agents.Customs, "")
	resp = advance(t, request)
	if resp.State.Step != "draft" {
		t.Fatalf("after both tasks the request is at %q, want draft", resp.State.Step)
	}
	if draft := tasksAt(t, request, "draft", 0); len(draft) != 1 || draft["crosscheck"] == "" {
		t.Fatalf("draft tasks = %v, want crosscheck", draft)
	}
}

func TestAdvance_BarrierWaitsForAllTasks(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, nil)
	docs := tasksAt(t, request, "collect", 0)["docs"]

	// The status is written but the run has not ended: the barrier holds.
	attachAs(t, docs, f.Agents.Docs)
	updateIssueForTest(t, docs, map[string]any{"status": "done"}).Want(http.StatusOK)
	dbfx.Exec(t, `UPDATE agent_task_queue SET status = 'running' WHERE issue_id = $1 AND status IN ('queued', 'dispatched')`, docs)
	if dbfx.Count(t, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1 AND status = 'running'`, docs) == 0 {
		runtimeID := ""
		dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, f.Agents.Docs).Scan(&runtimeID)
		dbfx.Task(t, f.Agents.Docs, testutil.Cols{"runtime_id": runtimeID, "issue_id": docs, "status": "running", "started_at": testutil.Raw("now()")})
	}
	if resp := advance(t, request); resp.State.Step != "collect" {
		t.Fatalf("the request left collect (%q) while the run of docs was still going", resp.State.Step)
	}
	endRuns(t, docs)
	if resp := advance(t, request); resp.State.Step != "draft" {
		t.Fatalf("the request is at %q after the run ended, want draft", resp.State.Step)
	}
}

func TestAdvance_OptionalTaskSkippedAndOpenedBySwitch(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)

	f.plan(t, request, nil)
	if collect := tasksAt(t, request, "collect", 0); len(collect) != 1 || collect["docs"] == "" {
		t.Fatalf("with the switch off the collect tasks are %v, want only docs", collect)
	}
	// A person who manages the squad turns the lookup on at collect.
	if _, err := testHandler.SetFlowSwitch(context.Background(), loadIssueRow(t, uuidToString(request.ID)), "customs_lookup", true, true); err != nil {
		t.Fatalf("SetFlowSwitch: %v", err)
	}
	resp := advance(t, request)
	if collect := tasksAt(t, request, "collect", 0); len(collect) != 2 || collect["customs"] == "" {
		t.Fatalf("after the switch the collect tasks are %v, want docs and customs", collect)
	}
	if resp.Actions[0].Kind != "opened_tasks" || len(resp.Actions[0].Tasks) != 1 || resp.Actions[0].Tasks[0] != "customs" {
		t.Fatalf("actions = %+v, want customs opened", resp.Actions)
	}
	// The lead cannot change a switch once the plan is done.
	_, err := testHandler.SetFlowSwitch(context.Background(), loadIssueRow(t, uuidToString(request.ID)), "export", true, false)
	var rej *flowRejection
	if !asFlowRejection(err, &rej) || rej.Code != "flow_switch_locked" {
		t.Fatalf("SetFlowSwitch by the lead after the plan = %v, want flow_switch_locked", err)
	}
	_, err = testHandler.SetFlowSwitch(context.Background(), loadIssueRow(t, uuidToString(request.ID)), "route", "nowhere", true)
	if !asFlowRejection(err, &rej) || rej.Code != "flow_switch_invalid" {
		t.Fatalf("SetFlowSwitch with a value outside the enum = %v, want flow_switch_invalid", err)
	}
}

// driveToGate plans the request, finishes collect and draft, and returns the
// gate task issues of the round.
func (f tradeFlowFixture) driveToGate(t *testing.T, request db.Issue) map[string]string {
	t.Helper()
	f.plan(t, request, nil)
	finishTask(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")
	advance(t, request)
	return f.finishDraft(t, request, 0)
}

// finishDraft closes the draft task of a round and returns the gates the
// advance opens.
func (f tradeFlowFixture) finishDraft(t *testing.T, request db.Issue, round int) map[string]string {
	t.Helper()
	finishTask(t, tasksAt(t, request, "draft", round)["crosscheck"], f.Agents.Drafter, "")
	resp := advance(t, request)
	if resp.State.Step != "gate" {
		t.Fatalf("after the draft of round %d the request is at %q, want gate", round, resp.State.Step)
	}
	gates := tasksAt(t, request, "gate", round)
	if len(gates) != 2 {
		t.Fatalf("gate tasks of round %d = %v, want both gates", round, gates)
	}
	return gates
}

func TestAdvance_ReworkTwiceThenStop3(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	gates := f.driveToGate(t, request)

	for round := 0; round < 3; round++ {
		// Only one gate rejects; the other approves: a rejection still sends the draft back.
		finishTask(t, gates["docs_gate"], f.Agents.DocsGate, "approved")
		finishTask(t, gates["customs_gate"], f.Agents.CustomsGate, "rejected")
		resp := advance(t, request)
		if round < 2 {
			if resp.State.Step != "draft" || resp.State.Rework != round+1 {
				t.Fatalf("round %d: the request is at %q with rework %d, want draft with rework %d", round, resp.State.Step, resp.State.Rework, round+1)
			}
			gates = f.finishDraft(t, request, round+1)
			continue
		}
		if resp.State.Step != "blocked" || stopOf(resp.State) != issueworkflow.Stop3 || resp.State.Rework != 2 {
			t.Fatalf("after the third rejection: step %q, stop %v, rework %d; want blocked, stop 3, rework 2", resp.State.Step, stopOf(resp.State), resp.State.Rework)
		}
	}
	// Every round opened every gate again: 2 gates over 3 rounds.
	if n := dbfx.Count(t, `SELECT count(*) FROM issue WHERE parent_issue_id = $1 AND flow_step = 'gate'`, request.ID); n != 6 {
		t.Fatalf("gate sub-issues = %d, want 6", n)
	}
	for round := 0; round < 3; round++ {
		if got := len(tasksAt(t, request, "gate", round)); got != 2 {
			t.Fatalf("gate tasks of round %d = %d, want 2", round, got)
		}
		if got := len(tasksAt(t, request, "draft", round)); got != 1 {
			t.Fatalf("draft tasks of round %d = %d, want 1", round, got)
		}
	}
	if requestStatus(t, request) != "blocked" {
		t.Fatalf("request status = %q, want blocked", requestStatus(t, request))
	}
	var comment string
	dbfx.QueryRow(t, `SELECT content FROM comment WHERE issue_id = $1 ORDER BY created_at DESC LIMIT 1`, request.ID).Scan(&comment)
	if !containsAll(comment, "BLOCKED · stop 3 · hết 2 vòng sửa", "mention://member/"+testUserID, "2 vòng sửa") {
		t.Fatalf("stop 3 comment = %q", comment)
	}
}

func TestAdvance_BlockedOutcomeStop1(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, map[string]any{"customs_lookup": true})
	collect := tasksAt(t, request, "collect", 0)

	// The docs reader reports that an invoice is missing while customs still runs.
	blockTask(t, collect["docs"])
	resp := advance(t, request)
	if resp.State.Step != "blocked" || stopOf(resp.State) != issueworkflow.Stop1 {
		t.Fatalf("step %q, stop %v; want blocked, stop 1", resp.State.Step, stopOf(resp.State))
	}
	if requestStatus(t, request) != "blocked" {
		t.Fatalf("request status = %q, want blocked", requestStatus(t, request))
	}
	// The other task is not cancelled.
	if status, _, _ := issueStatusAndAssignee(t, collect["customs"]); status == "cancelled" {
		t.Fatal("stop 1 cancelled the task that was still running")
	}
	var comment string
	dbfx.QueryRow(t, `SELECT content FROM comment WHERE issue_id = $1 ORDER BY created_at DESC LIMIT 1`, request.ID).Scan(&comment)
	if !containsAll(comment, "BLOCKED · stop 1 · thiếu căn cứ", "mention://member/"+testUserID, "docs") {
		t.Fatalf("stop 1 comment = %q", comment)
	}
	if kinds := actionKinds(resp); len(kinds) == 0 || kinds[len(kinds)-1] != "stopped" {
		t.Fatalf("actions = %v, want the last to be stopped", kinds)
	}
}

func TestAdvance_Idempotent(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, nil)

	first := advance(t, request)
	issues := dbfx.Count(t, `SELECT count(*) FROM issue WHERE workspace_id = $1 AND parent_issue_id = $2`, testWorkspaceID, request.ID)
	comments := dbfx.Count(t, `SELECT count(*) FROM comment WHERE issue_id = $1`, request.ID)
	second := advance(t, request)
	if kinds := actionKinds(second); len(kinds) != 1 || kinds[0] != "noop" {
		t.Fatalf("second advance actions = %v, want [noop] (first was %v)", kinds, actionKinds(first))
	}
	if got := dbfx.Count(t, `SELECT count(*) FROM issue WHERE workspace_id = $1 AND parent_issue_id = $2`, testWorkspaceID, request.ID); got != issues {
		t.Fatalf("sub-issues %d -> %d: a repeated advance opened issues", issues, got)
	}
	if got := dbfx.Count(t, `SELECT count(*) FROM comment WHERE issue_id = $1`, request.ID); got != comments {
		t.Fatalf("comments %d -> %d: a repeated advance commented", comments, got)
	}
}

func TestAdvance_CompletesPartialCreation(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, map[string]any{"customs_lookup": true})
	customs := tasksAt(t, request, "collect", 0)["customs"]

	dbfx.Exec(t, `DELETE FROM issue WHERE id = $1`, customs)
	resp := advance(t, request)
	if again := tasksAt(t, request, "collect", 0); len(again) != 2 || again["customs"] == "" || again["customs"] == customs {
		t.Fatalf("collect tasks after the repair = %v, want customs opened again", again)
	}
	if resp.Actions[0].Kind != "opened_tasks" || len(resp.Actions[0].Tasks) != 1 || resp.Actions[0].Tasks[0] != "customs" {
		t.Fatalf("actions = %+v, want only customs opened", resp.Actions)
	}
}

func TestAdvance_ConcurrentCallsCreateOneSet(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	// Put the request at collect with no task yet, as if a crash fell between the phases.
	dbfx.Exec(t, `UPDATE issue SET flow = flow || '{"step":"collect"}'::jsonb, status = 'collect' WHERE id = $1`, request.ID)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := testHandler.AdvanceFlow(context.Background(), loadIssueRow(t, uuidToString(request.ID))); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("a concurrent advance failed: %v", err)
	}
	if got := countTasks(t, request); got != 1 {
		t.Fatalf("tasks after 8 concurrent advances = %d, want 1", got)
	}
}

// Two decisions made from the same state cannot both win: the write is
// conditional on the step and seq the decision started from.
func TestAdvance_ConcurrentWritersLoseOnFlowCondition(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	ctx := context.Background()
	write := func(step string) error {
		_, err := testHandler.Queries.SetIssueFlow(ctx, db.SetIssueFlowParams{
			Flow:         []byte(fmt.Sprintf(`{"workflow_id":"%s","step":"%s","rework":0}`, f.Workflow.ID, step)),
			ID:           request.ID,
			WorkspaceID:  request.WorkspaceID,
			ExpectedStep: "todo",
			ExpectedSeq:  0,
		})
		return err
	}
	if err := write("collect"); err != nil {
		t.Fatalf("the first writer lost: %v", err)
	}
	if err := write("blocked"); err != pgx.ErrNoRows {
		t.Fatalf("the second writer, from the same state, got %v, want no rows", err)
	}
}

func TestAdvance_AgentUnavailableBlocks(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	// Docs is the workspace's seeded agent that every other test uses: put it back when this test ends.
	dbfx.Exec(t, `UPDATE agent SET archived_at = now() WHERE id = $1`, f.Agents.Docs)
	t.Cleanup(func() { dbfx.Exec(t, `UPDATE agent SET archived_at = NULL WHERE id = $1`, f.Agents.Docs) })

	state := f.plan(t, request, nil)
	if state.Step != "blocked" || stopOf(state) != issueworkflow.StopStalled {
		t.Fatalf("step %q, stop %v; want blocked, stalled", state.Step, stopOf(state))
	}
	if got := countTasks(t, request); got != 0 {
		t.Fatalf("tasks opened for an unavailable agent: %d", got)
	}
	var comment string
	dbfx.QueryRow(t, `SELECT content FROM comment WHERE issue_id = $1 ORDER BY created_at DESC LIMIT 1`, request.ID).Scan(&comment)
	if !containsAll(comment, "BLOCKED · treo", "docs") {
		t.Fatalf("stalled comment = %q, want it to name the task", comment)
	}
}

func TestAdvance_ManualMoveSyncs(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, nil)
	docs := tasksAt(t, request, "collect", 0)["docs"]

	// A person who manages the squad moves the request to draft by hand.
	dbfx.Exec(t, `UPDATE issue SET status = 'draft' WHERE id = $1`, request.ID)
	resp := advance(t, request)
	if resp.State.Step != "draft" {
		t.Fatalf("flow step = %q after a manual move to draft", resp.State.Step)
	}
	if status, _, _ := issueStatusAndAssignee(t, docs); status != "cancelled" {
		t.Fatalf("the open task of the step left has status %q, want cancelled", status)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = 'flow_manual_move'`, request.ID); n != 1 {
		t.Fatalf("flow_manual_move entries = %d, want 1", n)
	}
	if draft := tasksAt(t, request, "draft", 0); len(draft) != 1 {
		t.Fatalf("draft tasks = %v, want the draft task opened", draft)
	}
	// Moving it to done ends the flow and cancels what is left.
	dbfx.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, request.ID)
	advance(t, request)
	var closed *string
	dbfx.QueryRow(t, `SELECT flow->>'closed_at' FROM issue WHERE id = $1`, request.ID).Scan(&closed)
	if closed == nil {
		t.Fatal("a request moved to done has no closed_at")
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM issue WHERE parent_issue_id = $1 AND flow_task IS NOT NULL AND status NOT IN ('done', 'cancelled')`, request.ID); n != 0 {
		t.Fatalf("%d tasks are still open under a closed request", n)
	}
}

func TestEnsureFlowStarted_Idempotent(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	again, err := testHandler.EnsureFlowStarted(context.Background(), loadIssueRow(t, uuidToString(request.ID)))
	if err != nil {
		t.Fatal(err)
	}
	if string(again.Flow) != string(loadIssueRow(t, uuidToString(request.ID)).Flow) {
		t.Fatalf("a second start changed the flow: %s", again.Flow)
	}
	state, ok := flowRequestState(again)
	if !ok || state.Step != "todo" || state.WorkflowID != f.Workflow.ID || state.Switches["route"] != "shipment_dossier" {
		t.Fatalf("started state = %+v", state)
	}
}

func TestEnsureFlowStarted_SkipsChildAndNonSquadIssues(t *testing.T) {
	f := newTradeFlowFixture(t)
	parent := f.newRequest(t)
	cases := map[string]testutil.Cols{
		"a child issue":       {"assignee_type": "squad", "assignee_id": f.SquadID, "parent_issue_id": parent.ID},
		"an agent's issue":    {"assignee_type": "agent", "assignee_id": f.Agents.Docs},
		"an unassigned issue": {},
		"a finished request":  {"assignee_type": "squad", "assignee_id": f.SquadID, "status": "done"},
	}
	for name, cols := range cases {
		t.Run(name, func(t *testing.T) {
			id := dbfx.Issue(t, "not a request "+workflowTestSuffix(), cols)
			got, err := testHandler.EnsureFlowStarted(context.Background(), loadIssueRow(t, id))
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Flow) != 0 {
				t.Fatalf("flow started on %s: %s", name, got.Flow)
			}
		})
	}
	// A squad whose active workflow is not a flow starts nothing.
	plainSquad := dbfx.Squad(t, "Plain squad "+workflowTestSuffix(), f.Agents.Docs)
	plain := insertSquadWorkflow(t, testWorkspaceID, plainSquad, "Plain library "+workflowTestSuffix(), "backlog", "in_progress", "blocked")
	dbfx.Exec(t, `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, plainSquad, plain)
	id := dbfx.Issue(t, "plain squad request "+workflowTestSuffix(), testutil.Cols{"assignee_type": "squad", "assignee_id": plainSquad})
	if got, _ := testHandler.EnsureFlowStarted(context.Background(), loadIssueRow(t, id)); len(got.Flow) != 0 {
		t.Fatalf("flow started on a squad with an ordinary workflow: %s", got.Flow)
	}
}

func TestEnsureFlowStarted_AfterHTTPCreate(t *testing.T) {
	f := newTradeFlowFixture(t)
	var created IssueResponse
	testutil.Call(t, testHandler.CreateIssue, newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title": "Trade request over HTTP " + workflowTestSuffix(), "assignee_type": "squad", "assignee_id": f.SquadID,
	})).Want(http.StatusCreated).JSON(&created)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1 OR parent_issue_id = $1`, created.ID)
	})

	if created.Flow == nil || created.Flow.Role != issueworkflow.RoleRequest || created.Flow.Step != "todo" {
		t.Fatalf("created issue flow = %+v, want a request at todo", created.Flow)
	}
	if state, ok := flowRequestState(loadIssueRow(t, created.ID)); !ok || state.WorkflowID != f.Workflow.ID {
		t.Fatalf("stored state = %+v (ok %v)", state, ok)
	}
}

func TestEnsureFlowStarted_WithoutProject(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	if request.ProjectID.Valid {
		t.Fatal("the fixture request has a project")
	}
	state := f.plan(t, request, nil)
	if state.Step != "collect" || len(tasksAt(t, request, "collect", 0)) != 1 {
		t.Fatalf("a request with no project did not run: step %q", state.Step)
	}
}

func TestResume_Stop3ResetsRework(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	gates := f.driveToGate(t, request)
	for round := 0; round < 3; round++ {
		finishTask(t, gates["docs_gate"], f.Agents.DocsGate, "rejected")
		finishTask(t, gates["customs_gate"], f.Agents.CustomsGate, "rejected")
		advance(t, request)
		if round < 2 {
			gates = f.finishDraft(t, request, round+1)
		}
	}
	if st := advance(t, request).State; stopOf(st) != issueworkflow.Stop3 {
		t.Fatalf("the request is not at stop 3: %+v", st)
	}

	state, err := testHandler.ResumeFlow(context.Background(), loadIssueRow(t, uuidToString(request.ID)), FlowResumeInput{})
	if err != nil {
		t.Fatalf("ResumeFlow: %v", err)
	}
	if state.Step != "gate" || state.Rework != 0 || stopOf(state) != issueworkflow.StopNone {
		t.Fatalf("after resume: step %q, rework %d, stop %v; want gate, 0, none", state.Step, state.Rework, stopOf(state))
	}
	// Every gate of the step opened again, in a round the old ones do not use.
	var round int32
	for _, task := range state.Tasks {
		if task.Round > int(round) {
			round = int32(task.Round)
		}
	}
	if round < 3 {
		t.Fatalf("highest task round = %d, want a new round above 2", round)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM issue WHERE parent_issue_id = $1 AND flow_step = 'gate'`, request.ID); n != 8 {
		t.Fatalf("gate sub-issues = %d, want 8 (6 before, 2 new)", n)
	}
	if requestStatus(t, request) != "gate" {
		t.Fatalf("request status = %q, want gate", requestStatus(t, request))
	}
}

func TestResume_Stop1ReopensOnlyBlockedTasks(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, map[string]any{"customs_lookup": true})
	collect := tasksAt(t, request, "collect", 0)
	finishTask(t, collect["customs"], f.Agents.Customs, "")
	blockTask(t, collect["docs"])
	if st := advance(t, request).State; stopOf(st) != issueworkflow.Stop1 {
		t.Fatalf("the request is not at stop 1: %+v", st)
	}

	state, err := testHandler.ResumeFlow(context.Background(), loadIssueRow(t, uuidToString(request.ID)), FlowResumeInput{})
	if err != nil {
		t.Fatalf("ResumeFlow: %v", err)
	}
	if state.Step != "collect" || stopOf(state) != issueworkflow.StopNone {
		t.Fatalf("after resume: step %q, stop %v; want collect, none", state.Step, stopOf(state))
	}
	if status, _, _ := issueStatusAndAssignee(t, collect["docs"]); status != "todo" {
		t.Fatalf("the blocked task has status %q after resume, want todo", status)
	}
	if issueOutcome(t, collect["docs"]) != "" {
		t.Fatal("the reopened task kept its outcome")
	}
	if status, _, _ := issueStatusAndAssignee(t, collect["customs"]); status != "done" {
		t.Fatalf("the finished task has status %q after resume, want done", status)
	}
	if got := len(tasksAt(t, request, "collect", 0)); got != 2 {
		t.Fatalf("collect tasks = %d, want the same 2", got)
	}
}

func TestDetach_RestoresPlainIssues(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, nil)
	docs := tasksAt(t, request, "collect", 0)["docs"]

	if err := testHandler.DetachFlow(context.Background(), loadIssueRow(t, uuidToString(request.ID)), false); err != nil {
		t.Fatalf("DetachFlow: %v", err)
	}
	row := loadIssueRow(t, uuidToString(request.ID))
	if len(row.Flow) != 0 {
		t.Fatalf("the request still has a flow: %s", row.Flow)
	}
	task := loadIssueRow(t, docs)
	if task.FlowTask.Valid || task.FlowStep.Valid || task.FlowRound.Valid {
		t.Fatalf("the task kept its flow columns: %+v", task)
	}
	if task.Status == "cancelled" {
		t.Fatal("a detach without cancel_tasks cancelled a task")
	}
	// The task is an ordinary issue again: it closes with no attachment and no outcome.
	updateIssueForTest(t, docs, map[string]any{"status": "done"}).Want(http.StatusOK)
	if _, err := testHandler.AdvanceFlow(context.Background(), row); err != errFlowNotManaged {
		t.Fatalf("AdvanceFlow on a detached request = %v, want flow_not_managed", err)
	}

	// With cancel_tasks the open tasks are cancelled.
	other := f.newRequest(t)
	f.plan(t, other, nil)
	openTask := tasksAt(t, other, "collect", 0)["docs"]
	if err := testHandler.DetachFlow(context.Background(), loadIssueRow(t, uuidToString(other.ID)), true); err != nil {
		t.Fatalf("DetachFlow(cancel): %v", err)
	}
	if status, _, _ := issueStatusAndAssignee(t, openTask); status != "cancelled" {
		t.Fatalf("task status after detach with cancel = %q, want cancelled", status)
	}
}

func asFlowRejection(err error, target **flowRejection) bool {
	rej, ok := err.(*flowRejection)
	if ok {
		*target = rej
	}
	return ok
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
