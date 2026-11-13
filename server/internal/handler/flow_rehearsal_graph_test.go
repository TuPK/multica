package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Rehearsals of workflows of schema 2 (plan P13, spec 12.1 K6). The workflows are
// the FlowDoc fixtures of internal/issueworkflow/testdata/flows, with their roles
// bound to fake agents and members, written as rows the way a published version
// would be. The agents are fake: a test closes a task the way the agent's CLI call
// would, as the rehearsals of schema 1 do.

// graphRig is a squad running a workflow of schema 2.
type graphRig struct {
	t       *testing.T
	doc     map[string]any
	row     db.IssueWorkflow
	squadID string
	// agents and members are the people bound to each role, in pool order.
	agents  map[string][]string
	members map[string][]string
	// as acts as a bound member.
	as map[string]func(*http.Request) *http.Request
}

// graphRigOpts adjusts a fixture before it is written.
type graphRigOpts struct {
	// Mutate changes the document (a map) before the roles are bound.
	Mutate func(doc map[string]any)
	// Pool sets how many people a role is bound to (default: the pool of the role).
	Pool map[string]int
}

func loadFlowFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "issueworkflow", "testdata", "flows", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func asMaps(v any) []map[string]any {
	list, _ := v.([]any)
	out := make([]map[string]any, len(list))
	for i, e := range list {
		out[i], _ = e.(map[string]any)
	}
	return out
}

func newGraphRig(t *testing.T, fixture string, opts graphRigOpts) *graphRig {
	t.Helper()
	return newGraphRigDoc(t, fixture, loadFlowFixture(t, fixture), opts)
}

// newGraphRigDoc writes a workflow row for a document (a fixture, or one the test builds).
func newGraphRigDoc(t *testing.T, fixture string, doc map[string]any, opts graphRigOpts) *graphRig {
	t.Helper()
	cleanupFlowRows(t)
	seedTestCatalog(t)
	if opts.Mutate != nil {
		opts.Mutate(doc)
	}
	rig := &graphRig{t: t, doc: doc, agents: map[string][]string{}, members: map[string][]string{}, as: map[string]func(*http.Request) *http.Request{}}
	first := seededReadyAgentID(t)
	var everyone []string
	roles := asMaps(doc["roles"])
	for _, role := range roles {
		key, _ := role["key"].(string)
		kind, _ := role["kind"].(string)
		pool := 1
		if n, ok := role["pool"].(float64); ok && n > 0 {
			pool = int(n)
		}
		if n, ok := opts.Pool[key]; ok {
			pool = n
		}
		switch kind {
		case issueworkflow.RoleKindAgent:
			var ids []string
			for i := 0; i < pool; i++ {
				ids = append(ids, goldenLeadAgent(t, first))
			}
			rig.agents[key] = ids
			everyone = append(everyone, ids...)
			role["bind"] = map[string]any{"agent_ids": ids}
		case issueworkflow.RoleKindMember:
			var ids []string
			for i := 0; i < pool; i++ {
				id, as := addWorkflowTestMember(t, fmt.Sprintf("%s%d", key, i))
				ids = append(ids, id)
				rig.as[id] = as
			}
			rig.members[key] = ids
			role["bind"] = map[string]any{"member_ids": ids}
		}
	}
	// Every step shows its own board status, which the workspace has to know.
	statuses := []string{}
	seen := map[string]bool{}
	note := func(key string) {
		if !seen[key] {
			seen[key] = true
			statuses = append(statuses, key)
		}
	}
	for _, step := range asMaps(doc["steps"]) {
		status, _ := step["status"].(map[string]any)
		key, _ := status["key"].(string)
		if key == "" {
			key, _ = step["key"].(string)
			ensureGoldenStatus(t, key, "started")
			step["status"] = map[string]any{"key": key}
		}
		note(key)
	}
	for _, term := range asMaps(doc["terminals"]) {
		key, _ := term["status"].(string)
		note(key)
	}
	note("blocked")
	flowConfig, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	steps, err := issueworkflow.EncodeKeys(statuses)
	if err != nil {
		t.Fatal(err)
	}
	if _, issues := issueworkflow.ParseFlowDoc(flowConfig); len(issues) > 0 {
		t.Fatalf("the fixture %s is not a flow document: %v", fixture, issues)
	}
	if len(everyone) == 0 {
		everyone = []string{first}
	}
	rig.squadID = flowSquad(t, "Graph squad "+fixture, everyone...)
	row, err := testHandler.Queries.CreateIssueWorkflow(context.Background(), db.CreateIssueWorkflowParams{
		WorkspaceID:      parseUUID(testWorkspaceID),
		Name:             "Graph " + fixture + " " + workflowTestSuffix(),
		InitialStatusKey: issueworkflow.StartStatus,
		Steps:            steps,
		OwnerID:          parseUUID(testUserID),
		SquadID:          parseUUID(rig.squadID),
		FlowConfig:       flowConfig,
	})
	if err != nil {
		t.Fatalf("write the workflow row: %v", err)
	}
	rig.row = row
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `UPDATE squad SET active_workflow_id = NULL WHERE active_workflow_id = $1`, row.ID)
		testPool.Exec(context.Background(), `DELETE FROM issue_workflow WHERE id = $1`, row.ID)
	})
	dbfx.Exec(t, `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, rig.squadID, row.ID)
	return rig
}

func (r *graphRig) newRequest() db.Issue {
	r.t.Helper()
	id := dbfx.Issue(r.t, "Graph request "+workflowTestSuffix(), testutil.Cols{
		"status": "todo", "assignee_type": "squad", "assignee_id": r.squadID,
	})
	started, err := testHandler.EnsureFlowStarted(context.Background(), loadIssueRow(r.t, id))
	if err != nil || len(started.Flow) == 0 {
		r.t.Fatalf("EnsureFlowStarted: flow %q, err %v", started.Flow, err)
	}
	return started
}

// complete completes the lead step of the request with these variables.
func (r *graphRig) complete(request db.Issue, vars map[string]any) IssueFlowStateResponse {
	r.t.Helper()
	state, err := testHandler.CompleteFlowStep(context.Background(), loadIssueRow(r.t, uuidToString(request.ID)), "", FlowCompleteInput{Vars: vars})
	if err != nil {
		r.t.Fatalf("CompleteFlowStep: %v", err)
	}
	return state
}

func (r *graphRig) state(request db.Issue) IssueFlowStateResponse {
	r.t.Helper()
	state, err := testHandler.flowStateResponse(context.Background(), loadIssueRow(r.t, uuidToString(request.ID)), issueworkflow.RoleRequest)
	if err != nil {
		r.t.Fatalf("flowStateResponse: %v", err)
	}
	return state
}

func (r *graphRig) advance(request db.Issue) IssueFlowStateResponse {
	r.t.Helper()
	return advance(r.t, request).State
}

// rows are the task rows of the request at a step in an entry (round), by row key.
func (r *graphRig) rows(request db.Issue, step string, round int) map[string]string {
	r.t.Helper()
	return tasksAt(r.t, request, step, round)
}

// assigneeOf is the agent or member a task issue is given to.
func (r *graphRig) assigneeOf(issueID string) (typ, id string) {
	r.t.Helper()
	var t, i *string
	dbfx.QueryRow(r.t, `SELECT assignee_type, assignee_id::text FROM issue WHERE id = $1`, issueID).Scan(&t, &i)
	if t != nil {
		typ = *t
	}
	if i != nil {
		id = *i
	}
	return typ, id
}

// finish closes a task the way its agent would: it attaches a file as the agent
// and closes the issue, with the verdict when the task has one.
func (r *graphRig) finish(issueID, outcome string) {
	r.t.Helper()
	typ, id := r.assigneeOf(issueID)
	if typ == "agent" {
		finishTask(r.t, issueID, id, outcome)
		return
	}
	body := map[string]any{"status": "done"}
	if outcome != "" {
		body["outcome"] = outcome
	}
	updateIssueForTest(r.t, issueID, body).Want(200)
}

// finishAll closes every row of the step entry with the outcome the function gives.
func (r *graphRig) finishAll(request db.Issue, step string, round int, outcome func(key string) string) {
	r.t.Helper()
	keys := []string{}
	rows := r.rows(request, step, round)
	for k := range rows {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		r.finish(rows[k], outcome(k))
	}
}

func always(outcome string) func(string) string { return func(string) string { return outcome } }

func (r *graphRig) subIssues(request db.Issue) int { return countTasks(r.t, request) }

func TestRehearsalGraph_TradeV2MatchesTradeV1Counts(t *testing.T) {
	// The schema 1 rehearsal opens 1 collect + 1 draft + 2 gates + 1 handover.
	v1 := newRehearsalFixture(t)
	v1Request := v1.newRequest(t)
	gates := v1.driveToGate(t, v1Request)
	handover := v1.toHandover(t, v1Request, gates)
	finishTask(t, handover, v1.Agents.Handover, "approved")
	advance(t, v1Request)
	want := countTasks(t, v1Request)

	rig := newGraphRig(t, "trade", graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	rig.finishAll(request, "collect", 0, always(""))
	rig.advance(request)
	rig.finishAll(request, "draft", 0, always(""))
	if st := rig.advance(request); st.Step != "gate" {
		t.Fatalf("after the draft the request is at %q, want gate", st.Step)
	}
	rig.finishAll(request, "gate", 0, always("approved"))
	if st := rig.advance(request); st.Step != "handover" {
		t.Fatalf("after both gates the request is at %q, want handover", st.Step)
	}
	rig.finishAll(request, "handover", 0, always("approved"))
	st := rig.advance(request)
	if st.Step != "release" || requestStatus(t, request) != "in_review" {
		t.Fatalf("step %q, status %q; want release at in_review", st.Step, requestStatus(t, request))
	}
	if got := rig.subIssues(request); got != want {
		t.Fatalf("schema 2 opened %d sub-issues, schema 1 opened %d", got, want)
	}
	// The request waits at the manual release step: it left the plan and four more steps, and the
	// agents' work is unreviewed until a person chooses the exit.
	if st.Schema != 2 || len(st.Trail) != 5 || !st.Unreviewed {
		t.Fatalf("schema %d, trail %d, unreviewed %v; want schema 2, five hops, unreviewed", st.Schema, len(st.Trail), st.Unreviewed)
	}
}

// memberActor is a bound member as the engine sees someone who calls its API.
func memberActor(t *testing.T, userID string) flowActor {
	t.Helper()
	member, err := testHandler.getWorkspaceMember(context.Background(), userID, testWorkspaceID)
	if err != nil {
		t.Fatalf("load the member %s: %v", userID, err)
	}
	return flowActor{Type: "member", ID: userID, Member: &member}
}

// exit takes a manual exit as the actor.
func (r *graphRig) exit(request db.Issue, edge string, actor flowActor) (IssueFlowStateResponse, error) {
	r.t.Helper()
	return testHandler.ExitFlow(context.Background(), loadIssueRow(r.t, uuidToString(request.ID)), edge, actor)
}

func (r *graphRig) mustExit(request db.Issue, edge string, actor flowActor) IssueFlowStateResponse {
	r.t.Helper()
	st, err := r.exit(request, edge, actor)
	if err != nil {
		r.t.Fatalf("ExitFlow %s: %v", edge, err)
	}
	return st
}

func (r *graphRig) wantClosedAt(request db.Issue, status string) {
	r.t.Helper()
	if requestStatus(r.t, request) != status || dbfx.Count(r.t, `SELECT count(*) FROM issue WHERE id = $1 AND flow->>'closed_at' IS NOT NULL`, request.ID) != 1 {
		r.t.Fatalf("the request is at status %q, closed %v; want it closed at %q", requestStatus(r.t, request), false, status)
	}
}

func TestRehearsalGraph_ResearchNestedLoopsAndVarMax(t *testing.T) {
	rig := newGraphRig(t, "research", graphRigOpts{})
	request := rig.newRequest()

	// The number of rounds is a variable clamped by its declaration (max 5).
	_, err := testHandler.CompleteFlowStep(context.Background(), loadIssueRow(t, uuidToString(request.ID)), "", FlowCompleteInput{Vars: map[string]any{"max_rounds": 9}})
	if rej, ok := err.(*flowRejection); !ok || rej.Code != "flow_var_invalid" {
		t.Fatalf("max_rounds 9 = %v, want flow_var_invalid", err)
	}
	rig.complete(request, map[string]any{"max_rounds": 2})

	// The decision refines the query as long as the loop has room: twice, then it goes on.
	for round := 0; round < 3; round++ {
		rig.finishAll(request, "search", round, always(""))
		rig.advance(request)
		rig.finishAll(request, "screen", round, always(""))
		rig.advance(request)
		rig.finishAll(request, "decide", round, always("refine_query"))
		st := rig.advance(request)
		want := "search"
		if round == 2 {
			want = "synthesize"
		}
		if st.Step != want {
			t.Fatalf("decision %d sent the request to %q, want %q (loops %v)", round, st.Step, want, st.Loops)
		}
	}
	// The inner loop: the audit asks for changes once, then passes with conditions.
	rig.finishAll(request, "synthesize", 0, always(""))
	rig.advance(request)
	rig.finishAll(request, "audit", 0, always("request_changes"))
	if st := rig.advance(request); st.Step != "synthesize" || st.Loops["fixes"] != 1 || st.Loops["rounds"] != 2 {
		t.Fatalf("after the audit: step %q, loops %v; want synthesize with fixes 1 and rounds 2", st.Step, st.Loops)
	}
	rig.finishAll(request, "synthesize", 1, always(""))
	rig.advance(request)
	rig.finishAll(request, "audit", 1, always("pass_with_conditions"))
	rig.advance(request)
	rig.finishAll(request, "review", 0, always("approved"))
	st := rig.advance(request)
	if st.Step != "approve" || st.Activations["search"] != 3 || st.Activations["synthesize"] != 2 {
		t.Fatalf("step %q, activations %v; want approve after 3 searches and 2 syntheses", st.Step, st.Activations)
	}
	// The requester approves; the request leaves.
	rig.mustExit(request, "approve_go", memberActor(t, testUserID))
	rig.wantClosedAt(request, "done")
}

func TestRehearsalGraph_MarketInstancesAndAfter(t *testing.T) {
	rig := newGraphRig(t, "market", graphRigOpts{Mutate: func(doc map[string]any) {
		collect := asMaps(doc["steps"])[1]
		collect["tasks"] = append(collect["tasks"].([]any), map[string]any{
			"key": "index_sources", "title": "Index the sources", "kind": "work", "role": "critic",
			"after": []any{"collect_source"}, "instructions": "Index.",
		})
	}})
	request := rig.newRequest()
	rig.complete(request, map[string]any{"sources": []string{"alpha", "beta", "gamma"}, "depth": "full"})

	// One row per source, each to a different collector; the indexing waits for all of them.
	rows := rig.rows(request, "collect", 0)
	if len(rows) != 3 || rows["collect_source~1"] == "" || rows["collect_source~3"] == "" {
		t.Fatalf("collect rows = %v, want collect_source~1..3 and no index yet", rows)
	}
	holders := map[string]bool{}
	for _, id := range rows {
		_, who := rig.assigneeOf(id)
		holders[who] = true
	}
	if len(holders) != 3 {
		t.Fatalf("the 3 rows went to %d people, want 3 different ones", len(holders))
	}
	if !containsAll(taskDescription(t, rows["collect_source~2"]), "beta", "part 2 of 3") {
		t.Fatalf("the description of the second row does not carry its item:\n%s", taskDescription(t, rows["collect_source~2"]))
	}
	rig.finish(rows["collect_source~1"], "")
	rig.finish(rows["collect_source~2"], "")
	rig.advance(request)
	if got := rig.rows(request, "collect", 0); len(got) != 3 {
		t.Fatalf("with one source open the rows are %v, want the index to wait", got)
	}
	rig.finish(rows["collect_source~3"], "")
	rig.advance(request)
	index := rig.rows(request, "collect", 0)["index_sources"]
	if index == "" {
		t.Fatalf("the indexing did not open after the last source: %v", rig.rows(request, "collect", 0))
	}
	rig.finish(index, "")
	if st := rig.advance(request); st.Step != "verify" {
		t.Fatalf("step %q, want verify", st.Step)
	}

	// One source is not verified: the whole check goes back to collecting, and each source
	// goes to the person who collected it before.
	before := map[string]string{}
	for key, id := range rig.rows(request, "collect", 0) {
		_, before[key] = rig.assigneeOf(id)
	}
	checks := rig.rows(request, "verify", 0)
	rig.finish(checks["source_check~1"], "verified")
	rig.finish(checks["source_check~2"], "unverified")
	rig.finish(checks["source_check~3"], "verified")
	if st := rig.advance(request); st.Step != "collect" || st.Loops["recheck"] != 1 {
		t.Fatalf("step %q, loops %v; want collect again with recheck 1", st.Step, st.Loops)
	}
	again := rig.rows(request, "collect", 1)
	for key, who := range before {
		if again[key] == "" {
			continue // the indexing waits for the sources
		}
		if _, holder := rig.assigneeOf(again[key]); holder != who {
			t.Fatalf("%s went to %s the second time, was %s", key, holder, who)
		}
	}
	for _, id := range again {
		rig.finish(id, "")
	}
	rig.advance(request)
	rig.finish(rig.rows(request, "collect", 1)["index_sources"], "")
	rig.advance(request)
	rig.finishAll(request, "verify", 1, always("verified"))
	rig.advance(request)
	rig.finishAll(request, "synthesize", 0, always(""))
	rig.advance(request)
	rig.finishAll(request, "critique", 0, always("approved"))
	if st := rig.advance(request); st.Step != "edit" {
		t.Fatalf("step %q, want edit", st.Step)
	}
	rig.mustExit(request, "edit_go", memberActor(t, rig.members["editor"][0]))
	rig.wantClosedAt(request, "done")
}

func TestRehearsalGraph_LegalQuorumEarlyFailAndSpecificBackStep(t *testing.T) {
	reviews := func(rig *graphRig, request db.Issue, round int, outcome string, n int) {
		t.Helper()
		rows := rig.rows(request, "review", round)
		for i := 1; i <= n; i++ {
			rig.finish(rows[fmt.Sprintf("reviewer~%d", i)], outcome)
		}
	}
	runToReview := func(rig *graphRig, request db.Issue, intakeRound, clausesRound int) {
		t.Helper()
		rig.finishAll(request, "intake", intakeRound, always(""))
		rig.advance(request)
		rig.finishAll(request, "redraft_clauses", clausesRound, always(""))
		rig.advance(request)
	}

	t.Run("three rejections fail the quorum at once and the clauses are redrafted", func(t *testing.T) {
		rig := newGraphRig(t, "legal", graphRigOpts{})
		request := rig.newRequest()
		rig.complete(request, nil)
		runToReview(rig, request, 0, 0)
		rows := rig.rows(request, "review", 0)
		holders := map[string]bool{}
		for _, id := range rows {
			typ, who := rig.assigneeOf(id)
			holders[who] = typ == "member"
		}
		if len(rows) != 5 || len(holders) != 5 {
			t.Fatalf("review rows %v held by %d people, want 5 rows for 5 different members", rows, len(holders))
		}
		reviews(rig, request, 0, "reject", 3)
		st := rig.advance(request)
		// Pass 0 and two open cannot reach 3: the join fails with two reviews still open, and goes
		// back to the clauses, not to the start.
		if st.Step != "redraft_clauses" || st.Loops["clauses"] != 1 || st.Loops["cutback"] != 0 {
			t.Fatalf("step %q, loops %v; want redraft_clauses with clauses 1", st.Step, st.Loops)
		}
		superseded := dbfx.Count(t, `SELECT count(*) FROM issue WHERE parent_issue_id = $1 AND flow_step = 'review' AND flow_round = 0 AND outcome = 'superseded' AND status = 'cancelled'`, request.ID)
		if superseded != 2 {
			t.Fatalf("%d open reviews were superseded, want 2", superseded)
		}
		// The second review: three approvals pass at once and the other two are superseded.
		rig.finishAll(request, "redraft_clauses", 1, always(""))
		rig.advance(request)
		reviews(rig, request, 1, "approve", 3)
		if st := rig.advance(request); st.Step != "counsel_signoff" {
			t.Fatalf("step %q, want counsel_signoff", st.Step)
		}
		rig.mustExit(request, "signoff_go", memberActor(t, rig.members["counsel"][0]))
		rig.wantClosedAt(request, "done")
	})

	t.Run("a critical severity goes back to the intake", func(t *testing.T) {
		rig := newGraphRig(t, "legal", graphRigOpts{})
		request := rig.newRequest()
		rig.complete(request, map[string]any{"severity": "critical"})
		runToReview(rig, request, 0, 0)
		reviews(rig, request, 0, "reject", 3)
		if st := rig.advance(request); st.Step != "intake" || st.Loops["cutback"] != 1 {
			t.Fatalf("step %q, loops %v; want intake with cutback 1", st.Step, st.Loops)
		}
	})
}

func taskDescription(t *testing.T, issueID string) string {
	t.Helper()
	var d *string
	dbfx.QueryRow(t, `SELECT description FROM issue WHERE id = $1`, issueID).Scan(&d)
	if d == nil {
		return ""
	}
	return *d
}

// agentOnIssue calls a handler as an agent that has a running run on the issue.
func agentOnIssue(t *testing.T, issueID, agentID, step string, h http.HandlerFunc, method, path string, body map[string]any, params ...string) *testutil.Response {
	t.Helper()
	runID := goldenRun(t, issueID, agentID, step)
	req := withURLParams(newRequest(method, path, body), append([]string{"id", issueID}, params...)...)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", runID)
	return testutil.Call(t, h, req)
}

func (r *graphRig) otherAgent(than string) string {
	r.t.Helper()
	for _, ids := range r.agents {
		for _, id := range ids {
			if id != than {
				return id
			}
		}
	}
	// A flow with a single agent: any other agent of the workspace will do.
	return goldenLeadAgent(r.t, than)
}

// The agent that misbehaves on the first task of each of the four kinds: the server
// refuses it for the same reasons whatever the flow says (spec section 5, E1 to E8).
func TestRehearsalGraph_MisbehavingAgentMatrix(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		// start brings the request to the step the target is at.
		start func(rig *graphRig, request db.Issue)
		step  string
		key   string
		// files: the task requires a file, so E6 and E7 apply.
		files bool
	}{
		{"trade", "trade", func(rig *graphRig, request db.Issue) {
			rig.complete(request, nil)
			rig.finishAll(request, "collect", 0, always(""))
			rig.advance(request)
		}, "draft", "crosscheck", true},
		{"research", "research", func(rig *graphRig, request db.Issue) { rig.complete(request, nil) }, "search", "scout", true},
		{"market", "market", func(rig *graphRig, request db.Issue) {
			rig.complete(request, map[string]any{"sources": []string{"a", "b"}})
		}, "collect", "collect_source~1", true},
		{"legal", "legal", func(rig *graphRig, request db.Issue) { rig.complete(request, nil) }, "intake", "intake_draft", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newGraphRig(t, tc.fixture, graphRigOpts{})
			request := rig.newRequest()
			tc.start(rig, request)
			target := rig.rows(request, tc.step, 0)[tc.key]
			if target == "" {
				t.Fatalf("no row %s/%s: %v", tc.step, tc.key, rig.rows(request, tc.step, 0))
			}
			_, assignee := rig.assigneeOf(target)
			other := rig.otherAgent(assignee)
			put := func(who, step string, body map[string]any) *testutil.Response {
				return agentOnIssue(t, target, who, step, testHandler.UpdateIssue, "PUT", "/api/issues/"+target, body)
			}

			// E1: another agent does not close the task.
			wantFlowRejection(t, put(other, tc.step, map[string]any{"status": "done"}), 403, "flow_task_not_assignee")
			// E2: the agent does not hand the task to someone else.
			wantFlowRejection(t, put(assignee, tc.step, map[string]any{"assignee_type": "agent", "assignee_id": other}), 409, "flow_task_assignee_locked")
			// E3: the agent does not cancel it.
			wantFlowRejection(t, put(assignee, tc.step, map[string]any{"status": "cancelled"}), 409, "flow_task_cancel_forbidden")
			// E5: a verdict the task does not have.
			wantFlowRejection(t, put(assignee, tc.step, map[string]any{"status": "done", "outcome": "bogus_verdict"}), 422, "flow_outcome_invalid")
			// E5: the verdict of the old kinds is not the verdict of this set either.
			wantFlowRejection(t, put(assignee, tc.step, map[string]any{"status": "done", "outcome": "approved"}), 422, "flow_outcome_invalid")
			// E6: a task that requires a file does not close without one.
			if tc.files {
				wantFlowRejection(t, put(assignee, tc.step, map[string]any{"status": "done"}), 422, "flow_attachment_required")
			}
			// E8: the agent does not move the request.
			wantFlowRejection(t, agentOnIssue(t, uuidToString(request.ID), other, "plan", testHandler.UpdateIssue, "PUT",
				"/api/issues/"+uuidToString(request.ID), map[string]any{"status": "in_progress"}), 409, "flow_managed_request")

			// E4: once the task is closed its verdict stands.
			attachAs(t, target, assignee)
			put(assignee, tc.step, map[string]any{"status": "done"}).Want(200)
			wantFlowRejection(t, put(assignee, tc.step, map[string]any{"status": "done", "outcome": "blocked"}), 409, "flow_task_closed")
			if status, _, _ := issueStatusAndAssignee(t, target); status != "done" {
				t.Fatalf("the task is %q, want done", status)
			}
		})
	}

	// E7: a task that requires a file and was cancelled by a person opens again, so the
	// step cannot pass without the file.
	for _, tc := range cases {
		if !tc.files {
			continue
		}
		t.Run(tc.name+" reopens a cancelled task", func(t *testing.T) {
			rig := newGraphRig(t, tc.fixture, graphRigOpts{})
			request := rig.newRequest()
			tc.start(rig, request)
			target := rig.rows(request, tc.step, 0)[tc.key]
			updateIssueForTest(t, target, map[string]any{"status": "cancelled"}).Want(200)
			resp := advance(t, request)
			if resp.Actions[0].Kind != "reopened_tasks" {
				t.Fatalf("actions = %+v, want the cancelled task reopened", resp.Actions)
			}
			if status, _, _ := issueStatusAndAssignee(t, target); status != "todo" {
				t.Fatalf("the task is %q, want todo", status)
			}
		})
	}
}

func TestRehearsalGraph_CapReachedStalls(t *testing.T) {
	rig := newGraphRig(t, "trade", graphRigOpts{Mutate: func(doc map[string]any) {
		doc["policy"] = map[string]any{"max_task_issues": 3}
	}})
	request := rig.newRequest()
	rig.complete(request, map[string]any{"customs_lookup": true})
	rig.finishAll(request, "collect", 0, always(""))
	rig.advance(request)
	rig.finishAll(request, "draft", 0, always(""))
	st := rig.advance(request)
	// Three task issues are open or closed; the two gates would be the fourth and the fifth.
	if st.StopInfo == nil || st.StopInfo.Code != issueworkflow.DocStopStalled || requestStatus(t, request) != "blocked" {
		t.Fatalf("stop %+v, status %q; want stalled at blocked", st.StopInfo, requestStatus(t, request))
	}
	if n := len(rig.rows(request, "gate", 0)); n != 0 {
		t.Fatalf("%d gates opened past the cap", n)
	}
	if !containsAll(lastComment(t, request), "BLOCKED · stalled", "limit of steps or task issues") {
		t.Fatalf("stall comment = %q", lastComment(t, request))
	}
	logged := flowTransitions(t, request)
	if last := logged[len(logged)-1]; last["reason"] != issueworkflow.ReasonFlowCapReached {
		t.Fatalf("the stop was logged with %v, want the reason flow_cap_reached", last)
	}
}

func TestRehearsalGraph_PoolTooSmallStalls(t *testing.T) {
	// Three sources need three different collectors; the pool has two.
	rig := newGraphRig(t, "market", graphRigOpts{Pool: map[string]int{"collector": 2}})
	request := rig.newRequest()
	rig.complete(request, map[string]any{"sources": []string{"a", "b", "c"}})
	st := rig.state(request)
	if st.StopInfo == nil || st.StopInfo.Code != issueworkflow.DocStopStalled {
		t.Fatalf("stop %+v, want stalled", st.StopInfo)
	}
	if n := rig.subIssues(request); n != 0 {
		t.Fatalf("%d tasks opened for a pool too small, want none", n)
	}
	if !containsAll(lastComment(t, request), "BLOCKED · stalled", "No one in the pool") {
		t.Fatalf("stall comment = %q", lastComment(t, request))
	}
	logged := flowTransitions(t, request)
	if last := logged[len(logged)-1]; last["stop"] != issueworkflow.DocStopStalled {
		t.Fatalf("the stop was logged as %v", last)
	}
}

func TestRehearsalGraph_ReleaseWithoutCheckpointStalls(t *testing.T) {
	// The handover goes straight to done: no person looks at what the agents made.
	rig := newGraphRig(t, "trade", graphRigOpts{Mutate: func(doc map[string]any) {
		for _, step := range asMaps(doc["steps"]) {
			if step["key"] != "handover" {
				continue
			}
			for _, edge := range asMaps(step["edges"]) {
				if edge["id"] == "handover_ok" {
					edge["to"] = "done"
				}
			}
		}
	}})
	request := rig.newRequest()
	rig.complete(request, nil)
	rig.finishAll(request, "collect", 0, always(""))
	rig.advance(request)
	rig.finishAll(request, "draft", 0, always(""))
	rig.advance(request)
	rig.finishAll(request, "gate", 0, always("approved"))
	rig.advance(request)
	rig.finishAll(request, "handover", 0, always("approved"))
	st := rig.advance(request)
	if st.StopInfo == nil || st.StopInfo.Code != issueworkflow.DocStopStalled || requestStatus(t, request) != "blocked" {
		t.Fatalf("stop %+v, status %q; want stalled at blocked", st.StopInfo, requestStatus(t, request))
	}
	logged := flowTransitions(t, request)
	if last := logged[len(logged)-1]; last["reason"] != issueworkflow.ReasonReleaseWithoutCheckpoint {
		t.Fatalf("the stop was logged with %v, want release_without_checkpoint", last)
	}
}

func TestRehearsalGraph_RunningRequestKeepsPinnedVersion(t *testing.T) {
	first := newGraphRig(t, "trade", graphRigOpts{})
	running := first.newRequest()
	first.complete(running, nil)

	// A second version of the workflow sends the collected request straight to release.
	doc := first.doc
	for _, step := range asMaps(doc["steps"]) {
		if step["key"] == "collect" {
			for _, edge := range asMaps(step["edges"]) {
				if edge["id"] == "collect_ok" {
					edge["to"] = "release"
				}
			}
		}
	}
	raw, _ := json.Marshal(doc)
	second, err := testHandler.Queries.CreateIssueWorkflow(context.Background(), db.CreateIssueWorkflowParams{
		WorkspaceID: first.row.WorkspaceID, Name: "Graph trade v2 " + workflowTestSuffix(), InitialStatusKey: issueworkflow.StartStatus,
		Steps: first.row.Steps, OwnerID: first.row.OwnerID, SquadID: first.row.SquadID, FlowConfig: raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, first.squadID, first.row.ID)
		testPool.Exec(context.Background(), `DELETE FROM issue_workflow WHERE id = $1`, second.ID)
	})
	dbfx.Exec(t, `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, first.squadID, second.ID)

	fresh := first.newRequest()
	first.complete(fresh, nil)
	first.finishAll(fresh, "collect", 0, always(""))
	if st := first.advance(fresh); st.Step != "release" || st.WorkflowID != uuidToString(second.ID) {
		t.Fatalf("the new request is at %q on %s, want release on the new version", st.Step, st.WorkflowID)
	}
	first.finishAll(running, "collect", 0, always(""))
	if st := first.advance(running); st.Step != "draft" || st.WorkflowID != uuidToString(first.row.ID) {
		t.Fatalf("the running request is at %q on %s, want draft on the version it started on", st.Step, st.WorkflowID)
	}
}

func TestRehearsalGraph_SweepRemindsHumanTaskThenStalls(t *testing.T) {
	rig := newGraphRig(t, "legal", graphRigOpts{Mutate: func(doc map[string]any) {
		doc["policy"] = map[string]any{"human_remind_minutes": 10, "human_stall_minutes": 20}
	}})
	withFeatureFlag(t, testHandler, "flow_runner", true)
	setFlowAuto(t, uuidToString(rig.row.ID), true)
	request := rig.newRequest()
	rig.complete(request, nil)
	rig.finishAll(request, "intake", 0, always(""))
	rig.advance(request)
	rig.finishAll(request, "redraft_clauses", 0, always(""))
	rig.advance(request)
	rows := rig.rows(request, "review", 0)
	if len(rows) != 5 {
		t.Fatalf("review rows = %v", rows)
	}
	// The rows are aged into the past, out of reach of the cleanup by creation time.
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE parent_issue_id = $1`, request.ID)
	})
	age := func(minutes int) {
		dbfx.Exec(t, `UPDATE issue SET created_at = now() - make_interval(mins => $2) WHERE parent_issue_id = $1 AND flow_step = 'review'`, request.ID, minutes)
	}

	sweep(t)
	if n := dbfx.Count(t, `SELECT count(*) FROM activity_log WHERE action = 'flow_human_reminded' AND issue_id IN (SELECT id FROM issue WHERE parent_issue_id = $1)`, request.ID); n != 0 {
		t.Fatalf("%d reminders before the wait ran out", n)
	}
	age(15)
	sweep(t)
	sweep(t)
	if n := dbfx.Count(t, `SELECT count(*) FROM activity_log WHERE action = 'flow_human_reminded' AND issue_id IN (SELECT id FROM issue WHERE parent_issue_id = $1)`, request.ID); n != 5 {
		t.Fatalf("%d reminders after two sweeps, want one for each of the 5 tasks", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM comment WHERE type = 'system' AND content LIKE '%mention://member/%' AND issue_id = $1`, rows["reviewer~1"]); n != 1 {
		t.Fatalf("%d reminders mention the reviewer, want 1", n)
	}
	if st := rig.state(request); st.StopInfo != nil {
		t.Fatalf("the request stalled after the reminder: %+v", st.StopInfo)
	}
	age(25)
	sweep(t)
	st := rig.state(request)
	if st.StopInfo == nil || st.StopInfo.Code != issueworkflow.DocStopStalled || requestStatus(t, request) != "blocked" {
		t.Fatalf("stop %+v, status %q; want stalled at blocked after the wait", st.StopInfo, requestStatus(t, request))
	}
}

func TestRehearsalGraph_CrossReviewTwoAgents(t *testing.T) {
	rig := newGraphRig(t, "crossreview", graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	rig.finishAll(request, "build", 0, always(""))
	rig.advance(request)
	// A checks what B wrote; B checks what A wrote.
	gates := rig.rows(request, "review", 0)
	ana, ben := rig.agents["ana"][0], rig.agents["ben"][0]
	if _, who := rig.assigneeOf(gates["ana_checks_ben"]); who != ana {
		t.Fatalf("ana_checks_ben went to %s, want ana", who)
	}
	if _, who := rig.assigneeOf(gates["ben_checks_ana"]); who != ben {
		t.Fatalf("ben_checks_ana went to %s, want ben", who)
	}
	rig.finishAll(request, "review", 0, always("approved"))
	rig.advance(request)
	rig.wantClosedAt(request, "done")

	// A flow in which one agent writes and then judges its own work stalls instead of opening the gate.
	own := newGraphRig(t, "crossreview", graphRigOpts{Mutate: func(doc map[string]any) {
		for _, step := range asMaps(doc["steps"]) {
			for _, task := range asMaps(step["tasks"]) {
				if task["key"] == "ana_checks_ben" {
					task["role"] = "ben"
				}
			}
		}
	}})
	selfRequest := own.newRequest()
	own.complete(selfRequest, nil)
	own.finishAll(selfRequest, "build", 0, always(""))
	own.advance(selfRequest)
	if st := own.state(selfRequest); st.StopInfo == nil || st.StopInfo.Code != issueworkflow.DocStopStalled {
		t.Fatalf("stop %+v, want stalled for an agent that judges its own work", st.StopInfo)
	}
	if n := len(own.rows(selfRequest, "review", 0)); n != 0 {
		t.Fatalf("%d gates opened, want none", n)
	}
}

func TestRehearsalGraph_PlannerTaskSetsListThenFanOut(t *testing.T) {
	// The list may hold 8 items; the step fans out over at most 6.
	rig := newGraphRig(t, "planner_fanout", graphRigOpts{Mutate: func(doc map[string]any) {
		for _, v := range asMaps(doc["vars"]) {
			v["max_items"] = 10
		}
	}})
	request := rig.newRequest()
	rig.complete(request, nil)
	plan := rig.rows(request, "planning", 0)["make_plan"]
	planner := rig.agents["planner"][0]
	setTopics := func(who string, items []string) *testutil.Response {
		return agentOnIssue(t, plan, who, "planning", testHandler.SetIssueFlowVar, "PUT", "/api/issues/"+plan+"/flow/vars/topics",
			map[string]any{"value": items}, "key", "topics")
	}
	// Not another variable, and not before the task sets the list: it does not close without it.
	wantFlowRejection(t, agentOnIssue(t, plan, planner, "planning", testHandler.SetIssueFlowVar, "PUT", "/api/issues/"+plan+"/flow/vars/route",
		map[string]any{"value": "x"}, "key", "route"), 403, "flow_var_forbidden")
	attachAs(t, plan, planner)
	wantFlowRejection(t, agentOnIssue(t, plan, planner, "planning", testHandler.UpdateIssue, "PUT", "/api/issues/"+plan, map[string]any{"status": "done"}), 422, "flow_vars_required")

	topics := []string{"t1", "t2", "t3", "t4", "t5", "t6", "t7", "t8"}
	setTopics(planner, topics).Want(200)
	agentOnIssue(t, plan, planner, "planning", testHandler.UpdateIssue, "PUT", "/api/issues/"+plan, map[string]any{"status": "done"}).Want(200)
	endRuns(t, plan)
	st0 := rig.advance(request)

	rows := rig.rows(request, "fanout", 0)
	if len(rows) == 0 {
		t.Logf("state after planning: step %s stop %+v vars %v comment %q", st0.Step, st0.StopInfo, st0.Vars, lastComment(t, request))
	}
	if len(rows) != 6 || rows["research_topic~6"] == "" || rows["research_topic~7"] != "" {
		t.Fatalf("fan-out rows = %v, want research_topic~1..6", rows)
	}
	workers := map[string]bool{}
	for _, id := range rows {
		_, who := rig.assigneeOf(id)
		workers[who] = true
	}
	if len(workers) != 6 {
		t.Fatalf("the 6 rows went to %d workers, want 6 different ones", len(workers))
	}
	// The list cannot change while the fan-out runs, not even for a manager.
	res := testutil.Call(t, testHandler.SetIssueFlowVar, withURLParams(newRequest("PUT", "/api/issues/"+uuidToString(request.ID)+"/flow/vars/topics", map[string]any{"value": []string{"x"}}),
		"id", uuidToString(request.ID), "key", "topics"))
	wantFlowRejection(t, res, 409, "flow_var_in_use")

	rig.finishAll(request, "fanout", 0, always(""))
	rig.advance(request)
	rig.finishAll(request, "check", 0, always("approved"))
	rig.advance(request)
	rig.wantClosedAt(request, "done")
}

func TestRehearsalGraph_ReworkOnlyTheFailedBranch(t *testing.T) {
	rig := newGraphRig(t, "patterns/p12_redo_failed_branch", graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	if got := rig.rows(request, "build", 0); len(got) != 2 {
		t.Fatalf("first build rows = %v, want fe and be", got)
	}
	rig.finishAll(request, "build", 0, always(""))
	rig.advance(request)
	reviews := rig.rows(request, "review", 0)
	rig.finish(reviews["fe_review"], "rejected")
	rig.finish(reviews["be_review"], "approved")
	if st := rig.advance(request); st.Step != "build" || st.Loops["redo"] != 1 {
		t.Fatalf("step %q, loops %v; want build with redo 1", st.Step, st.Loops)
	}
	redo := rig.rows(request, "build", 1)
	if len(redo) != 1 || redo["fe"] == "" {
		t.Fatalf("the second build opened %v, want only the failed branch fe", redo)
	}
}

// branchDoc is a flow with two ways through the work: the variable picks one.
func branchDoc(requireOther bool) map[string]any {
	agent := func(key string) map[string]any { return map[string]any{"key": key, "kind": "agent"} }
	edge := func(id, to string, extra map[string]any) map[string]any {
		e := map[string]any{"id": id, "label": id, "to": to}
		for k, v := range extra {
			e[k] = v
		}
		return e
	}
	check := map[string]any{"key": "verify", "title": "Verify", "kind": "gate", "role": "checker", "instructions": "Verify."}
	if requireOther {
		check["inputs"] = []any{map[string]any{"from": "work_b.make_b", "required": true}}
	}
	return map[string]any{
		"version": 2, "start": "plan",
		"roles":     []any{agent("dev_a"), agent("dev_b"), agent("checker")},
		"vars":      []any{map[string]any{"key": "branch", "type": "enum", "values": []any{"a", "b"}, "default": "a", "set_by": []any{"lead"}, "set_at": []any{"plan"}}},
		"loops":     []any{map[string]any{"key": "redo", "header": "work_a", "max": 1}},
		"stops":     []any{map[string]any{"code": "loop_exhausted", "class": "loop_exhausted", "message": "stop3", "legacy_no": 3}},
		"terminals": []any{map[string]any{"key": "done", "status": "done"}},
		"messages":  map[string]any{"stop3": "BLOCKED · rework used\n{request}"},
		"steps": []any{
			map[string]any{"key": "plan", "kind": "lead", "status": map[string]any{"key": "todo"}, "lead": map[string]any{"sets": []any{"branch"}},
				"edges": []any{edge("plan_a", "work_a", map[string]any{"when": map[string]any{"field": "var.branch", "op": "eq", "value": "a"}}), edge("plan_b", "work_b", nil)}},
			map[string]any{"key": "work_a", "title": "Work A", "kind": "tasks", "status": map[string]any{"name": "Work A", "category": "started"},
				"tasks": []any{map[string]any{"key": "make_a", "title": "Make A", "kind": "work", "role": "dev_a", "instructions": "Make A."}},
				"edges": []any{edge("a_next", "check", nil)}},
			map[string]any{"key": "work_b", "title": "Work B", "kind": "tasks", "status": map[string]any{"name": "Work B", "category": "started"},
				"tasks": []any{map[string]any{"key": "make_b", "title": "Make B", "kind": "work", "role": "dev_b", "instructions": "Make B."}},
				"edges": []any{edge("b_next", "check", nil)}},
			map[string]any{"key": "check", "title": "Check", "kind": "tasks", "status": map[string]any{"name": "Check", "category": "started"},
				"tasks": []any{check},
				"edges": []any{
					edge("check_ok", "done", map[string]any{"when": map[string]any{"field": "join.result", "op": "eq", "value": "pass"}}),
					edge("check_redo", "work_a", map[string]any{"loop": "redo", "when": map[string]any{"all": []any{
						map[string]any{"field": "join.result", "op": "eq", "value": "fail"}, map[string]any{"field": "loop.redo.count", "op": "lt", "value": 1}}}}),
					map[string]any{"id": "check_stop", "label": "Stop", "stop": "loop_exhausted"},
				}},
		},
		"policy": map[string]any{},
	}
}

func TestRehearsalGraph_InputsFollowThePathTaken(t *testing.T) {
	linksTo := func(links []IssueFlowLinkResponse) []string {
		var out []string
		for _, l := range links {
			out = append(out, l.Task)
		}
		return out
	}
	gateTask := func(st IssueFlowStateResponse) IssueFlowTaskResponse {
		for _, task := range st.Tasks {
			if task.Task == "verify" && task.ClosedAt == nil {
				return task
			}
		}
		t.Fatalf("no open verify task in %+v", st.Tasks)
		return IssueFlowTaskResponse{}
	}

	rig := newGraphRigDoc(t, "branch", branchDoc(false), graphRigOpts{})
	// Branch b: the gate reads the work of the branch taken.
	viaB := rig.newRequest()
	rig.complete(viaB, map[string]any{"branch": "b"})
	rig.finishAll(viaB, "work_b", 0, always(""))
	rig.advance(viaB)
	if got := linksTo(gateTask(rig.state(viaB)).Inputs); len(got) != 1 || got[0] != "make_b" {
		t.Fatalf("gate inputs on branch b = %v, want make_b", got)
	}
	// Branch a, rejected once: the gate reads make_a, and the redo answers the gate.
	viaA := rig.newRequest()
	rig.complete(viaA, map[string]any{"branch": "a"})
	rig.finishAll(viaA, "work_a", 0, always(""))
	rig.advance(viaA)
	if got := linksTo(gateTask(rig.state(viaA)).Inputs); len(got) != 1 || got[0] != "make_a" {
		t.Fatalf("gate inputs on branch a = %v, want make_a", got)
	}
	rig.finishAll(viaA, "check", 0, always("rejected"))
	if st := rig.advance(viaA); st.Step != "work_a" || st.Loops["redo"] != 1 {
		t.Fatalf("step %q, loops %v; want work_a with redo 1", st.Step, st.Loops)
	}
	redo := rig.rows(viaA, "work_a", 1)["make_a"]
	var found IssueFlowTaskResponse
	for _, task := range rig.state(viaA).Tasks {
		if task.IssueID == redo {
			found = task
		}
	}
	if got := linksTo(found.Feedback); len(got) != 1 || got[0] != "verify" {
		t.Fatalf("feedback of the redo = %v, want the gate verify", got)
	}
	if !containsAll(taskDescription(t, redo), "Feedback (round 2)") {
		t.Fatalf("the description of the redo does not carry the feedback:\n%s", taskDescription(t, redo))
	}

	// An input marked required, from a step the path did not take, stops the request before the task opens.
	needs := newGraphRigDoc(t, "branch-required", branchDoc(true), graphRigOpts{})
	missing := needs.newRequest()
	needs.complete(missing, map[string]any{"branch": "a"})
	needs.finishAll(missing, "work_a", 0, always(""))
	needs.advance(missing)
	st := needs.state(missing)
	if st.StopInfo == nil || st.StopInfo.Code != issueworkflow.DocStopStalled {
		t.Fatalf("stop %+v, want stalled for a required input that never ran", st.StopInfo)
	}
	if logged := flowTransitions(t, missing); logged[len(logged)-1]["reason"] != issueworkflow.ReasonInputMissing {
		t.Fatalf("the stop was logged with %v, want input_missing", logged[len(logged)-1])
	}
}
