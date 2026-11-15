package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// The workspace state matrix through the real router and the real workspace
// middleware: which requests a read_only, suspended or pending_deletion
// workspace lets through, and with which code it refuses the rest.
//
// Database-backed. TestMain connects through DATABASE_URL, so this test runs
// only when PLATFORM_TEST_DATABASE_URL is set to the same dedicated *_test DSN.
func TestWorkspaceStateMatrixThroughRouter(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" || dsn != os.Getenv("DATABASE_URL") {
		t.Skip("PLATFORM_TEST_DATABASE_URL not set or differs from DATABASE_URL")
	}
	ctx := context.Background()

	const slug = "integration-tests-workspace-state"
	_, _ = testPool.Exec(ctx, `DELETE FROM workspace WHERE slug = $1`, slug)
	var wsID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO workspace (name, slug, description)
		VALUES ($1, $2, $3)
		RETURNING id
	`, "Integration Tests Workspace State", slug, "workspace state matrix").Scan(&wsID); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = testPool.Exec(bg, `DELETE FROM platform_workspace WHERE workspace_id = $1`, wsID)
		_, _ = testPool.Exec(bg, `DELETE FROM member WHERE workspace_id = $1`, wsID)
		_, _ = testPool.Exec(bg, `DELETE FROM workspace WHERE id = $1`, wsID)
	})
	setRole := func(role string) {
		t.Helper()
		if _, err := testPool.Exec(ctx, `DELETE FROM member WHERE workspace_id = $1`, wsID); err != nil {
			t.Fatalf("reset members: %v", err)
		}
		if _, err := testPool.Exec(ctx, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, $3)`, wsID, testUserID, role); err != nil {
			t.Fatalf("create member: %v", err)
		}
	}
	setState := func(status string) {
		t.Helper()
		if status == "active" {
			// A workspace with no row is active.
			if _, err := testPool.Exec(ctx, `DELETE FROM platform_workspace WHERE workspace_id = $1`, wsID); err != nil {
				t.Fatalf("clear state: %v", err)
			}
			return
		}
		if _, err := testPool.Exec(ctx, `
			INSERT INTO platform_workspace (workspace_id, status) VALUES ($1, $2)
			ON CONFLICT (workspace_id) DO UPDATE SET status = EXCLUDED.status`, wsID, status); err != nil {
			t.Fatalf("set state: %v", err)
		}
	}
	call := func(method, path, body string) (int, string) {
		t.Helper()
		req, err := http.NewRequest(method, testServer.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+testToken)
		req.Header.Set("X-Workspace-ID", wsID)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var out struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(raw, &out)
		return resp.StatusCode, out.Code
	}

	const (
		ro  = "workspace_read_only"
		sus = "workspace_suspended"
		pd  = "workspace_pending_deletion"
	)
	ws := "/api/workspaces/" + wsID
	cases := []struct {
		name         string
		role         string
		method, path string
		body         string
		// want is the refusal code per status; "" means the state check lets the
		// request through (the handler may still answer with any status).
		readOnly, suspended, pendingDeletion string
	}{
		{"list issues", "owner", "GET", "/api/issues", "", "", sus, pd},
		{"create an issue", "owner", "POST", "/api/issues", `{"title":"x"}`, ro, sus, pd},
		{"read the workspace", "member", "GET", ws, "", "", "", ""},
		{"list members", "member", "GET", ws + "/members", "", "", sus, pd},
		{"update the workspace", "owner", "PATCH", ws, `{"description":"x"}`, ro, sus, pd},
		{"revoke an invitation", "owner", "DELETE", ws + "/invitations/00000000-0000-0000-0000-000000000001", "", "", sus, pd},
		{"subscription summary, owner", "owner", "GET", "/api/cloud-subscriptions/summary", "", "", "", ""},
		{"subscription summary, member", "member", "GET", "/api/cloud-subscriptions/summary", "", "", sus, pd},
	}
	for _, c := range cases {
		for _, s := range []struct{ status, want string }{
			{"active", ""}, {"read_only", c.readOnly}, {"suspended", c.suspended}, {"pending_deletion", c.pendingDeletion},
		} {
			t.Run(c.name+"/"+s.status, func(t *testing.T) {
				setRole(c.role)
				setState(s.status)
				status, code := call(c.method, c.path, c.body)
				if s.want == "" {
					if strings.HasPrefix(code, "workspace_") {
						t.Fatalf("%s %s refused with %q (status %d), want it to pass the state check", c.method, c.path, code, status)
					}
					return
				}
				if status != http.StatusForbidden || code != s.want {
					t.Fatalf("%s %s = %d %q, want 403 %q", c.method, c.path, status, code, s.want)
				}
			})
		}
	}
}
