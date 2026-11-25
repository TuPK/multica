package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const (
	testOutcomeID  = "33333333-3333-4333-8333-333333333333"
	testCriterion  = "44444444-4444-4444-8444-444444444444"
	testVerifyPath = "/api/outcomes/" + testOutcomeID + "/verifications"
)

// newOutcomeTestCmd returns the verify command with the given flags parsed, so a test runs
// the real flag set without the root command.
func newOutcomeTestCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "verify"}
	cmd.Flags().String("outcome", "", "")
	cmd.Flags().String("criterion", "", "")
	cmd.Flags().String("result", "", "")
	cmd.Flags().String("severity", "none", "")
	cmd.Flags().String("note", "", "")
	cmd.Flags().StringArray("evidence", nil, "")
	cmd.Flags().String("output", "table", "")
	cmd.Flags().String("server-url", "", "")
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	return cmd
}

// TestOutcomeVerifySendsTheCheck: the request names the outcome and the criterion, carries
// the result, note and evidence, and the command prints the recorded check.
func TestOutcomeVerifySendsTheCheck(t *testing.T) {
	var gotPath string
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "check-1", "result": "fail"})
	}))
	defer srv.Close()
	setCLITestServerEnv(t, srv.URL)

	out, err := captureStdout(t, func() error {
		return runOutcomeVerify(newOutcomeTestCmd(t, "--outcome", testOutcomeID, "--criterion", testCriterion,
			"--result", "fail", "--note", "header missing", "--severity", "high", "--evidence", "run:pnpm test#4"), nil)
	})
	if err != nil {
		t.Fatalf("runOutcomeVerify: %v", err)
	}
	if gotPath != "POST "+testVerifyPath {
		t.Fatalf("request = %s, want POST %s", gotPath, testVerifyPath)
	}
	want := map[string]any{"criterion_id": testCriterion, "result": "fail", "note": "header missing", "severity": "high"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("body[%s] = %v, want %v (body %v)", k, got[k], v, got)
		}
	}
	evidence, _ := got["evidence"].([]any)
	if len(evidence) != 1 || evidence[0].(map[string]any)["type"] != "run" || evidence[0].(map[string]any)["ref"] != "pnpm test#4" {
		t.Errorf("evidence = %v, want one run ref", got["evidence"])
	}
	if !strings.Contains(out, "check-1") {
		t.Errorf("output should name the recorded check: %q", out)
	}
}

// TestOutcomeVerifyExitsNonZeroOn4xx: a refusal from the server is an error, so the process
// exits non-zero, and the error carries the status and the machine code.
func TestOutcomeVerifyExitsNonZeroOn4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"this task is not the verification task of the criterion","code":"not_verification_task"}`))
	}))
	defer srv.Close()
	setCLITestServerEnv(t, srv.URL)

	err := runOutcomeVerify(newOutcomeTestCmd(t, "--outcome", testOutcomeID, "--result", "pass"), nil)
	if err == nil {
		t.Fatal("a 403 must be an error, so that the command exits non-zero")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %v, want it to carry the status 403", err)
	}
}

// TestOutcomeVerifyValidatesLocally: a bad id, result, severity or evidence is refused before
// the server is called.
func TestOutcomeVerifyValidatesLocally(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer srv.Close()
	setCLITestServerEnv(t, srv.URL)

	cases := map[string][]string{
		"--outcome":   {"--outcome", "not-an-id", "--result", "pass"},
		"--result":    {"--outcome", testOutcomeID},
		"--severity":  {"--outcome", testOutcomeID, "--result", "pass", "--severity", "huge"},
		"--evidence":  {"--outcome", testOutcomeID, "--result", "pass", "--evidence", "nope"},
		"--criterion": {"--outcome", testOutcomeID, "--result", "pass", "--criterion", "x"},
	}
	for want, args := range cases {
		err := runOutcomeVerify(newOutcomeTestCmd(t, args...), nil)
		if err == nil || !strings.Contains(err.Error(), strings.TrimPrefix(want, "--")) && !strings.Contains(err.Error(), want) {
			t.Errorf("args %v: err = %v, want it to mention %s", args, err, want)
		}
	}
	if calls != 0 {
		t.Fatalf("the server was called %d times for invalid input", calls)
	}
}

// TestIssueCriteriaAddAndArchiveSendTheRightBodies: a criterion goes to the issue's list with
// its kind and spec, and an archive is a PATCH that sets archived.
func TestIssueCriteriaAddAndArchiveSendTheRightBodies(t *testing.T) {
	var added, archived map[string]any
	var archivePath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/issues/MUL-7/criteria":
			_ = json.NewDecoder(r.Body).Decode(&added)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": testCriterion})
		case r.Method == http.MethodPatch:
			archivePath = r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&archived)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": testCriterion})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	setCLITestServerEnv(t, srv.URL)

	add := &cobra.Command{Use: "add"}
	addIssueCriteriaAddFlags(add)
	if err := add.ParseFlags([]string{"--check", "command", "--spec", `{"run":"pnpm test","expect_exit":0}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := captureStdout(t, func() error { return runIssueCriteriaAdd(add, []string{"MUL-7", "The suite passes."}) }); err != nil {
		t.Fatalf("add: %v", err)
	}
	if added["text"] != "The suite passes." || added["check_kind"] != "command" {
		t.Fatalf("added body = %v", added)
	}
	spec, _ := added["check_spec"].(map[string]any)
	if spec["run"] != "pnpm test" {
		t.Fatalf("check_spec = %v, want the run", added["check_spec"])
	}

	arch := &cobra.Command{Use: "archive"}
	arch.Flags().String("output", "table", "")
	if _, err := captureStdout(t, func() error { return runIssueCriteriaArchive(arch, []string{"MUL-7", testCriterion}) }); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if archived["archived"] != true || archivePath != "/api/issues/MUL-7/criteria/"+testCriterion {
		t.Fatalf("archive = %s %v, want PATCH with archived true", archivePath, archived)
	}
}
