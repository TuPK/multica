package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Rehearsals of the Kit Viet squads (S23 to S26 of docs/engineering/kitviet-workflow-plan.md). Every
// squad's flow is the generated catalog flow, one fake agent per role. The same driver runs every
// route of every squad: a request that succeeds, one that succeeds with every switch on, one whose
// gate rejects once, one blocked for lack of grounds (stop 1) and one that is rejected until the
// rounds run out (stop 3).

type squadRig struct {
	flow    catalogFlow
	flowKey string
	wf      IssueWorkflowResponse
	squadID string
	agents  map[string]string
	kinds   map[string]string // "step/task" -> work or gate
	human   map[string]bool   // steps only a person moves the request out of
	routes  []string          // the values of the route switch; "" when the flow has none
	bools   []string
}

func newSquadRig(t *testing.T, squadKey string) squadRig {
	t.Helper()
	cleanupFlowRows(t)
	seedTestCatalog(t)
	flow := loadCatalogFlow(t, "flow_"+squadKey)
	first := seededReadyAgentID(t)
	rig := squadRig{flow: flow, flowKey: squadKey, agents: map[string]string{}, kinds: map[string]string{}, human: map[string]bool{}, routes: []string{""}}
	for _, role := range flow.roles() {
		rig.agents[role] = goldenLeadAgent(t, first)
	}
	for _, s := range flow.Steps {
		rig.human[s.StatusKey] = s.HumanOnly
		for _, task := range s.Tasks {
			rig.kinds[s.StatusKey+"/"+task.Key] = task.Kind
		}
	}
	var cfg struct {
		Switches []struct {
			Key    string   `json:"key"`
			Type   string   `json:"type"`
			Values []string `json:"values"`
		} `json:"switches"`
	}
	if err := json.Unmarshal(flow.Config, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, s := range cfg.Switches {
		switch {
		case s.Key == "route":
			rig.routes = s.Values
		case s.Type == "bool":
			rig.bools = append(rig.bools, s.Key)
		}
	}
	rig.wf = createFlowWorkflowForTest(t, flow.body(t, "Rehearsal "+squadKey+" "+workflowTestSuffix(), func(role string) string { return rig.agents[role] }))
	rig.squadID = dbfx.Squad(t, "Rehearsal squad "+squadKey+" "+workflowTestSuffix(), first)
	dbfx.Exec(t, `UPDATE issue_workflow SET squad_id = $2 WHERE id = $1`, rig.wf.ID, rig.squadID)
	dbfx.Exec(t, `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, rig.squadID, rig.wf.ID)
	return rig
}

func (r squadRig) newRequest(t *testing.T) db.Issue {
	t.Helper()
	id := dbfx.Issue(t, "Trade request rehearsal "+r.flowKey+" "+workflowTestSuffix(), testutil.Cols{
		"status": "todo", "assignee_type": "squad", "assignee_id": r.squadID,
	})
	started, err := testHandler.EnsureFlowStarted(context.Background(), loadIssueRow(t, id))
	if err != nil || len(started.Flow) == 0 {
		t.Fatalf("EnsureFlowStarted: flow %q, err %v", started.Flow, err)
	}
	return started
}

func (r squadRig) switches(route string, allOn bool) map[string]any {
	out := map[string]any{}
	if route != "" {
		out["route"] = route
	}
	if allOn {
		for _, key := range r.bools {
			out[key] = true
		}
	}
	// A request is only planned with the records approved; the plan refuses it otherwise (see the
	// Teaching test of that name).
	for _, key := range r.bools {
		if key == "records_anonymised" {
			out[key] = true
		}
	}
	return out
}

// settled is what a drive saw on the way.
type settled struct {
	sawDraft, sawGate, sawWork bool
	rejected                   int
}

// drive closes every open task the way its agent would and advances, until the request stops, waits
// for a person, or has nothing left to do. verdict gives the outcome for a task ("" for a work task
// that is simply done).
func (r squadRig) drive(t *testing.T, request db.Issue, verdict func(step, task string, round int, kind string) string) settled {
	t.Helper()
	var seen settled
	for i := 0; i < 60; i++ {
		row := loadIssueRow(t, uuidToString(request.ID))
		state, ok := flowRequestState(row)
		if !ok {
			t.Fatal("the request lost its flow")
		}
		if state.ClosedAt != "" || state.Stop != issueworkflow.StopNone {
			return seen
		}
		round := state.Round()
		open := map[string]string{}
		for key, id := range tasksAt(t, request, state.Step, round) {
			if status, _, _ := issueStatusAndAssignee(t, id); status != "done" && status != "cancelled" && status != "blocked" {
				open[key] = id
			}
		}
		if r.human[state.Step] && state.Step != "in_review" {
			return seen // waits for a person's approval
		}
		if state.Step == "in_review" {
			return seen
		}
		for key, id := range open {
			kind := r.kinds[state.Step+"/"+key]
			switch {
			case state.Step == "draft":
				seen.sawDraft = true
			}
			if kind == "gate" && state.Step == "gate" {
				seen.sawGate = true
			}
			if kind == "work" {
				seen.sawWork = true
			}
			_, _, agentID := issueStatusAndAssignee(t, id)
			outcome := verdict(state.Step, key, round, kind)
			switch outcome {
			case "blocked":
				blockTask(t, id)
			case "rejected":
				seen.rejected++
				finishTask(t, id, agentID, outcome)
			default:
				finishTask(t, id, agentID, outcome)
			}
		}
		resp := advance(t, request)
		if len(open) == 0 && allNoop(resp) {
			return seen // nothing opened and nothing moved
		}
	}
	t.Fatal("the request did not settle in 60 passes")
	return seen
}

func approveAll(step, task string, round int, kind string) string {
	if kind == "gate" {
		return "approved"
	}
	return ""
}

// approvalMove is what a person does at a wait step: move the request to the next step of the flow.
func (r squadRig) approve(t *testing.T, request db.Issue) {
	t.Helper()
	state, _ := flowRequestState(loadIssueRow(t, uuidToString(request.ID)))
	if !r.human[state.Step] || state.Step == "in_review" {
		return
	}
	updateIssueForTest(t, uuidToString(request.ID), map[string]any{"status": "draft"}).Want(http.StatusOK)
}

func (r squadRig) finalState(t *testing.T, request db.Issue) issueworkflow.FlowState {
	t.Helper()
	state, _ := flowRequestState(loadIssueRow(t, uuidToString(request.ID)))
	return state
}

func (r squadRig) wantNoDuplicates(t *testing.T, request db.Issue) {
	t.Helper()
	if n := duplicateTasks(t, request); n != 0 {
		t.Fatalf("%d duplicated tasks", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM issue WHERE parent_issue_id = $1 AND creator_type = 'agent'`, request.ID); n != 0 {
		t.Fatalf("%d sub-issues created by an agent", n)
	}
}

// reachReview plans the request and drives it (with the approval move a flow with an approval step
// needs) until it waits in in_review.
func (r squadRig) reachReview(t *testing.T, route string, allOn bool, verdict func(step, task string, round int, kind string) string) (db.Issue, settled) {
	t.Helper()
	request := r.newRequest(t)
	if _, err := testHandler.PlanFlow(context.Background(), loadIssueRow(t, uuidToString(request.ID)), FlowPlanInput{Switches: r.switches(route, allOn)}); err != nil {
		t.Fatalf("PlanFlow(%s): %v", route, err)
	}
	seen := r.drive(t, request, verdict)
	if state := r.finalState(t, request); r.human[state.Step] && state.Step != "in_review" {
		r.approve(t, request)
		next := r.drive(t, request, verdict)
		seen.sawDraft = seen.sawDraft || next.sawDraft
		seen.sawGate = seen.sawGate || next.sawGate
		seen.sawWork = seen.sawWork || next.sawWork
		seen.rejected += next.rejected
	}
	return request, seen
}

func rehearseSquad(t *testing.T, squadKey string) {
	rig := newSquadRig(t, squadKey)
	for _, route := range rig.routes {
		name := route
		if name == "" {
			name = "only"
		}
		t.Run(name+"/succeeds", func(t *testing.T) {
			request, seen := rig.reachReview(t, route, false, approveAll)
			state := rig.finalState(t, request)
			if state.Step != "in_review" || requestStatus(t, request) != "in_review" {
				t.Fatalf("step %q, status %q; want in_review", state.Step, requestStatus(t, request))
			}
			if !seen.sawWork {
				t.Fatal("no work task ran on this route")
			}
			rig.wantNoDuplicates(t, request)
			if !containsAll(lastComment(t, request), "IN REVIEW · stop 2") {
				t.Fatalf("stop 2 comment = %q", lastComment(t, request))
			}
		})
		t.Run(name+"/succeeds with every switch on", func(t *testing.T) {
			request, _ := rig.reachReview(t, route, true, approveAll)
			if state := rig.finalState(t, request); state.Step != "in_review" {
				t.Fatalf("step %q, want in_review", state.Step)
			}
			rig.wantNoDuplicates(t, request)
			for _, s := range rig.bools {
				if s == "export" {
					if tasksAt(t, request, "in_review", 0)["export"] == "" {
						t.Fatal("the export task did not open at in_review")
					}
				}
			}
		})
		t.Run(name+"/gate rejects once", func(t *testing.T) {
			rejectedOnce := false
			request, seen := rig.reachReview(t, route, false, func(step, task string, round int, kind string) string {
				if step == "gate" && kind == "gate" && round == 0 && !rejectedOnce {
					rejectedOnce = true
					return "rejected"
				}
				return approveAll(step, task, round, kind)
			})
			if !seen.sawGate {
				t.Skip("this route has no gate")
			}
			state := rig.finalState(t, request)
			if seen.sawDraft {
				if state.Step != "in_review" || state.Rework != 1 {
					t.Fatalf("step %q, rework %d; want in_review after one fix", state.Step, state.Rework)
				}
			} else if state.Step != "blocked" || state.Stop != issueworkflow.Stop3 {
				t.Fatalf("step %q, stop %v; a route with nothing to fix stops at 3", state.Step, state.Stop)
			}
			rig.wantNoDuplicates(t, request)
		})
		t.Run(name+"/stop 1 when grounds are missing", func(t *testing.T) {
			blockedOne := false
			request, seen := rig.reachReview(t, route, true, func(step, task string, round int, kind string) string {
				if kind == "work" && !blockedOne {
					blockedOne = true
					return "blocked"
				}
				return approveAll(step, task, round, kind)
			})
			if !seen.sawWork {
				t.Fatal("no work task ran")
			}
			state := rig.finalState(t, request)
			if state.Step != "blocked" || state.Stop != issueworkflow.Stop1 {
				t.Fatalf("step %q, stop %v; want blocked at stop 1", state.Step, state.Stop)
			}
			if !containsAll(lastComment(t, request), "BLOCKED · stop 1 · thiếu căn cứ") {
				t.Fatalf("stop 1 comment = %q", lastComment(t, request))
			}
		})
		t.Run(name+"/stop 3 when every round is rejected", func(t *testing.T) {
			request, seen := rig.reachReview(t, route, false, func(step, task string, round int, kind string) string {
				if step == "gate" && kind == "gate" {
					return "rejected"
				}
				return approveAll(step, task, round, kind)
			})
			if !seen.sawGate {
				t.Skip("this route has no gate")
			}
			state := rig.finalState(t, request)
			if state.Step != "blocked" || state.Stop != issueworkflow.Stop3 {
				t.Fatalf("step %q, stop %v; want blocked at stop 3", state.Step, state.Stop)
			}
			if seen.sawDraft && state.Rework != 2 {
				t.Fatalf("rework = %d, want 2 rounds used", state.Rework)
			}
			if !containsAll(lastComment(t, request), "BLOCKED · stop 3 · hết 2 vòng sửa") {
				t.Fatalf("stop 3 comment = %q", lastComment(t, request))
			}
		})
	}
}

func TestRehearsal_B2B(t *testing.T)          { rehearseSquad(t, "kv_b2b") }
func TestRehearsal_Admin(t *testing.T)        { rehearseSquad(t, "kv_admin") }
func TestRehearsal_Marketing(t *testing.T)    { rehearseSquad(t, "kv_marketing") }
func TestRehearsal_PR(t *testing.T)           { rehearseSquad(t, "kv_pr") }
func TestRehearsal_Ecom(t *testing.T)         { rehearseSquad(t, "kv_ecom") }
func TestRehearsal_Academic(t *testing.T)     { rehearseSquad(t, "kv_academic") }
func TestRehearsal_Legal(t *testing.T)        { rehearseSquad(t, "kv_legal") }
func TestRehearsal_Accounting(t *testing.T)   { rehearseSquad(t, "kv_accounting") }
func TestRehearsal_Realestate(t *testing.T)   { rehearseSquad(t, "kv_realestate") }
func TestRehearsal_Talent(t *testing.T)       { rehearseSquad(t, "kv_talent") }
func TestRehearsal_Education(t *testing.T)    { rehearseSquad(t, "kv_education") }
func TestRehearsal_Teaching(t *testing.T)     { rehearseSquad(t, "kv_teaching") }
func TestRehearsal_Hospitality(t *testing.T)  { rehearseSquad(t, "kv_hospitality") }
func TestRehearsal_Officeops(t *testing.T)    { rehearseSquad(t, "kv_officeops") }
func TestRehearsal_Health(t *testing.T)       { rehearseSquad(t, "kv_health") }
func TestRehearsal_Construction(t *testing.T) { rehearseSquad(t, "kv_construction") }
func TestRehearsal_Fnb(t *testing.T)          { rehearseSquad(t, "kv_fnb") }
func TestRehearsal_Insurance(t *testing.T)    { rehearseSquad(t, "kv_insurance") }
func TestRehearsal_Banking(t *testing.T)      { rehearseSquad(t, "kv_banking") }
func TestRehearsal_Factory(t *testing.T)      { rehearseSquad(t, "kv_factory") }

// Teaching refuses class records that are not anonymised or approved, at the plan.
func TestRehearsal_Teaching_RecordsNotAnonymisedStopAtPlan(t *testing.T) {
	rig := newSquadRig(t, "kv_teaching")
	request := rig.newRequest(t)

	state, err := testHandler.PlanFlow(context.Background(), loadIssueRow(t, uuidToString(request.ID)),
		FlowPlanInput{Switches: map[string]any{"route": "class_records", "records_anonymised": false}})
	if err != nil {
		t.Fatalf("PlanFlow: %v", err)
	}
	if state.Step != "blocked" || state.Stop != issueworkflow.Stop1 {
		t.Fatalf("step %q, stop %v; want blocked at stop 1 before any task opens", state.Step, state.Stop)
	}
	if n := countTasks(t, request); n != 0 {
		t.Fatalf("%d tasks opened for records that are not anonymised", n)
	}
}

func allNoop(resp FlowAdvanceResponse) bool {
	for _, a := range resp.Actions {
		if a.Kind != "noop" {
			return false
		}
	}
	return true
}
