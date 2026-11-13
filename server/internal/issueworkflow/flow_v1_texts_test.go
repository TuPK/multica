package issueworkflow

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var updateV1Texts = flag.Bool("update", false, "rewrite testdata/v1_texts/*.golden")

// The texts the schema 1 engine writes for each catalog flow: the task
// description, the task brief, the lead brief and the stop messages. They are
// frozen so that the generalised engine, which renders the same flows from a
// graph, can be shown to say the same thing. Run `go test -run TestV1TextsGolden
// -update` only on an engine whose texts are meant to change.

// v1TextRounds are the rounds each task text is rendered in.
var v1TextRounds = []int{0, 1}

// v1TaskRef is a task issue of the fake request the texts are rendered for.
type v1TaskRef struct {
	step, key, kind string
	round, number   int
}

// v1TextWorld numbers the task issues of a flow the way a request would open
// them: round by round, step by step, task by task, after the request KV-1.
func v1TextWorld(def Definition) []v1TaskRef {
	var rows []v1TaskRef
	number := 1
	for _, round := range v1TextRounds {
		for _, step := range def.Steps {
			for _, task := range step.Tasks {
				number++
				rows = append(rows, v1TaskRef{step: step.StatusKey, key: task.Key, kind: task.Kind, round: round, number: number})
			}
		}
	}
	return rows
}

// v1TaskLinks mirrors flowTaskLinks (handler/flow_engine.go): the tasks of the
// nearest earlier step that has any in this round are the inputs, and on a
// later round every gate of the previous round is a rejection.
func v1TaskLinks(def Definition, rows []v1TaskRef, step string, round int) (inputs, rejected []TaskInput) {
	ref := func(r v1TaskRef) TaskInput {
		return TaskInput{Identifier: fmt.Sprintf("KV-%d", r.number), Key: r.key}
	}
	index := -1
	for i, s := range def.Steps {
		if s.StatusKey == step {
			index = i
		}
	}
	for i := index - 1; i >= 0 && len(inputs) == 0; i-- {
		for _, r := range rows {
			if r.step == def.Steps[i].StatusKey && r.round == round {
				inputs = append(inputs, ref(r))
			}
		}
	}
	if round > 0 {
		for _, r := range rows {
			if r.round == round-1 && r.kind == KindGate {
				rejected = append(rejected, ref(r))
			}
		}
	}
	return inputs, rejected
}

func v1StepName(f catalogFlow, key string) string {
	for _, s := range f.Steps {
		if s.StatusKey == key && s.Status.Name != "" {
			return s.Status.Name
		}
	}
	return key
}

// v1DefaultStopMessages are the texts the server uses for a stop a flow_config
// has no template for. handler.defaultStopMessages is the source; a test in the
// handler package keeps this file equal to it.
func v1DefaultStopMessages(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "v1_stop_messages.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"stop1", "stop2", "stop3", "stalled", "human"} {
		if strings.TrimSpace(out[key]) == "" {
			t.Fatalf("testdata/v1_stop_messages.json has no text for %q", key)
		}
	}
	return out
}

// renderV1Texts renders every frozen text of a catalog flow.
func renderV1Texts(t *testing.T, f catalogFlow) string {
	t.Helper()
	def, _, err := f.definition()
	if err != nil {
		t.Fatal(err)
	}
	rows := v1TextWorld(def)
	var b strings.Builder
	section := func(title, body string) {
		fmt.Fprintf(&b, "=== %s ===\n%s\n", title, strings.TrimRight(body, "\n"))
	}
	for _, round := range v1TextRounds {
		for _, step := range def.Steps {
			inputs, rejected := v1TaskLinks(def, rows, step.StatusKey, round)
			for _, task := range step.Tasks {
				where := fmt.Sprintf("round %d / %s / %s", round, step.StatusKey, task.Key)
				section(where+" / TaskDescription", TaskDescription(TaskDescriptionInput{
					Task: task, RequestIdentifier: "KV-1", Round: round, Inputs: inputs, Rejected: rejected,
				}))
				number := 0
				for _, r := range rows {
					if r.step == step.StatusKey && r.key == task.Key && r.round == round {
						number = r.number
					}
				}
				section(where+" / FlowTaskBrief", FlowTaskBrief(FlowTaskBriefInput{
					WorkflowName: def.Name, RequestIdentifier: "KV-1", TaskIdentifier: fmt.Sprintf("KV-%d", number),
					StepName: v1StepName(f, step.StatusKey), Task: task, Round: round, Inputs: inputs, Rejected: rejected,
				}))
			}
		}
	}

	switches := def.Flow.DefaultSwitches()
	firstWork := ""
	for _, s := range def.Steps {
		if len(s.Tasks) > 0 {
			firstWork = s.StatusKey
			break
		}
	}
	lead := func(title string, in FlowLeadBriefInput) {
		in.WorkflowName, in.RequestIdentifier = def.Name, "KV-1"
		in.StepName = v1StepName(f, in.Step)
		in.Switches, in.Values = def.Flow.Switches, switches
		section("lead / "+title+" / FlowLeadBrief", FlowLeadBrief(in))
	}
	lead("plan step", FlowLeadBriefInput{Step: StartStatus, PlanStep: true})
	lead("after the plan", FlowLeadBriefInput{Step: firstWork})
	lead("stop 1", FlowLeadBriefInput{Step: firstWork, Stop: Stop1})
	lead("stop 3", FlowLeadBriefInput{Step: firstWork, Stop: Stop3})
	lead("stalled", FlowLeadBriefInput{Step: firstWork, Stop: StopStalled})

	defaults := v1DefaultStopMessages(t)
	messages := []struct{ name, template string }{
		{"stop1", def.Flow.Messages.Stop1},
		{"stop2", def.Flow.Messages.Stop2},
		{"stop3", def.Flow.Messages.Stop3},
		{"stalled", def.Flow.Messages.Stalled},
		{"human", def.Flow.Messages.Human},
	}
	for _, round := range v1TextRounds {
		var taskLines []string
		for _, r := range rows {
			if r.step == firstWork && r.round == round {
				taskLines = append(taskLines, fmt.Sprintf("- KV-%d · %s · done · -", r.number, r.key))
			}
		}
		vars := map[string]string{
			"requester": "[@Requester](mention://member/00000000-0000-4000-8000-0000000000aa)",
			"request":   "KV-1",
			"step":      v1StepName(f, firstWork),
			"rework":    fmt.Sprint(round),
			"tasks":     strings.Join(taskLines, "\n"),
			"missing":   "- the signed contract",
			"reason":    "The task did not report in time.",
		}
		for _, m := range messages {
			// A blank template falls back to the server default, as postStopComment does.
			template := m.template
			if strings.TrimSpace(template) == "" {
				template = defaults[m.name]
			}
			section(fmt.Sprintf("round %d / message %s", round, m.name), RenderMessage(template, vars))
		}
	}
	return b.String()
}

func TestV1TextsGolden(t *testing.T) {
	flows := loadCatalogFlows(t)
	if len(flows) == 0 {
		t.Skip("the catalog is not built")
	}
	ids := make([]string, 0, len(flows))
	for id := range flows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			got := renderV1Texts(t, flows[id])
			path := filepath.Join("testdata", "v1_texts", id+".golden")
			if *updateV1Texts {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s (generate it with -update on the unchanged engine): %v", path, err)
			}
			if got != string(want) {
				t.Fatalf("the texts of %s changed; first difference:\n%s", id, firstDifference(string(want), got))
			}
		})
	}
}

// firstDifference shows the first line where two texts differ.
func firstDifference(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			return fmt.Sprintf("line %d\n  golden: %q\n  actual: %q", i+1, wl, gl)
		}
	}
	return "(none)"
}

// The issue.flow JSON the schema 1 engine writes, hand-written for the shapes a
// request goes through. The decoder must read each one and the encoder must
// write every key and value back: the lifted engine adapts these states and
// must not lose a field of a running request.
func TestV1StatesDecode(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "v1_states", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 8 {
		t.Fatalf("testdata/v1_states has %d samples, want 8", len(paths))
	}
	for _, path := range paths {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".json"), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var state FlowState
			if err := json.Unmarshal(raw, &state); err != nil {
				t.Fatalf("decode: %v", err)
			}
			again, err := json.Marshal(state)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			var want, got map[string]any
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(again, &got); err != nil {
				t.Fatal(err)
			}
			for key := range want {
				if _, ok := got[key]; !ok {
					t.Errorf("key %q was lost on re-encode: %s", key, again)
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("re-encoded state differs\n  sample:  %v\n  encoded: %v", want, got)
			}
		})
	}
}
