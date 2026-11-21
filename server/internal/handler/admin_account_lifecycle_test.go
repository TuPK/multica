package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/server/internal/accountprovision"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Tests of an account an admin created from the platform console (platform
// spec 7b.5, AC41 and AC47). The row is built the way production builds it: a
// pending platform_account_request that accountprovision turns into a user and
// a workspace, so no fixture shortcut stands in for the platform's write.

type adminCreatedAccount struct {
	userID, workspaceID, email string
}

func provisionAdminCreatedAccount(t *testing.T) adminCreatedAccount {
	t.Helper()
	ctx := context.Background()
	requestID := uuid.New()
	email := "admin-made-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10] + "@example.test"
	if _, err := testPool.Exec(ctx, `
		INSERT INTO platform_account_request
			(id, kind, email, display_name, workspace_name, workspace_slug, plan_key, created_by)
		VALUES ($1, 'individual', $2, 'Made By Admin', 'Admin Made Co', '', 'free', $3)`,
		requestID, email, uuid.New()); err != nil {
		t.Fatalf("insert account request: %v", err)
	}
	var acc adminCreatedAccount
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM verification_code WHERE email = $1`, email)
		if acc.workspaceID != "" {
			_, _ = testPool.Exec(ctx, `DELETE FROM audit_event WHERE workspace_id = $1::uuid`, acc.workspaceID)
			_, _ = testPool.Exec(ctx, `DELETE FROM member WHERE workspace_id = $1::uuid`, acc.workspaceID)
			_, _ = testPool.Exec(ctx, `DELETE FROM workspace WHERE id = $1::uuid`, acc.workspaceID)
		}
		if acc.userID != "" {
			_, _ = testPool.Exec(ctx, `DELETE FROM audit_event WHERE actor_id = $1 OR target_id = $1`, acc.userID)
			_, _ = testPool.Exec(ctx, `DELETE FROM user_consent WHERE user_id = $1::uuid`, acc.userID)
			_, _ = testPool.Exec(ctx, `DELETE FROM platform_account WHERE user_id = $1::uuid`, acc.userID)
			_, _ = testPool.Exec(ctx, `DELETE FROM "user" WHERE id = $1::uuid`, acc.userID)
		}
		_, _ = testPool.Exec(ctx, `DELETE FROM platform_account_request WHERE id = $1`, requestID)
	})

	runner := accountprovision.New(accountprovision.Deps{Pool: testPool, Queries: testHandler.Queries})
	if _, err := runner.RunOnce(ctx); err != nil {
		t.Fatalf("run account provisioning: %v", err)
	}
	var userID, workspaceID pgtype.UUID
	var state string
	if err := testPool.QueryRow(ctx,
		`SELECT state, user_id, workspace_id FROM platform_account_request WHERE id = $1`, requestID).
		Scan(&state, &userID, &workspaceID); err != nil {
		t.Fatal(err)
	}
	if state != "provisioned" || !userID.Valid || !workspaceID.Valid {
		t.Fatalf("request state = %q, user %v, workspace %v; want provisioned with both", state, userID.Valid, workspaceID.Valid)
	}
	acc = adminCreatedAccount{userID: uuidToString(userID), workspaceID: uuidToString(workspaceID), email: email}
	return acc
}

// TestAdminCreatedAccountSignsInByEmailCodeWithSignupOff is AC41: the person an
// admin created signs in by an email code while ALLOW_SIGNUP is off, lands in
// the workspace with no onboarding, and is asked for consent on the first login.
func TestAdminCreatedAccountSignsInByEmailCodeWithSignupOff(t *testing.T) {
	requirePlatformTestDatabase(t)
	acc := provisionAdminCreatedAccount(t)
	setLegal(t, "1", "1")
	h := loginHandler(t)

	// The switch is really off: a stranger cannot get a code.
	stranger := testutil.UniqueEmail("stranger")
	cleanupVerificationCodes(t, stranger, acc.email)
	testutil.Call(t, h.SendCode, testutil.JSONRequest(http.MethodPost, "/auth/send-code",
		map[string]string{"email": stranger})).Want(http.StatusForbidden)

	testutil.Call(t, h.SendCode, testutil.JSONRequest(http.MethodPost, "/auth/send-code",
		map[string]string{"email": acc.email})).Want(http.StatusOK)
	sent, err := h.Queries.GetLatestVerificationCode(context.Background(), acc.email)
	if err != nil {
		t.Fatalf("read the code that was sent: %v", err)
	}

	var login struct {
		Token string `json:"token"`
		User  struct {
			ID          string  `json:"id"`
			OnboardedAt *string `json:"onboarded_at"`
		} `json:"user"`
	}
	testutil.Call(t, h.VerifyCode, testutil.JSONRequest(http.MethodPost, "/auth/verify-code",
		map[string]string{"email": acc.email, "code": sent.Code})).Want(http.StatusOK).JSON(&login)
	if login.Token == "" || login.User.ID != acc.userID {
		t.Fatalf("login = token %q, user %q; want a token for the provisioned user %q", login.Token, login.User.ID, acc.userID)
	}
	if login.User.OnboardedAt == nil {
		t.Error("onboarded_at is empty: the user would be sent through onboarding")
	}

	// The workspace is already there, so the client goes straight into it.
	listReq := newRequest(http.MethodGet, "/api/workspaces", nil)
	listReq.Header.Set("X-User-ID", acc.userID)
	var workspaces []struct {
		ID string `json:"id"`
	}
	testutil.Call(t, h.ListWorkspaces, listReq).Want(http.StatusOK).JSON(&workspaces)
	if len(workspaces) != 1 || workspaces[0].ID != acc.workspaceID {
		t.Fatalf("workspaces = %+v, want exactly the provisioned %s", workspaces, acc.workspaceID)
	}

	// The first login still shows the consent screen.
	meReq := newRequest(http.MethodGet, "/api/me", nil)
	meReq.Header.Set("X-User-ID", acc.userID)
	me := testutil.Call(t, h.GetMe, meReq).Want(http.StatusOK).Map()
	required, _ := me["consent_required"].([]any)
	if len(required) != 2 {
		t.Fatalf("consent_required = %v, want terms and privacy", me["consent_required"])
	}
}

func cleanupVerificationCodes(t *testing.T, emails ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, e := range emails {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM verification_code WHERE email = $1`, e)
		}
	})
}

// TestAdminCreatedAccountIsSuspendedRevokedAndDeletedLikeAnyAccount is AC47: an
// account an admin created is locked, signed out and deleted by the same rules
// as every other account (spec 4.1 and 4.6).
func TestAdminCreatedAccountIsSuspendedRevokedAndDeletedLikeAnyAccount(t *testing.T) {
	requirePlatformTestDatabase(t)
	ctx := context.Background()
	acc := provisionAdminCreatedAccount(t)
	uid := parseUUID(acc.userID)

	guard := accountguard.New(queriesAccountStore{testHandler.Queries}, &recordingAccountCache{})
	protected := middleware.Auth(testHandler.Queries, nil, nil, nil, guard)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	call := func(token string) (int, string) {
		req := httptest.NewRequest(http.MethodGet, "/api/issues", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)
		var body struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec.Code, body.Code
	}
	sessionAt := func(age time.Duration) string {
		at := time.Now().Add(-age).Unix()
		tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub": acc.userID, "iat": at, auth.AuthTimeClaim: at, "exp": time.Now().Add(time.Hour).Unix(),
		}).SignedString(auth.JWTSecret())
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	setState := func(status string) {
		t.Helper()
		if _, err := testHandler.Queries.SetAccountState(ctx, db.SetAccountStateParams{
			UserID: uid, Status: status, Reason: "test", ChangedBy: uid,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// A new session of the account passes.
	token := sessionAt(time.Minute)
	if status, _ := call(token); status != http.StatusOK {
		t.Fatalf("a session of the active account got %d, want 200", status)
	}

	// Suspended: the next request with the same JWT is refused.
	setState("suspended")
	if status, code := call(token); status != http.StatusForbidden || code != "account_suspended" {
		t.Fatalf("suspended account got %d %q, want 403 account_suspended", status, code)
	}
	setState("active")
	if status, _ := call(token); status != http.StatusOK {
		t.Fatalf("a reinstated account got %d, want 200", status)
	}

	// Sign out everywhere: the session from before the mark is refused.
	older := sessionAt(30 * time.Second)
	logoutReq := revokeAllRequest()
	logoutReq.Header.Set("X-User-ID", acc.userID)
	decodeRevokeAll(t, testutil.Call(t, testHandler.RevokeAllSessions(guard), logoutReq))
	if status, code := call(older); status != http.StatusUnauthorized || code != "session_revoked" {
		t.Fatalf("a session from before logout-all got %d %q, want 401 session_revoked", status, code)
	}

	// Deletion: the job anonymises the user and removes the workspace it owned alone.
	makeDue(t, acc.userID)
	done, err := testHandler.DeleteAccount(ctx, acc.userID, time.Now())
	if err != nil || !done {
		t.Fatalf("DeleteAccount = %v, %v", done, err)
	}
	if status, _ := accountStatus(t, acc.userID); status != "deleted" {
		t.Errorf("account status = %q, want deleted", status)
	}
	var email string
	if err := testPool.QueryRow(ctx, `SELECT email FROM "user" WHERE id = $1::uuid`, acc.userID).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if email == acc.email {
		t.Error("the user still carries the admin-created email")
	}
	var workspaces, members int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM workspace WHERE id = $1::uuid`, acc.workspaceID).Scan(&workspaces); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM member WHERE user_id = $1::uuid`, acc.userID).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if workspaces != 0 || members != 0 {
		t.Errorf("after deletion: %d workspaces, %d memberships; want none", workspaces, members)
	}
	if status, code := call(sessionAt(0)); status != http.StatusForbidden || code != "account_suspended" {
		t.Errorf("a deleted account got %d %q, want 403 account_suspended", status, code)
	}
}
