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
		"show":    {"multica issue flow show <id>", "not_managed", "step", "tasks"},
		"plan":    {"--switch", "key=value", "--blocked", "--missing", "plan step"},
		"advance": {"idempotent", "do not set the status of the request"},
		"switch":  {"multica issue flow switch <id> <key> <value>"},
		"resume":  {"--to-step", "--reset-rework", "squad"},
		"detach":  {"--cancel-tasks", "ordinary sub-issues"},
		"start":   {"multica issue flow start <id>"},
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
