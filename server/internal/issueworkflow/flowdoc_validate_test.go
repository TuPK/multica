package issueworkflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func vctx() ValidateContext {
	return ValidateContext{Statuses: DefaultStatusCatalog(), CanCreateStatuses: true}
}

func ruleOf(i Issue) string {
	r, _ := i.Params["rule"].(string)
	return r
}

func issuesOfRule(issues []Issue, rule string) []Issue {
	var out []Issue
	for _, i := range issues {
		if ruleOf(i) == rule {
			out = append(out, i)
		}
	}
	return out
}

func findIssue(issues []Issue, rule, code string) *Issue {
	for i := range issues {
		if ruleOf(issues[i]) == rule && issues[i].Code == code {
			return &issues[i]
		}
	}
	return nil
}

func describeIssues(issues []Issue) string {
	var b strings.Builder
	for _, i := range issues {
		fmt.Fprintf(&b, "\n  %s %s %s %s: %s", i.Severity, ruleOf(i), i.Code, i.Path, i.Message)
	}
	return b.String()
}

func tradeDoc(t *testing.T, edit func(d *FlowDoc)) FlowDoc {
	t.Helper()
	return modify(loadFlowDoc(t, "trade.json"), edit)
}

func fileDoc(t *testing.T, name string, edit func(d *FlowDoc)) FlowDoc {
	t.Helper()
	return modify(loadFlowDoc(t, name), edit)
}

func boolPtr(b bool) *bool { return &b }

// ruleCase is a document that must break one rule.
type ruleCase struct {
	rule, code, severity string
	// path is a prefix of the path of the issue.
	path string
	// base is the sample document; trade.json when empty.
	base string
	edit func(d *FlowDoc)
	ctx  *ValidateContext
	ack  string
}

func manyStatuses(n int) func(d *FlowDoc) {
	return func(d *FlowDoc) {
		for i := 0; i < n; i++ {
			d.Steps = append(d.Steps, DocStep{Key: fmt.Sprintf("extra%d", i), Kind: StepKindTasks,
				Status: StepStatus{Name: fmt.Sprintf("Extra %d", i), Category: "started"}, Edges: []DocEdge{{ID: "e", Label: "e", To: "done"}}})
		}
	}
}

func ruleCases() []ruleCase {
	noCreate := vctx()
	noCreate.CanCreateStatuses = false
	publish := vctx()
	publish.Publish = true
	roster := vctx()
	roster.Roster = Roster{Lead: "lead-id", Agents: []string{"agent-1", "agent-2"}, Archived: map[string]bool{"agent-2": true}}
	members := vctx()
	members.WorkspaceMembers = map[string]bool{"member-1": true}
	skills := publish
	skills.Roster = Roster{Agents: []string{"agent-1"}, AgentSkills: map[string][]string{"agent-1": {"drafting"}}}
	longTitle := strings.Repeat("x", 70)
	return []ruleCase{
		{rule: "S01", code: CodeFlowDocInvalid, severity: "error", path: "version", edit: func(d *FlowDoc) { d.Version = 1 }},
		{rule: "S01", code: CodeFlowSchemaUnsupported, severity: "error", path: "version", edit: func(d *FlowDoc) { d.Version = 3 }},
		{rule: "S02", code: "limit_exceeded", severity: "error", path: "steps", edit: manyStatuses(36)},
		{rule: "S02", code: "limit_exceeded", severity: "error", path: "policy.max_task_issues", edit: func(d *FlowDoc) { d.Policy.MaxTaskIssues = 600 }},
		{rule: "S02", code: "limit_exceeded", severity: "error", path: "steps[collect].tasks", edit: func(d *FlowDoc) {
			s := stepIdx(d, "collect")
			for i := 0; i < 12; i++ {
				s.Tasks = append(s.Tasks, DocTask{Key: fmt.Sprintf("more%d", i), Title: "t", Kind: KindWork, Role: "docs_reader"})
			}
		}},
		{rule: "S03", code: "key_duplicate", severity: "error", path: "steps[gate].tasks[docs_gate]", edit: func(d *FlowDoc) { stepIdx(d, "gate").Tasks[1].Key = "docs_gate" }},
		{rule: "S03", code: CodeKeyInvalid, severity: "error", path: "steps[Draft]", edit: func(d *FlowDoc) { stepIdx(d, "draft").Key = "Draft" }},
		{rule: "S04", code: "start_invalid", severity: "error", path: "start", edit: func(d *FlowDoc) { d.Start = "nowhere" }},
		{rule: "S04", code: "start_invalid", severity: "error", path: "start", edit: func(d *FlowDoc) { d.Start = "release" }},
		{rule: "S05", code: "status_invalid", severity: "error", path: "steps[collect].status", edit: func(d *FlowDoc) { stepIdx(d, "collect").Status = StepStatus{Key: "ghost"} }},
		{rule: "S05", code: "status_invalid", severity: "error", path: "steps[collect].status", edit: func(d *FlowDoc) {
			stepIdx(d, "collect").Status = StepStatus{Key: "todo", Name: "Both"}
		}},
		{rule: "S05", code: "status_create_forbidden", severity: "error", path: "steps[collect].status", ctx: &noCreate, edit: func(d *FlowDoc) {}},
		{rule: "S06", code: "too_many_statuses", severity: "error", path: "steps", edit: manyStatuses(8)},
		{rule: "S07", code: "status_shared", severity: "warning", path: "steps[draft].status", edit: func(d *FlowDoc) {
			stepIdx(d, "collect").Status = StepStatus{Key: "in_progress"}
			stepIdx(d, "draft").Status = StepStatus{Key: "in_progress"}
		}},
		{rule: "S08", code: "terminal_status_invalid", severity: "error", path: "terminals[done].status", edit: func(d *FlowDoc) { d.Terminals[0].Status = "in_progress" }},
		{rule: "S08", code: "stop_status_invalid", severity: "error", path: "stops[grounds_missing].status", edit: func(d *FlowDoc) { d.Stops[0].Status = "done" }},
		{rule: "S09", code: "text_too_long", severity: "error", path: "steps[plan].title", edit: func(d *FlowDoc) { stepIdx(d, "plan").Title = longTitle }},
		{rule: "S09", code: "text_too_long", severity: "error", path: "messages[stop1]", edit: func(d *FlowDoc) { d.Messages["stop1"] = strings.Repeat("é", 4001) }},

		{rule: "G01", code: "edge_target_unknown", severity: "error", path: "steps[gate].edges[gate_ok].to", edit: func(d *FlowDoc) { stepIdx(d, "gate").Edges[1].To = "nowhere" }},
		{rule: "G01", code: "stop_unknown", severity: "error", path: "steps[gate].edges[gate_blocked].stop", edit: func(d *FlowDoc) { stepIdx(d, "gate").Edges[0].Stop = "ghost" }},
		{rule: "G01", code: "edge_target_invalid", severity: "error", path: "steps[gate].edges[gate_ok]", edit: func(d *FlowDoc) { stepIdx(d, "gate").Edges[1].Stop = "loop_exhausted" }},
		{rule: "G02", code: "step_unreachable", severity: "error", path: "steps[orphan]", edit: func(d *FlowDoc) {
			d.Steps = append(d.Steps, DocStep{Key: "orphan", Kind: StepKindTasks, Status: StepStatus{Key: "in_progress"}, Edges: []DocEdge{{ID: "e", Label: "e", To: "done"}}})
		}},
		{rule: "G03", code: "dead_step", severity: "error", path: "steps[release]", edit: func(d *FlowDoc) { stepIdx(d, "release").Edges = nil }},
		{rule: "G03", code: "dead_step", severity: "error", path: "steps[handover]", edit: func(d *FlowDoc) {
			stepIdx(d, "handover").Edges = []DocEdge{{ID: "h", Label: "h", To: "release", When: cond("join.result", OpEq, "pass")}}
		}},
		{rule: "G03", code: "default_edge_not_last", severity: "error", path: "steps[handover].edges[handover_stop]", edit: func(d *FlowDoc) {
			e := stepIdx(d, "handover").Edges
			e[0], e[2] = e[2], e[0]
		}},
		{rule: "G04", code: "cycle_without_loop", severity: "error", path: "steps[", edit: func(d *FlowDoc) {
			g := stepIdx(d, "gate")
			g.Edges = append([]DocEdge{g.Edges[0], {ID: "gate_cycle", Label: "c", To: "collect", When: cond("join.result", OpEq, "fail")}}, g.Edges[1:]...)
		}},
		{rule: "G05", code: "loop_edge_invalid", severity: "error", path: "steps[gate].edges[gate_fix].to", edit: func(d *FlowDoc) { stepIdx(d, "gate").Edges[2].To = "collect" }},
		{rule: "G05", code: "loop_edge_invalid", severity: "error", path: "steps[gate].edges[gate_fix].loop", edit: func(d *FlowDoc) { stepIdx(d, "gate").Edges[2].Loop = "ghost" }},
		{rule: "G06", code: "loop_invalid", severity: "error", path: "loops[fix]", edit: func(d *FlowDoc) { stepIdx(d, "gate").Edges[2].Loop = "" }},
		{rule: "G06", code: "loop_invalid", severity: "error", path: "loops[fix].header", edit: func(d *FlowDoc) { d.Loops[0].Header = "nowhere" }},
		{rule: "G07", code: "loop_exit_missing", severity: "error", path: "steps[gate]", edit: func(d *FlowDoc) {
			g := stepIdx(d, "gate")
			g.Edges = g.Edges[:3]
		}},
		{rule: "G08", code: "no_live_path", severity: "error", path: "steps", edit: func(d *FlowDoc) {
			stepIdx(d, "release").Edges[0] = DocEdge{ID: "release_done", Label: "x", Stop: "loop_exhausted", Trigger: EdgeTriggerManual}
		}},
		{rule: "G09", code: "edge_trigger_invalid", severity: "error", path: "steps[collect].edges[collect_ok].trigger", edit: func(d *FlowDoc) { stepIdx(d, "collect").Edges[1].Trigger = EdgeTriggerManual }},
		{rule: "G09", code: "edge_trigger_invalid", severity: "error", path: "steps[release].edges[release_done].trigger", edit: func(d *FlowDoc) { stepIdx(d, "release").Edges[0].Trigger = EdgeTriggerAuto }},
		{rule: "G10", code: "lead_step_invalid", severity: "error", path: "steps[plan].lead.sets", edit: func(d *FlowDoc) { stepIdx(d, "plan").Lead.Sets = []string{"ghost"} }},
		{rule: "G10", code: "lead_step_invalid", severity: "error", path: "steps[plan].tasks", edit: func(d *FlowDoc) {
			stepIdx(d, "plan").Tasks = []DocTask{{Key: "t", Title: "t", Kind: KindWork, Role: "docs_reader"}}
		}},
		{rule: "G11", code: "flow_too_expensive", severity: "error", path: "steps", edit: func(d *FlowDoc) { d.Policy.MaxTaskIssues = 5 }},

		{rule: "R01", code: "role_unknown", severity: "error", path: "steps[draft].tasks[crosscheck].role", edit: func(d *FlowDoc) { stepIdx(d, "draft").Tasks[0].Role = "ghost" }},
		{rule: "R01", code: "role_unknown", severity: "error", path: "steps[release].exit_role", edit: func(d *FlowDoc) { stepIdx(d, "release").ExitRole = "ghost" }},
		{rule: "R02", code: "role_unbound", severity: "error", path: "roles[docs_reader].bind", ctx: &publish, edit: func(d *FlowDoc) {}},
		{rule: "R03", code: "role_not_member", severity: "error", path: "roles[docs_reader].bind", ctx: &roster, edit: func(d *FlowDoc) {
			d.Roles[0].Bind = &RoleBind{AgentIDs: []string{"stranger"}}
		}},
		{rule: "R03", code: "role_not_member", severity: "error", path: "roles[docs_reader].bind", ctx: &roster, edit: func(d *FlowDoc) {
			d.Roles[0].Bind = &RoleBind{AgentIDs: []string{"agent-2"}}
		}},
		{rule: "R03", code: "role_is_lead", severity: "error", path: "roles[docs_reader].bind", ctx: &roster, edit: func(d *FlowDoc) {
			d.Roles[0].Bind = &RoleBind{AgentIDs: []string{"lead-id"}}
		}},
		{rule: "R03", code: "role_not_member", severity: "error", path: "roles[counsel].bind", ctx: &members, base: "legal.json", edit: func(d *FlowDoc) {
			d.Roles[2].Bind = &RoleBind{MemberIDs: []string{"member-2"}}
		}},
		{rule: "R04", code: "role_unused", severity: "error", path: "roles[spare]", edit: func(d *FlowDoc) { d.Roles = append(d.Roles, Role{Key: "spare", Kind: RoleKindAgent}) }},
		{rule: "R05", code: "lead_cannot_work", severity: "error", path: "steps[draft].tasks[crosscheck].role", edit: func(d *FlowDoc) { stepIdx(d, "draft").Tasks[0].Role = "lead" }},
		{rule: "R06", code: "pool_too_small", severity: "error", path: "steps[collect].tasks[docs].instances", edit: func(d *FlowDoc) {
			stepIdx(d, "collect").Tasks[0].Instances = &Instances{Count: 3}
		}},
		{rule: "R06", code: "instance_var_invalid", severity: "error", path: "steps[collect].tasks[docs].instances.from_var", edit: func(d *FlowDoc) {
			stepIdx(d, "collect").Tasks[0].Instances = &Instances{FromVar: "export", Max: 1}
		}},
		{rule: "R08", code: "deadline_kind_invalid", severity: "error", path: "steps[draft].tasks[crosscheck].deadline", edit: func(d *FlowDoc) {
			stepIdx(d, "draft").Tasks[0].Deadline = &TaskDeadline{RemindAfterMinutes: 10}
		}},
		{rule: "R08", code: "deadline_kind_invalid", severity: "error", path: "steps[review].tasks[reviewer].deadline", base: "legal.json", edit: func(d *FlowDoc) {
			stepIdx(d, "review").Tasks[0].Deadline = &TaskDeadline{IdleMinutes: 10}
		}},

		{rule: "R07", code: "separation_violation", severity: "error", path: "steps[gate].tasks[docs_gate].role", edit: func(d *FlowDoc) { stepIdx(d, "gate").Tasks[0].Role = "drafter" }},
		{rule: "R09", code: "skill_not_attached", severity: "error", path: "steps[draft].tasks[crosscheck].skills", ctx: &skills, edit: func(d *FlowDoc) {
			d.Roles[2].Bind = &RoleBind{AgentIDs: []string{"agent-1"}}
			stepIdx(d, "draft").Tasks[0].Skills = []string{"tax"}
		}},
		{rule: "T07", code: "input_invalid", severity: "error", path: "steps[gate].tasks[docs_gate].inputs", edit: func(d *FlowDoc) {
			stepIdx(d, "gate").Tasks[0].Inputs = []Input{{From: "ghost.crosscheck"}}
		}},
		{rule: "T08", code: "var_set_invalid", severity: "error", path: "steps[draft].tasks[crosscheck].sets", edit: func(d *FlowDoc) { stepIdx(d, "draft").Tasks[0].Sets = []string{"ghost"} }},
		{rule: "J01", code: "join_invalid", severity: "error", path: "steps[gate].join.k", edit: func(d *FlowDoc) {
			stepIdx(d, "gate").Join = &Join{Over: JoinOver{Kind: JoinOverGates}, Mode: JoinModeQuorum, K: 5}
		}},
		{rule: "J02", code: "join_result_unrouted", severity: "error", path: "steps[gate]", edit: func(d *FlowDoc) {
			g := stepIdx(d, "gate")
			g.Edges = g.Edges[:2]
		}},
		{rule: "T01", code: "verdict_set_invalid", severity: "error", path: "steps[gate].tasks[docs_gate].verdicts", edit: func(d *FlowDoc) { stepIdx(d, "gate").Tasks[0].Verdicts = "ghost" }},
		{rule: "T01", code: "verdict_set_invalid", severity: "error", path: "steps[gate].tasks[docs_gate].verdicts", edit: func(d *FlowDoc) {
			d.VerdictSets = append(d.VerdictSets, VerdictSet{Key: "yes_only", Verdicts: []Verdict{{Key: "yes", Class: ClassPass}}})
			stepIdx(d, "gate").Tasks[0].Verdicts = "yes_only"
		}},
		{rule: "T01", code: "verdict_set_invalid", severity: "error", path: "verdicts[bad]", edit: func(d *FlowDoc) {
			d.VerdictSets = append(d.VerdictSets, VerdictSet{Key: "bad", Verdicts: []Verdict{{Key: "blocked", Class: ClassPass}}})
		}},
		{rule: "T02", code: "gate_verdict_required", severity: "error", path: "steps[gate].tasks[docs_gate].requires.verdict", edit: func(d *FlowDoc) {
			stepIdx(d, "gate").Tasks[0].Requires = &Requires{Verdict: boolPtr(false)}
		}},
		{rule: "T03", code: "after_invalid", severity: "error", path: "steps[collect].tasks[customs].after", edit: func(d *FlowDoc) { stepIdx(d, "collect").Tasks[1].After = []string{"ghost"} }},
		{rule: "T03", code: "after_invalid", severity: "error", path: "steps[collect].tasks", edit: func(d *FlowDoc) {
			s := stepIdx(d, "collect")
			s.Tasks[0].After = []string{"customs"}
			s.Tasks[1].After = []string{"docs"}
		}},
		{rule: "T03", code: "after_maybe_skipped", severity: "warning", path: "steps[collect].tasks[docs].after", edit: func(d *FlowDoc) { stepIdx(d, "collect").Tasks[0].After = []string{"customs"} }},
		{rule: "T04", code: "fact_not_available", severity: "error", path: "steps[collect].tasks[customs].when", edit: func(d *FlowDoc) {
			stepIdx(d, "collect").Tasks[1].When = cond("join.result", OpEq, "pass")
		}},
		{rule: "T04", code: "fact_not_available", severity: "error", path: "steps[collect].tasks[customs].when", edit: func(d *FlowDoc) {
			stepIdx(d, "collect").Tasks[1].When = cond("task.docs.pass", OpGte, 1)
		}},
		{rule: "T05", code: "file_req_invalid", severity: "error", path: "steps[draft].tasks[crosscheck].requires.files[draft]", edit: func(d *FlowDoc) {
			stepIdx(d, "draft").Tasks[0].Requires = &Requires{Files: []FileReq{{Slot: "draft", Min: 1, Ext: []string{"MD"}}}}
		}},
		{rule: "T05", code: "file_req_invalid", severity: "error", path: "steps[draft].tasks[crosscheck].requires.files[draft].max", edit: func(d *FlowDoc) {
			stepIdx(d, "draft").Tasks[0].Requires = &Requires{Files: []FileReq{{Slot: "draft", Min: 3, Max: 2}}}
		}},
		{rule: "T06", code: "deadline_invalid", severity: "error", path: "steps[draft].tasks[crosscheck].deadline", edit: func(d *FlowDoc) {
			stepIdx(d, "draft").Tasks[0].Deadline = &TaskDeadline{IdleMinutes: 500, HardCapMinutes: 100}
		}},

		{rule: "J03", code: "manual_step_invalid", severity: "error", path: "steps[release].exit_role", edit: func(d *FlowDoc) { stepIdx(d, "release").ExitRole = "docs_reader" }},
		{rule: "J03", code: "manual_step_invalid", severity: "error", path: "steps[release].edges[release_two].label", edit: func(d *FlowDoc) {
			s := stepIdx(d, "release")
			s.Edges = append(s.Edges, DocEdge{ID: "release_two", Label: "release", To: "cancelled", Trigger: EdgeTriggerManual})
		}},

		{rule: "F01", code: CodeUnknownFact, severity: "error", path: "steps[collect].tasks[customs].when", edit: func(d *FlowDoc) { stepIdx(d, "collect").Tasks[1].When = cond("var.ghost", OpEq, true) }},
		{rule: "F01", code: CodeUnknownFact, severity: "error", path: "steps[gate].edges[gate_ok].when", edit: func(d *FlowDoc) { stepIdx(d, "gate").Edges[1].When = cond("task.ghost.pass", OpGte, 1) }},
		{rule: "F02", code: "fact_op_invalid", severity: "error", path: "steps[gate].edges[gate_ok].when", edit: func(d *FlowDoc) { stepIdx(d, "gate").Edges[1].When = cond("var.customs_lookup", OpGt, true) }},
		{rule: "F02", code: "fact_value_invalid", severity: "error", path: "steps[gate].edges[gate_ok].when", edit: func(d *FlowDoc) { stepIdx(d, "gate").Edges[1].When = cond("join.result", OpEq, "maybe") }},
		{rule: "F02", code: "condition_invalid", severity: "error", path: "steps[gate].edges[gate_ok].when", edit: func(d *FlowDoc) { stepIdx(d, "gate").Edges[1].When = &Condition{All: []Condition{}} }},
		{rule: "F03", code: "var_invalid", severity: "error", path: "vars[customs_lookup].default", edit: func(d *FlowDoc) { d.Vars[1].Default = "yes" }},
		{rule: "F03", code: "var_invalid", severity: "error", path: "vars[export].set_by", edit: func(d *FlowDoc) { d.Vars[2].SetBy = []string{"ghost"} }},
		{rule: "F03", code: "var_invalid", severity: "error", path: "vars[export].set_at", edit: func(d *FlowDoc) { d.Vars[2].SetAt = []string{"ghost"} }},
		{rule: "F04", code: "var_unused", severity: "warning", path: "vars[route]", edit: func(d *FlowDoc) {}},
		{rule: "F05", code: "loop_max_invalid", severity: "error", path: "loops[fix].max", edit: func(d *FlowDoc) { d.Loops[0].Max = LoopMax{Var: "export", Cap: 3} }},
		{rule: "F06", code: CodeLegacyFieldInV2, severity: "error", path: "steps[collect].tasks[customs].when", edit: func(d *FlowDoc) {
			stepIdx(d, "collect").Tasks[1].When = cond("switch.customs_lookup", OpEq, true)
		}},
		{rule: "F06", code: CodeLegacyFieldInV2, severity: "error", path: "steps[gate].edges[gate_ok].when", edit: func(d *FlowDoc) { stepIdx(d, "gate").Edges[1].When = cond("tasks_outcome", OpEq, "all_approved") }},

		{rule: "P01", code: "message_missing", severity: "error", path: "steps[release].notice", edit: func(d *FlowDoc) { stepIdx(d, "release").Notice = "ghost" }},
		{rule: "P01", code: "message_missing", severity: "error", path: "stops[grounds_missing].message", edit: func(d *FlowDoc) { d.Stops[0].Message = "ghost" }},
		{rule: "P01", code: "message_placeholder", severity: "warning", path: "messages[stop1]", edit: func(d *FlowDoc) { d.Messages["stop1"] += " {bogus}" }},
		{rule: "P02", code: "message_mention", severity: "error", path: "messages[stop2]", edit: func(d *FlowDoc) { d.Messages["stop2"] += " [x](mention://member/1)" }},

		{rule: "Y01", code: "release_without_human", severity: "warning", path: "steps[", ack: AckUnattendedRelease, base: "patterns/p09_nested_cycles.json", edit: func(d *FlowDoc) {
			d.Terminals[0].LeavesWorkspace = true
		}},
		{rule: "Y02", code: "no_gate", severity: "warning", path: "steps", ack: AckNoGate, base: "patterns/p01_sequential.json", edit: func(d *FlowDoc) {}},
		{rule: "Y03", code: "loops_overlap", severity: "warning", path: "loops[extra]", edit: func(d *FlowDoc) {
			d.Loops = append(d.Loops, Loop{Key: "extra", Header: "draft", Max: LoopMax{N: 1}, Reset: LoopResetNever})
			g := stepIdx(d, "gate")
			g.Edges = append([]DocEdge{g.Edges[0], {ID: "gate_extra", Label: "x", To: "draft", Loop: "extra", When: cond("join.result", OpEq, "fail")}}, g.Edges[1:]...)
		}},
		{rule: "Y04", code: "ack_required", severity: "error", path: "policy.allow_unattended_release", edit: func(d *FlowDoc) { d.Policy.AllowUnattendedRelease = true }},
		{rule: "Y04", code: "ack_required", severity: "error", path: "policy.lead_may_work", edit: func(d *FlowDoc) { d.Policy.LeadMayWork = true }},
		{rule: "Y05", code: "manual_without_notice", severity: "warning", path: "steps[release]", edit: func(d *FlowDoc) { stepIdx(d, "release").Notice = "" }},
	}
}

// Every rule of spec section 3 has a document that is clean for it and documents
// that break it. A rule is clean when the document has no issue of its rule, except
// F04 where the variable that is read must not be reported.
func TestValidate_EveryRulePositiveAndNegative(t *testing.T) {
	cases := ruleCases()
	byRule := map[string][]ruleCase{}
	for _, c := range cases {
		byRule[c.rule] = append(byRule[c.rule], c)
	}
	all := []string{"S01", "S02", "S03", "S04", "S05", "S06", "S07", "S08", "S09", "G01", "G02", "G03", "G04", "G05", "G06", "G07", "G08", "G09", "G10", "G11",
		"R01", "R02", "R03", "R04", "R05", "R06", "R07", "R08", "R09", "T01", "T02", "T03", "T04", "T05", "T06", "T07", "T08", "J01", "J02", "J03",
		"F01", "F02", "F03", "F04", "F05", "F06", "P01", "P02", "Y01", "Y02", "Y03", "Y04", "Y05"}
	for _, rule := range all {
		t.Run(rule, func(t *testing.T) {
			if len(byRule[rule]) == 0 {
				t.Fatalf("no negative case for %s", rule)
			}
			// Positive: the sample document is clean for the rule.
			issues := ValidateFlowDoc(tradeDoc(t, func(*FlowDoc) {}), vctx())
			for _, i := range issuesOfRule(issues, rule) {
				if rule == "F04" && i.Path == "vars[route]" {
					continue
				}
				t.Fatalf("the sample document breaks %s:%s", rule, describeIssues([]Issue{i}))
			}
			// Negative.
			for n, c := range byRule[rule] {
				t.Run(fmt.Sprintf("%s#%d", c.code, n), func(t *testing.T) {
					base := c.base
					if base == "" {
						base = "trade.json"
					}
					ctx := vctx()
					if c.ctx != nil {
						ctx = *c.ctx
					}
					issues := ValidateFlowDoc(fileDoc(t, base, c.edit), ctx)
					var hit *Issue
					for i := range issues {
						if ruleOf(issues[i]) == c.rule && issues[i].Code == c.code && strings.HasPrefix(issues[i].Path, c.path) {
							hit = &issues[i]
							break
						}
					}
					if hit == nil {
						t.Fatalf("no %s %s at %q in:%s", c.rule, c.code, c.path, describeIssues(issues))
					}
					if hit.Severity != c.severity {
						t.Errorf("severity %s, want %s", hit.Severity, c.severity)
					}
					if hit.Ack != c.ack {
						t.Errorf("ack %q, want %q", hit.Ack, c.ack)
					}
					if hit.Message == "" {
						t.Error("no message")
					}
				})
			}
		})
	}
}

func TestValidate_JSONLevelRules(t *testing.T) {
	raw := func(edit func(m map[string]any)) []byte {
		doc := loadFlowDoc(t, "trade.json")
		b, _ := json.Marshal(doc)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		edit(m)
		out, _ := json.Marshal(m)
		return out
	}
	for name, tt := range map[string]struct {
		raw  []byte
		rule string
		code string
	}{
		"S01 unknown key":  {raw(func(m map[string]any) { m["colour"] = 1 }), "S01", CodeFlowDocInvalid},
		"S01 newer schema": {raw(func(m map[string]any) { m["version"] = 3 }), "S01", CodeFlowSchemaUnsupported},
		"S02 too large":    {raw(func(m map[string]any) { m["messages"] = map[string]any{"stop1": strings.Repeat("x", DocMaxBytes)} }), "S02", CodeFlowDocTooLarge},
		"S03 key pattern":  {raw(func(m map[string]any) { m["roles"].([]any)[0].(map[string]any)["key"] = "Bad Key" }), "S03", CodeKeyInvalid},
		"T08 requires": {raw(func(m map[string]any) {
			m["steps"].([]any)[2].(map[string]any)["tasks"].([]any)[0].(map[string]any)["requires"] = map[string]any{"vars": []any{"route"}}
		}), "T08", CodeVarSetInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			issues := ValidateFlowDocJSON(tt.raw, vctx())
			if findIssue(issues, tt.rule, tt.code) == nil {
				t.Fatalf("no %s %s in:%s", tt.rule, tt.code, describeIssues(issues))
			}
		})
	}
	if issues := ValidateFlowDocJSON(raw(func(map[string]any) {}), vctx()); HasErrors(issues) {
		t.Fatalf("the sample document through JSON:%s", describeIssues(issues))
	}
}

func TestValidate_ReturnsAllIssuesAtOnce(t *testing.T) {
	doc := tradeDoc(t, func(d *FlowDoc) {
		stepIdx(d, "draft").Tasks[0].Role = "ghost"                               // R01
		stepIdx(d, "gate").Edges[1].To = "nowhere"                                // G01
		stepIdx(d, "collect").Tasks[1].When = cond("var.ghost", OpEq, true)       // F01
		stepIdx(d, "release").Notice = "ghost"                                    // P01
		d.Terminals[0].Status = "in_progress"                                     // S08
		stepIdx(d, "gate").Tasks[0].Requires = &Requires{Verdict: boolPtr(false)} // T02
		d.Vars[1].Default = "yes"                                                 // F03
		d.Messages["stop2"] += " mention://member/1"                              // P02
		stepIdx(d, "plan").Title = strings.Repeat("x", 70)                        // S09
	})
	issues := ValidateFlowDoc(doc, vctx())
	rules := map[string]bool{}
	for _, i := range issues {
		if i.Severity == SeverityError {
			rules[ruleOf(i)] = true
		}
	}
	for _, want := range []string{"R01", "G01", "F01", "P01", "S08", "T02", "F03", "P02", "S09"} {
		if !rules[want] {
			t.Errorf("missing %s in:%s", want, describeIssues(issues))
		}
	}
	// Nothing short-circuits: the same call reports a problem of each group.
	var groups []string
	for r := range rules {
		groups = append(groups, r[:1])
	}
	sort.Strings(groups)
	if got := strings.Join(groups, ""); !strings.Contains(got, "S") || !strings.Contains(got, "G") || !strings.Contains(got, "R") || !strings.Contains(got, "T") || !strings.Contains(got, "F") || !strings.Contains(got, "P") {
		t.Errorf("groups reported: %s", got)
	}
	// A second call gives the same answer.
	if again := ValidateFlowDoc(doc, vctx()); !reflect.DeepEqual(again, issues) {
		t.Error("the validator is not deterministic")
	}
}

var indexPath = regexp.MustCompile(`\[\d+\]`)

func TestValidate_PathsUseKeysNotIndexes(t *testing.T) {
	broken := func(d *FlowDoc) {
		stepIdx(d, "draft").Tasks[0].Role = "ghost"
		stepIdx(d, "gate").Edges[1].To = "nowhere"
		stepIdx(d, "gate").Tasks[1].Role = "docs_gate" // gates of one step with one role
		stepIdx(d, "collect").Tasks[1].When = cond("var.ghost", OpEq, true)
		stepIdx(d, "release").Notice = "ghost"
		stepIdx(d, "handover").Tasks[0].Requires = &Requires{Verdict: boolPtr(false)}
		stepIdx(d, "collect").Tasks[0].Requires = &Requires{Files: []FileReq{{Slot: "a", Min: 1, Ext: []string{"MD"}}}}
		d.Vars[1].Default = "yes"
		d.Roles = append(d.Roles, Role{Key: "spare", Kind: RoleKindAgent})
	}
	keyed := func(d FlowDoc) []string {
		var out []string
		for _, i := range ValidateFlowDoc(d, vctx()) {
			out = append(out, fmt.Sprintf("%s|%s|%s", ruleOf(i), i.Code, i.Path))
		}
		sort.Strings(out)
		return out
	}
	doc := tradeDoc(t, broken)
	before := keyed(doc)
	if len(before) < 8 {
		t.Fatalf("only %d issues: %v", len(before), before)
	}
	for _, p := range before {
		if indexPath.MatchString(p) {
			t.Errorf("a path uses a position: %s", p)
		}
	}
	// Reorder the steps, the tasks of each step, the roles and the vars.
	reordered := modify(doc, func(d *FlowDoc) {
		for i, j := 1, len(d.Steps)-1; i < j; i, j = i+1, j-1 {
			d.Steps[i], d.Steps[j] = d.Steps[j], d.Steps[i]
		}
		for i := range d.Steps {
			tasks := d.Steps[i].Tasks
			for a, b := 0, len(tasks)-1; a < b; a, b = a+1, b-1 {
				tasks[a], tasks[b] = tasks[b], tasks[a]
			}
		}
		d.Roles = append([]Role(nil), d.Roles...)
		for a, b := 0, len(d.Roles)-1; a < b; a, b = a+1, b-1 {
			d.Roles[a], d.Roles[b] = d.Roles[b], d.Roles[a]
		}
		d.Vars = append([]Var(nil), d.Vars...)
		for a, b := 0, len(d.Vars)-1; a < b; a, b = a+1, b-1 {
			d.Vars[a], d.Vars[b] = d.Vars[b], d.Vars[a]
		}
	})
	after := keyed(reordered)
	// Reordering the steps changes which ones sit next to each other, so the
	// default-edge and status-sharing findings are the only ones allowed to move.
	if !reflect.DeepEqual(before, after) {
		t.Errorf("paths changed when the document was reordered\nbefore only: %v\nafter only:  %v", minus(before, after), minus(after, before))
	}
}

func TestValidate_G11WorstCaseNumbers(t *testing.T) {
	for _, name := range []string{"trade.json", "research.json", "legal.json", "market.json", "patterns/p09_nested_cycles.json"} {
		t.Run(name, func(t *testing.T) {
			doc := loadFlowDoc(t, name)
			a := Analyze(doc)
			// A cap one below each number is exceeded and one at the number is not.
			over := modify(doc, func(d *FlowDoc) { d.Policy.MaxTaskIssues = a.WorstCaseTaskIssues - 1; d.Policy.MaxActivations = 500 })
			issue := findIssue(ValidateFlowDoc(over, vctx()), "G11", "flow_too_expensive")
			if a.WorstCaseTaskIssues > 1 {
				if issue == nil {
					t.Fatalf("a cap of %d task issues is below the worst case %d and was accepted", a.WorstCaseTaskIssues-1, a.WorstCaseTaskIssues)
				}
				if issue.Params["worst_case_task_issues"] != a.WorstCaseTaskIssues || issue.Params["worst_case_activations"] != a.WorstCaseActivations {
					t.Errorf("params %v, want the numbers of Analyze: %d task issues, %d activations", issue.Params, a.WorstCaseTaskIssues, a.WorstCaseActivations)
				}
			}
			exact := modify(doc, func(d *FlowDoc) {
				d.Policy.MaxTaskIssues = a.WorstCaseTaskIssues
				d.Policy.MaxActivations = a.WorstCaseActivations
			})
			if findIssue(ValidateFlowDoc(exact, vctx()), "G11", "flow_too_expensive") != nil {
				t.Errorf("a cap equal to the worst case (%d, %d) was refused", a.WorstCaseTaskIssues, a.WorstCaseActivations)
			}
			fewer := modify(doc, func(d *FlowDoc) { d.Policy.MaxTaskIssues = 500; d.Policy.MaxActivations = a.WorstCaseActivations - 1 })
			if findIssue(ValidateFlowDoc(fewer, vctx()), "G11", "flow_too_expensive") == nil {
				t.Errorf("a cap of %d activations is below the worst case %d and was accepted", a.WorstCaseActivations-1, a.WorstCaseActivations)
			}
		})
	}
	// Trade by hand: 10 entries into steps and 13 task issues.
	a := Analyze(loadFlowDoc(t, "trade.json"))
	if a.WorstCaseTaskIssues != 13 || a.WorstCaseActivations != 10 {
		t.Fatalf("trade: %d task issues, %d activations", a.WorstCaseTaskIssues, a.WorstCaseActivations)
	}
}

// roleDoc is plan, a work step, a gate step and the end: the work is done by
// workRole and judged by gateRole.
func roleDoc(roles []Role, workRole, gateRole string, gateInputs []Input) FlowDoc {
	return FlowDoc{Version: 2, Start: "plan", Roles: roles,
		Terminals: []Terminal{{Key: "done", Status: "done"}},
		Steps: []DocStep{
			{Key: "plan", Kind: StepKindLead, Status: StepStatus{Key: "todo"}, Edges: []DocEdge{{ID: "p", Label: "p", To: "work"}}},
			{Key: "work", Kind: StepKindTasks, Status: StepStatus{Key: "in_progress"}, Tasks: []DocTask{{Key: "make", Title: "m", Kind: KindWork, Role: workRole}},
				Edges: []DocEdge{{ID: "w", Label: "w", To: "check"}}},
			{Key: "check", Kind: StepKindTasks, Status: StepStatus{Key: "in_review"}, Tasks: []DocTask{{Key: "judge", Title: "j", Kind: KindGate, Role: gateRole, Inputs: gateInputs}},
				Edges: []DocEdge{{ID: "c", Label: "c", To: "done"}}},
		}}
}

func agentRole(key string, pool int, ids ...string) Role {
	r := Role{Key: key, Kind: RoleKindAgent, Pool: pool}
	if len(ids) > 0 {
		r.Bind = &RoleBind{AgentIDs: ids}
	}
	return r
}

func TestValidate_R07SeparationByInputs(t *testing.T) {
	violations := func(d FlowDoc) []Issue { return issuesOfRule(ValidateFlowDoc(d, vctx()), "R07") }

	t.Run("two agents who review each other are valid", func(t *testing.T) {
		if got := violations(loadFlowDoc(t, "crossreview.json")); len(got) != 0 {
			t.Fatalf("cross review: %s", describeIssues(got))
		}
	})
	t.Run("an agent who reviews its own work is rejected", func(t *testing.T) {
		d := fileDoc(t, "crossreview.json", func(d *FlowDoc) {
			stepIdx(d, "review").Tasks[0].Role = "ben" // Ben now checks Ben's part
		})
		got := violations(d)
		if len(got) == 0 || got[0].Code != "separation_violation" || !strings.HasPrefix(got[0].Path, "steps[review].tasks[ana_checks_ben]") {
			t.Fatalf("self review: %s", describeIssues(got))
		}
		if judged, _ := got[0].Params["judged"].([]string); !reflect.DeepEqual(judged, []string{"build.write_b"}) {
			t.Errorf("judged = %v, want the work of the input", got[0].Params["judged"])
		}
	})
	t.Run("one agent in both roles of one pool of one is rejected", func(t *testing.T) {
		d := roleDoc([]Role{agentRole("doer", 1)}, "doer", "doer", nil)
		if got := violations(d); len(got) != 1 {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("a pool with enough people is valid", func(t *testing.T) {
		// One role of three people: one makes, another judges.
		if got := violations(roleDoc([]Role{agentRole("crew", 3)}, "crew", "crew", nil)); len(got) != 0 {
			t.Fatalf("a crew of 3: %s", describeIssues(got))
		}
		// A gate pool that includes the worker and two others.
		d := roleDoc([]Role{agentRole("doer", 1, "x"), agentRole("checker", 1, "x", "y", "z")}, "doer", "checker", nil)
		if got := violations(d); len(got) != 0 {
			t.Fatalf("checkers x, y, z over the work of x: %s", describeIssues(got))
		}
		// Two instances of work could use two of the three, one is left.
		d = roleDoc([]Role{agentRole("doer", 2, "x", "y"), agentRole("checker", 1, "x", "y", "z")}, "doer", "checker", nil)
		stepIdx(&d, "work").Tasks[0].Instances = &Instances{Count: 2}
		if got := violations(d); len(got) != 0 {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("a pool the work can take entirely is rejected", func(t *testing.T) {
		d := roleDoc([]Role{agentRole("doer", 3, "x", "y", "z"), agentRole("checker", 1, "x", "y")}, "doer", "checker", nil)
		stepIdx(&d, "work").Tasks[0].Instances = &Instances{Count: 3}
		if got := violations(d); len(got) != 1 {
			t.Fatalf("the work may be done by x and y: %s", describeIssues(got))
		}
		// Different people entirely.
		d = roleDoc([]Role{agentRole("doer", 1, "x"), agentRole("checker", 1, "y")}, "doer", "checker", nil)
		if got := violations(d); len(got) != 0 {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("a gate that needs many distinct people", func(t *testing.T) {
		d := roleDoc([]Role{agentRole("crew", 3)}, "crew", "crew", nil)
		stepIdx(&d, "check").Tasks[0].Instances = &Instances{Count: 3}
		if got := violations(d); len(got) != 1 {
			t.Fatalf("3 distinct gates from a crew of 3 who also did the work: %s", describeIssues(got))
		}
	})
	t.Run("the default inputs follow the path, not the order of the document", func(t *testing.T) {
		// plan, a (work, ana), b (work, ben), c (gate, ben). c judges b, the nearest
		// step with work before it, so ben judging his own work is rejected, even
		// though ana's step is declared first.
		d := FlowDoc{Version: 2, Start: "plan", Roles: []Role{agentRole("ana", 1), agentRole("ben", 1)},
			Terminals: []Terminal{{Key: "done", Status: "done"}},
			Steps: []DocStep{
				{Key: "plan", Kind: StepKindLead, Status: StepStatus{Key: "todo"}, Edges: []DocEdge{{ID: "p", Label: "p", To: "a"}}},
				{Key: "c", Kind: StepKindTasks, Status: StepStatus{Key: "in_review"}, Tasks: []DocTask{{Key: "judge", Title: "j", Kind: KindGate, Role: "ben"}}, Edges: []DocEdge{{ID: "c", Label: "c", To: "done"}}},
				{Key: "b", Kind: StepKindTasks, Status: StepStatus{Key: "in_progress"}, Tasks: []DocTask{{Key: "make_b", Title: "b", Kind: KindWork, Role: "ben"}}, Edges: []DocEdge{{ID: "b", Label: "b", To: "c"}}},
				{Key: "a", Kind: StepKindTasks, Status: StepStatus{Key: "in_progress"}, Tasks: []DocTask{{Key: "make_a", Title: "a", Kind: KindWork, Role: "ana"}}, Edges: []DocEdge{{ID: "a", Label: "a", To: "b"}}},
			}}
		got := violations(d)
		if len(got) != 1 || got[0].Params["judged"].([]string)[0] != "b.make_b" {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("a task of the same step that the gate comes after is judged", func(t *testing.T) {
		d := FlowDoc{Version: 2, Start: "plan", Roles: []Role{agentRole("ana", 1)},
			Terminals: []Terminal{{Key: "done", Status: "done"}},
			Steps: []DocStep{
				{Key: "plan", Kind: StepKindLead, Status: StepStatus{Key: "todo"}, Edges: []DocEdge{{ID: "p", Label: "p", To: "s"}}},
				{Key: "s", Kind: StepKindTasks, Status: StepStatus{Key: "in_progress"}, Tasks: []DocTask{
					{Key: "make", Title: "m", Kind: KindWork, Role: "ana"},
					{Key: "judge", Title: "j", Kind: KindGate, Role: "ana", After: []string{"make"}}}, Edges: []DocEdge{{ID: "e", Label: "e", To: "done"}}},
			}}
		if got := violations(d); len(got) != 1 {
			t.Fatalf("%s", describeIssues(got))
		}
		stepIdx(&d, "s").Tasks[1].After = nil // no longer judges it
		if got := violations(d); len(got) != 0 {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("two gates of a step that share one person", func(t *testing.T) {
		d := fileDoc(t, "crossreview.json", func(d *FlowDoc) { stepIdx(d, "review").Tasks[1].Role = "ana" })
		found := false
		for _, i := range violations(d) {
			found = found || i.Params["reason"] == "gates_share_role"
		}
		if !found {
			t.Fatal("two gates by one agent were accepted")
		}
	})
}

func TestValidate_Y01WorkAfterHumanReview(t *testing.T) {
	// plan, a person reviews (a gate for a member), an agent rewrites, release.
	build := func(rewriteStep DocStep) FlowDoc {
		return FlowDoc{Version: 2, Start: "plan",
			Roles:     []Role{agentRole("writer", 1), {Key: "editor", Kind: RoleKindMember}, agentRole("fixer", 1)},
			Terminals: []Terminal{{Key: "done", Status: "done", LeavesWorkspace: true}},
			Policy:    Policy{},
			Steps: []DocStep{
				{Key: "plan", Kind: StepKindLead, Status: StepStatus{Key: "todo"}, Edges: []DocEdge{{ID: "p", Label: "p", To: "draft"}}},
				{Key: "draft", Kind: StepKindTasks, Status: StepStatus{Key: "in_progress"}, Tasks: []DocTask{{Key: "write", Title: "w", Kind: KindWork, Role: "writer"}},
					Edges: []DocEdge{{ID: "d", Label: "d", To: "review"}}},
				{Key: "review", Kind: StepKindTasks, Status: StepStatus{Key: "in_review"}, Tasks: []DocTask{{Key: "read", Title: "r", Kind: KindGate, Role: "editor"}},
					Edges: []DocEdge{{ID: "r_ok", Label: "ok", To: "rewrite", When: cond("join.result", OpEq, "pass")}, {ID: "r_stop", Label: "stop", Stop: "stalled"}}},
				rewriteStep,
			}}
	}
	rewrite := DocStep{Key: "rewrite", Kind: StepKindTasks, Status: StepStatus{Key: "in_progress"},
		Tasks: []DocTask{{Key: "fix", Title: "f", Kind: KindWork, Role: "fixer"}}, Edges: []DocEdge{{ID: "rw", Label: "rw", To: "done"}}}

	t.Run("an agent rewrites after the person reviewed", func(t *testing.T) {
		issues := ValidateFlowDoc(build(rewrite), vctx())
		got := findIssue(issues, "Y01", "release_without_human")
		if got == nil || got.Path != "steps[rewrite]" || got.Severity != SeverityWarning || got.Ack != AckUnattendedRelease {
			t.Fatalf("expected a warning at steps[rewrite]:%s", describeIssues(issues))
		}
		// The first draft is covered by the review.
		for _, i := range issuesOfRule(issues, "Y01") {
			if i.Path == "steps[draft]" {
				t.Errorf("the draft is reviewed before it can leave: %s", describeIssues([]Issue{i}))
			}
		}
	})
	t.Run("agent work inside a manual step is covered by its exit", func(t *testing.T) {
		manual := DocStep{Key: "rewrite", Kind: StepKindManual, Status: StepStatus{Key: "in_review"}, ExitRole: "managers", Notice: "n",
			Tasks: []DocTask{{Key: "fix", Title: "f", Kind: KindWork, Role: "fixer"}},
			Edges: []DocEdge{{ID: "rw", Label: "rw", To: "done", Trigger: EdgeTriggerManual}}}
		d := build(manual)
		d.Messages = map[string]string{"n": "x"}
		if got := issuesOfRule(ValidateFlowDoc(d, vctx()), "Y01"); len(got) != 0 {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("an acknowledged exception stays visible but needs no ack", func(t *testing.T) {
		d := build(rewrite)
		d.Policy = Policy{AllowUnattendedRelease: true, Acks: []Ack{{Kind: AckUnattendedRelease, By: "admin"}}}
		issues := ValidateFlowDoc(d, vctx())
		got := findIssue(issues, "Y01", "release_without_human")
		if got == nil || got.Ack != "" || got.Params["acknowledged"] != true {
			t.Fatalf("%s", describeIssues(issues))
		}
		if HasErrors(issuesOfRule(issues, "Y04")) {
			t.Fatalf("the ack is there: %s", describeIssues(issuesOfRule(issues, "Y04")))
		}
	})
}

func TestValidate_Y01ReleasePaths(t *testing.T) {
	releases := func(d FlowDoc) []Issue { return issuesOfRule(ValidateFlowDoc(d, vctx()), "Y01") }

	t.Run("an agent's gate lets the work leave", func(t *testing.T) {
		got := releases(releaseDoc(false, "checker"))
		if len(got) != 1 {
			t.Fatalf("%s", describeIssues(got))
		}
		if path, _ := got[0].Params["path"].([]string); !reflect.DeepEqual(path, []string{"work", "check", "done"}) || got[0].Params["terminal"] != "done" {
			t.Errorf("params = %v", got[0].Params)
		}
	})
	t.Run("a gate for a person is a checkpoint", func(t *testing.T) {
		d := releaseDoc(false, "person")
		if got := releases(d); len(got) != 0 {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("the trade flow ends in a manual release", func(t *testing.T) {
		if got := releases(loadFlowDoc(t, "trade.json")); len(got) != 0 {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("a terminal that stays inside the workspace is not a release", func(t *testing.T) {
		d := releaseDoc(false, "checker")
		d.Terminals[0].LeavesWorkspace = false
		if got := releases(d); len(got) != 0 {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("a stop edge is not a way out", func(t *testing.T) {
		d := releaseDoc(false, "checker")
		stepIdx(&d, "check").Edges = []DocEdge{{ID: "check_stop", Label: "s", Stop: "stalled"}}
		if got := releases(d); len(got) != 0 {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("a checkpoint edge that does not require a pass is not one", func(t *testing.T) {
		d := releaseDoc(false, "person")
		stepIdx(&d, "check").Edges[0].When = nil
		if got := releases(d); len(got) != 1 {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("a gate for a person in a quorum is not a checkpoint", func(t *testing.T) {
		d := releaseDoc(false, "person")
		stepIdx(&d, "check").Join = &Join{Over: JoinOver{Kind: JoinOverGates}, Mode: JoinModeAny}
		if got := releases(d); len(got) != 1 {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("a gate for a person that may not open is not a checkpoint", func(t *testing.T) {
		d := releaseDoc(false, "person")
		stepIdx(&d, "check").Tasks[0].When = cond("var.flag", OpEq, true)
		if got := releases(d); len(got) != 1 {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("the validator and Analyze agree on the first path", func(t *testing.T) {
		d := releaseDoc(false, "checker")
		a := Analyze(d)
		if len(a.ReleasePathsWithoutHuman) == 0 || len(releases(d)) == 0 {
			t.Fatal("one of them found no path")
		}
	})
}

func TestValidate_J01QuorumCountsOnlyCertainInstances(t *testing.T) {
	quorum := func(edit func(t *DocTask), k int) []Issue {
		d := fileDoc(t, "legal.json", func(d *FlowDoc) {
			s := stepIdx(d, "review")
			s.Join.K = k
			edit(&s.Tasks[0])
		})
		return issuesOfRule(ValidateFlowDoc(d, vctx()), "J01")
	}
	// Five certain instances: k up to 5 is fine, 6 is not.
	if got := quorum(func(*DocTask) {}, 5); len(got) != 0 {
		t.Fatalf("k=5 of 5: %s", describeIssues(got))
	}
	if got := quorum(func(*DocTask) {}, 6); len(got) != 1 || got[0].Code != "join_invalid" || got[0].Params["certain"] != 5 {
		t.Fatalf("k=6 of 5: %s", describeIssues(got))
	}
	// A list gives at least its min.
	fromVar := func(task *DocTask) { task.Instances = &Instances{FromVar: "severity", Min: 2, Max: 5} }
	if got := quorum(fromVar, 2); len(got) != 0 {
		t.Fatalf("k=2 of at least 2: %s", describeIssues(got))
	}
	if got := quorum(fromVar, 3); len(got) != 1 || got[0].Params["certain"] != 2 {
		t.Fatalf("k=3 of at least 2: %s", describeIssues(got))
	}
	// A task with a condition may not open at all.
	conditional := func(task *DocTask) { task.When = cond("var.severity", OpEq, "major") }
	if got := quorum(conditional, 1); len(got) != 1 || got[0].Params["certain"] != 0 {
		t.Fatalf("a conditional task counts for none: %s", describeIssues(got))
	}
	if got := quorum(func(task *DocTask) {}, 0); len(got) != 1 {
		t.Fatalf("k=0: %s", describeIssues(got))
	}
	t.Run("the other parts of a join", func(t *testing.T) {
		bad := map[string]func(d *FlowDoc){
			"mode":  func(d *FlowDoc) { stepIdx(d, "review").Join.Mode = "most" },
			"block": func(d *FlowDoc) { stepIdx(d, "review").Join.Block = "later" },
			"over a task": func(d *FlowDoc) {
				stepIdx(d, "review").Join.Over = JoinOver{Kind: JoinOverTasks, Tasks: []string{"ghost"}}
			},
			"over nothing": func(d *FlowDoc) { stepIdx(d, "review").Join.Over = JoinOver{Kind: JoinOverTasks, Tasks: []string{}} },
			"on a lead step": func(d *FlowDoc) {
				stepIdx(d, "plan").Join = &Join{}
			},
		}
		for name, edit := range bad {
			if len(issuesOfRule(ValidateFlowDoc(fileDoc(t, "legal.json", edit), vctx()), "J01")) == 0 {
				t.Errorf("%s: accepted", name)
			}
		}
		if got := issuesOfRule(ValidateFlowDoc(loadFlowDoc(t, "legal.json"), vctx()), "J01"); len(got) != 0 {
			t.Errorf("legal: %s", describeIssues(got))
		}
	})
}

func TestValidate_J02NeutralNeedsNoneRoute(t *testing.T) {
	// A gate whose verdicts include an abstention can end with no verdict: result none.
	d := func(edges []DocEdge) FlowDoc {
		return FlowDoc{Version: 2, Start: "plan",
			Roles:       []Role{agentRole("w", 1), agentRole("g", 1)},
			VerdictSets: []VerdictSet{{Key: "vote", Verdicts: []Verdict{{Key: "yes", Class: ClassPass}, {Key: "no", Class: ClassFail}, {Key: "abstain", Class: ClassNeutral}}}},
			Terminals:   []Terminal{{Key: "done", Status: "done"}},
			Steps: []DocStep{
				{Key: "plan", Kind: StepKindLead, Status: StepStatus{Key: "todo"}, Edges: []DocEdge{{ID: "p", Label: "p", To: "vote"}}},
				{Key: "vote", Kind: StepKindTasks, Status: StepStatus{Key: "in_progress"}, Tasks: []DocTask{{Key: "judge", Title: "j", Kind: KindGate, Role: "g", Verdicts: "vote"}}, Edges: edges},
			}}
	}
	res := func(v string) *Condition { return cond("join.result", OpEq, v) }
	all := []DocEdge{{ID: "ok", Label: "ok", To: "done", When: res("pass")}, {ID: "no", Label: "no", Stop: "stalled", When: res("fail")}, {ID: "bl", Label: "bl", Stop: "stalled", When: res("block")}}

	got := issuesOfRule(ValidateFlowDoc(d(all), vctx()), "J02")
	if len(got) != 1 || got[0].Code != "join_result_unrouted" || !reflect.DeepEqual(got[0].Params["unrouted"], []string{"none"}) {
		t.Fatalf("pass, fail and block routed, none not: %s", describeIssues(got))
	}
	withNone := append(append([]DocEdge(nil), all...), DocEdge{ID: "nn", Label: "nn", Stop: "stalled", When: res("none")})
	if got := issuesOfRule(ValidateFlowDoc(d(withNone), vctx()), "J02"); len(got) != 0 {
		t.Fatalf("with a none edge: %s", describeIssues(got))
	}
	withDefault := append(append([]DocEdge(nil), all...), DocEdge{ID: "dd", Label: "dd", Stop: "stalled"})
	if got := issuesOfRule(ValidateFlowDoc(d(withDefault), vctx()), "J02"); len(got) != 0 {
		t.Fatalf("with a default edge: %s", describeIssues(got))
	}
	// Without an abstention in the set, none is not a result.
	plain := d(all)
	plain.VerdictSets = nil
	stepIdx(&plain, "vote").Tasks[0].Verdicts = ""
	if got := issuesOfRule(ValidateFlowDoc(plain, vctx()), "J02"); len(got) != 0 {
		t.Fatalf("a plain gate needs no none route: %s", describeIssues(got))
	}
	// A task that may not open can leave the join empty.
	cond1 := plain
	cond1 = modify(cond1, func(d *FlowDoc) { stepIdx(d, "vote").Tasks[0].When = cond("var.flag", OpEq, true) })
	if got := issuesOfRule(ValidateFlowDoc(cond1, vctx()), "J02"); len(got) != 1 {
		t.Fatalf("a conditional gate can leave the join empty: %s", describeIssues(got))
	}
	// Edges that need more than the result do not cover it.
	partial := []DocEdge{{ID: "ok", Label: "ok", To: "done", When: &Condition{All: []Condition{*res("pass"), *cond("var.flag", OpEq, true)}}}, {ID: "f", Label: "f", Stop: "stalled", When: res("fail")}, {ID: "b", Label: "b", Stop: "stalled", When: res("block")}}
	plain2 := modify(plain, func(d *FlowDoc) { stepIdx(d, "vote").Edges = partial })
	if got := issuesOfRule(ValidateFlowDoc(plain2, vctx()), "J02"); len(got) != 1 || !reflect.DeepEqual(got[0].Params["unrouted"], []string{"pass"}) {
		t.Fatalf("pass only with a flag: %s", describeIssues(got))
	}
	// A step with no gate: only none and block.
	if got := issuesOfRule(ValidateFlowDoc(loadFlowDoc(t, "trade.json"), vctx()), "J02"); len(got) != 0 {
		t.Fatalf("trade: %s", describeIssues(got))
	}
}

func TestValidate_R09SkillNotAttached(t *testing.T) {
	doc := func(skills []string, bind ...string) FlowDoc {
		d := roleDoc([]Role{agentRole("writer", 2, bind...), agentRole("checker", 1)}, "writer", "checker", nil)
		stepIdx(&d, "work").Tasks[0].Skills = skills
		return d
	}
	ctx := vctx()
	ctx.Publish = true
	ctx.Roster = Roster{Agents: []string{"a1", "a2"}, AgentSkills: map[string][]string{"a1": {"drafting", "tax"}, "a2": {"drafting"}}}
	run := func(d FlowDoc, c ValidateContext) []Issue { return issuesOfRule(ValidateFlowDoc(d, c), "R09") }

	if got := run(doc([]string{"drafting"}, "a1", "a2"), ctx); len(got) != 0 {
		t.Fatalf("both agents have drafting: %s", describeIssues(got))
	}
	got := run(doc([]string{"drafting", "tax"}, "a1", "a2"), ctx)
	if len(got) != 1 || got[0].Code != "skill_not_attached" || got[0].Params["agent_id"] != "a2" || got[0].Params["skill"] != "tax" ||
		got[0].Path != "steps[work].tasks[make].skills" {
		t.Fatalf("a2 lacks tax: %s", describeIssues(got))
	}
	// Only at publish.
	draft := ctx
	draft.Publish = false
	if got := run(doc([]string{"tax"}, "a1", "a2"), draft); len(got) != 0 {
		t.Fatalf("a draft is not checked: %s", describeIssues(got))
	}
	// A role that is a person has no skills.
	d := doc([]string{"drafting"})
	d.Roles[0] = Role{Key: "writer", Kind: RoleKindMember, Pool: 1}
	if got := run(d, ctx); len(got) != 1 || got[0].Params["reason"] != "human_role" {
		t.Fatalf("a member with skills: %s", describeIssues(got))
	}
	// An unbound agent role has nobody to check yet.
	if got := run(doc([]string{"tax"}), ctx); len(got) != 0 {
		t.Fatalf("%s", describeIssues(got))
	}
}

// branchDoc is plan, then a or b, then merge, then the end. a and b each have a
// task, and merge has a task that reads from the task of a step given.
func branchDoc(from string, edit func(merge *DocTask)) FlowDoc {
	d := FlowDoc{Version: 2, Start: "plan",
		Roles:     []Role{agentRole("w", 1)},
		Vars:      []Var{{Key: "pick", Type: VarTypeBool, Default: false, SetBy: []string{"lead"}, SetAt: []string{"plan"}}},
		Terminals: []Terminal{{Key: "done", Status: "done"}},
		Steps: []DocStep{
			{Key: "plan", Kind: StepKindLead, Status: StepStatus{Key: "todo"}, Lead: &StepLead{Sets: []string{"pick"}},
				Edges: []DocEdge{{ID: "to_a", Label: "a", To: "a", When: cond("var.pick", OpEq, true)}, {ID: "to_b", Label: "b", To: "b"}}},
			{Key: "a", Kind: StepKindTasks, Status: StepStatus{Key: "in_progress"}, Tasks: []DocTask{{Key: "do_a", Title: "a", Kind: KindWork, Role: "w", Requires: &Requires{Files: []FileReq{{Slot: "out", Min: 1}}}}},
				Edges: []DocEdge{{ID: "a1", Label: "a", To: "merge"}}},
			{Key: "b", Kind: StepKindTasks, Status: StepStatus{Key: "in_progress"}, Tasks: []DocTask{{Key: "do_b", Title: "b", Kind: KindWork, Role: "w"}},
				Edges: []DocEdge{{ID: "b1", Label: "b", To: "merge"}}},
			{Key: "merge", Kind: StepKindTasks, Status: StepStatus{Key: "in_progress"}, Tasks: []DocTask{{Key: "wrap", Title: "w", Kind: KindWork, Role: "w"}},
				Edges: []DocEdge{{ID: "m1", Label: "m", To: "done"}}},
		}}
	task := &d.Steps[3].Tasks[0]
	task.Inputs = []Input{{From: from}}
	if edit != nil {
		edit(task)
	}
	return d
}

func TestValidate_T07InputsAndMaybeMissing(t *testing.T) {
	run := func(d FlowDoc) []Issue { return issuesOfRule(ValidateFlowDoc(d, vctx()), "T07") }

	t.Run("an input from a step before is valid", func(t *testing.T) {
		if got := run(loadFlowDoc(t, "crossreview.json")); len(got) != 0 {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	bad := map[string]string{
		"not a task":                     "a.nothing",
		"no step":                        "ghost.do_a",
		"no dot":                         "do_a",
		"a later step":                   "merge.wrap",
		"the same step":                  "merge.wrap",
		"a step that does not lead here": "b.do_b2",
	}
	for name, from := range bad {
		t.Run("invalid: "+name, func(t *testing.T) {
			got := run(branchDoc(from, nil))
			if len(got) == 0 || got[0].Code != "input_invalid" {
				t.Fatalf("%s: %s", from, describeIssues(got))
			}
			if !strings.HasPrefix(got[0].Path, "steps[merge].tasks[wrap].inputs[") {
				t.Errorf("path %q", got[0].Path)
			}
		})
	}
	t.Run("invalid: a step that is not before", func(t *testing.T) {
		d := branchDoc("a.do_a", nil)
		// b does not lead to a.
		stepIdx(&d, "b").Tasks[0].Inputs = []Input{{From: "a.do_a"}}
		got := run(d)
		if len(got) == 0 || got[0].Code != "input_invalid" || !strings.HasPrefix(got[0].Path, "steps[b].tasks[do_b]") {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("a slot the source does not declare", func(t *testing.T) {
		got := run(branchDoc("a.do_a", func(task *DocTask) { task.Inputs[0].Slot = "other" }))
		if len(got) == 0 || got[0].Code != "input_invalid" || got[0].Params["slot"] != "other" {
			t.Fatalf("%s", describeIssues(got))
		}
		if got := run(branchDoc("a.do_a", func(task *DocTask) { task.Inputs[0].Slot = "out" })); len(got) != 1 || got[0].Code != "input_maybe_missing" {
			t.Fatalf("a declared slot is valid (only the maybe-missing remains): %s", describeIssues(got))
		}
	})
	t.Run("a source on only one of the paths is a warning, or an error when required", func(t *testing.T) {
		got := run(branchDoc("a.do_a", nil))
		if len(got) != 1 || got[0].Code != "input_maybe_missing" || got[0].Severity != SeverityWarning || got[0].Params["required"] != false {
			t.Fatalf("%s", describeIssues(got))
		}
		got = run(branchDoc("a.do_a", func(task *DocTask) { task.Inputs[0].Required = true }))
		if len(got) != 1 || got[0].Code != "input_maybe_missing" || got[0].Severity != SeverityError {
			t.Fatalf("required: %s", describeIssues(got))
		}
	})
	t.Run("a source on every path is not maybe missing", func(t *testing.T) {
		d := branchDoc("do_a", nil)
		d = modify(d, func(d *FlowDoc) {
			// plan -> a -> merge only.
			stepIdx(d, "plan").Edges = []DocEdge{{ID: "to_a", Label: "a", To: "a"}}
			stepIdx(d, "merge").Tasks[0].Inputs = []Input{{From: "a.do_a", Required: true}}
			stepIdx(d, "b").Edges = []DocEdge{{ID: "b1", Label: "b", To: "merge"}}
		})
		if got := run(d); len(got) != 0 {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("a source with a condition may not open", func(t *testing.T) {
		d := modify(branchDoc("a.do_a", nil), func(d *FlowDoc) {
			stepIdx(d, "plan").Edges = []DocEdge{{ID: "to_a", Label: "a", To: "a"}}
			stepIdx(d, "a").Tasks[0].When = cond("var.pick", OpEq, true)
		})
		if got := run(d); len(got) != 1 || got[0].Code != "input_maybe_missing" {
			t.Fatalf("%s", describeIssues(got))
		}
	})
}

func TestValidate_T08VarsAndMaybeUnset(t *testing.T) {
	run := func(d FlowDoc) []Issue { return issuesOfRule(ValidateFlowDoc(d, vctx()), "T08") }
	if got := run(loadFlowDoc(t, "planner_fanout.json")); len(got) != 0 {
		t.Fatalf("planner: %s", describeIssues(got))
	}
	cases := map[string]func(d *FlowDoc){
		"unknown variable":   func(d *FlowDoc) { stepIdx(d, "planning").Tasks[0].Sets = []string{"ghost"} },
		"role not in set_by": func(d *FlowDoc) { d.Vars[0].SetBy = []string{"lead"} },
		"step not in set_at": func(d *FlowDoc) { d.Vars[0].SetAt = []string{"check"} },
		"requires without sets": func(d *FlowDoc) {
			stepIdx(d, "planning").Tasks[0].Sets = nil
		},
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			got := run(fileDoc(t, "planner_fanout.json", edit))
			if len(got) == 0 || got[0].Code != "var_set_invalid" || !strings.HasPrefix(got[0].Path, "steps[planning].tasks[make_plan]") {
				t.Fatalf("%s", describeIssues(got))
			}
		})
	}
	t.Run("set_at any step", func(t *testing.T) {
		got := run(fileDoc(t, "planner_fanout.json", func(d *FlowDoc) { d.Vars[0].SetAt = []string{"*"} }))
		if len(got) != 0 {
			t.Fatalf("%s", describeIssues(got))
		}
	})

	// A variable with no default that a step reads.
	noDefault := func(setter func(d *FlowDoc)) FlowDoc {
		return fileDoc(t, "planner_fanout.json", func(d *FlowDoc) {
			d.Vars[0].Default = nil
			setter(d)
		})
	}
	t.Run("set by a task before the step on every path", func(t *testing.T) {
		if got := run(noDefault(func(d *FlowDoc) {})); len(got) != 0 {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("nothing sets it before", func(t *testing.T) {
		got := run(noDefault(func(d *FlowDoc) {
			stepIdx(d, "planning").Tasks[0].Sets = nil
			stepIdx(d, "planning").Tasks[0].Requires = nil
		}))
		if len(got) != 1 || got[0].Code != "var_maybe_unset" || got[0].Severity != SeverityWarning || got[0].Path != "steps[fanout].tasks[research_topic]" {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("set on only one path", func(t *testing.T) {
		got := run(noDefault(func(d *FlowDoc) {
			// plan -> skip (past planning) or planning, both lead to fanout.
			stepIdx(d, "plan").Edges = []DocEdge{
				{ID: "skip", Label: "skip", To: "fanout", When: cond("var.topics", OpExists, nil)},
				{ID: "plan_ok", Label: "ok", To: "planning"}}
		}))
		found := false
		for _, i := range got {
			found = found || i.Code == "var_maybe_unset"
		}
		if !found {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("a variable with a default is set", func(t *testing.T) {
		got := run(fileDoc(t, "planner_fanout.json", func(d *FlowDoc) {
			stepIdx(d, "planning").Tasks[0].Sets = nil
			stepIdx(d, "planning").Tasks[0].Requires = nil
		}))
		if len(got) != 0 {
			t.Fatalf("%s", describeIssues(got))
		}
	})
	t.Run("a lead that sets it", func(t *testing.T) {
		got := run(fileDoc(t, "patterns/p06_instances_from_var.json", func(d *FlowDoc) { d.Vars[0].Default = nil }))
		if len(got) != 0 {
			t.Fatalf("p06 sets items in the lead step: %s", describeIssues(got))
		}
	})
}

// A document of every pattern validates without an error.
func TestPatterns_ValidateClean(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "flows", "patterns", "*.json"))
	if len(files) != 12 {
		t.Fatalf("%d patterns", len(files))
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			issues := ValidateFlowDocJSON(readFile(t, f), vctx())
			for _, i := range issues {
				if i.Severity == SeverityError {
					t.Fatalf("an error:%s", describeIssues(issues))
				}
			}
		})
	}
}

// The sample flows are valid too.
func TestValidate_SampleFlowsClean(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "flows", "*.json"))
	if len(files) != 6 {
		t.Fatalf("%d sample flows", len(files))
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			issues := ValidateFlowDocJSON(readFile(t, f), vctx())
			if HasErrors(issues) {
				t.Fatalf("an error:%s", describeIssues(issues))
			}
		})
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func minus(a, b []string) []string {
	in := map[string]bool{}
	for _, x := range b {
		in[x] = true
	}
	var out []string
	for _, x := range a {
		if !in[x] {
			out = append(out, x)
		}
	}
	return out
}

// The gate of plan step P9: the role rules the catalog tool applies hold for all
// 21 Kit Viet flows on the rosters of their squads, with no manifest changed.
func TestValidateSchema1ForSquad_AllCatalogFlowsPass(t *testing.T) {
	flows := loadCatalogFlows(t)
	if len(flows) == 0 {
		t.Skip("the catalog is not built")
	}
	ids := make([]string, 0, len(flows))
	for id := range flows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	warnings := map[string]int{}
	checked := 0
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			f := flows[id]
			def, _, err := f.definition()
			if err != nil {
				t.Fatal(err)
			}
			issues := ValidateSchema1ForSquad(def, squadRoster(t, f))
			checked++
			for _, i := range issues {
				if i.Severity == SeverityWarning {
					warnings[i.Code]++
				}
			}
			if HasErrors(issues) {
				t.Fatalf("%s:%s", id, describeIssues(issues))
			}
		})
	}
	if checked != 21 {
		t.Errorf("%d flows checked, want 21", checked)
	}
	t.Logf("%d flows, warnings by code: %v", checked, warnings)

	// The same flows read as drafts (a fork) and checked by all of section 3, as a
	// workflow row holds them. TestLift_ForkDocIsCleanV2 is the gate for them; this
	// logs what they still warn about.
	t.Run("forks", func(t *testing.T) {
		counts := map[string]int{}
		for _, id := range ids {
			def, ctx := forkInputs(t, flows[id])
			g, err := Lift(def, LiftOpts{RoleKeys: ctx.Roster.RoleKeys})
			if err != nil {
				t.Fatal(err)
			}
			for _, i := range ValidateFlowDoc(g.Doc, ctx) {
				counts[fmt.Sprintf("%s %s %s", i.Severity, ruleOf(i), i.Code)]++
			}
		}
		keys := make([]string, 0, len(counts))
		for k := range counts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			t.Logf("fork documents: %-50s %d", k, counts[k])
		}
	})
}

func TestValidateSchema1ForSquad_FindsWhatItShould(t *testing.T) {
	def := tradeFlow()
	// The agents of tradeFlow in order of first appearance.
	agents := []string{agentDocs, agentCustoms, agentDrafter, agentDocsGate, agentCustomsGate, agentHandover}
	roster := Roster{Lead: "lead", Agents: append([]string(nil), agents...)}
	if issues := ValidateSchema1ForSquad(def, roster); len(issues) != 0 {
		t.Fatalf("the trade flow with its own roster:%s", describeIssues(issues))
	}
	cases := map[string]struct {
		roster Roster
		def    func(d *Definition)
		rule   string
		code   string
	}{
		"the lead takes a task":        {Roster{Lead: agentDocs, Agents: agents[1:]}, nil, "R03", "role_is_lead"},
		"an agent is not in the squad": {Roster{Lead: "lead", Agents: agents[1:]}, nil, "R03", "role_not_member"},
		"an agent is archived":         {Roster{Lead: "lead", Agents: agents, Archived: map[string]bool{agentDocs: true}}, nil, "R03", "role_not_member"},
		"a squad agent has no task":    {Roster{Lead: "lead", Agents: append(append([]string(nil), agents...), "idle")}, nil, "R04", "role_unused"},
		"a gate by the agent who drafted": {roster, func(d *Definition) {
			d.Steps[3].Tasks[0].Handler = agentHandler(agentDrafter)
		}, "R07", "separation_violation"},
		"two gates by one agent": {roster, func(d *Definition) {
			d.Steps[3].Tasks[1].Handler = agentHandler(agentDocsGate)
		}, "R07", "separation_violation"},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			d := tradeFlow()
			if tt.def != nil {
				tt.def(&d)
			}
			issues := ValidateSchema1ForSquad(d, tt.roster)
			if findIssue(issues, tt.rule, tt.code) == nil {
				t.Fatalf("no %s %s in:%s", tt.rule, tt.code, describeIssues(issues))
			}
		})
	}
	t.Run("a definition that cannot be read", func(t *testing.T) {
		d := tradeFlow()
		d.Steps[1].Tasks[1].When = cond(FieldPriority, OpEq, "high")
		issues := ValidateSchema1ForSquad(d, roster)
		if len(issues) != 1 || issues[0].Code != "schema1_not_liftable" || issues[0].Severity != SeverityError {
			t.Fatalf("%s", describeIssues(issues))
		}
	})
	t.Run("it does not change the definition", func(t *testing.T) {
		d := tradeFlow()
		before, _ := json.Marshal(d)
		ValidateSchema1ForSquad(d, roster)
		if after, _ := json.Marshal(d); string(before) != string(after) {
			t.Fatal("the definition changed")
		}
	})
}
