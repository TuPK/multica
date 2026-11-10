package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
)

func newSquadMemberSetRoleTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "set-role"}
	cmd.Flags().String("server-url", "", "")
	cmd.Flags().String("workspace-id", "", "")
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("member-id", "", "")
	cmd.Flags().String("member-type", "agent", "")
	cmd.Flags().String("role", "", "")
	cmd.Flags().String("output", "json", "")
	return cmd
}

func TestRunSquadMemberSetRolePatchesRole(t *testing.T) {
	redirectTestHome(t, t.TempDir())
	t.Setenv("MULTICA_TOKEN", "test-token")
	t.Setenv("MULTICA_WORKSPACE_ID", "workspace-123")

	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if r.Header.Get("X-Workspace-ID") != "workspace-123" {
			t.Fatalf("X-Workspace-ID = %q, want workspace-123", r.Header.Get("X-Workspace-ID"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"squad_id":    "squad-123",
			"member_id":   "member-456",
			"member_type": "agent",
			"role":        "reviewer",
		})
	}))
	defer srv.Close()
	t.Setenv("MULTICA_SERVER_URL", srv.URL)

	cmd := newSquadMemberSetRoleTestCmd()
	_ = cmd.Flags().Set("member-id", "member-456")
	_ = cmd.Flags().Set("member-type", "agent")
	_ = cmd.Flags().Set("role", "reviewer")
	_ = cmd.Flags().Set("output", "json")

	if err := runSquadMemberSetRole(cmd, []string{"squad-123"}); err != nil {
		t.Fatalf("runSquadMemberSetRole: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Fatalf("method = %s, want PATCH", gotMethod)
	}
	if gotPath != "/api/squads/squad-123/members/role" {
		t.Fatalf("path = %q, want /api/squads/squad-123/members/role", gotPath)
	}
	wantBody := map[string]any{"member_id": "member-456", "member_type": "agent", "role": "reviewer"}
	for k, want := range wantBody {
		if gotBody[k] != want {
			t.Fatalf("body[%s] = %v, want %v (full body: %#v)", k, gotBody[k], want, gotBody)
		}
	}
}

func TestRunSquadMemberSetRoleValidatesRequiredFlags(t *testing.T) {
	cmd := newSquadMemberSetRoleTestCmd()
	if err := runSquadMemberSetRole(cmd, []string{"squad-123"}); err == nil {
		t.Fatal("expected missing --member-id error")
	}

	cmd = newSquadMemberSetRoleTestCmd()
	_ = cmd.Flags().Set("member-id", "member-456")
	_ = cmd.Flags().Set("member-type", "invalid")
	if err := runSquadMemberSetRole(cmd, []string{"squad-123"}); err == nil {
		t.Fatal("expected invalid --member-type error")
	}

	cmd = newSquadMemberSetRoleTestCmd()
	_ = cmd.Flags().Set("member-id", "member-456")
	if err := runSquadMemberSetRole(cmd, []string{"squad-123"}); err == nil {
		t.Fatal("expected missing --role error")
	}
}

func newSquadHandoffTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "handoff"}
	cmd.Flags().String("server-url", "", "")
	cmd.Flags().String("workspace-id", "", "")
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("to", "", "")
	cmd.Flags().String("mode", "", "")
	cmd.Flags().String("output", "json", "")
	return cmd
}

func TestRunSquadHandoffPostsTheRequest(t *testing.T) {
	redirectTestHome(t, t.TempDir())
	t.Setenv("MULTICA_TOKEN", "test-token")
	t.Setenv("MULTICA_WORKSPACE_ID", "workspace-123")

	const issueID = "11111111-1111-4111-8111-111111111111"
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "off"})
	}))
	defer srv.Close()
	t.Setenv("MULTICA_SERVER_URL", srv.URL)

	if err := runSquadHandoff(newSquadHandoffTestCmd(), []string{issueID}); err != nil {
		t.Fatalf("runSquadHandoff: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/issues/"+issueID+"/handoff" {
		t.Fatalf("request = %s %s, want POST /api/issues/%s/handoff", gotMethod, gotPath, issueID)
	}
}

func TestRunSquadHandoffRuleSetSendsTargetAndMode(t *testing.T) {
	redirectTestHome(t, t.TempDir())
	t.Setenv("MULTICA_TOKEN", "test-token")
	t.Setenv("MULTICA_WORKSPACE_ID", "workspace-123")

	const (
		source = "22222222-2222-4222-8222-222222222222"
		target = "33333333-3333-4333-8333-333333333333"
	)
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotBody = nil
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"squad_id": source, "target_squad_id": target, "mode": "auto"})
	}))
	defer srv.Close()
	t.Setenv("MULTICA_SERVER_URL", srv.URL)

	cmd := newSquadHandoffTestCmd()
	_ = cmd.Flags().Set("to", target)
	_ = cmd.Flags().Set("mode", "auto")
	if err := runSquadHandoffRuleSet(cmd, []string{source}); err != nil {
		t.Fatalf("runSquadHandoffRuleSet: %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/squads/"+source+"/handoff-rule" {
		t.Fatalf("request = %s %s, want PUT /api/squads/%s/handoff-rule", gotMethod, gotPath, source)
	}
	if gotBody["target_squad_id"] != target || gotBody["mode"] != "auto" {
		t.Fatalf("body = %#v, want target %s and mode auto", gotBody, target)
	}

	// No --to removes the rule: the body carries no target.
	if err := runSquadHandoffRuleSet(newSquadHandoffTestCmd(), []string{source}); err != nil {
		t.Fatalf("runSquadHandoffRuleSet (remove): %v", err)
	}
	if _, has := gotBody["target_squad_id"]; has {
		t.Fatalf("removing the rule sent a target: %#v", gotBody)
	}
}

func TestRunSquadHandoffRuleSetNeedsAMode(t *testing.T) {
	cmd := newSquadHandoffTestCmd()
	_ = cmd.Flags().Set("to", "33333333-3333-4333-8333-333333333333")
	if err := runSquadHandoffRuleSet(cmd, []string{"22222222-2222-4222-8222-222222222222"}); err == nil {
		t.Fatal("expected a missing --mode error")
	}
}
