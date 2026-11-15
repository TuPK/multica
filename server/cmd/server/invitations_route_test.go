package main

import (
	"context"
	"net/http"
	"os"
	"testing"
)

// The pending-invitation list exposes invitee emails, so only owners and
// admins may read it.
//
// Database-backed. TestMain connects through DATABASE_URL, so this test runs
// only when PLATFORM_TEST_DATABASE_URL is set to the same dedicated *_test DSN.
func TestListInvitationsThroughRouterRequiresOwnerOrAdmin(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" || dsn != os.Getenv("DATABASE_URL") {
		t.Skip("PLATFORM_TEST_DATABASE_URL not set or differs from DATABASE_URL")
	}
	ctx := context.Background()

	const slug = "integration-tests-invitations-403"
	_, _ = testPool.Exec(ctx, `DELETE FROM workspace WHERE slug = $1`, slug)

	var wsID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO workspace (name, slug, description)
		VALUES ($1, $2, $3)
		RETURNING id
	`, "Integration Tests Invitations 403", slug, "invitation list permission test").Scan(&wsID); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM member WHERE workspace_id = $1`, wsID)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, wsID)
	})

	for _, tc := range []struct {
		role string
		want int
	}{
		{"member", http.StatusForbidden},
		{"admin", http.StatusOK},
		{"owner", http.StatusOK},
	} {
		t.Run(tc.role, func(t *testing.T) {
			if _, err := testPool.Exec(ctx, `DELETE FROM member WHERE workspace_id = $1`, wsID); err != nil {
				t.Fatalf("reset members: %v", err)
			}
			if _, err := testPool.Exec(ctx, `
				INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, $3)
			`, wsID, testUserID, tc.role); err != nil {
				t.Fatalf("create member: %v", err)
			}

			req, err := http.NewRequest("GET", testServer.URL+"/api/workspaces/"+wsID+"/invitations", nil)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+testToken)
			req.Header.Set("X-Workspace-ID", wsID)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatalf("role %s: got %d, want %d", tc.role, resp.StatusCode, tc.want)
			}
		})
	}
}
