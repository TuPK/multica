package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/platform-management/accountdeletion"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Database-backed: they run only against the dedicated *_test database named by
// PLATFORM_TEST_DATABASE_URL (see requirePlatformTestDatabase). Each test makes
// its own users and workspaces and never touches the shared handler-test user.
// The pure rules (the 14 days, the 10-minute window, the sole-owner split, the
// due check) are tested in platform-management/accountdeletion, and the job's
// batch loop in internal/scheduler.

// deletionToken signs a session token for userID whose login was loginAge ago.
// withAuthTime false leaves the auth_time claim out.
func deletionToken(t *testing.T, userID string, loginAge time.Duration, withAuthTime bool) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": userID,
		"iat": time.Now().Add(-loginAge).Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	if withAuthTime {
		claims[auth.AuthTimeClaim] = time.Now().Add(-loginAge).Unix()
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(auth.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func deletionRequest(t *testing.T, method, path, userID, token string) *http.Request {
	t.Helper()
	req := newRequest(method, path, nil)
	req.Header.Set("X-User-ID", userID)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func requestDeletionHandler() http.HandlerFunc {
	return RequireHumanActor(testHandler.RequestAccountDeletion(nil)).ServeHTTP
}

func cancelDeletionHandler() http.HandlerFunc {
	return RequireHumanActor(testHandler.CancelAccountDeletion(nil)).ServeHTTP
}

// deletionUser makes a user with an unused account state.
func deletionUser(t *testing.T, name string) (id, email string) {
	t.Helper()
	email = testutil.UniqueEmail("del")
	id = dbfx.User(t, name, email)
	dbfx.Cleanup(t, `DELETE FROM platform_account WHERE user_id = $1::uuid`, id)
	dbfx.Cleanup(t, `DELETE FROM audit_event WHERE actor_id = $1 OR target_id = $1`, id)
	return id, email
}

func accountStatus(t *testing.T, userID string) (status string, scheduled *time.Time) {
	t.Helper()
	err := testPool.QueryRow(context.Background(),
		`SELECT status, deletion_scheduled_at FROM platform_account WHERE user_id = $1::uuid`, userID).Scan(&status, &scheduled)
	if err == pgx.ErrNoRows {
		return "active", nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return status, scheduled
}

func freshToken(t *testing.T, userID string) string {
	return deletionToken(t, userID, time.Minute, true)
}

func TestDeletionPreviewListsSoleWorkspacesAndChangesNothing(t *testing.T) {
	requirePlatformTestDatabase(t)
	uid, _ := deletionUser(t, "Preview")
	solo := dbfx.Workspace(t, "Solo space", "solo-"+uuid.NewString()[:8])
	dbfx.Member(t, solo, uid, "owner")

	resp := testutil.Call(t, requestDeletionHandler(),
		deletionRequest(t, http.MethodPost, "/api/me/deletion?preview=1", uid, freshToken(t, uid))).Want(http.StatusOK)
	var out AccountDeletionPreviewResponse
	resp.JSON(&out)
	if len(out.SoleWorkspaces) != 1 || out.SoleWorkspaces[0].ID != solo || out.SoleWorkspaces[0].Name != "Solo space" {
		t.Fatalf("sole_workspaces = %+v", out.SoleWorkspaces)
	}
	if out.ScheduledAt != nil {
		t.Fatalf("scheduled_at = %v, want null", *out.ScheduledAt)
	}
	if status, _ := accountStatus(t, uid); status != "active" {
		t.Fatalf("status = %q after a preview", status)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM audit_event WHERE action = 'account.deletion_requested' AND actor_id = $1`, uid); n != 0 {
		t.Fatalf("a preview wrote %d audit rows", n)
	}
}

func TestDeletionSoleOwnerOfSharedWorkspaceIs409AndChangesNothing(t *testing.T) {
	requirePlatformTestDatabase(t)
	uid, _ := deletionUser(t, "Owner")
	other, _ := deletionUser(t, "Other")
	shared := dbfx.Workspace(t, "Shared space", "shared-"+uuid.NewString()[:8])
	dbfx.Member(t, shared, uid, "owner")
	dbfx.Member(t, shared, other, "member")

	for _, path := range []string{"/api/me/deletion", "/api/me/deletion?preview=1"} {
		resp := testutil.Call(t, requestDeletionHandler(),
			deletionRequest(t, http.MethodPost, path, uid, freshToken(t, uid))).Want(http.StatusConflict)
		body := resp.Map()
		if body["code"] != "sole_owner" {
			t.Fatalf("%s: code = %v", path, body["code"])
		}
		ws, _ := body["workspaces"].([]any)
		if len(ws) != 1 {
			t.Fatalf("%s: workspaces = %v", path, body["workspaces"])
		}
		first, _ := ws[0].(map[string]any)
		if first["id"] != shared || first["name"] != "Shared space" || first["member_count"] != float64(2) {
			t.Fatalf("%s: workspace = %v", path, first)
		}
	}
	if status, _ := accountStatus(t, uid); status != "active" {
		t.Fatalf("status = %q after a refused request", status)
	}
}

func TestDeletionRequestSchedulesRevokesSessionsAndAudits(t *testing.T) {
	requirePlatformTestDatabase(t)
	uid, _ := deletionUser(t, "Requester")
	before := time.Now().Unix()

	resp := testutil.Call(t, requestDeletionHandler(),
		deletionRequest(t, http.MethodPost, "/api/me/deletion", uid, freshToken(t, uid))).Want(http.StatusAccepted)
	var out AccountDeletionScheduledResponse
	resp.JSON(&out)
	at, err := time.Parse(time.RFC3339, out.ScheduledAt)
	if err != nil {
		t.Fatalf("scheduled_at %q: %v", out.ScheduledAt, err)
	}
	if want := time.Now().Add(accountdeletion.Wait); at.Sub(want).Abs() > time.Minute {
		t.Fatalf("scheduled_at = %v, want about %v", at, want)
	}
	if _, has := resp.Map()["token"]; has {
		t.Fatal("the response must not carry a token")
	}

	var status, reason string
	var validAfter time.Time
	var changedBy *string
	if err := testPool.QueryRow(context.Background(),
		`SELECT status, reason, changed_by::text, sessions_valid_after FROM platform_account WHERE user_id = $1::uuid`, uid).
		Scan(&status, &reason, &changedBy, &validAfter); err != nil {
		t.Fatal(err)
	}
	if status != "pending_deletion" || reason != "user_requested" || changedBy == nil || *changedBy != uid {
		t.Fatalf("state = %q %q %v", status, reason, changedBy)
	}
	if validAfter.Unix() < before-1 || validAfter.Nanosecond() != 0 {
		t.Fatalf("sessions_valid_after = %v, want now in whole seconds", validAfter)
	}
	// The session that made the request is now older than the mark.
	if !auth.SessionRevoked(validAfter, jwt.MapClaims{auth.AuthTimeClaim: float64(before - 60)}) {
		t.Fatal("the requesting session should be revoked")
	}

	if n := dbfx.Count(t,
		`SELECT count(*) FROM audit_event WHERE action = 'account.deletion_requested' AND actor_id = $1 AND workspace_id IS NULL AND detail = '{}'::jsonb`, uid); n != 1 {
		t.Fatalf("want one account.deletion_requested row, got %d", n)
	}
}

func TestDeletionSecondRequestReturns200WithTheSameDate(t *testing.T) {
	requirePlatformTestDatabase(t)
	uid, _ := deletionUser(t, "Twice")
	var first, second AccountDeletionScheduledResponse
	testutil.Call(t, requestDeletionHandler(),
		deletionRequest(t, http.MethodPost, "/api/me/deletion", uid, freshToken(t, uid))).Want(http.StatusAccepted).JSON(&first)
	testutil.Call(t, requestDeletionHandler(),
		deletionRequest(t, http.MethodPost, "/api/me/deletion", uid, freshToken(t, uid))).Want(http.StatusOK).JSON(&second)
	if first.ScheduledAt != second.ScheduledAt {
		t.Fatalf("scheduled_at changed: %q then %q", first.ScheduledAt, second.ScheduledAt)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM audit_event WHERE action = 'account.deletion_requested' AND actor_id = $1`, uid); n != 1 {
		t.Fatalf("want one audit row, got %d", n)
	}
}

func TestDeletionCancelRestoresActiveAndKeepsTheRevocation(t *testing.T) {
	requirePlatformTestDatabase(t)
	uid, _ := deletionUser(t, "Canceler")
	testutil.Call(t, requestDeletionHandler(),
		deletionRequest(t, http.MethodPost, "/api/me/deletion", uid, freshToken(t, uid))).Want(http.StatusAccepted)
	var markBefore time.Time
	if err := testPool.QueryRow(context.Background(),
		`SELECT sessions_valid_after FROM platform_account WHERE user_id = $1::uuid`, uid).Scan(&markBefore); err != nil {
		t.Fatal(err)
	}

	// Cancelling needs no recent login: an old but valid session token works.
	old := deletionToken(t, uid, 2*time.Hour, true)
	resp := testutil.Call(t, cancelDeletionHandler(),
		deletionRequest(t, http.MethodDelete, "/api/me/deletion", uid, old)).Want(http.StatusOK)
	if resp.Map()["status"] != "active" {
		t.Fatalf("body = %s", resp.Text())
	}
	status, scheduled := accountStatus(t, uid)
	if status != "active" || scheduled != nil {
		t.Fatalf("state = %q %v", status, scheduled)
	}
	var markAfter time.Time
	if err := testPool.QueryRow(context.Background(),
		`SELECT sessions_valid_after FROM platform_account WHERE user_id = $1::uuid`, uid).Scan(&markAfter); err != nil {
		t.Fatal(err)
	}
	if !markAfter.Equal(markBefore) {
		t.Fatalf("sessions_valid_after moved from %v to %v", markBefore, markAfter)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM audit_event WHERE action = 'account.deletion_canceled' AND actor_id = $1`, uid); n != 1 {
		t.Fatalf("want one account.deletion_canceled row, got %d", n)
	}
}

func TestDeletionCancelWhenNotPendingIs409(t *testing.T) {
	requirePlatformTestDatabase(t)
	uid, _ := deletionUser(t, "NotPending")
	resp := testutil.Call(t, cancelDeletionHandler(),
		deletionRequest(t, http.MethodDelete, "/api/me/deletion", uid, freshToken(t, uid))).Want(http.StatusConflict)
	if resp.Map()["code"] != "not_pending_deletion" {
		t.Fatalf("body = %s", resp.Text())
	}
}

func TestDeletionRequiresARecentHumanLogin(t *testing.T) {
	requirePlatformTestDatabase(t)
	uid, _ := deletionUser(t, "Stale")

	cases := []struct {
		name   string
		token  string
		header [2]string
		want   int
		code   string
	}{
		{name: "login 11 minutes ago", token: deletionToken(t, uid, 11*time.Minute, true), want: http.StatusForbidden, code: "recent_login_required"},
		{name: "token without auth_time", token: deletionToken(t, uid, time.Minute, false), want: http.StatusForbidden, code: "recent_login_required"},
		{name: "personal access token", token: "mul_" + uuid.NewString(), want: http.StatusForbidden, code: "recent_login_required"},
		{name: "no token at all", token: "", want: http.StatusForbidden, code: "recent_login_required"},
		{name: "agent task token", token: freshToken(t, uid), header: [2]string{"X-Actor-Source", "task_token"}, want: http.StatusForbidden},
		{name: "cloud node token", token: freshToken(t, uid), header: [2]string{"X-Actor-Source", "cloud_pat"}, want: http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := deletionRequest(t, http.MethodPost, "/api/me/deletion", uid, c.token)
			if c.header[0] != "" {
				req.Header.Set(c.header[0], c.header[1])
			}
			resp := testutil.Call(t, requestDeletionHandler(), req).Want(c.want)
			if c.code != "" && resp.Map()["code"] != c.code {
				t.Fatalf("body = %s", resp.Text())
			}
			if status, _ := accountStatus(t, uid); status != "active" {
				t.Fatalf("status = %q after a refused request", status)
			}
		})
	}
}

func TestGetMeShowsDeletionDateOnlyWhilePending(t *testing.T) {
	requirePlatformTestDatabase(t)
	uid, _ := deletionUser(t, "Me")
	get := func() map[string]any {
		return testutil.Call(t, testHandler.GetMe, deletionRequest(t, http.MethodGet, "/api/me", uid, "")).Want(http.StatusOK).Map()
	}
	if _, has := get()["deletion_scheduled_at"]; has {
		t.Fatal("deletion_scheduled_at present on an active account")
	}
	var scheduled AccountDeletionScheduledResponse
	testutil.Call(t, requestDeletionHandler(),
		deletionRequest(t, http.MethodPost, "/api/me/deletion", uid, freshToken(t, uid))).Want(http.StatusAccepted).JSON(&scheduled)
	if got := get()["deletion_scheduled_at"]; got != scheduled.ScheduledAt {
		t.Fatalf("deletion_scheduled_at = %v, want %v", got, scheduled.ScheduledAt)
	}
	testutil.Call(t, cancelDeletionHandler(),
		deletionRequest(t, http.MethodDelete, "/api/me/deletion", uid, freshToken(t, uid))).Want(http.StatusOK)
	if _, has := get()["deletion_scheduled_at"]; has {
		t.Fatal("deletion_scheduled_at present after cancel")
	}
}

// makeDue puts the user's deletion in the past.
func makeDue(t *testing.T, userID string) {
	t.Helper()
	dbfx.Exec(t, `INSERT INTO platform_account (user_id, status, reason, changed_by, sessions_valid_after, deletion_scheduled_at)
		VALUES ($1::uuid, 'pending_deletion', 'user_requested', $1::uuid, now(), now() - interval '1 minute')
		ON CONFLICT (user_id) DO UPDATE SET status = 'pending_deletion', deletion_scheduled_at = now() - interval '1 minute'`, userID)
}

type deletionFixture struct {
	userID, email string
	soloWS        string // only member: deleted by the job
	sharedWS      string // other members: the user is removed, the rest stays
	otherUserID   string
	patID         string
}

func newDeletionFixture(t *testing.T) deletionFixture {
	t.Helper()
	f := deletionFixture{}
	f.userID, f.email = deletionUser(t, "Leaver")
	f.otherUserID, _ = deletionUser(t, "Stayer")
	if _, err := testPool.Exec(context.Background(),
		`UPDATE "user" SET avatar_url = 'https://example.test/a.png', profile_description = 'private bio', onboarding_questionnaire = '{"x":1}'::jsonb WHERE id = $1::uuid`, f.userID); err != nil {
		t.Fatal(err)
	}
	f.soloWS = dbfx.Workspace(t, "Solo", "solo-"+uuid.NewString()[:8])
	dbfx.Member(t, f.soloWS, f.userID, "owner")
	f.sharedWS = dbfx.Workspace(t, "Shared", "shared-"+uuid.NewString()[:8])
	dbfx.Member(t, f.sharedWS, f.otherUserID, "owner")
	dbfx.Member(t, f.sharedWS, f.userID, "member")

	f.patID = dbfx.Insert(t, "personal_access_token", testutil.Cols{
		"user_id": f.userID, "name": "ci", "token_hash": "hash-" + uuid.NewString(), "token_prefix": "mul_del",
	})
	dbfx.InsertNoID(t, "user_consent", testutil.Cols{"user_id": f.userID, "document": "terms", "version": "v1"},
		"user_id = $1::uuid", f.userID)
	dbfx.Insert(t, "user_composio_connection", testutil.Cols{
		"user_id": f.userID, "toolkit_slug": "github", "auth_config_id": "ac", "connected_account_id": "ca-" + uuid.NewString(), "composio_user_id": "cu",
	})
	// Audit rows: written by the user in the shared workspace, naming the user as
	// the target, and written by someone else.
	dbfx.Exec(t, `INSERT INTO audit_event (workspace_id, actor_type, actor_id, actor_label, action, target_type, target_id, target_label)
		VALUES ($1::uuid, 'member', $2, $3, 'pat.created', 'user', $2, $3)`, f.sharedWS, f.userID, f.email)
	dbfx.Exec(t, `INSERT INTO audit_event (workspace_id, actor_type, actor_id, actor_label, action, target_type, target_id, target_label)
		VALUES ($1::uuid, 'member', $2, 'stayer@example.test', 'member.role_changed', 'user', $3, $4)`, f.sharedWS, f.otherUserID, f.userID, f.email)
	dbfx.Exec(t, `INSERT INTO audit_event (workspace_id, actor_type, actor_id, actor_label, action, target_type, target_id, target_label)
		VALUES ($1::uuid, 'member', $2, 'stayer@example.test', 'workspace.updated', 'workspace', $1::text, 'Shared')`, f.sharedWS, f.otherUserID)
	dbfx.Cleanup(t, `DELETE FROM audit_event WHERE workspace_id = ANY($1::uuid[]) OR actor_id = $2 OR target_id = $2`,
		[]string{f.soloWS, f.sharedWS}, f.userID)
	return f
}

func TestAccountDeletionJobAnonymisesAndRemoves(t *testing.T) {
	requirePlatformTestDatabase(t)
	ctx := context.Background()
	f := newDeletionFixture(t)
	makeDue(t, f.userID)

	done, err := testHandler.DeleteAccount(ctx, f.userID, time.Now())
	if err != nil || !done {
		t.Fatalf("DeleteAccount = %v, %v", done, err)
	}

	var name, email string
	var avatar *string
	var bio string
	var questionnaire string
	if err := testPool.QueryRow(ctx,
		`SELECT name, email, avatar_url, profile_description, onboarding_questionnaire::text FROM "user" WHERE id = $1::uuid`, f.userID).
		Scan(&name, &email, &avatar, &bio, &questionnaire); err != nil {
		t.Fatal(err)
	}
	if name != "Deleted user" || email != "deleted+"+f.userID+"@deleted.invalid" || avatar != nil || bio != "" || questionnaire != "{}" {
		t.Fatalf("user row = %q %q %v %q %q", name, email, avatar, bio, questionnaire)
	}
	if status, _ := accountStatus(t, f.userID); status != "deleted" {
		t.Fatalf("status = %q", status)
	}

	// The sole-member workspace is gone; the shared one stays with the other member.
	if n := dbfx.Count(t, `SELECT count(*) FROM workspace WHERE id = $1::uuid`, f.soloWS); n != 0 {
		t.Fatal("the sole-member workspace was not deleted")
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM workspace WHERE id = $1::uuid`, f.sharedWS); n != 1 {
		t.Fatal("the shared workspace was deleted")
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM member WHERE workspace_id = $1::uuid AND user_id = $2::uuid`, f.sharedWS, f.otherUserID); n != 1 {
		t.Fatal("the other member was removed")
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM member WHERE user_id = $1::uuid`, f.userID); n != 0 {
		t.Fatalf("%d memberships remain", n)
	}

	// Personal rows are gone.
	for _, q := range []string{
		`SELECT count(*) FROM personal_access_token WHERE user_id = $1::uuid`,
		`SELECT count(*) FROM user_consent WHERE user_id = $1::uuid`,
		`SELECT count(*) FROM user_composio_connection WHERE user_id = $1::uuid`,
	} {
		if n := dbfx.Count(t, q, f.userID); n != 0 {
			t.Errorf("%s left %d rows", q, n)
		}
	}

	// Audit labels: the user's rows are renamed, other people's rows are not.
	if n := dbfx.Count(t, `SELECT count(*) FROM audit_event WHERE actor_id = $1 AND actor_label <> 'deleted user'`, f.userID); n != 0 {
		t.Errorf("%d rows by the user keep their label", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM audit_event WHERE target_type = 'user' AND target_id = $1 AND target_label <> 'deleted user'`, f.userID); n != 0 {
		t.Errorf("%d rows about the user keep their label", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM audit_event WHERE actor_id = $1 AND actor_label = 'stayer@example.test'`, f.otherUserID); n < 2 {
		t.Errorf("another user's audit rows were changed")
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM audit_event WHERE target_label = $1 OR actor_label = $1`, f.email); n != 0 {
		t.Errorf("%d audit rows still carry the email", n)
	}

	// Every registry entry marked delete is empty for this user.
	for _, c := range accountdeletion.DeletedColumns() {
		q := `SELECT count(*) FROM ` + pgx.Identifier{c.Table}.Sanitize() + ` WHERE ` + pgx.Identifier{c.Column}.Sanitize() + ` = $1::uuid`
		if n := dbfx.Count(t, q, f.userID); n != 0 {
			t.Errorf("registry says %s.%s is deleted, but %d rows remain", c.Table, c.Column, n)
		}
	}

	// A second run changes nothing.
	done, err = testHandler.DeleteAccount(ctx, f.userID, time.Now())
	if err != nil || done {
		t.Fatalf("second run = %v, %v; want false, nil", done, err)
	}
}

// AC5: once the job has anonymised the account, its original email is free, so
// the same address registers again as a new person and never reaches the old row.
func TestAccountDeletionFreesTheEmailForANewAccount(t *testing.T) {
	requirePlatformTestDatabase(t)
	ctx := context.Background()
	f := newDeletionFixture(t)
	makeDue(t, f.userID)
	if done, err := testHandler.DeleteAccount(ctx, f.userID, time.Now()); err != nil || !done {
		t.Fatalf("DeleteAccount = %v, %v", done, err)
	}

	dbfx.Cleanup(t, `DELETE FROM verification_code WHERE email = $1`, f.email)
	createVerificationCodeForTest(t, f.email, "123456")
	var out struct {
		Token string `json:"token"`
		User  struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
	}
	testutil.Call(t, testHandler.VerifyCode, testutil.JSONRequest(http.MethodPost, "/auth/verify-code",
		map[string]string{"email": f.email, "code": "123456"})).Want(http.StatusOK).JSON(&out)
	newID := out.User.ID
	if newID != "" {
		dbfx.Cleanup(t, `DELETE FROM member WHERE user_id = $1::uuid`, newID)
		dbfx.Cleanup(t, `DELETE FROM platform_account WHERE user_id = $1::uuid`, newID)
		dbfx.Cleanup(t, `DELETE FROM "user" WHERE id = $1::uuid`, newID)
	}
	if out.Token == "" || newID == "" {
		t.Fatalf("sign-up returned token %q, user %q; want a token for a new user", out.Token, newID)
	}
	if newID == f.userID {
		t.Fatalf("sign-up returned the deleted user's id %s", newID)
	}
	if out.User.Email != f.email {
		t.Errorf("new user email = %q, want %q", out.User.Email, f.email)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE id = $1::uuid AND email = $2`, newID, f.email); n != 1 {
		t.Errorf("new row with the original email: %d, want 1", n)
	}
	var oldEmail string
	if err := testPool.QueryRow(ctx, `SELECT email FROM "user" WHERE id = $1::uuid`, f.userID).Scan(&oldEmail); err != nil {
		t.Fatal(err)
	}
	if oldEmail != "deleted+"+f.userID+"@deleted.invalid" {
		t.Errorf("old row email = %q, want it to stay anonymised", oldEmail)
	}
}

// Platform account requests that named the user (platform spec 7b.5, rule 11)
// keep their rows and ids, but lose the personal values: the email and display
// name. Another person's request is left alone.
func TestAccountDeletionJobAnonymisesAccountRequests(t *testing.T) {
	requirePlatformTestDatabase(t)
	ctx := context.Background()
	f := newDeletionFixture(t)
	makeDue(t, f.userID)

	clientEmail := "client-" + uuid.NewString()[:8] + "@example.test"
	reqID := dbfx.Insert(t, "platform_account_request", testutil.Cols{
		"id": uuid.NewString(), "kind": "individual", "email": clientEmail, "display_name": "Client Person",
		"workspace_name": "Client workspace", "plan_key": "free", "state": "provisioned",
		"user_id": f.userID, "created_by": f.otherUserID,
	})
	otherEmail := "other-" + uuid.NewString()[:8] + "@example.test"
	otherID := dbfx.Insert(t, "platform_account_request", testutil.Cols{
		"id": uuid.NewString(), "kind": "individual", "email": otherEmail, "display_name": "Other Person",
		"workspace_name": "Other workspace", "plan_key": "free", "state": "pending",
		"created_by": f.otherUserID,
	})

	done, err := testHandler.DeleteAccount(ctx, f.userID, time.Now())
	if err != nil || !done {
		t.Fatalf("DeleteAccount = %v, %v", done, err)
	}

	var email, displayName string
	var userID *string
	if err := testPool.QueryRow(ctx,
		`SELECT email, display_name, user_id::text FROM platform_account_request WHERE id = $1::uuid`, reqID).
		Scan(&email, &displayName, &userID); err != nil {
		t.Fatal(err)
	}
	if email != "deleted+"+f.userID+"@deleted.invalid" || displayName != "Deleted user" {
		t.Fatalf("request row = %q %q; want the anonymised values", email, displayName)
	}
	if userID == nil || *userID != f.userID {
		t.Fatal("the request no longer names the user it was made for")
	}

	var otherName, otherAddr string
	if err := testPool.QueryRow(ctx,
		`SELECT email, display_name FROM platform_account_request WHERE id = $1::uuid`, otherID).
		Scan(&otherAddr, &otherName); err != nil {
		t.Fatal(err)
	}
	if otherAddr != otherEmail || otherName != "Other Person" {
		t.Fatalf("another person's request changed: %q %q", otherAddr, otherName)
	}
}

// The job can be retried, so deleting an already deleted account must change
// nothing and report nothing deleted.
func TestAccountDeletionJobRunsTwiceWithoutError(t *testing.T) {
	requirePlatformTestDatabase(t)
	ctx := context.Background()
	f := newDeletionFixture(t)
	makeDue(t, f.userID)

	// Everything the job writes about the account, in one comparable value.
	snapshot := func() string {
		var s string
		if err := testPool.QueryRow(ctx, `SELECT
			(SELECT to_jsonb(p)::text FROM platform_account p WHERE p.user_id = $1::uuid) ||
			(SELECT to_jsonb(u)::text FROM "user" u WHERE u.id = $1::uuid) ||
			(SELECT count(*)::text FROM member WHERE user_id = $1::uuid) ||
			(SELECT count(*)::text FROM workspace WHERE id = ANY($2::uuid[])) ||
			(SELECT count(*)::text FROM audit_event WHERE workspace_id = ANY($2::uuid[]))`,
			f.userID, []string{f.soloWS, f.sharedWS}).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}

	deleted, err := testHandler.DeleteAccount(ctx, f.userID, time.Now())
	if err != nil || !deleted {
		t.Fatalf("first run: deleted %v, err %v; want deleted, nil", deleted, err)
	}
	after := snapshot()

	deleted, err = testHandler.DeleteAccount(ctx, f.userID, time.Now())
	if err != nil || deleted {
		t.Fatalf("second run: deleted %v, err %v; want false, nil", deleted, err)
	}
	if got := snapshot(); got != after {
		t.Fatalf("second run changed the account state:\nafter first:  %s\nafter second: %s", after, got)
	}
	if status, _ := accountStatus(t, f.userID); status != "deleted" {
		t.Fatalf("status = %q, want deleted", status)
	}
}

func TestAccountDeletionJobLeavesAccountsThatAreNotDue(t *testing.T) {
	requirePlatformTestDatabase(t)
	ctx := context.Background()
	f := newDeletionFixture(t)

	// Active account.
	if done, err := testHandler.DeleteAccount(ctx, f.userID, time.Now()); err != nil || done {
		t.Fatalf("active account: %v, %v", done, err)
	}
	// Pending, but the date has not come.
	dbfx.Exec(t, `INSERT INTO platform_account (user_id, status, deletion_scheduled_at) VALUES ($1::uuid, 'pending_deletion', now() + interval '1 day')
		ON CONFLICT (user_id) DO UPDATE SET status = 'pending_deletion', deletion_scheduled_at = now() + interval '1 day'`, f.userID)
	if done, err := testHandler.DeleteAccount(ctx, f.userID, time.Now()); err != nil || done {
		t.Fatalf("not yet due: %v, %v", done, err)
	}
	ids, err := testHandler.DueAccountDeletions(ctx, time.Now(), 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if id == f.userID {
			t.Fatal("an account that is not due was listed")
		}
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM workspace WHERE id = $1::uuid`, f.soloWS); n != 1 {
		t.Fatal("a workspace was deleted for an account that is not due")
	}
	if name := userName(t, f.userID); name != "Leaver" {
		t.Fatalf("user name = %q", name)
	}

	// Due and listed.
	makeDue(t, f.userID)
	ids, err = testHandler.DueAccountDeletions(ctx, time.Now(), 50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range ids {
		found = found || strings.EqualFold(id, f.userID)
	}
	if !found {
		t.Fatal("a due account was not listed")
	}
}

func userName(t *testing.T, userID string) string {
	t.Helper()
	var name string
	if err := testPool.QueryRow(context.Background(), `SELECT name FROM "user" WHERE id = $1::uuid`, userID).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestAccountDeletionJobKeepsOtherMembersAndRefusesToOrphanAWorkspace(t *testing.T) {
	requirePlatformTestDatabase(t)
	ctx := context.Background()
	uid, _ := deletionUser(t, "LastOwner")
	other, _ := deletionUser(t, "Joiner")
	ws := dbfx.Workspace(t, "Grew", "grew-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, uid, "owner")
	dbfx.Member(t, ws, other, "member")
	makeDue(t, uid)

	done, err := testHandler.DeleteAccount(ctx, uid, time.Now())
	if err == nil || done {
		t.Fatalf("DeleteAccount = %v, %v; want an error, so the workspace keeps an owner", done, err)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM member WHERE workspace_id = $1::uuid`, ws); n != 2 {
		t.Fatalf("members = %d, want 2", n)
	}
	if status, _ := accountStatus(t, uid); status != "pending_deletion" {
		t.Fatalf("status = %q, want the account left pending", status)
	}
}

// Spec 13, AC28: deleting an account removes every group row of that user in the
// same run, and leaves the other members' rows.
func TestAccountDeletionJobRemovesTheUsersGroupRows(t *testing.T) {
	requirePlatformTestDatabase(t)
	ctx := context.Background()
	f := newDeletionFixture(t)
	groups := make([]string, 2)
	for i := range groups {
		groups[i] = dbfx.Insert(t, "member_group", testutil.Cols{
			"workspace_id": f.sharedWS, "name": "g" + strconv.Itoa(i) + "-" + uuid.NewString()[:8],
			"description": "", "permissions": "{}", "created_by": f.otherUserID,
		})
		for _, u := range []string{f.userID, f.otherUserID} {
			dbfx.Exec(t, `INSERT INTO member_group_member (workspace_id, group_id, user_id) VALUES ($1::uuid, $2::uuid, $3::uuid)`,
				f.sharedWS, groups[i], u)
		}
	}
	dbfx.Cleanup(t, `DELETE FROM member_group_member WHERE workspace_id = $1::uuid`, f.sharedWS)
	dbfx.Cleanup(t, `DELETE FROM member_group WHERE workspace_id = $1::uuid`, f.sharedWS)
	makeDue(t, f.userID)

	if done, err := testHandler.DeleteAccount(ctx, f.userID, time.Now()); err != nil || !done {
		t.Fatalf("DeleteAccount = %v, %v", done, err)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM member_group_member WHERE user_id = $1::uuid`, f.userID); n != 0 {
		t.Fatalf("group rows naming the deleted user = %d, want 0", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM member_group_member WHERE workspace_id = $1::uuid AND user_id = $2::uuid`, f.sharedWS, f.otherUserID); n != 2 {
		t.Fatalf("the other member's group rows = %d, want 2", n)
	}
}

// Spec 13, AC25: the account export never carries a billing profile.
func TestExportHasNoBillingProfile(t *testing.T) {
	requirePlatformTestDatabase(t)
	dbfx.Exec(t, `INSERT INTO billing_profile (workspace_id, account_kind, legal_name, tax_code, address, invoice_email)
		VALUES ($1::uuid, 'business', 'Sentinel Legal Name Co', 'TAX-SENTINEL-0123', '1 Sentinel Street', 'invoice-sentinel@example.test')`, testWorkspaceID)
	dbfx.Cleanup(t, `DELETE FROM billing_profile WHERE workspace_id = $1::uuid`, testWorkspaceID)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM audit_event WHERE action = 'account.export_requested' AND actor_id = $1`, testUserID)
	})

	body := testutil.Call(t, exportHandler(newExportLimiter()), exportRequest()).Want(http.StatusOK).Text()
	for _, leak := range []string{"Sentinel Legal Name Co", "TAX-SENTINEL-0123", "1 Sentinel Street", "invoice-sentinel@example.test", "billing_profile", "tax_code", "legal_name"} {
		if strings.Contains(body, leak) {
			t.Errorf("account export contains %q", leak)
		}
	}
}
