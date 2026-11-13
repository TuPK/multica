package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestIssueFlowHelp(t *testing.T) {
	want := map[string][]string{
		"show":     {"multica issue flow show <id>", "not_managed", "step", "tasks"},
		"plan":     {"--switch", "key=value", "--blocked", "--missing", "plan step"},
		"advance":  {"idempotent", "do not set the status of the request"},
		"switch":   {"multica issue flow switch <id> <key> <value>"},
		"resume":   {"--to-step", "--reset-rework", "--reset-loops", "--skip-cancelled", "squad"},
		"detach":   {"--cancel-tasks", "ordinary sub-issues"},
		"start":    {"multica issue flow start <id>"},
		"complete": {"multica issue flow complete <id> <step>", "--verdict", "--set", "--blocked", "plan"},
		"vars":     {"multica issue flow vars <id>", "comma separated", "role the variable names"},
		"set":      {"multica issue flow set <task-id> <key> <value>", "while the task is open", "requires a variable"},
		"exits":    {"multica issue flow exits <id>", "manual step", "human checkpoint"},
	}
	commands := map[string]bool{}
	for _, c := range issueFlowCmd.Commands() {
		commands[c.Name()] = true
		parts, ok := want[c.Name()]
		if !ok {
			t.Errorf("unexpected subcommand %q", c.Name())
			continue
		}
		text := c.Long + "\n" + c.Example + "\n" + c.UsageString()
		for _, part := range parts {
			if !strings.Contains(text, part) {
				t.Errorf("help of %q is missing %q:\n%s", c.Name(), part, text)
			}
		}
	}
	for name := range want {
		if !commands[name] {
			t.Errorf("missing subcommand %q", name)
		}
	}
	if status := issueStatusCmd.Flag("outcome"); status == nil || !strings.Contains(status.Usage, "rejects") {
		t.Errorf("the --outcome help of `issue status` does not say the server rejects a close without a verdict: %v", status)
	}
	if flag := issueStatusCmd.Flag("outcome"); !strings.Contains(flag.Usage, "verdict key of your task") {
		t.Errorf("the --outcome help of `issue status` does not say a task has verdicts of its own: %q", flag.Usage)
	}
	if !strings.Contains(issueStatusCmd.Long, "multica issue flow") {
		t.Errorf("`issue status` help does not point at the flow commands:\n%s", issueStatusCmd.Long)
	}
}

func TestParseFlowSwitches(t *testing.T) {
	got, err := parseFlowSwitches([]string{"customs_lookup=true", "route=shipment_dossier", "export=false"})
	if err != nil {
		t.Fatal(err)
	}
	if got["customs_lookup"] != true || got["export"] != false || got["route"] != "shipment_dossier" {
		t.Fatalf("switches = %v", got)
	}
	for _, bad := range []string{"novalue", "=x", "key="} {
		if _, err := parseFlowSwitches([]string{bad}); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestIssueFlowCommandsCallTheEndpoints(t *testing.T) {
	t.Chdir(t.TempDir())
	const issueID = "1881a167-4bb6-4602-944b-f40ce4192fe6"
	type call struct {
		method, path string
		body         map[string]any
	}
	var calls []call
	notManaged := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		calls = append(calls, call{r.Method, r.URL.Path, body})
		switch {
		case r.URL.Path == "/api/issues/"+issueID:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": issueID, "identifier": "MUL-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/"+issueID+"/flow" && notManaged:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "not run by a flow", "code": "flow_not_managed"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"managed": true, "step": "collect"})
		}
	}))
	defer srv.Close()
	t.Setenv("MULTICA_SERVER_URL", srv.URL)
	t.Setenv("MULTICA_WORKSPACE_ID", "ws-1")
	t.Setenv("MULTICA_TOKEN", "test-token")

	flowCall := func() call {
		t.Helper()
		for _, c := range calls {
			if strings.Contains(c.path, "/flow") {
				return c
			}
		}
		t.Fatalf("no flow call in %v", calls)
		return call{}
	}
	newCmd := func(flags func(*cobra.Command)) *cobra.Command {
		cmd := &cobra.Command{Use: "flow"}
		cmd.Flags().String("output", "json", "")
		flags(cmd)
		return cmd
	}

	t.Run("show", func(t *testing.T) {
		calls = nil
		out, err := captureStdout(t, func() error { return runIssueFlowShow(newCmd(func(*cobra.Command) {}), []string{issueID}) })
		if err != nil || !strings.Contains(out, `"managed": true`) {
			t.Fatalf("err=%v out=%s", err, out)
		}
		if c := flowCall(); c.method != "GET" || c.path != "/api/issues/"+issueID+"/flow" {
			t.Fatalf("call = %+v", c)
		}
	})
	t.Run("show an issue no flow runs", func(t *testing.T) {
		calls, notManaged = nil, true
		defer func() { notManaged = false }()
		out, err := captureStdout(t, func() error { return runIssueFlowShow(newCmd(func(*cobra.Command) {}), []string{issueID}) })
		if err != nil || !strings.Contains(out, `"managed": false`) {
			t.Fatalf("err=%v out=%s, want managed false and no error", err, out)
		}
	})
	t.Run("plan", func(t *testing.T) {
		calls = nil
		cmd := newCmd(func(c *cobra.Command) {
			c.Flags().StringArray("switch", []string{"customs_lookup=true", "route=shipment_dossier"}, "")
			c.Flags().Bool("blocked", false, "")
			c.Flags().StringArray("missing", nil, "")
			c.Flags().String("reason", "", "")
		})
		if _, err := captureStdout(t, func() error { return runIssueFlowPlan(cmd, []string{issueID}) }); err != nil {
			t.Fatal(err)
		}
		c := flowCall()
		switches, _ := c.body["switches"].(map[string]any)
		if c.method != "POST" || c.path != "/api/issues/"+issueID+"/flow/plan" || switches["customs_lookup"] != true || switches["route"] != "shipment_dossier" || c.body["blocked"] != nil {
			t.Fatalf("call = %+v", c)
		}
	})
	t.Run("plan blocked", func(t *testing.T) {
		calls = nil
		cmd := newCmd(func(c *cobra.Command) {
			c.Flags().StringArray("switch", nil, "")
			c.Flags().Bool("blocked", true, "")
			c.Flags().StringArray("missing", []string{"invoice", "packing list"}, "")
			c.Flags().String("reason", "no invoice", "")
		})
		if _, err := captureStdout(t, func() error { return runIssueFlowPlan(cmd, []string{issueID}) }); err != nil {
			t.Fatal(err)
		}
		blocked, _ := flowCall().body["blocked"].(map[string]any)
		missing, _ := blocked["missing"].([]any)
		if len(missing) != 2 || blocked["reason"] != "no invoice" {
			t.Fatalf("blocked = %v", blocked)
		}
	})
	t.Run("plan --missing needs --blocked", func(t *testing.T) {
		cmd := newCmd(func(c *cobra.Command) {
			c.Flags().StringArray("switch", nil, "")
			c.Flags().Bool("blocked", false, "")
			c.Flags().StringArray("missing", []string{"invoice"}, "")
			c.Flags().String("reason", "", "")
		})
		if _, err := captureStdout(t, func() error { return runIssueFlowPlan(cmd, []string{issueID}) }); err == nil {
			t.Fatal("--missing without --blocked was accepted")
		}
	})
	t.Run("advance", func(t *testing.T) {
		calls = nil
		if _, err := captureStdout(t, func() error { return runIssueFlowAdvance(newCmd(func(*cobra.Command) {}), []string{issueID}) }); err != nil {
			t.Fatal(err)
		}
		if c := flowCall(); c.method != "POST" || c.path != "/api/issues/"+issueID+"/flow/advance" {
			t.Fatalf("call = %+v", c)
		}
	})
	t.Run("switch", func(t *testing.T) {
		calls = nil
		if _, err := captureStdout(t, func() error {
			return runIssueFlowSwitch(newCmd(func(*cobra.Command) {}), []string{issueID, "export", "true"})
		}); err != nil {
			t.Fatal(err)
		}
		if c := flowCall(); c.method != "PUT" || c.path != "/api/issues/"+issueID+"/flow/switches/export" || c.body["value"] != true {
			t.Fatalf("call = %+v", c)
		}
	})
	t.Run("resume", func(t *testing.T) {
		calls = nil
		cmd := newCmd(func(c *cobra.Command) {
			c.Flags().String("to-step", "draft", "")
			c.Flags().Bool("reset-rework", true, "")
		})
		if _, err := captureStdout(t, func() error { return runIssueFlowResume(cmd, []string{issueID}) }); err != nil {
			t.Fatal(err)
		}
		if c := flowCall(); c.path != "/api/issues/"+issueID+"/flow/resume" || c.body["to_step"] != "draft" || c.body["reset_rework"] != true {
			t.Fatalf("call = %+v", c)
		}
	})
	t.Run("detach", func(t *testing.T) {
		calls = nil
		cmd := newCmd(func(c *cobra.Command) { c.Flags().Bool("cancel-tasks", true, "") })
		if _, err := captureStdout(t, func() error { return runIssueFlowDetach(cmd, []string{issueID}) }); err != nil {
			t.Fatal(err)
		}
		if c := flowCall(); c.path != "/api/issues/"+issueID+"/flow/detach" || c.body["cancel_tasks"] != true {
			t.Fatalf("call = %+v", c)
		}
	})
	t.Run("start", func(t *testing.T) {
		calls = nil
		if _, err := captureStdout(t, func() error { return runIssueFlowStart(newCmd(func(*cobra.Command) {}), []string{issueID}) }); err != nil {
			t.Fatal(err)
		}
		if c := flowCall(); c.method != "POST" || c.path != "/api/issues/"+issueID+"/flow/start" {
			t.Fatalf("call = %+v", c)
		}
	})
}

func TestIssueStatusPassesAnyOutcomeKey(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/MUL-1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "issue-1", "identifier": "MUL-1", "status": "todo"})
		case r.Method == http.MethodPut && r.URL.Path == "/api/issues/issue-1":
			_ = json.NewDecoder(r.Body).Decode(&body)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "issue-1", "identifier": "MUL-1", "status": "done"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	setCLITestServerEnv(t, srv.URL)
	t.Setenv("MULTICA_TASK_CONFIG_ROOT", t.TempDir())
	t.Setenv("MULTICA_TOKEN", "mat_test-token")

	// A verdict the CLI has never heard of goes to the server, which knows the set of the task.
	cmd := newIssueStatusTestCmd()
	cmd.Flags().String("outcome", "", "")
	_ = cmd.Flags().Set("outcome", "pass_with_conditions")
	if err := runIssueStatus(cmd, []string{"MUL-1", "done"}); err != nil {
		t.Fatalf("runIssueStatus: %v", err)
	}
	if body["status"] != "done" || body["outcome"] != "pass_with_conditions" {
		t.Fatalf("body = %v, want the outcome passed through", body)
	}
}

func TestIssueFlowVarsExitsCompleteCommandsCallEndpoints(t *testing.T) {
	t.Chdir(t.TempDir())
	const issueID = "1881a167-4bb6-4602-944b-f40ce4192fe6"
	type call struct {
		method, path string
		body         map[string]any
	}
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		calls = append(calls, call{r.Method, r.URL.Path, body})
		switch {
		case r.URL.Path == "/api/issues/"+issueID:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": issueID, "identifier": "MUL-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/"+issueID+"/flow":
			_ = json.NewEncoder(w).Encode(map[string]any{"managed": true, "step": "release", "vars": map[string]any{"depth": "full"},
				"next": []any{map[string]any{"id": "release_done", "trigger": "manual"}, map[string]any{"id": "auto_edge", "trigger": "auto"}}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"managed": true, "step": "collect"})
		}
	}))
	defer srv.Close()
	t.Setenv("MULTICA_SERVER_URL", srv.URL)
	t.Setenv("MULTICA_WORKSPACE_ID", "ws-1")
	t.Setenv("MULTICA_TOKEN", "test-token")
	base := "/api/issues/" + issueID + "/flow"
	last := func() call { return calls[len(calls)-1] }
	newCmd := func(flags func(*cobra.Command)) *cobra.Command {
		cmd := &cobra.Command{Use: "flow"}
		cmd.Flags().String("output", "json", "")
		flags(cmd)
		return cmd
	}

	t.Run("complete", func(t *testing.T) {
		calls = nil
		cmd := newCmd(func(c *cobra.Command) {
			c.Flags().String("verdict", "planned", "")
			c.Flags().StringArray("set", []string{"depth=full", "sources=a, b", "rounds=3", "export=true"}, "")
			c.Flags().Bool("blocked", false, "")
			c.Flags().StringArray("missing", nil, "")
			c.Flags().String("reason", "", "")
		})
		if _, err := captureStdout(t, func() error { return runIssueFlowComplete(cmd, []string{issueID, "plan"}) }); err != nil {
			t.Fatal(err)
		}
		vars, _ := last().body["vars"].(map[string]any)
		sources, _ := vars["sources"].([]any)
		if c := last(); c.method != "POST" || c.path != base+"/steps/plan/complete" || c.body["verdict"] != "planned" ||
			vars["depth"] != "full" || vars["rounds"] != float64(3) || vars["export"] != true || len(sources) != 2 || sources[1] != "b" {
			t.Fatalf("call = %+v", c)
		}
	})
	t.Run("vars set", func(t *testing.T) {
		calls = nil
		if _, err := captureStdout(t, func() error {
			return runIssueFlowVars(newCmd(func(*cobra.Command) {}), []string{issueID, "depth", "full"})
		}); err != nil {
			t.Fatal(err)
		}
		if c := last(); c.method != "PUT" || c.path != base+"/vars/depth" || c.body["value"] != "full" {
			t.Fatalf("call = %+v", c)
		}
	})
	t.Run("vars show", func(t *testing.T) {
		calls = nil
		out, err := captureStdout(t, func() error { return runIssueFlowVars(newCmd(func(*cobra.Command) {}), []string{issueID}) })
		if err != nil || !strings.Contains(out, `"depth": "full"`) {
			t.Fatalf("out %q, err %v; want the variables of the state", out, err)
		}
	})
	t.Run("set from a task", func(t *testing.T) {
		calls = nil
		if _, err := captureStdout(t, func() error {
			return runIssueFlowSet(newCmd(func(*cobra.Command) {}), []string{issueID, "topics", "a,b,c"})
		}); err != nil {
			t.Fatal(err)
		}
		topics, _ := last().body["value"].([]any)
		if c := last(); c.method != "PUT" || c.path != base+"/vars/topics" || len(topics) != 3 {
			t.Fatalf("call = %+v", c)
		}
	})
	t.Run("exits", func(t *testing.T) {
		calls = nil
		out, err := captureStdout(t, func() error { return runIssueFlowExits(newCmd(func(*cobra.Command) {}), []string{issueID}) })
		if err != nil || !strings.Contains(out, "release_done") || strings.Contains(out, "auto_edge") {
			t.Fatalf("out %q, err %v; want only the manual exits", out, err)
		}
		calls = nil
		if _, err := captureStdout(t, func() error {
			return runIssueFlowExits(newCmd(func(*cobra.Command) {}), []string{issueID, "release_done"})
		}); err != nil {
			t.Fatal(err)
		}
		if c := last(); c.method != "POST" || c.path != base+"/exits/release_done" {
			t.Fatalf("call = %+v", c)
		}
	})
	t.Run("resume sends skip_cancelled and reset_loops", func(t *testing.T) {
		calls = nil
		cmd := newCmd(func(c *cobra.Command) {
			c.Flags().String("to-step", "", "")
			c.Flags().Bool("reset-rework", false, "")
			c.Flags().Bool("skip-cancelled", true, "")
			c.Flags().StringArray("reset-loops", []string{"fixes"}, "")
		})
		if _, err := captureStdout(t, func() error { return runIssueFlowResume(cmd, []string{issueID}) }); err != nil {
			t.Fatal(err)
		}
		loops, _ := last().body["reset_loops"].([]any)
		if c := last(); c.path != base+"/resume" || c.body["skip_cancelled"] != true || len(loops) != 1 || loops[0] != "fixes" {
			t.Fatalf("call = %+v", c)
		}
	})
}
