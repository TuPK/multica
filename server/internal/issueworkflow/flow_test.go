package issueworkflow

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// Tests of the flow model: step tasks, flow_config, ValidateFlow and Decide
// (docs/engineering/kitviet-workflow-spec.md sections 3 and 4). They build the
// Trade flow by hand; the shipped catalog flows are checked in
// flow_catalog_test.go. None of them touches a database.

const (
	agentDocs        = "00000000-0000-4000-8000-000000000001"
	agentCustoms     = "00000000-0000-4000-8000-000000000002"
	agentDrafter     = "00000000-0000-4000-8000-000000000003"
	agentDocsGate    = "00000000-0000-4000-8000-000000000004"
	agentCustomsGate = "00000000-0000-4000-8000-000000000005"
	agentHandover    = "00000000-0000-4000-8000-000000000006"
)

func agentHandler(id string) Handler { return Handler{Type: HandlerAgent, ID: id} }

func tr(id string, when *Condition, to string, rework bool) Transition {
	return Transition{ID: id, Label: id, When: when, ToStatusKey: to, Rework: rework}
}

func tasksIs(v string) *Condition { return cond(FieldTasksOutcome, OpEq, v) }

func reworkBelow(n int) *Condition { return cond(FieldReworkCount, OpLt, n) }

// tradeFlow is the Trade flow of spec section 3.6.
func tradeFlow() Definition {
	cfg := &FlowConfig{
		Version: 1,
		Switches: []Switch{
			{Key: "route", Type: SwitchEnum, Values: []string{"shipment_dossier"}, Default: "shipment_dossier"},
			{Key: "customs_lookup", Type: SwitchBool, Default: false},
			{Key: "export", Type: SwitchBool, Default: false},
		},
	}
	steps := []Step{
		{StatusKey: "todo", Handler: Handler{Type: HandlerNone}, Transitions: []Transition{
			tr("blocked", cond(FieldOutcome, OpEq, OutcomeBlocked), "blocked", false),
			tr("default", nil, "collect", false),
		}},
		{StatusKey: "collect", Handler: Handler{Type: HandlerNone},
			Tasks: []Task{
				{Key: "docs", Title: "Read the documents", Kind: KindWork, Handler: agentHandler(agentDocs), Instructions: "Read every document."},
				{Key: "customs", Title: "Customs lookup", Kind: KindWork, Handler: agentHandler(agentCustoms), Instructions: "Look up customs.",
					When: cond(SwitchField("customs_lookup"), OpEq, true)},
			},
			Transitions: []Transition{
				tr("blocked", tasksIs(TasksAnyBlocked), "blocked", false),
				tr("default", nil, "draft", false),
			}},
		{StatusKey: "draft", Handler: Handler{Type: HandlerNone},
			Tasks: []Task{
				{Key: "crosscheck", Title: "Draft the cross-check", Kind: KindWork, Handler: agentHandler(agentDrafter), Instructions: "Draft it.",
					Requires: []string{RequireAttachment}},
			},
			Transitions: []Transition{
				tr("blocked", tasksIs(TasksAnyBlocked), "blocked", false),
				tr("default", nil, "gate", false),
			}},
		{StatusKey: "gate", Handler: Handler{Type: HandlerNone},
			Tasks: []Task{
				{Key: "docs_gate", Title: "Documents gate", Kind: KindGate, Handler: agentHandler(agentDocsGate), Instructions: "Check the documents.", Requires: []string{RequireOutcome}},
				{Key: "customs_gate", Title: "Customs gate", Kind: KindGate, Handler: agentHandler(agentCustomsGate), Instructions: "Check customs.", Requires: []string{RequireOutcome}},
			},
			Transitions: []Transition{
				tr("blocked", tasksIs(TasksAnyBlocked), "blocked", false),
				tr("pass", tasksIs(TasksAllApproved), "handover", false),
				tr("redo", &Condition{All: []Condition{*tasksIs(TasksAnyRejected), *reworkBelow(2)}}, "draft", true),
				tr("default", nil, "blocked", false),
			}},
		{StatusKey: "handover", Handler: Handler{Type: HandlerNone},
			Tasks: []Task{
				{Key: "handover", Title: "Handover", Kind: KindGate, Handler: agentHandler(agentHandover), Instructions: "Hand over.", Requires: []string{RequireOutcome}},
			},
			Transitions: []Transition{
				tr("pass", tasksIs(TasksAllApproved), "in_review", false),
				tr("default", nil, "blocked", false),
			}},
		{StatusKey: "in_review", Handler: Handler{Type: HandlerNone}, HumanOnly: true,
			Tasks: []Task{
				{Key: "export", Title: "Export", Kind: KindWork, Handler: agentHandler(agentDrafter), Instructions: "Export.",
					When: cond(SwitchField("export"), OpEq, true)},
			}},
		{StatusKey: "blocked", Handler: Handler{Type: HandlerNone}},
		{StatusKey: "done", Handler: Handler{Type: HandlerNone}},
		{StatusKey: "cancelled", Handler: Handler{Type: HandlerNone}},
	}
	return Definition{Name: "Trade", Steps: steps, Flow: cfg}
}

func stepIndex(t *testing.T, def *Definition, key string) int {
	t.Helper()
	for i, s := range def.Steps {
		if s.StatusKey == key {
			return i
		}
	}
	t.Fatalf("no step %q", key)
	return -1
}

func TestDecodeStepsLegacyUnchanged(t *testing.T) {
	raw := []byte(`[{"status_key":"backlog"},` +
		`{"status_key":"in_progress","handler":{"type":"agent","id":"` + testAgentID + `"},"instructions":"Build it.","next_status_key":"in_review","back_status_key":"todo"},` +
		`{"status_key":"in_review","transitions":[{"id":"default","label":"Default","when":null,"to_status_key":"done"}]},` +
		`{"status_key":"done"}]`)
	steps, err := DecodeSteps(raw)
	if err != nil {
		t.Fatalf("DecodeSteps: %v", err)
	}
	for _, s := range steps {
		if len(s.Tasks) != 0 || s.HumanOnly {
			t.Fatalf("legacy step %q decoded with flow fields: %+v", s.StatusKey, s)
		}
	}
	encoded, err := EncodeSteps(steps)
	if err != nil {
		t.Fatalf("EncodeSteps: %v", err)
	}
	for _, key := range []string{`"tasks"`, `"human_only"`, `"rework"`} {
		if strings.Contains(string(encoded), key) {
			t.Fatalf("legacy steps re-encode with %s: %s", key, encoded)
		}
	}
	again, err := DecodeSteps(encoded)
	if err != nil || !reflect.DeepEqual(again, steps) {
		t.Fatalf("decode after encode = %+v, %v; want %+v", again, err, steps)
	}
	second, err := EncodeSteps(again)
	if err != nil || string(second) != string(encoded) {
		t.Fatalf("second encode = %s, %v; want %s", second, err, encoded)
	}
}

func TestEncodeStepsRoundTripsTasksAndHumanOnly(t *testing.T) {
	def := tradeFlow()
	raw, err := EncodeSteps(def.Steps)
	if err != nil {
		t.Fatalf("EncodeSteps: %v", err)
	}
	got, err := DecodeSteps(raw)
	if err != nil {
		t.Fatalf("DecodeSteps: %v", err)
	}
	// JSON decodes condition numbers and lists to their own types; compare the
	// second encoding, which is what a stored row holds.
	again, err := EncodeSteps(got)
	if err != nil || string(again) != string(raw) {
		t.Fatalf("round trip changed the steps:\n got %s\nwant %s (err %v)", again, raw, err)
	}
	collect := got[stepIndex(t, &def, "collect")]
	if len(collect.Tasks) != 2 || collect.Tasks[1].Key != "customs" || collect.Tasks[1].When == nil {
		t.Fatalf("collect tasks = %+v", collect.Tasks)
	}
	if !got[stepIndex(t, &def, "in_review")].HumanOnly {
		t.Fatal("human_only was lost")
	}
	if !got[stepIndex(t, &def, "gate")].Transitions[2].Rework {
		t.Fatal("the rework flag was lost")
	}
}

func TestParseFlowConfig(t *testing.T) {
	cfg, err := ParseFlowConfig([]byte(`{"version":1,"switches":[{"key":"x","type":"bool","default":false}],"messages":{"stop1":"S1"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TaskTimeout() != 120 || cfg.TaskHardCap() != 240 || cfg.TaskAttempts() != 2 {
		t.Fatalf("defaults = %d/%d/%d, want 120/240/2", cfg.TaskTimeout(), cfg.TaskHardCap(), cfg.TaskAttempts())
	}
	if cfg.Messages.Stop1 != "S1" || len(cfg.Switches) != 1 {
		t.Fatalf("config = %+v", cfg)
	}
	if _, err := ParseFlowConfig([]byte(`{"version":`)); err == nil {
		t.Fatal("malformed flow_config parsed")
	}
	if got, err := ParseFlowConfig(nil); got != nil || err != nil {
		t.Fatalf("ParseFlowConfig(nil) = %v, %v; want nil, nil", got, err)
	}
}

func TestValidateFlow(t *testing.T) {
	if err := ValidateFlow(tradeFlow()); err != nil {
		t.Fatalf("the Trade flow is invalid: %v", err)
	}
	tests := map[string]struct {
		mutate  func(d *Definition)
		wantErr string
	}{
		"duplicate task key": {func(d *Definition) {
			s := &d.Steps[stepIndex(t, d, "collect")]
			s.Tasks[1].Key = "docs"
		}, "more than once"},
		"task key shape": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "collect")].Tasks[0].Key = "Docs"
		}, "task key"},
		"task title too long": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "collect")].Tasks[0].Title = strings.Repeat("t", MaxTaskTitleRunes+1)
		}, "title"},
		"task instructions too long": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "collect")].Tasks[0].Instructions = strings.Repeat("i", MaxInstructionsRunes+1)
		}, "instructions"},
		"unknown task kind": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "collect")].Tasks[0].Kind = "review"
		}, "kind"},
		"unknown requirement": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "collect")].Tasks[0].Requires = []string{"screenshot"}
		}, "requires"},
		"gate without requires outcome": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "gate")].Tasks[0].Requires = nil
		}, "outcome"},
		"task handler is not an agent": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "collect")].Tasks[0].Handler = Handler{Type: HandlerMember, ID: agentDocs}
		}, "agent"},
		"task agent without id": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "collect")].Tasks[0].Handler = Handler{Type: HandlerAgent}
		}, "agent"},
		"two gates with the same agent": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "gate")].Tasks[1].Handler = agentHandler(agentDocsGate)
		}, "same agent"},
		"gate reuses the agent of the previous work task": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "gate")].Tasks[0].Handler = agentHandler(agentDrafter)
		}, "previous step"},
		"undeclared switch in a task condition": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "collect")].Tasks[1].When = cond(SwitchField("web"), OpEq, true)
		}, "not declared"},
		"undeclared switch in a transition": {func(d *Definition) {
			s := &d.Steps[stepIndex(t, d, "collect")]
			s.Transitions = append([]Transition{tr("web", cond(SwitchField("web"), OpEq, true), "blocked", false)}, s.Transitions...)
		}, "not declared"},
		"enum value outside the declared values": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "collect")].Tasks[1].When = cond(SwitchField("route"), OpEq, "other")
		}, "not one of"},
		"bool switch compared with a string": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "collect")].Tasks[1].When = cond(SwitchField("customs_lookup"), OpEq, "yes")
		}, "bool"},
		"switch declared twice": {func(d *Definition) {
			d.Flow.Switches = append(d.Flow.Switches, Switch{Key: "export", Type: SwitchBool, Default: false})
		}, "more than once"},
		"enum switch without values": {func(d *Definition) {
			d.Flow.Switches[0].Values = nil
		}, "values"},
		"enum default outside the values": {func(d *Definition) {
			d.Flow.Switches[0].Default = "other"
		}, "default"},
		"bool default is not a bool": {func(d *Definition) {
			d.Flow.Switches[1].Default = "no"
		}, "default"},
		"unknown switch type": {func(d *Definition) {
			d.Flow.Switches[1].Type = "number"
		}, "type"},
		"switch key shape": {func(d *Definition) {
			d.Flow.Switches[1].Key = "Customs"
		}, "switch key"},
		"nine switches": {func(d *Definition) {
			for i := 0; i < 6; i++ {
				d.Flow.Switches = append(d.Flow.Switches, Switch{Key: "extra_" + string(rune('a'+i)), Type: SwitchBool, Default: false})
			}
		}, "at most 8 switches"},
		"nine tasks in a step": {func(d *Definition) {
			s := &d.Steps[stepIndex(t, d, "collect")]
			for i := 0; i < 7; i++ {
				s.Tasks = append(s.Tasks, Task{Key: "extra_" + string(rune('a'+i)), Title: "x", Kind: KindWork, Handler: agentHandler(agentDocs), Instructions: "x"})
			}
		}, "at most 8 tasks"},
		"thirteen steps": {func(d *Definition) {
			for i := 0; i < 4; i++ {
				d.Steps = append(d.Steps, Step{StatusKey: "extra_" + string(rune('a'+i)), Handler: Handler{Type: HandlerNone}})
			}
		}, "at most 12"},
		"a plan step with tasks": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "todo")].Tasks = []Task{{Key: "plan", Title: "x", Kind: KindWork, Handler: agentHandler(agentDocs), Instructions: "x"}}
		}, "plan step"},
		"a plan step without a default transition": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "todo")].Transitions = d.Steps[stepIndex(t, d, "todo")].Transitions[:1]
		}, "default"},
		"a step with tasks and no transitions": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "draft")].Transitions = nil
		}, "transitions"},
		"a step with tasks and no default transition": {func(d *Definition) {
			s := &d.Steps[stepIndex(t, d, "draft")]
			s.Transitions = s.Transitions[:1]
		}, "default"},
		"a gate step that can never pass": {func(d *Definition) {
			s := &d.Steps[stepIndex(t, d, "gate")]
			s.Transitions = []Transition{s.Transitions[0], s.Transitions[3]}
		}, "all_approved"},
		"a cycle without rework": {func(d *Definition) {
			s := &d.Steps[stepIndex(t, d, "gate")]
			s.Transitions[2].Rework = false
		}, "cycle"},
		"rework to a later step": {func(d *Definition) {
			s := &d.Steps[stepIndex(t, d, "draft")]
			s.Transitions = append([]Transition{tr("later", tasksIs(TasksAnyRejected), "handover", true)}, s.Transitions...)
		}, "earlier step"},
		"rework without a limit": {func(d *Definition) {
			s := &d.Steps[stepIndex(t, d, "gate")]
			s.Transitions[2].When = tasksIs(TasksAnyRejected)
		}, "rework_count"},
		"rework limit above five": {func(d *Definition) {
			s := &d.Steps[stepIndex(t, d, "gate")]
			s.Transitions[2].When = &Condition{All: []Condition{*tasksIs(TasksAnyRejected), *reworkBelow(6)}}
		}, "at most 5"},
		"a human-only step with a required agent task": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "in_review")].Tasks[0].When = nil
		}, "human"},
		"a human-only step with a task other than export": {func(d *Definition) {
			d.Steps[stepIndex(t, d, "in_review")].Tasks[0].Key = "report"
		}, "human"},
		"flow without any task": {func(d *Definition) {
			for i := range d.Steps {
				d.Steps[i].Tasks = nil
				d.Steps[i].HumanOnly = false
			}
		}, "at least one"},
		"a transition target outside the workflow": {func(d *Definition) {
			s := &d.Steps[stepIndex(t, d, "collect")]
			s.Transitions[1].ToStatusKey = "nowhere"
		}, "not another status"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			def := tradeFlow()
			tt.mutate(&def)
			err := ValidateFlow(def)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateFlow = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFlow_PlainWorkflowCannotCarryFlowFields(t *testing.T) {
	plain := Definition{Name: "Plain", Steps: []Step{
		{StatusKey: "todo", Handler: Handler{Type: HandlerNone}},
		{StatusKey: "done", Handler: Handler{Type: HandlerNone}},
	}}
	if err := ValidateFlow(plain); err != nil {
		t.Fatalf("a plain workflow is invalid: %v", err)
	}
	withTasks := plain
	withTasks.Steps = []Step{{StatusKey: "todo", Handler: Handler{Type: HandlerNone}, Tasks: []Task{{Key: "a", Title: "a", Kind: KindWork, Handler: agentHandler(agentDocs)}}}}
	if err := ValidateFlow(withTasks); err == nil || !strings.Contains(err.Error(), "flow_config") {
		t.Fatalf("tasks without flow_config: %v, want an error naming flow_config", err)
	}
	humanOnly := plain
	humanOnly.Steps = []Step{{StatusKey: "todo", Handler: Handler{Type: HandlerNone}, HumanOnly: true}}
	if err := ValidateFlow(humanOnly); err == nil || !strings.Contains(err.Error(), "flow_config") {
		t.Fatalf("human_only without flow_config: %v, want an error naming flow_config", err)
	}
}

func TestExpectedTasks(t *testing.T) {
	def := tradeFlow()
	collect := def.Steps[stepIndex(t, &def, "collect")]
	keys := func(tasks []Task) []string {
		var out []string
		for _, task := range tasks {
			out = append(out, task.Key)
		}
		return out
	}
	if got := keys(ExpectedTasks(collect, map[string]any{"customs_lookup": false})); !reflect.DeepEqual(got, []string{"docs"}) {
		t.Fatalf("switch off: %v, want [docs]", got)
	}
	if got := keys(ExpectedTasks(collect, map[string]any{"customs_lookup": true})); !reflect.DeepEqual(got, []string{"docs", "customs"}) {
		t.Fatalf("switch on: %v, want [docs customs]", got)
	}
	if got := keys(ExpectedTasks(collect, nil)); !reflect.DeepEqual(got, []string{"docs"}) {
		t.Fatalf("no switches: %v, want [docs]", got)
	}
}

func TestDefaultSwitches(t *testing.T) {
	cfg := tradeFlow().Flow
	got := cfg.DefaultSwitches()
	want := map[string]any{"route": "shipment_dossier", "customs_lookup": false, "export": false}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultSwitches = %v, want %v", got, want)
	}
	got["export"] = true
	if cfg.DefaultSwitches()["export"] != false {
		t.Fatal("DefaultSwitches shares its map between calls")
	}
}

// closedTask is a closed task of the step and round with the outcome reported.
func closedTask(step, key, kind string, round int, outcome string) TaskState {
	return TaskState{Step: step, Key: key, Kind: kind, Round: round, Closed: true, Outcome: outcome}
}

func openTask(step, key, kind string, round int) TaskState {
	return TaskState{Step: step, Key: key, Kind: kind, Round: round}
}

func TestDecide(t *testing.T) {
	def := tradeFlow()
	gateState := func(rework int) FlowState {
		return FlowState{Step: "gate", Rework: rework, Switches: def.Flow.DefaultSwitches()}
	}
	bothGates := func(round int, docs, customs string) []TaskState {
		return []TaskState{
			closedTask("gate", "docs_gate", KindGate, round, docs),
			closedTask("gate", "customs_gate", KindGate, round, customs),
		}
	}
	tests := map[string]struct {
		state       FlowState
		tasks       []TaskState
		kind        DecisionKind
		to          string
		rework      bool
		stop        Stop
		humanStop   bool
		outcome     string
		missingKeys []string
	}{
		"both gates approve": {state: gateState(0), tasks: bothGates(0, "approved", "approved"),
			kind: DecisionRoute, to: "handover", outcome: TasksAllApproved},
		"both gates approve after two reworks": {state: gateState(2), tasks: bothGates(2, "approved", "approved"),
			kind: DecisionRoute, to: "handover", outcome: TasksAllApproved},
		"one gate rejects, no rework yet": {state: gateState(0), tasks: bothGates(0, "approved", "rejected"),
			kind: DecisionRoute, to: "draft", rework: true, outcome: TasksAnyRejected},
		"one gate rejects, one rework used": {state: gateState(1), tasks: bothGates(1, "rejected", "approved"),
			kind: DecisionRoute, to: "draft", rework: true, outcome: TasksAnyRejected},
		"both gates reject at the limit": {state: gateState(2), tasks: bothGates(2, "rejected", "rejected"),
			kind: DecisionRoute, to: "blocked", stop: Stop3, outcome: TasksAnyRejected},
		"a gate closed with no outcome counts as rejected": {state: gateState(0), tasks: bothGates(0, "approved", ""),
			kind: DecisionRoute, to: "draft", rework: true, outcome: TasksAnyRejected},
		"a cancelled gate counts as rejected": {state: gateState(0), tasks: []TaskState{
			closedTask("gate", "docs_gate", KindGate, 0, "approved"),
			{Step: "gate", Key: "customs_gate", Kind: KindGate, Round: 0, Closed: true, Cancelled: true}},
			kind: DecisionRoute, to: "draft", rework: true, outcome: TasksAnyRejected},
		"an outcome the gate may not report counts as rejected": {state: gateState(0), tasks: bothGates(0, "approved", "done"),
			kind: DecisionRoute, to: "draft", rework: true, outcome: TasksAnyRejected},
		"blocked beats rejected": {state: gateState(0), tasks: bothGates(0, "rejected", "blocked"),
			kind: DecisionRoute, to: "blocked", stop: Stop1, outcome: TasksAnyBlocked},
		"blocked stops at once while the other gate is open": {state: gateState(0), tasks: []TaskState{
			{Step: "gate", Key: "docs_gate", Kind: KindGate, Round: 0, Outcome: "blocked"},
			openTask("gate", "customs_gate", KindGate, 0)},
			kind: DecisionRoute, to: "blocked", stop: Stop1, outcome: TasksAnyBlocked},
		"a gate is still open": {state: gateState(0), tasks: []TaskState{
			closedTask("gate", "docs_gate", KindGate, 0, "approved"),
			openTask("gate", "customs_gate", KindGate, 0)},
			kind: DecisionWait},
		"a gate has not been opened yet": {state: gateState(0), tasks: []TaskState{
			closedTask("gate", "docs_gate", KindGate, 0, "approved")},
			kind: DecisionOpen, missingKeys: []string{"customs_gate"}},
		"the step has no tasks yet": {state: gateState(0), tasks: nil,
			kind: DecisionOpen, missingKeys: []string{"docs_gate", "customs_gate"}},
		"tasks of an earlier round are ignored": {state: gateState(1), tasks: bothGates(0, "approved", "approved"),
			kind: DecisionOpen, missingKeys: []string{"docs_gate", "customs_gate"}},
		"tasks of another step are ignored": {state: gateState(0), tasks: []TaskState{
			closedTask("draft", "crosscheck", KindWork, 0, "done")},
			kind: DecisionOpen, missingKeys: []string{"docs_gate", "customs_gate"}},
		"collect with the optional task off": {
			state: FlowState{Step: "collect", Switches: map[string]any{"customs_lookup": false}},
			tasks: []TaskState{closedTask("collect", "docs", KindWork, 0, "done")},
			kind:  DecisionRoute, to: "draft", outcome: TasksNone},
		"collect with the optional task on, only docs opened": {
			state: FlowState{Step: "collect", Switches: map[string]any{"customs_lookup": true}},
			tasks: []TaskState{closedTask("collect", "docs", KindWork, 0, "done")},
			kind:  DecisionOpen, missingKeys: []string{"customs"}},
		"collect with the optional task on, both done": {
			state: FlowState{Step: "collect", Switches: map[string]any{"customs_lookup": true}},
			tasks: []TaskState{closedTask("collect", "docs", KindWork, 0, "done"), closedTask("collect", "customs", KindWork, 0, "done")},
			kind:  DecisionRoute, to: "draft", outcome: TasksNone},
		"collect where docs reports blocked while customs runs": {
			state: FlowState{Step: "collect", Switches: map[string]any{"customs_lookup": true}},
			tasks: []TaskState{{Step: "collect", Key: "docs", Kind: KindWork, Outcome: "blocked"}, openTask("collect", "customs", KindWork, 0)},
			kind:  DecisionRoute, to: "blocked", stop: Stop1, outcome: TasksAnyBlocked},
		"draft done": {state: FlowState{Step: "draft", Rework: 1, Switches: def.Flow.DefaultSwitches()},
			tasks: []TaskState{closedTask("draft", "crosscheck", KindWork, 1, "done")},
			kind:  DecisionRoute, to: "gate", outcome: TasksNone},
		"handover approved enters the human step": {state: FlowState{Step: "handover", Rework: 1, Switches: def.Flow.DefaultSwitches()},
			tasks: []TaskState{closedTask("handover", "handover", KindGate, 1, "approved")},
			kind:  DecisionRoute, to: "in_review", humanStop: true, outcome: TasksAllApproved},
		"handover rejected stops for the third time": {state: FlowState{Step: "handover", Switches: def.Flow.DefaultSwitches()},
			tasks: []TaskState{closedTask("handover", "handover", KindGate, 0, "rejected")},
			kind:  DecisionRoute, to: "blocked", stop: Stop3, outcome: TasksAnyRejected},
		"the human step never routes": {state: FlowState{Step: "in_review", Switches: def.Flow.DefaultSwitches()},
			kind: DecisionWait},
		"the human step opens the optional export": {state: FlowState{Step: "in_review", Switches: map[string]any{"export": true}},
			kind: DecisionOpen, missingKeys: []string{"export"}},
		"the human step with a finished export still waits": {
			state: FlowState{Step: "in_review", Switches: map[string]any{"export": true}},
			tasks: []TaskState{closedTask("in_review", "export", KindWork, 0, "done")},
			kind:  DecisionWait},
		"a step the workflow does not have": {state: FlowState{Step: "nowhere"},
			kind: DecisionRoute, to: "blocked", stop: StopStalled},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := Decide(def, tt.state, tt.tasks)
			if got.Kind != tt.kind {
				t.Fatalf("Kind = %q, want %q (decision %+v)", got.Kind, tt.kind, got)
			}
			if tt.kind == DecisionOpen {
				var keys []string
				for _, task := range got.Missing {
					keys = append(keys, task.Key)
				}
				if !reflect.DeepEqual(keys, tt.missingKeys) {
					t.Fatalf("Missing = %v, want %v", keys, tt.missingKeys)
				}
				return
			}
			if tt.kind == DecisionWait {
				return
			}
			if got.To != tt.to || got.Rework != tt.rework || got.Stop != tt.stop || got.HumanStop != tt.humanStop {
				t.Fatalf("decision = %+v, want to %q rework %v stop %q humanStop %v", got, tt.to, tt.rework, tt.stop, tt.humanStop)
			}
			if tt.outcome != "" && got.TasksOutcome != tt.outcome {
				t.Fatalf("TasksOutcome = %q, want %q", got.TasksOutcome, tt.outcome)
			}
		})
	}
}

func TestDecide_ReworkCountsByRoundAcrossTheWholeLoop(t *testing.T) {
	def := tradeFlow()
	state := FlowState{Step: "gate", Switches: def.Flow.DefaultSwitches()}
	rounds := 0
	for {
		tasks := []TaskState{
			closedTask("gate", "docs_gate", KindGate, state.Rework, "rejected"),
			closedTask("gate", "customs_gate", KindGate, state.Rework, "rejected"),
		}
		d := Decide(def, state, tasks)
		if d.Kind != DecisionRoute {
			t.Fatalf("round %d: %+v", rounds, d)
		}
		if !d.Rework {
			if d.To != "blocked" || d.Stop != Stop3 {
				t.Fatalf("the loop ended at %q stop %q, want blocked stop 3", d.To, d.Stop)
			}
			break
		}
		rounds++
		state.Rework++
		if rounds > 10 {
			t.Fatal("the rework loop never stopped")
		}
	}
	if rounds != 2 || state.Rework != 2 {
		t.Fatalf("rework rounds = %d (count %d), want 2", rounds, state.Rework)
	}
}

func TestRoutePlan(t *testing.T) {
	def := tradeFlow()
	state := FlowState{Step: "todo", Switches: def.Flow.DefaultSwitches()}
	if got := RoutePlan(def, state, false); got.Kind != DecisionRoute || got.To != "collect" || got.Stop != StopNone {
		t.Fatalf("plan ok: %+v, want a route to collect", got)
	}
	if got := RoutePlan(def, state, true); got.Kind != DecisionRoute || got.To != "blocked" || got.Stop != Stop1 {
		t.Fatalf("plan blocked: %+v, want a route to blocked with stop 1", got)
	}
}

func TestStopJSON(t *testing.T) {
	tests := map[string]struct {
		in   string
		want Stop
		out  string
	}{
		"null":    {in: `null`, want: StopNone, out: `null`},
		"one":     {in: `1`, want: Stop1, out: `1`},
		"three":   {in: `3`, want: Stop3, out: `3`},
		"stalled": {in: `"stalled"`, want: StopStalled, out: `"stalled"`},
		"unknown": {in: `"later"`, want: StopStalled, out: `"stalled"`},
		"number":  {in: `7`, want: StopStalled, out: `"stalled"`},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var got Stop
			if err := json.Unmarshal([]byte(tt.in), &got); err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("Unmarshal(%s) = %q, want %q", tt.in, got, tt.want)
			}
			raw, err := json.Marshal(got)
			if err != nil || string(raw) != tt.out {
				t.Fatalf("Marshal = %s, %v; want %s", raw, err, tt.out)
			}
		})
	}
	state := FlowState{Step: "gate"}
	raw, _ := json.Marshal(state)
	if strings.Contains(string(raw), "stop") || strings.Contains(string(raw), "closed_at") {
		t.Fatalf("an idle state serializes stop or closed_at: %s", raw)
	}
}

func TestIssueFlowRef(t *testing.T) {
	text := func(v string) pgtype.Text { return pgtype.Text{String: v, Valid: true} }
	if got := IssueFlowRef(nil, pgtype.Text{}, pgtype.Text{}, pgtype.Int4{}); got != nil {
		t.Fatalf("an ordinary issue has a flow ref: %+v", got)
	}
	if got := IssueFlowRef([]byte(`not json`), pgtype.Text{}, pgtype.Text{}, pgtype.Int4{}); got != nil {
		t.Fatalf("an unreadable state has a flow ref: %+v", got)
	}
	request := IssueFlowRef([]byte(`{"workflow_id":"wf-1","step":"gate","rework":2,"stop":3}`), pgtype.Text{}, pgtype.Text{}, pgtype.Int4{})
	if request == nil || request.Role != RoleRequest || request.Step != "gate" || request.WorkflowID != "wf-1" ||
		request.Rework == nil || *request.Rework != 2 || request.Stop != Stop3 {
		t.Fatalf("request ref = %+v", request)
	}
	task := IssueFlowRef(nil, text("gate"), text("docs_gate"), pgtype.Int4{Int32: 1, Valid: true})
	if task == nil || task.Role != RoleTask || task.Step != "gate" || task.Task != "docs_gate" || task.Round == nil || *task.Round != 1 {
		t.Fatalf("task ref = %+v", task)
	}
	raw, err := json.Marshal(IssueFlowRef([]byte(`{"workflow_id":"wf-1","step":"gate","rework":0}`), pgtype.Text{}, pgtype.Text{}, pgtype.Int4{}))
	if err != nil || string(raw) != `{"role":"request","step":"gate","workflow_id":"wf-1","rework":0}` {
		t.Fatalf("request ref JSON = %s, %v", raw, err)
	}
}

func TestFlowStateRound(t *testing.T) {
	if got := (FlowState{Rework: 2}).Round(); got != 2 {
		t.Fatalf("Round = %d, want 2", got)
	}
	if got := (FlowState{Rework: 0, RoundBase: 3}).Round(); got != 3 {
		t.Fatalf("Round after a resume = %d, want 3", got)
	}
	def := tradeFlow()
	state := FlowState{Step: "gate", RoundBase: 3, Switches: def.Flow.DefaultSwitches()}
	tasks := []TaskState{
		closedTask("gate", "docs_gate", KindGate, 0, "rejected"),
		closedTask("gate", "customs_gate", KindGate, 0, "rejected"),
	}
	if got := Decide(def, state, tasks); got.Kind != DecisionOpen || len(got.Missing) != 2 {
		t.Fatalf("tasks of the old round must not count in the new one: %+v", got)
	}
	tasks = []TaskState{
		closedTask("gate", "docs_gate", KindGate, 3, "approved"),
		closedTask("gate", "customs_gate", KindGate, 3, "approved"),
	}
	if got := Decide(def, state, tasks); got.Kind != DecisionRoute || got.To != "handover" {
		t.Fatalf("tasks of the new round must count: %+v", got)
	}
}

func TestRenderMessage(t *testing.T) {
	got := RenderMessage("BLOCKED · stop 1\n{requester} {request} at {step} {unknown}", map[string]string{
		"requester": "[@An](mention://member/1)", "request": "MUL-1", "step": "collect",
	})
	if got != "BLOCKED · stop 1\n[@An](mention://member/1) MUL-1 at collect {unknown}" {
		t.Fatalf("RenderMessage = %q", got)
	}
	title, body := SplitMessage("  BLOCKED · stop 1 \n\nBody line\nmore ")
	if title != "BLOCKED · stop 1" || body != "Body line\nmore" {
		t.Fatalf("SplitMessage = %q, %q", title, body)
	}
	if title, body := SplitMessage("only a title"); title != "only a title" || body != "" {
		t.Fatalf("SplitMessage(one line) = %q, %q", title, body)
	}
}

func TestTaskDescription(t *testing.T) {
	gate := Task{Key: "docs_gate", Title: "Documents gate", Kind: KindGate, Instructions: "Check the documents.", Requires: []string{RequireOutcome}}
	got := TaskDescription(TaskDescriptionInput{
		Task: gate, RequestIdentifier: "MUL-7", Round: 0,
		Inputs: []TaskInput{{Identifier: "MUL-9", Key: "crosscheck"}},
	})
	for _, want := range []string{
		"Check the documents.", "MUL-7", "MUL-9 · crosscheck", "multica attachment download",
		"multica issue status <id> done --outcome approved", "--outcome rejected",
		"multica issue status <id> blocked --outcome blocked", "Closing this issue without",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("gate description is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Rework") {
		t.Fatalf("a first-round description mentions rework:\n%s", got)
	}
	if strings.Contains(got, "Closing this issue before you attached") {
		t.Fatalf("a gate without an attachment rule mentions one:\n%s", got)
	}

	work := Task{Key: "crosscheck", Title: "Draft", Kind: KindWork, Instructions: "Draft it.", Requires: []string{RequireAttachment}}
	got = TaskDescription(TaskDescriptionInput{
		Task: work, RequestIdentifier: "MUL-7", Round: 1,
		Rejected: []TaskInput{{Identifier: "MUL-11", Key: "docs_gate"}},
	})
	for _, want := range []string{"Rework (round 2)", "MUL-11 · docs_gate", "--attachment", "multica issue status <id> done`", "Closing this issue before you attached"} {
		if !strings.Contains(got, want) {
			t.Fatalf("work description is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "--outcome approved") {
		t.Fatalf("a work task without an outcome rule asks for a verdict:\n%s", got)
	}
}

func TestFlowBrief_GateTask(t *testing.T) {
	gate := Task{Key: "docs_gate", Title: "Documents gate", Kind: KindGate, Instructions: "Check the documents.", Requires: []string{RequireOutcome}}
	got := FlowTaskBrief(FlowTaskBriefInput{
		WorkflowName: "Import & Export Desk", RequestIdentifier: "MUL-7", TaskIdentifier: "MUL-9",
		StepName: "Gate", Task: gate, Round: 0,
		Inputs: []TaskInput{{Identifier: "MUL-8", Key: "crosscheck"}},
	})
	for _, want := range []string{
		"Import & Export Desk", "MUL-7", "MUL-9", "docs_gate",
		"multica issue status MUL-9 done --outcome approved",
		"multica issue status MUL-9 done --outcome rejected",
		"multica issue status MUL-9 blocked --outcome blocked",
		"MUL-8 · crosscheck", "multica attachment download",
		"The server refuses",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("gate brief is missing %q:\n%s", want, got)
		}
	}
	// A gate judges the files, not the comments of whoever made them.
	if !strings.Contains(got, "files") || strings.Contains(got, "VERDICT") {
		t.Fatalf("gate brief must send the gate to the attached files and ask for no VERDICT line:\n%s", got)
	}
	if strings.Contains(got, "flow advance") || strings.Contains(got, "flow plan") {
		t.Fatalf("a task brief sends the agent to the flow commands of the lead:\n%s", got)
	}
}

func TestFlowBrief_WorkTaskNeedsAttachment(t *testing.T) {
	work := Task{Key: "crosscheck", Title: "Cross-check", Kind: KindWork, Instructions: "Draft.", Requires: []string{RequireAttachment}}
	got := FlowTaskBrief(FlowTaskBriefInput{WorkflowName: "Trade", RequestIdentifier: "MUL-7", TaskIdentifier: "MUL-8", StepName: "Draft", Task: work})
	for _, want := range []string{"multica issue comment add MUL-8 --attachment", "multica issue status MUL-8 done", "attach"} {
		if !strings.Contains(got, want) {
			t.Fatalf("work brief is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "--outcome approved") {
		t.Fatalf("a work task with no outcome rule asks for a verdict:\n%s", got)
	}
}

func TestFlowBrief_Rework(t *testing.T) {
	gate := Task{Key: "docs_gate", Title: "Documents gate", Kind: KindGate, Requires: []string{RequireOutcome}}
	got := FlowTaskBrief(FlowTaskBriefInput{
		WorkflowName: "Trade", RequestIdentifier: "MUL-7", TaskIdentifier: "MUL-20", StepName: "Gate", Task: gate, Round: 1,
		Rejected: []TaskInput{{Identifier: "MUL-11", Key: "customs_gate"}},
	})
	for _, want := range []string{"round 2", "MUL-11 · customs_gate", "rejected"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rework brief is missing %q:\n%s", want, got)
		}
	}
}

func TestFlowBrief_LeadPlan(t *testing.T) {
	def := tradeFlow()
	got := FlowLeadBrief(FlowLeadBriefInput{
		WorkflowName: "Trade", RequestIdentifier: "MUL-7", Step: "todo", StepName: "To Do",
		PlanStep: true, Switches: def.Flow.Switches, Values: def.Flow.DefaultSwitches(),
	})
	for _, want := range []string{
		"multica issue flow show MUL-7", "multica issue flow plan MUL-7",
		"--switch customs_lookup=true", "route", "shipment_dossier", "export",
		"--blocked --missing", "Do not create sub-issues", "do not set the status",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("lead plan brief is missing %q:\n%s", want, got)
		}
	}
}

func TestFlowBrief_LeadAfterThePlan(t *testing.T) {
	got := FlowLeadBrief(FlowLeadBriefInput{WorkflowName: "Trade", RequestIdentifier: "MUL-7", Step: "gate", StepName: "Gate"})
	for _, want := range []string{"multica issue flow advance MUL-7", "Gate", "Do not create sub-issues"} {
		if !strings.Contains(got, want) {
			t.Fatalf("lead brief is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "multica issue flow plan") {
		t.Fatalf("a lead brief after the plan asks for a plan:\n%s", got)
	}
	stopped := FlowLeadBrief(FlowLeadBriefInput{WorkflowName: "Trade", RequestIdentifier: "MUL-7", Step: "blocked", StepName: "Blocked", Stop: Stop1})
	if !strings.Contains(stopped, "stopped") || strings.Contains(stopped, "multica issue flow advance") {
		t.Fatalf("a stopped request waits for a person:\n%s", stopped)
	}
}

// The squad rules and the lead's skill tell a lead to post the stop comment. The server also posts
// one when the request stops. The server skips its copy when the lead already wrote the comment for
// that stop, so the brief says both: the server sets the status and posts the comment only when the
// lead has not, and the lead may post it.
func TestFlowBrief_StopCommentIsPostedOnce(t *testing.T) {
	def := tradeFlow()
	briefs := map[string]string{
		"plan": FlowLeadBrief(FlowLeadBriefInput{
			WorkflowName: "Trade", RequestIdentifier: "MUL-7", Step: "todo", StepName: "To Do",
			PlanStep: true, Switches: def.Flow.Switches, Values: def.Flow.DefaultSwitches(),
		}),
		"after the plan": FlowLeadBrief(FlowLeadBriefInput{WorkflowName: "Trade", RequestIdentifier: "MUL-7", Step: "gate", StepName: "Gate"}),
		"stopped":        FlowLeadBrief(FlowLeadBriefInput{WorkflowName: "Trade", RequestIdentifier: "MUL-7", Step: "blocked", StepName: "Blocked", Stop: Stop1}),
	}
	for name, got := range briefs {
		for _, want := range []string{
			"The server sets the status of the request when it stops",
			"unless you already posted one for that stop",
			"You may post the stop comment yourself",
			"BLOCKED",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("%s brief is missing %q:\n%s", name, want, got)
			}
		}
		if strings.Contains(got, "Post none of them yourself") {
			t.Errorf("%s brief forbids the lead to post the stop comment; the server only skips its own copy:\n%s", name, got)
		}
	}
}

// A plan step that routes to blocked on what the lead chose is a stop for missing grounds, not for
// used-up rework rounds.
func TestRoutePlan_BlockedByASwitchIsStop1(t *testing.T) {
	def := tradeFlow()
	plan := def.Steps[0]
	plan.Transitions = []Transition{
		{ID: "records", Label: "Records", When: &Condition{All: []Condition{
			{Field: FieldSwitchPrefix + "export", Op: OpEq, Value: true},
		}}, ToStatusKey: "blocked"},
		{ID: "default", Label: "Default", ToStatusKey: "collect"},
	}
	def.Steps[0] = plan
	state := FlowState{Step: "todo", Switches: map[string]any{"export": true}}

	d := RoutePlan(def, state, false)

	if d.To != "blocked" || d.Stop != Stop1 {
		t.Fatalf("decision = %+v, want blocked at stop 1", d)
	}
}
