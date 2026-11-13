package handler

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The engine runs a workflow of schema 1 through the graph lifted from it (spec
// section 9, plan P11): the stored row and the stored form of the request state
// stay as they were, and every hop is written to the trail and the activity log.

func loadWorkflowRow(t *testing.T, id string) db.IssueWorkflow {
	t.Helper()
	row, err := testHandler.Queries.GetIssueWorkflow(context.Background(), db.GetIssueWorkflowParams{ID: parseUUID(id), WorkspaceID: parseUUID(testWorkspaceID)})
	if err != nil {
		t.Fatalf("load workflow %s: %v", id, err)
	}
	return row
}

// workflowRowText is the whole row of a workflow as text: it changes when any column does.
func workflowRowText(t *testing.T, id string) string {
	t.Helper()
	var text string
	dbfx.QueryRow(t, `SELECT to_jsonb(w)::text FROM issue_workflow w WHERE id = $1`, id).Scan(&text)
	return text
}

func storedFlowState(t *testing.T, request db.Issue) map[string]any {
	t.Helper()
	var raw []byte
	dbfx.QueryRow(t, `SELECT flow FROM issue WHERE id = $1`, request.ID).Scan(&raw)
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("decode issue.flow %s: %v", raw, err)
	}
	return state
}

// flowTransitions are the flow_transition rows of a request, oldest first.
func flowTransitions(t *testing.T, request db.Issue) []map[string]any {
	t.Helper()
	rows, err := testPool.Query(context.Background(), `SELECT details FROM activity_log
		WHERE issue_id = $1 AND action = 'flow_transition' ORDER BY created_at, id`, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var details map[string]any
		if err := json.Unmarshal(raw, &details); err != nil {
			t.Fatalf("details of a flow_transition: %v", err)
		}
		out = append(out, details)
	}
	return out
}

func trailOf(t *testing.T, state map[string]any) []map[string]any {
	t.Helper()
	list, _ := state["trail"].([]any)
	out := make([]map[string]any, len(list))
	for i, e := range list {
		out[i], _ = e.(map[string]any)
	}
	return out
}

// A row of a catalog flow reads as a graph with the schema 1 compat mark, the
// steps, tasks, variables and roles of the row.
func TestEngine_LoadsV1RowThroughLift(t *testing.T) {
	f := newRehearsalFixture(t)
	def, err := issueworkflow.Decode(loadWorkflowRow(t, f.Workflow.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !def.HasFlow() || def.Graph.Compat != issueworkflow.CompatV1 {
		t.Fatalf("flow %v, compat %q; want a flow marked compat v1", def.HasFlow(), def.Graph.Compat)
	}
	g := def.Graph
	if g.Doc.Start != issueworkflow.StartStatus {
		t.Fatalf("start = %q, want %q", g.Doc.Start, issueworkflow.StartStatus)
	}
	if !g.Doc.Policy.AllowUnattendedRelease {
		t.Fatal("the lifted policy refuses an unattended release: a schema 1 request would stall at the end")
	}
	if start, _ := g.Step(issueworkflow.StartStatus); start.Kind != issueworkflow.StepKindLead {
		t.Fatalf("the plan step is a %q step, want lead", start.Kind)
	}
	if review, ok := g.Step("in_review"); !ok || review.Kind != issueworkflow.StepKindManual || review.Notice != issueworkflow.MessageStop2 {
		t.Fatalf("in_review = %+v, want a manual step with the stop 2 notice", review)
	}
	for _, key := range []string{"done", "cancelled"} {
		if _, ok := g.Terminal(key); !ok {
			t.Fatalf("no terminal %q", key)
		}
	}
	if _, ok := g.Step("blocked"); ok {
		t.Fatal("blocked is a stop of the graph, not a step")
	}

	// Every task of the row is a task of the graph, of the same kind, held by a role bound to its agent.
	for _, s := range def.Steps {
		for _, task := range s.Tasks {
			doc, ok := flowGraphTask(g, s.StatusKey, task.Key)
			if !ok || doc.Kind != task.Kind {
				t.Fatalf("task %s/%s: graph task %+v, ok %v", s.StatusKey, task.Key, doc, ok)
			}
			bound := false
			for _, role := range g.Doc.Roles {
				if role.Key == doc.Role && role.Bind != nil && len(role.Bind.AgentIDs) == 1 && role.Bind.AgentIDs[0] == task.Handler.ID {
					bound = true
				}
			}
			if !bound {
				t.Fatalf("task %s/%s: role %q is not bound to its agent %s", s.StatusKey, task.Key, doc.Role, task.Handler.ID)
			}
		}
	}
	var vars, switches []string
	for _, v := range g.Doc.Vars {
		vars = append(vars, v.Key)
	}
	for _, s := range def.Flow.Switches {
		switches = append(switches, s.Key)
	}
	sort.Strings(vars)
	sort.Strings(switches)
	if !reflect.DeepEqual(vars, switches) {
		t.Fatalf("variables %v, switches %v", vars, switches)
	}
}

// The engine reads the row and never writes it: no run, stop, resume or switch
// changes a column of the workflow (spec 9.1).
func TestEngine_V1RowIsNeverRewritten(t *testing.T) {
	f := newRehearsalFixture(t)
	before := workflowRowText(t, f.Workflow.ID)

	// A full run with a rework, then a request that stops and resumes, then a switch.
	request := f.newRequest(t)
	gates := f.driveToGate(t, request)
	finishTask(t, gates["docs_gate"], f.Agents.DocsGate, "approved")
	finishTask(t, gates["customs_gate"], f.Agents.CustomsGate, "rejected")
	advance(t, request)
	gates = f.finishDraft(t, request, 1)
	handover := f.toHandover(t, request, gates)
	finishTask(t, handover, f.Agents.Handover, "approved")
	if st := advance(t, request).State; st.Step != "in_review" {
		t.Fatalf("step %q, want in_review", st.Step)
	}

	stopped := f.newRequest(t)
	f.plan(t, stopped, nil)
	blockTask(t, tasksAt(t, stopped, "collect", 0)["docs"])
	if st := advance(t, stopped).State; stopOf(st) != issueworkflow.Stop1 {
		t.Fatalf("stop %v, want stop 1", stopOf(st))
	}
	if _, err := testHandler.ResumeFlow(context.Background(), loadIssueRow(t, uuidToString(stopped.ID)), FlowResumeInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := testHandler.SetFlowSwitch(context.Background(), loadIssueRow(t, uuidToString(stopped.ID)), "customs_lookup", true, true); err != nil {
		t.Fatal(err)
	}

	if after := workflowRowText(t, f.Workflow.ID); after != before {
		t.Fatalf("the workflow row changed while requests ran on it\nbefore: %s\nafter:  %s", before, after)
	}
}

// The state of a request on a schema 1 row keeps its schema 1 form: between two
// steps only the fields of the move change (spec 2.4, 9.2 rule 13).
func TestEngine_V1StateRoundTripInDB(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)

	snapshots := []map[string]any{storedFlowState(t, request)}
	take := func() { snapshots = append(snapshots, storedFlowState(t, request)) }

	gates := f.driveToGate(t, request)
	take()
	finishTask(t, gates["docs_gate"], f.Agents.DocsGate, "approved")
	finishTask(t, gates["customs_gate"], f.Agents.CustomsGate, "approved")
	advance(t, request)
	take()
	handover := tasksAt(t, request, "handover", 0)["handover"]
	finishTask(t, handover, f.Agents.Handover, "approved")
	advance(t, request)
	take()

	// What may change from one snapshot to the next: the move itself, the plan, and the review mark.
	moves := map[string]bool{"seq": true, "step": true, "entered_at": true, "trail": true, "unreviewed": true, "plan_done": true}
	for i := 1; i < len(snapshots); i++ {
		prev, next := snapshots[i-1], snapshots[i]
		for _, key := range keysOf(prev, next) {
			if moves[key] {
				continue
			}
			if !reflect.DeepEqual(prev[key], next[key]) {
				t.Errorf("snapshot %d changed %q: %v -> %v", i, key, prev[key], next[key])
			}
		}
		if num(next["seq"]) <= num(prev["seq"]) {
			t.Errorf("snapshot %d: seq %v did not grow from %v", i, next["seq"], prev["seq"])
		}
	}
	for _, absent := range []string{"loops", "vars", "activations", "stop"} {
		if v, ok := snapshots[len(snapshots)-1][absent]; ok && absent != "stop" {
			t.Errorf("a schema 1 state carries the schema 2 key %q = %v", absent, v)
		}
	}

	// The stored form reads and writes back unchanged through the schema 2 shape.
	var state issueworkflow.FlowState
	if err := json.Unmarshal(loadIssueRow(t, uuidToString(request.ID)).Flow, &state); err != nil {
		t.Fatal(err)
	}
	again, _ := json.Marshal(issueworkflow.Unadapt(issueworkflow.Adapt(state)))
	var got map[string]any
	_ = json.Unmarshal(again, &got)
	if want := snapshots[len(snapshots)-1]; !reflect.DeepEqual(got, want) {
		t.Fatalf("Unadapt(Adapt(state)) differs from the stored state\n got:  %v\n want: %v", got, want)
	}
}

func keysOf(maps ...map[string]any) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range maps {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

func num(v any) float64 {
	n, _ := v.(float64)
	return n
}

// Every hop adds one entry to the trail and one flow_transition row with the edge
// taken, the facts the edges read and the counters after the hop (spec 4.13, K22).
func TestEngine_TrailAndTransitionLogPerHop(t *testing.T) {
	t.Run("a request that passes and is reworked once", func(t *testing.T) {
		f := newRehearsalFixture(t)
		request := f.newRequest(t)
		gates := f.driveToGate(t, request)
		finishTask(t, gates["docs_gate"], f.Agents.DocsGate, "approved")
		finishTask(t, gates["customs_gate"], f.Agents.CustomsGate, "rejected")
		advance(t, request)
		gates = f.finishDraft(t, request, 1)
		handover := f.toHandover(t, request, gates)
		finishTask(t, handover, f.Agents.Handover, "approved")
		advance(t, request)

		wantSteps := []string{"todo", "collect", "draft", "gate", "draft", "gate", "handover"}
		trail := trailOf(t, storedFlowState(t, request))
		transitions := flowTransitions(t, request)
		if len(trail) != len(wantSteps) || len(transitions) != len(wantSteps) {
			t.Fatalf("trail %d entries, log %d rows; want %d each\ntrail %v\nlog %v", len(trail), len(transitions), len(wantSteps), trail, transitions)
		}
		for i, step := range wantSteps {
			if trail[i]["step"] != step || transitions[i]["from"] != step {
				t.Errorf("hop %d: trail step %v, log from %v; want %q", i, trail[i]["step"], transitions[i]["from"], step)
			}
			if trail[i]["edge"] == "" || trail[i]["edge"] != transitions[i]["edge"] {
				t.Errorf("hop %d: trail edge %v, log edge %v; want the same non-empty edge", i, trail[i]["edge"], transitions[i]["edge"])
			}
			if trail[i]["at"] == nil || trail[i]["at"] == "" {
				t.Errorf("hop %d: the trail entry has no time", i)
			}
		}

		// The first gate hop was a rework: the gates did not all approve.
		fail := transitions[3]
		if fail["join"] != issueworkflow.JoinFail || fail["to"] != "draft" {
			t.Errorf("the rejected gate: join %v to %v, want fail to draft", fail["join"], fail["to"])
		}
		if facts, _ := fail["facts"].(map[string]any); facts["join.result"] != issueworkflow.JoinFail {
			t.Errorf("the rejected gate recorded facts %v, want join.result fail", fail["facts"])
		}
		if loops, _ := fail["loops"].(map[string]any); num(loops[issueworkflow.LoopRework]) != 1 {
			t.Errorf("loops after the rework = %v, want rework 1", fail["loops"])
		}
		pass := transitions[5]
		if pass["join"] != issueworkflow.JoinPass || pass["to"] != "handover" {
			t.Errorf("the passing gate: join %v to %v, want pass to handover", pass["join"], pass["to"])
		}
		if _, hasStop := pass["stop"]; hasStop {
			t.Errorf("a hop that enters a step names a stop: %v", pass)
		}
	})

	t.Run("a request that parks at a stop", func(t *testing.T) {
		f := newRehearsalFixture(t)
		request := f.newRequest(t)
		f.plan(t, request, nil)
		blockTask(t, tasksAt(t, request, "collect", 0)["docs"])
		advance(t, request)

		trail := trailOf(t, storedFlowState(t, request))
		transitions := flowTransitions(t, request)
		if len(trail) != 2 || len(transitions) != 2 {
			t.Fatalf("trail %v, log %v; want the plan and the stop", trail, transitions)
		}
		stop := transitions[1]
		if stop["from"] != "collect" || stop["stop"] != issueworkflow.StopGroundsMissing || stop["join"] != issueworkflow.JoinBlock {
			t.Errorf("stop hop = %v, want from collect, stop grounds_missing, join block", stop)
		}
		if _, hasTo := stop["to"]; hasTo {
			t.Errorf("a hop to a stop names a target step: %v", stop)
		}
	})
}
