package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const (
	queueOrderIssueA = "11111111-1111-4111-8111-111111111111"
	queueOrderIssueB = "22222222-2222-4222-8222-222222222222"
)

func newIssueQueueOrderTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "queue-order"}
	cmd.Flags().Int("number", 0, "")
	cmd.Flags().Bool("clear", false, "")
	cmd.Flags().Bool("preview", false, "")
	cmd.Flags().String("output", "table", "")
	cmd.MarkFlagsMutuallyExclusive("number", "clear")
	cmd.MarkFlagsOneRequired("number", "clear")
	return cmd
}

func TestRunIssueQueueOrderSetsNumberOnIssues(t *testing.T) {
	chdirWithDaemonTaskMarker(t)
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"updated": 2})
	}))
	defer srv.Close()
	setCLITestServerEnv(t, srv.URL)
	t.Setenv("MULTICA_TOKEN", "mat_test-token")

	cmd := newIssueQueueOrderTestCmd()
	_ = cmd.Flags().Set("number", "3")
	out, err := captureStdout(t, func() error {
		return runIssueQueueOrder(cmd, []string{queueOrderIssueA, queueOrderIssueB})
	})
	if err != nil {
		t.Fatalf("runIssueQueueOrder: %v", err)
	}
	if gotPath != "/api/issues/batch-queue-order" {
		t.Fatalf("path = %q, want /api/issues/batch-queue-order", gotPath)
	}
	wantBody := map[string]any{
		"issue_ids":   []any{queueOrderIssueA, queueOrderIssueB},
		"queue_order": float64(3),
	}
	if !reflect.DeepEqual(gotBody, wantBody) {
		t.Fatalf("body = %#v, want %#v", gotBody, wantBody)
	}
	if want := "Set run queue order 3 on 2 issue(s).\n"; out != want {
		t.Fatalf("stdout = %q, want %q", out, want)
	}
}

func TestRunIssueQueueOrderClearSendsNull(t *testing.T) {
	chdirWithDaemonTaskMarker(t)
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"updated": 1})
	}))
	defer srv.Close()
	setCLITestServerEnv(t, srv.URL)
	t.Setenv("MULTICA_TOKEN", "mat_test-token")

	cmd := newIssueQueueOrderTestCmd()
	_ = cmd.Flags().Set("clear", "true")
	out, err := captureStdout(t, func() error {
		return runIssueQueueOrder(cmd, []string{queueOrderIssueA})
	})
	if err != nil {
		t.Fatalf("runIssueQueueOrder: %v", err)
	}
	if v, ok := gotBody["queue_order"]; !ok || v != nil {
		t.Fatalf("queue_order = %#v (present %v), want explicit null", v, ok)
	}
	if want := "Cleared the run queue order on 1 issue(s).\n"; out != want {
		t.Fatalf("stdout = %q, want %q", out, want)
	}
}

func TestRunIssueQueueOrderPreviewWritesNothing(t *testing.T) {
	chdirWithDaemonTaskMarker(t)
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{"release_now": 1, "run_count": 1})
	}))
	defer srv.Close()
	setCLITestServerEnv(t, srv.URL)
	t.Setenv("MULTICA_TOKEN", "mat_test-token")

	cmd := newIssueQueueOrderTestCmd()
	_ = cmd.Flags().Set("number", "1")
	_ = cmd.Flags().Set("preview", "true")
	out, err := captureStdout(t, func() error {
		return runIssueQueueOrder(cmd, []string{queueOrderIssueA})
	})
	if err != nil {
		t.Fatalf("runIssueQueueOrder: %v", err)
	}
	if gotPath != "/api/issues/queue-order/preview" {
		t.Fatalf("path = %q, want the preview endpoint only", gotPath)
	}
	if want := "1 issue(s) would move to todo now; 1 would start an agent run.\n"; out != want {
		t.Fatalf("stdout = %q, want %q", out, want)
	}
}

func TestRunIssueQueueOrderRejectsBadInputBeforeRequest(t *testing.T) {
	tests := []struct {
		name    string
		flags   map[string]string
		wantErr string
	}{
		{name: "zero number", flags: map[string]string{"number": "0"}, wantErr: "--number must be a whole number"},
		{name: "number above limit", flags: map[string]string{"number": "1000001"}, wantErr: "--number must be a whole number"},
		{name: "preview with clear", flags: map[string]string{"clear": "true", "preview": "true"}, wantErr: "--preview needs --number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chdirWithDaemonTaskMarker(t)
			var requests int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
			}))
			defer srv.Close()
			setCLITestServerEnv(t, srv.URL)
			t.Setenv("MULTICA_TOKEN", "mat_test-token")

			cmd := newIssueQueueOrderTestCmd()
			for name, value := range tt.flags {
				_ = cmd.Flags().Set(name, value)
			}
			err := runIssueQueueOrder(cmd, []string{queueOrderIssueA})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
			}
			if requests != 0 {
				t.Fatalf("requests = %d, want 0", requests)
			}
		})
	}
}

func TestIssueQueueOrderCommandRegistration(t *testing.T) {
	cmd, _, err := issueCmd.Find([]string{"queue-order"})
	if err != nil {
		t.Fatalf("find issue queue-order: %v", err)
	}
	if cmd != issueQueueOrderCmd {
		t.Fatalf("found command = %q, want issue queue-order", cmd.CommandPath())
	}
	if cmd.Flags().Lookup("number") == nil || cmd.Flags().Lookup("clear") == nil || cmd.Flags().Lookup("preview") == nil {
		t.Fatalf("queue-order is missing --number, --clear or --preview")
	}
}
