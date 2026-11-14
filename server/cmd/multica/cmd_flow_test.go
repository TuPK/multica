package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

const flowManifestFile = `{"format":"multica.flow-manifest/v1","name":"Intake","doc":{"version":2,"start":"plan"}}`

func newFlowValidateCmd(file, squad, output string) *cobra.Command {
	cmd := &cobra.Command{Use: "validate"}
	cmd.Flags().String("file", file, "")
	cmd.Flags().String("squad", squad, "")
	cmd.Flags().String("output", output, "")
	return cmd
}

func TestFlowValidateCommand_PrintsIssuesAndExitCode(t *testing.T) {
	t.Chdir(t.TempDir())
	const squadID = "0b7d5a0e-3f3c-4a6e-9d58-5b0c1f1b7a11"
	type call struct {
		method, path string
		body         map[string]any
	}
	var calls []call
	var answer func(w http.ResponseWriter)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		calls = append(calls, call{r.Method, r.URL.Path, body})
		answer(w)
	}))
	defer srv.Close()
	t.Setenv("MULTICA_SERVER_URL", srv.URL)
	t.Setenv("MULTICA_WORKSPACE_ID", "ws-1")
	t.Setenv("MULTICA_TOKEN", "test-token")

	file := filepath.Join(t.TempDir(), "flow.json")
	if err := os.WriteFile(file, []byte(flowManifestFile), 0o600); err != nil {
		t.Fatal(err)
	}
	reply := func(status int, body any) func(http.ResponseWriter) {
		return func(w http.ResponseWriter) {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(body)
		}
	}

	t.Run("errors are printed by path and the exit is not zero", func(t *testing.T) {
		calls = nil
		answer = reply(http.StatusOK, map[string]any{"ok": false, "issues": []map[string]any{
			{"code": "stop_unknown", "severity": "error", "path": "steps[2].edges[0].stop", "message": "the stop is not declared"},
			{"code": "role_unbound", "severity": "warning", "path": "roles[0]", "message": "the role has no one yet"},
			{"code": "role_unknown", "severity": "error", "path": "steps[1].tasks[0].role", "message": "the role ghost does not exist"},
			{"code": "doc_note", "severity": "warning", "message": "a note about the document"},
		}})
		out, err := captureStdout(t, func() error { return runFlowValidate(newFlowValidateCmd(file, squadID, "table"), nil) })
		if err == nil {
			t.Fatalf("no error for a manifest with errors; out:\n%s", out)
		}
		if cli.ExitCodeFor(err) == 0 {
			t.Fatalf("exit code = 0 for %v", err)
		}
		for _, want := range []string{
			"steps[1].tasks[0].role", "role_unknown", "the role ghost does not exist",
			"steps[2].edges[0].stop", "stop_unknown",
			"roles[0]", "role_unbound", "warning",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output lacks %q:\n%s", want, out)
			}
		}
		// Issues are listed by path, the issue of the whole document first.
		order := []string{"a note about the document", "roles[0]", "steps[1].tasks[0].role", "steps[2].edges[0].stop"}
		last := -1
		for _, part := range order {
			at := strings.Index(out, part)
			if at < last {
				t.Errorf("%q is out of order in:\n%s", part, out)
			}
			last = at
		}
		if !strings.Contains(err.Error(), "2 errors") {
			t.Errorf("error = %q, want the count of errors", err)
		}
		if len(calls) != 1 || calls[0].method != "POST" || calls[0].path != "/api/flow/validate" {
			t.Fatalf("calls = %+v", calls)
		}
		manifest, _ := calls[0].body["manifest"].(map[string]any)
		if manifest["format"] != "multica.flow-manifest/v1" || manifest["name"] != "Intake" || calls[0].body["squad_id"] != squadID {
			t.Fatalf("body = %+v, want the file as manifest and the squad id", calls[0].body)
		}
	})

	t.Run("warnings alone pass", func(t *testing.T) {
		calls = nil
		answer = reply(http.StatusOK, map[string]any{"ok": true, "issues": []map[string]any{
			{"code": "role_unbound", "severity": "warning", "path": "roles[0]", "message": "the role has no one yet"},
		}})
		out, err := captureStdout(t, func() error { return runFlowValidate(newFlowValidateCmd(file, "", "table"), nil) })
		if err != nil || !strings.Contains(out, "role_unbound") {
			t.Fatalf("err = %v, out:\n%s", err, out)
		}
		if _, sent := calls[0].body["squad_id"]; sent {
			t.Fatalf("a squad id was sent without --squad: %+v", calls[0].body)
		}
	})

	t.Run("a clean manifest says so", func(t *testing.T) {
		answer = reply(http.StatusOK, map[string]any{"ok": true, "issues": []any{}})
		out, err := captureStdout(t, func() error { return runFlowValidate(newFlowValidateCmd(file, "", "table"), nil) })
		if err != nil || !strings.Contains(out, "no errors") {
			t.Fatalf("err = %v, out:\n%s", err, out)
		}
	})

	t.Run("a file the server cannot read as a manifest is reported with its issues", func(t *testing.T) {
		answer = reply(http.StatusBadRequest, map[string]any{"code": "flow_manifest_invalid", "error": "the format is wrong", "issues": []map[string]any{
			{"code": "flow_manifest_invalid", "severity": "error", "message": "the format \"x\" is not multica.flow-manifest/v1"},
		}})
		out, err := captureStdout(t, func() error { return runFlowValidate(newFlowValidateCmd(file, "", "table"), nil) })
		if err == nil || !strings.Contains(out, "flow_manifest_invalid") || !strings.Contains(out, "multica.flow-manifest/v1") {
			t.Fatalf("err = %v, out:\n%s", err, out)
		}
	})

	t.Run("json output is the answer as it came", func(t *testing.T) {
		answer = reply(http.StatusOK, map[string]any{"ok": false, "issues": []map[string]any{
			{"code": "role_unknown", "severity": "error", "path": "steps[0]", "message": "m"},
		}})
		out, err := captureStdout(t, func() error { return runFlowValidate(newFlowValidateCmd(file, "", "json"), nil) })
		var got map[string]any
		if jsonErr := json.Unmarshal([]byte(out), &got); jsonErr != nil || got["ok"] != false {
			t.Fatalf("out is not the answer: %v\n%s", jsonErr, out)
		}
		if err == nil {
			t.Fatal("json output still exits non-zero on an error")
		}
	})

	t.Run("a missing file or a missing flag stops before any request", func(t *testing.T) {
		calls = nil
		if _, err := captureStdout(t, func() error { return runFlowValidate(newFlowValidateCmd("", "", "table"), nil) }); err == nil {
			t.Error("no --file was accepted")
		}
		if _, err := captureStdout(t, func() error {
			return runFlowValidate(newFlowValidateCmd(filepath.Join(t.TempDir(), "absent.json"), "", "table"), nil)
		}); err == nil {
			t.Error("a file that is not there was accepted")
		}
		if len(calls) != 0 {
			t.Errorf("requests were made: %+v", calls)
		}
	})
}

func TestFlowCapabilitiesCommand_PrintsTheAnswer(t *testing.T) {
	t.Chdir(t.TempDir())
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.Method + " " + r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{"schemas": []int{1, 2}, "features": map[string]any{"proposals": true}})
	}))
	defer srv.Close()
	t.Setenv("MULTICA_SERVER_URL", srv.URL)
	t.Setenv("MULTICA_WORKSPACE_ID", "ws-1")
	t.Setenv("MULTICA_TOKEN", "test-token")

	cmd := &cobra.Command{Use: "capabilities"}
	cmd.Flags().String("output", "json", "")
	out, err := captureStdout(t, func() error { return runFlowCapabilities(cmd, nil) })
	if err != nil || path != "GET /api/flow/capabilities" || !strings.Contains(out, `"proposals": true`) {
		t.Fatalf("err = %v, call = %q, out:\n%s", err, path, out)
	}
}

func TestFlowCommandHelp(t *testing.T) {
	commands := map[string]string{}
	for _, c := range flowCmd.Commands() {
		commands[c.Name()] = c.Long + "\n" + c.Example + "\n" + c.UsageString()
	}
	for name, parts := range map[string][]string{
		"validate":     {"--file", "--squad", "multica.flow-manifest/v1", "exit"},
		"capabilities": {"limits", "features"},
	} {
		text, ok := commands[name]
		if !ok {
			t.Errorf("missing subcommand %q", name)
			continue
		}
		for _, part := range parts {
			if !strings.Contains(text, part) {
				t.Errorf("help of %q lacks %q:\n%s", name, part, text)
			}
		}
	}
	if flowCmd.Parent() != rootCmd {
		t.Error("`multica flow` is not a command of the root")
	}
}
