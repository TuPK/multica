package issueworkflow

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDocTaskEffectsJSON(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{"unset", `{"key":"review","title":"Review","kind":"work","role":"reviewer"}`, ""},
		{"external", `{"key":"review","title":"Review","kind":"work","role":"reviewer","effects":"external"}`, "external"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var task DocTask
			if err := json.Unmarshal([]byte(tc.input), &task); err != nil {
				t.Fatal(err)
			}
			if task.Effects != tc.want {
				t.Fatalf("effects = %q, want %q", task.Effects, tc.want)
			}
			got, err := json.Marshal(task)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.input {
				t.Fatalf("JSON = %s, want %s", got, tc.input)
			}
		})
	}
}

func TestParseFlowDocTaskEffects(t *testing.T) {
	const validDoc = `{
		"version": 2,
		"start": "collect",
		"roles": [{"key": "worker", "kind": "agent"}],
		"terminals": [{"key": "done", "status": "done"}],
		"steps": [{
			"key": "collect",
			"status": {"key": "in_progress"},
			"kind": "tasks",
			"tasks": [{
				"key": "docs",
				"title": "Send docs",
				"kind": "work",
				"role": "worker",
				"effects": "external"
			}],
			"edges": [{"id": "finish", "to": "done"}]
		}],
		"policy": {}
	}`

	doc, issues := ParseFlowDoc([]byte(validDoc))
	if len(issues) != 0 {
		t.Fatalf("ParseFlowDoc valid effects=external returned unexpected issues: %v", issues)
	}
	if len(doc.Steps) != 1 || len(doc.Steps[0].Tasks) != 1 || doc.Steps[0].Tasks[0].Effects != EffectsExternal {
		t.Fatalf("parsed Effects = %q, want %q", doc.Steps[0].Tasks[0].Effects, EffectsExternal)
	}

	const unknownKeyDoc = `{
		"version": 2,
		"start": "collect",
		"roles": [{"key": "worker", "kind": "agent"}],
		"terminals": [{"key": "done", "status": "done"}],
		"steps": [{
			"key": "collect",
			"status": {"key": "in_progress"},
			"kind": "tasks",
			"tasks": [{
				"key": "docs",
				"title": "Send docs",
				"kind": "work",
				"role": "worker",
				"effects": "external",
				"bogus": 1
			}],
			"edges": [{"id": "finish", "to": "done"}]
		}],
		"policy": {}
	}`

	_, issues = ParseFlowDoc([]byte(unknownKeyDoc))
	var sawBogus, sawEffects bool
	for _, iss := range issues {
		if iss.Code == CodeFlowDocInvalid && strings.Contains(iss.Path, "bogus") {
			sawBogus = true
		}
		if iss.Code == CodeFlowDocInvalid && strings.Contains(iss.Path, "effects") {
			sawEffects = true
		}
	}
	if !sawBogus {
		t.Fatalf("expected unknown key issue for bogus, got: %v", issues)
	}
	if sawEffects {
		t.Fatalf("unexpected issue for effects: %v", issues)
	}
}
