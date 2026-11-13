package handler

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
)

// The web client shows verdict controls only on an open gate, so the task kind
// in the flow state has to match the definition the engine runs.
func TestFlowTaskKind(t *testing.T) {
	def := issueworkflow.Definition{Steps: []issueworkflow.Step{
		{StatusKey: "collect", Tasks: []issueworkflow.Task{{Key: "docs", Kind: issueworkflow.KindWork}}},
		{StatusKey: "gate", Tasks: []issueworkflow.Task{{Key: "docs_gate", Kind: issueworkflow.KindGate}}},
	}}
	cases := []struct {
		name string
		step string
		key  string
		want string
	}{
		{name: "work task of its step", step: "collect", key: "docs", want: issueworkflow.KindWork},
		{name: "gate task of its step", step: "gate", key: "docs_gate", want: issueworkflow.KindGate},
		{name: "gate key under another step", step: "collect", key: "docs_gate", want: ""},
		{name: "unknown step", step: "missing", key: "docs", want: ""},
		{name: "empty task key", step: "collect", key: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := flowTaskKind(def, tc.step, tc.key); got != tc.want {
				t.Fatalf("flowTaskKind(%q, %q) = %q, want %q", tc.step, tc.key, got, tc.want)
			}
		})
	}
}
