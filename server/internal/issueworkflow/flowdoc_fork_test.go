package issueworkflow

import (
	"encoding/json"
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
