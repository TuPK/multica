package issueworkflow

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// sampleDocJSON is a small schema 2 document that uses every part of the model:
// a lead step, tasks with inputs, sets, skills and instances, a gate step with a
// quorum join and a loop back to the draft, and a manual step.
const sampleDocJSON = `{
  "version": 2,
  "start": "plan",
  "roles": [
    {"key": "lead", "kind": "lead"},
    {"key": "writer", "label": "Writer", "kind": "agent", "pool": 1,
     "bind": {"agent_ids": ["00000000-0000-4000-8000-000000000001"]}},
    {"key": "checker", "label": "Checker", "kind": "agent", "pool": 2,
     "source": {"bundle": "bundle_one", "agent_key": "checker"}},
    {"key": "reviewer", "label": "Reviewer", "kind": "member"}
  ],
  "vars": [
    {"key": "urgent", "type": "bool", "default": false, "set_by": ["lead"], "set_at": ["plan"]},
    {"key": "route", "type": "enum", "values": ["fast", "full"], "default": "fast", "set_by": ["lead"], "set_at": ["plan"]},
    {"key": "rounds", "type": "int", "min": 1, "max": 5, "default": 2, "set_by": ["lead"], "set_at": ["plan"]},
    {"key": "note", "type": "string", "max_len": 100, "default": "", "set_by": ["managers"], "set_at": ["*"]},
    {"key": "sources", "type": "list_string", "max_items": 5, "default": [], "set_by": ["writer"], "set_at": ["research"]}
  ],
  "verdicts": [
    {"key": "review", "verdicts": [
      {"key": "ok", "class": "pass", "label": "OK"},
      {"key": "fix", "class": "fail"},
      {"key": "skip", "class": "neutral"}
    ]}
  ],
  "loops": [
    {"key": "fix", "header": "draft", "max": {"var": "rounds", "cap": 5}},
    {"key": "retry", "header": "research", "max": 3, "reset": "never"}
  ],
  "stops": [
    {"code": "grounds_missing", "class": "missing_grounds", "label": "Grounds missing", "message": "stop_grounds", "resume_by": "managers", "legacy_no": 1},
    {"code": "loop_exhausted", "class": "loop_exhausted", "label": "Rounds used", "message": "stop_loop", "resume_by": "managers", "legacy_no": 3}
  ],
  "terminals": [
    {"key": "done", "status": "done", "leaves_workspace": true},
    {"key": "cancelled", "status": "cancelled"}
  ],
  "steps": [
    {"key": "plan", "title": "Plan", "status": {"key": "todo"}, "kind": "lead",
     "lead": {"sets": ["urgent", "route", "rounds"], "verdicts": "plan"},
     "edges": [
       {"id": "plan_block", "label": "Missing grounds", "stop": "grounds_missing", "when": {"field": "lead.verdict", "op": "eq", "value": "blocked"}},
       {"id": "plan_ok", "label": "Planned", "to": "research"}
     ]},
    {"key": "research", "title": "Research", "status": {"key": "in_progress"}, "kind": "tasks",
     "tasks": [
       {"key": "collect", "title": "Collect sources", "kind": "work", "role": "writer",
        "sets": ["sources"], "skills": ["web-research"],
        "requires": {"files": [{"slot": "notes", "min": 1, "ext": [".md"]}], "vars": ["sources"]},
        "instructions": "Collect the sources."}
     ],
     "edges": [
       {"id": "research_block", "label": "Blocked", "stop": "grounds_missing", "when": {"field": "join.result", "op": "eq", "value": "block"}},
       {"id": "research_ok", "label": "Collected", "to": "draft"}
     ]},
    {"key": "draft", "title": "Draft", "status": {"key": "in_progress"}, "kind": "tasks",
     "tasks": [
       {"key": "write", "title": "Write the draft", "kind": "work", "role": "writer",
        "inputs": [{"from": "research.collect", "slot": "notes", "required": true}],
        "requires": {"files": [{"slot": "draft", "min": 1, "max": 2}]}}
     ],
     "edges": [{"id": "draft_ok", "label": "Drafted", "to": "check"}]},
    {"key": "check", "title": "Check", "status": {"name": "Checking", "category": "started"}, "kind": "tasks",
     "tasks": [
       {"key": "verify", "title": "Verify", "kind": "gate", "role": "checker",
        "instances": {"count": 2, "distinct": true}, "verdicts": "review",
        "requires": {"verdict": true}, "deadline": {"idle_minutes": 60}},
       {"key": "second", "title": "Second look", "kind": "gate", "role": "reviewer", "after": ["verify"],
        "when": {"field": "var.urgent", "op": "eq", "value": false}}
     ],
     "join": {"over": ["verify", "second"], "mode": "quorum", "k": 1, "block": "at_close"},
     "edges": [
       {"id": "check_fix", "label": "Fix", "to": "draft", "loop": "fix", "when": {"field": "join.result", "op": "eq", "value": "fail"}},
       {"id": "check_ok", "label": "Pass", "to": "release", "when": {"field": "join.result", "op": "eq", "value": "pass"}},
       {"id": "check_end", "label": "Rounds used", "stop": "loop_exhausted"}
     ],
     "layout": {"x": 120, "y": 40.5}},
    {"key": "release", "title": "Release", "status": {"key": "in_review"}, "kind": "manual",
     "exit_role": "reviewer", "notice": "notice_release",
     "edges": [{"id": "release_go", "label": "Release", "to": "done", "trigger": "manual"}]}
  ],
  "messages": {
    "stop_grounds": "BLOCKED · stop 1\n{requester} {request} stopped at {step}.",
    "stop_loop": "BLOCKED · stop 3\n{requester} {request} used its {rework} rounds.",
    "notice_release": "IN REVIEW\n{request} waits for you."
  },
  "policy": {"max_activations": 50, "acks": [{"kind": "unattended_release", "by": "0190c4a0-0000-7000-8000-00000000f001", "at": "2026-11-13T08:00:00Z"}]}
}`

func parseSample(t *testing.T) FlowDoc {
	t.Helper()
	doc, issues := ParseFlowDoc([]byte(sampleDocJSON))
	if len(issues) > 0 {
		t.Fatalf("the sample document has issues: %+v", issues)
	}
	return doc
}

// withJSON edits the sample as a JSON tree and returns the result as bytes.
func withJSON(t *testing.T, edit func(doc map[string]any)) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(sampleDocJSON), &doc); err != nil {
		t.Fatal(err)
	}
	edit(doc)
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func firstOf(doc map[string]any, list string) map[string]any {
	return doc[list].([]any)[0].(map[string]any)
}

func stepOf(doc map[string]any, key string) map[string]any {
	for _, s := range doc["steps"].([]any) {
		if step := s.(map[string]any); step["key"] == key {
			return step
		}
	}
	panic("no step " + key)
}

func issueCodes(issues []Issue) []string {
	var out []string
	for _, i := range issues {
		out = append(out, i.Code)
	}
	return out
}

func TestParseFlowDocStrict(t *testing.T) {
	doc, issues := ParseFlowDoc([]byte(sampleDocJSON))
	if len(issues) > 0 {
		t.Fatalf("the sample document must parse cleanly: %+v", issues)
	}
	if doc.Version != 2 || doc.Start != "plan" || len(doc.Steps) != 5 || len(doc.Roles) != 4 {
		t.Fatalf("the sample document was read wrong: %+v", doc)
	}

	t.Run("round trip", func(t *testing.T) {
		raw, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		again, issues := ParseFlowDoc(raw)
		if len(issues) > 0 {
			t.Fatalf("the marshalled document does not parse: %+v\n%s", issues, raw)
		}
		if !reflect.DeepEqual(again, doc) {
			t.Fatalf("round trip changed the document\n got: %+v\nwant: %+v", again, doc)
		}
	})

	unknownKey := map[string]func(doc map[string]any){
		"top level": func(d map[string]any) { d["colour"] = "red" },
		"role":      func(d map[string]any) { firstOf(d, "roles")["colour"] = 1 },
		"role bind": func(d map[string]any) {
			d["roles"].([]any)[1].(map[string]any)["bind"].(map[string]any)["team_ids"] = []any{}
		},
		"role source": func(d map[string]any) { d["roles"].([]any)[2].(map[string]any)["source"].(map[string]any)["url"] = "x" },
		"var":         func(d map[string]any) { firstOf(d, "vars")["unit"] = "kg" },
		"verdict set": func(d map[string]any) { firstOf(d, "verdicts")["weight"] = 1 },
		"verdict": func(d map[string]any) {
			firstOf(d, "verdicts")["verdicts"].([]any)[0].(map[string]any)["icon"] = "x"
		},
		"loop":            func(d map[string]any) { firstOf(d, "loops")["speed"] = 1 },
		"loop max object": func(d map[string]any) { firstOf(d, "loops")["max"].(map[string]any)["floor"] = 1 },
		"stop kind":       func(d map[string]any) { firstOf(d, "stops")["urgency"] = 1 },
		"terminal":        func(d map[string]any) { firstOf(d, "terminals")["colour"] = "x" },
		"step":            func(d map[string]any) { stepOf(d, "plan")["colour"] = "x" },
		"step status":     func(d map[string]any) { stepOf(d, "plan")["status"].(map[string]any)["icon"] = "x" },
		"step lead":       func(d map[string]any) { stepOf(d, "plan")["lead"].(map[string]any)["colour"] = "x" },
		"step layout":     func(d map[string]any) { stepOf(d, "check")["layout"].(map[string]any)["z"] = 1 },
		"task":            func(d map[string]any) { stepOf(d, "draft")["tasks"].([]any)[0].(map[string]any)["priority"] = 1 },
		"task instances": func(d map[string]any) {
			stepOf(d, "check")["tasks"].([]any)[0].(map[string]any)["instances"].(map[string]any)["step"] = 1
		},
		"task requires": func(d map[string]any) {
			stepOf(d, "draft")["tasks"].([]any)[0].(map[string]any)["requires"].(map[string]any)["lines"] = 1
		},
		"task file": func(d map[string]any) {
			stepOf(d, "draft")["tasks"].([]any)[0].(map[string]any)["requires"].(map[string]any)["files"].([]any)[0].(map[string]any)["colour"] = 1
		},
		"task input": func(d map[string]any) {
			stepOf(d, "draft")["tasks"].([]any)[0].(map[string]any)["inputs"].([]any)[0].(map[string]any)["colour"] = 1
		},
		"task deadline": func(d map[string]any) {
			stepOf(d, "check")["tasks"].([]any)[0].(map[string]any)["deadline"].(map[string]any)["colour"] = 1
		},
		"task when": func(d map[string]any) {
			stepOf(d, "check")["tasks"].([]any)[1].(map[string]any)["when"].(map[string]any)["colour"] = 1
		},
		"join": func(d map[string]any) { stepOf(d, "check")["join"].(map[string]any)["colour"] = 1 },
		"edge": func(d map[string]any) { stepOf(d, "check")["edges"].([]any)[0].(map[string]any)["colour"] = 1 },
		"edge when": func(d map[string]any) {
			stepOf(d, "check")["edges"].([]any)[0].(map[string]any)["when"].(map[string]any)["colour"] = 1
		},
		"policy":     func(d map[string]any) { d["policy"].(map[string]any)["max_colours"] = 1 },
		"policy ack": func(d map[string]any) { d["policy"].(map[string]any)["acks"].([]any)[0].(map[string]any)["colour"] = 1 },
	}
	for name, edit := range unknownKey {
		t.Run("unknown key in "+name, func(t *testing.T) {
			_, issues := ParseFlowDoc(withJSON(t, edit))
			if len(issues) == 0 {
				t.Fatal("an unknown key was accepted")
			}
			if issues[0].Code != CodeFlowDocInvalid || !strings.Contains(issues[0].Message, "unknown") {
				t.Fatalf("issue = %+v, want %s about an unknown key", issues[0], CodeFlowDocInvalid)
			}
		})
	}

	t.Run("an unknown key is reported with its path by key", func(t *testing.T) {
		_, issues := ParseFlowDoc(withJSON(t, func(d map[string]any) {
			stepOf(d, "check")["tasks"].([]any)[0].(map[string]any)["colour"] = 1
		}))
		if len(issues) != 1 || issues[0].Path != "steps[check].tasks[verify].colour" {
			t.Fatalf("issues = %+v, want one at steps[check].tasks[verify].colour", issues)
		}
	})

	t.Run("every unknown key is reported", func(t *testing.T) {
		_, issues := ParseFlowDoc(withJSON(t, func(d map[string]any) {
			d["a_new_key"] = 1
			stepOf(d, "plan")["another"] = 1
		}))
		if len(issues) != 2 {
			t.Fatalf("issues = %+v, want 2", issues)
		}
	})

	t.Run("version", func(t *testing.T) {
		for _, v := range []any{3, 99} {
			_, issues := ParseFlowDoc(withJSON(t, func(d map[string]any) { d["version"] = v; d["a_newer_key"] = true }))
			if len(issues) != 1 || issues[0].Code != CodeFlowSchemaUnsupported {
				t.Fatalf("version %v: issues = %+v, want one %s", v, issues, CodeFlowSchemaUnsupported)
			}
		}
		for _, v := range []any{1, 0, nil} {
			_, issues := ParseFlowDoc(withJSON(t, func(d map[string]any) {
				if v == nil {
					delete(d, "version")
				} else {
					d["version"] = v
				}
			}))
			if len(issues) != 1 || issues[0].Code != CodeFlowDocInvalid || issues[0].Path != "version" {
				t.Fatalf("version %v: issues = %+v, want one %s at version", v, issues, CodeFlowDocInvalid)
			}
		}
	})

	t.Run("size", func(t *testing.T) {
		big := withJSON(t, func(d map[string]any) {
			d["messages"].(map[string]any)["stop_grounds"] = strings.Repeat("x", DocMaxBytes)
		})
		_, issues := ParseFlowDoc(big)
		if len(issues) != 1 || issues[0].Code != CodeFlowDocTooLarge {
			t.Fatalf("a %d byte document: issues = %+v, want one %s", len(big), issues, CodeFlowDocTooLarge)
		}
		atLimit := make([]byte, DocMaxBytes)
		copy(atLimit, sampleDocJSON)
		for i := len(sampleDocJSON); i < len(atLimit); i++ {
			atLimit[i] = ' '
		}
		if _, issues := ParseFlowDoc(atLimit); len(issues) > 0 {
			t.Fatalf("a document of exactly the limit is refused: %+v", issues)
		}
	})

	t.Run("keys must match the pattern", func(t *testing.T) {
		tests := map[string]struct {
			edit func(d map[string]any)
			path string
		}{
			"role":        {func(d map[string]any) { firstOf(d, "roles")["key"] = "Lead" }, "roles[0]"},
			"var":         {func(d map[string]any) { firstOf(d, "vars")["key"] = "1urgent" }, "vars[0]"},
			"verdict set": {func(d map[string]any) { firstOf(d, "verdicts")["key"] = "re view" }, "verdicts[0]"},
			"verdict": {func(d map[string]any) {
				firstOf(d, "verdicts")["verdicts"].([]any)[1].(map[string]any)["key"] = "FIX"
			},
				"verdicts[review].verdicts[1]"},
			"loop":     {func(d map[string]any) { firstOf(d, "loops")["key"] = "fix-it" }, "loops[0]"},
			"stop":     {func(d map[string]any) { firstOf(d, "stops")["code"] = "Grounds" }, "stops[0]"},
			"terminal": {func(d map[string]any) { firstOf(d, "terminals")["key"] = "" }, "terminals[0]"},
			"step":     {func(d map[string]any) { stepOf(d, "plan")["key"] = "Plan" }, "steps[0]"},
			"task": {func(d map[string]any) {
				stepOf(d, "draft")["tasks"].([]any)[0].(map[string]any)["key"] = strings.Repeat("a", 33)
			}, "steps[draft].tasks[0]"},
			"edge":    {func(d map[string]any) { stepOf(d, "draft")["edges"].([]any)[0].(map[string]any)["id"] = "draft ok" }, "steps[draft].edges[0]"},
			"message": {func(d map[string]any) { d["messages"].(map[string]any)["Bad Key"] = "x" }, "messages[Bad Key]"},
			"file slot": {func(d map[string]any) {
				stepOf(d, "draft")["tasks"].([]any)[0].(map[string]any)["requires"].(map[string]any)["files"].([]any)[0].(map[string]any)["slot"] = "My Draft"
			}, "steps[draft].tasks[write].requires.files[0]"},
		}
		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				_, issues := ParseFlowDoc(withJSON(t, tt.edit))
				if len(issues) != 1 || issues[0].Code != CodeKeyInvalid || issues[0].Path != tt.path {
					t.Fatalf("issues = %+v, want one %s at %s", issues, CodeKeyInvalid, tt.path)
				}
			})
		}
		for _, ok := range []string{"a", "a1_b", strings.Repeat("a", 32)} {
			_, issues := ParseFlowDoc(withJSON(t, func(d map[string]any) { firstOf(d, "roles")["key"] = ok }))
			if len(issues) != 0 {
				t.Fatalf("key %q refused: %+v", ok, issues)
			}
		}
	})

	t.Run("not a document", func(t *testing.T) {
		for name, raw := range map[string]string{"not json": `{`, "an array": `[]`, "a string": `"x"`, "wrong type": `{"version": 2, "steps": "x"}`,
			"empty": ``} {
			if _, issues := ParseFlowDoc([]byte(raw)); len(issues) == 0 {
				t.Fatalf("%s was accepted", name)
			}
		}
	})

	t.Run("custom JSON refuses what it does not know", func(t *testing.T) {
		tests := map[string]func(d map[string]any){
			"loop max as a string":   func(d map[string]any) { firstOf(d, "loops")["max"] = "three" },
			"loop max fraction":      func(d map[string]any) { firstOf(d, "loops")["max"] = 2.5 },
			"loop max without a var": func(d map[string]any) { firstOf(d, "loops")["max"] = map[string]any{"cap": 3} },
			"join over a word":       func(d map[string]any) { stepOf(d, "check")["join"].(map[string]any)["over"] = "most" },
			"join over a number":     func(d map[string]any) { stepOf(d, "check")["join"].(map[string]any)["over"] = 3 },
			"join over a bad list":   func(d map[string]any) { stepOf(d, "check")["join"].(map[string]any)["over"] = []any{1} },
		}
		for name, edit := range tests {
			t.Run(name, func(t *testing.T) {
				if _, issues := ParseFlowDoc(withJSON(t, edit)); len(issues) == 0 {
					t.Fatal("accepted")
				}
			})
		}
	})
}

func TestLoopMaxAndJoinOverJSON(t *testing.T) {
	var fixed, fromVar Loop
	if err := json.Unmarshal([]byte(`{"key":"a","header":"h","max":3}`), &fixed); err != nil {
		t.Fatal(err)
	}
	if fixed.Max != (LoopMax{N: 3}) || fixed.Max.FromVar() || fixed.Max.Limit() != 3 || fixed.EffectiveReset() != LoopResetOnEntry {
		t.Fatalf("fixed loop = %+v", fixed)
	}
	if err := json.Unmarshal([]byte(`{"key":"a","header":"h","max":{"var":"rounds","cap":5},"reset":"never"}`), &fromVar); err != nil {
		t.Fatal(err)
	}
	if fromVar.Max != (LoopMax{Var: "rounds", Cap: 5}) || !fromVar.Max.FromVar() || fromVar.Max.Limit() != 5 || fromVar.EffectiveReset() != LoopResetNever {
		t.Fatalf("variable loop = %+v", fromVar)
	}
	for _, l := range []Loop{fixed, fromVar} {
		raw, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		var again Loop
		if err := json.Unmarshal(raw, &again); err != nil || again != l {
			t.Fatalf("round trip of %s = %+v, %v", raw, again, err)
		}
	}

	tests := map[string]struct {
		raw  string
		want JoinOver
		out  string
	}{
		"gates": {`"gates"`, JoinOver{Kind: JoinOverGates}, `"gates"`},
		"all":   {`"all"`, JoinOver{Kind: JoinOverAll}, `"all"`},
		"list":  {`["a","b"]`, JoinOver{Kind: JoinOverTasks, Tasks: []string{"a", "b"}}, `["a","b"]`},
		"empty": {`[]`, JoinOver{Kind: JoinOverTasks, Tasks: []string{}}, `[]`},
	}
	for name, tt := range tests {
		t.Run("join over "+name, func(t *testing.T) {
			var got JoinOver
			if err := json.Unmarshal([]byte(tt.raw), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			raw, err := json.Marshal(got)
			if err != nil || string(raw) != tt.out {
				t.Fatalf("Marshal = %s, %v; want %s", raw, err, tt.out)
			}
		})
	}
}

func TestDocDefaults(t *testing.T) {
	step := DocStep{}
	if j := step.EffectiveJoin(); j.Over.Kind != JoinOverGates || j.Mode != JoinModeAll || j.Block != JoinBlockImmediate {
		t.Fatalf("default join = %+v", j)
	}
	if !(Instances{}).IsDistinct() {
		t.Fatal("instances are distinct unless said otherwise")
	}
	no := false
	if (Instances{Distinct: &no}).IsDistinct() {
		t.Fatal("distinct:false was ignored")
	}
	p := Policy{MaxTaskIssues: 900}.Effective()
	if p.TaskIdleMinutes != 120 || p.TaskHardCapMinutes != 240 || p.TaskMaxAttempts != 2 || p.HumanRemindMinutes != 1440 ||
		p.HumanStallMinutes != 10080 || p.MaxActivations != 100 || p.MaxTaskIssues != DocMaxTaskIssuesHard {
		t.Fatalf("effective policy = %+v", p)
	}
	if p := (Policy{}).Effective(); p.MaxTaskIssues != 300 || p.LeadMayWork || p.AllowUnattendedRelease {
		t.Fatalf("default policy = %+v", p)
	}
}

func TestParseFlowDoc_TaskInputsSetsSkills(t *testing.T) {
	doc := parseSample(t)
	var collect, write, verify DocTask
	for _, s := range doc.Steps {
		for _, task := range s.Tasks {
			switch task.Key {
			case "collect":
				collect = task
			case "write":
				write = task
			case "verify":
				verify = task
			}
		}
	}
	if !reflect.DeepEqual(collect.Sets, []string{"sources"}) || !reflect.DeepEqual(collect.Skills, []string{"web-research"}) {
		t.Fatalf("collect sets/skills = %v / %v", collect.Sets, collect.Skills)
	}
	if collect.Requires == nil || !reflect.DeepEqual(collect.Requires.Vars, []string{"sources"}) ||
		len(collect.Requires.Files) != 1 || collect.Requires.Files[0].Slot != "notes" || collect.Requires.Files[0].Ext[0] != ".md" {
		t.Fatalf("collect requires = %+v", collect.Requires)
	}
	if len(write.Inputs) != 1 || write.Inputs[0] != (Input{From: "research.collect", Slot: "notes", Required: true}) {
		t.Fatalf("write inputs = %+v", write.Inputs)
	}
	if write.Requires.Files[0].Max != 2 {
		t.Fatalf("write files = %+v", write.Requires.Files)
	}
	if verify.Instances == nil || verify.Instances.Count != 2 || !verify.Instances.IsDistinct() || verify.Deadline == nil || verify.Deadline.IdleMinutes != 60 {
		t.Fatalf("verify = %+v", verify)
	}

	t.Run("requires.vars must be a subset of sets", func(t *testing.T) {
		_, issues := ParseFlowDoc(withJSON(t, func(d map[string]any) {
			stepOf(d, "research")["tasks"].([]any)[0].(map[string]any)["sets"] = []any{}
		}))
		if len(issues) != 1 || issues[0].Code != CodeVarSetInvalid || issues[0].Path != "steps[research].tasks[collect].requires.vars" {
			t.Fatalf("issues = %+v, want one %s at steps[research].tasks[collect].requires.vars", issues, CodeVarSetInvalid)
		}
	})
	t.Run("inputs, sets and skills are keys the parser knows", func(t *testing.T) {
		for _, key := range []string{"inputs", "sets", "skills"} {
			if _, issues := ParseFlowDoc(withJSON(t, func(d map[string]any) {
				stepOf(d, "draft")["tasks"].([]any)[0].(map[string]any)[key] = nil
			})); len(issues) != 0 {
				t.Fatalf("%s refused: %+v", key, issues)
			}
		}
	})
}
