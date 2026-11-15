package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/platform-management/accountexport"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Database-backed: they run only against the dedicated *_test database named by
// PLATFORM_TEST_DATABASE_URL (see requirePlatformTestDatabase). The pure secret
// scan and the rate-limit constants are tested in platform-management/accountexport.

const exportTokenHashSentinel = "sentinel-token-hash-0123456789abcdef"

func exportRequest() *http.Request { return newRequest(http.MethodGet, "/api/me/export", nil) }

func exportHandler(limiter SlidingWindowRateLimiter) http.HandlerFunc {
	return RequireHumanActor(http.HandlerFunc(testHandler.ExportMyAccount(limiter))).ServeHTTP
}

func newExportLimiter() SlidingWindowRateLimiter { return NewMemoryAccountExportLimiter() }

func TestExportRefusesMachineToken(t *testing.T) {
	requirePlatformTestDatabase(t)
	req := exportRequest()
	req.Header.Set("X-Actor-Source", "cloud_pat")
	testutil.Call(t, exportHandler(newExportLimiter()), req).Want(http.StatusForbidden)
}

func TestExportSecondRequestInsideAnHourIs429(t *testing.T) {
	requirePlatformTestDatabase(t)
	limiter := newExportLimiter()
	cleanup := func() {
		_, _ = testPool.Exec(context.Background(),
			`DELETE FROM audit_event WHERE action = 'account.export_requested' AND actor_id = $1`, testUserID)
	}
	cleanup()
	t.Cleanup(cleanup)

	testutil.Call(t, exportHandler(limiter), exportRequest()).Want(http.StatusOK)
	resp := testutil.Call(t, exportHandler(limiter), exportRequest()).Want(http.StatusTooManyRequests)
	if got := resp.Map()["code"]; got != "export_rate_limited" {
		t.Fatalf("code = %v", got)
	}
	if resp.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After is missing")
	}
	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_event WHERE action = 'account.export_requested' AND actor_id = $1 AND workspace_id IS NULL AND detail = '{}'::jsonb`,
		testUserID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want one account.export_requested row, got %d", n)
	}
}

func TestExportHasExactKeysNoSecretsAndNoOtherUsersRows(t *testing.T) {
	requirePlatformTestDatabase(t)
	ctx := context.Background()
	patID := uuid.NewString()
	otherUser := uuid.NewString()
	if _, err := testPool.Exec(ctx,
		`INSERT INTO personal_access_token (id, user_id, name, token_hash, token_prefix) VALUES ($1, $2, 'export-test', $3, 'mul_exp')`,
		patID, testUserID, exportTokenHashSentinel); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx,
		`INSERT INTO user_consent (user_id, document, version) VALUES ($1, 'terms', 'other-user-version')`, otherUser); err != nil {
		t.Fatal(err)
	}
	otherEvent := uuid.NewString()
	if _, err := testPool.Exec(ctx,
		`INSERT INTO audit_event (id, actor_type, actor_id, action, detail) VALUES ($1, 'member', $2, 'auth.login', '{"note":"other-user-detail"}')`,
		otherEvent, otherUser); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM personal_access_token WHERE id = $1`, patID)
		_, _ = testPool.Exec(ctx, `DELETE FROM user_consent WHERE user_id = $1::uuid`, otherUser)
		_, _ = testPool.Exec(ctx, `DELETE FROM audit_event WHERE id = $1`, otherEvent)
		_, _ = testPool.Exec(ctx, `DELETE FROM audit_event WHERE action = 'account.export_requested' AND actor_id = $1`, testUserID)
	})

	resp := testutil.Call(t, exportHandler(newExportLimiter()), exportRequest()).Want(http.StatusOK)
	if got := resp.Header().Get("Content-Disposition"); got != `attachment; filename="multica-account-export.json"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := resp.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	body := resp.Text()

	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &top); err != nil {
		t.Fatal(err)
	}
	want := []string{"exported_at", "profile", "memberships", "personal_access_tokens", "consents", "security_events", "external_connections"}
	if len(top) != len(want) {
		t.Fatalf("keys = %d, want %d", len(top), len(want))
	}
	for _, k := range want {
		if _, ok := top[k]; !ok {
			t.Errorf("missing key %q", k)
		}
	}
	var exportedAt string
	_ = json.Unmarshal(top["exported_at"], &exportedAt)
	if ts, err := time.Parse(time.RFC3339, exportedAt); err != nil || ts.Location() != time.UTC {
		t.Errorf("exported_at %q is not RFC 3339 UTC", exportedAt)
	}

	if !strings.Contains(body, "mul_exp") {
		t.Error("the token prefix should be exported")
	}
	for _, leak := range []string{exportTokenHashSentinel, "token_hash", "other-user-version", "other-user-detail", otherUser} {
		if strings.Contains(body, leak) {
			t.Errorf("export contains %q", leak)
		}
	}
	var decoded any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatal(err)
	}
	if found := accountexport.FindSecrets(decoded, []string{exportTokenHashSentinel}); len(found) != 0 {
		t.Errorf("secret scan found %v", found)
	}
}
