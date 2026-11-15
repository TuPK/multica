package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// These tests write audit_event rows and add a failing trigger on that table.
// They run only against the dedicated *_test database named by
// PLATFORM_TEST_DATABASE_URL, which must equal DATABASE_URL.

const auditFailPrefix = "audit-fail-"

type auditRow struct {
	WorkspaceID *string
	ActorType   string
	ActorID     *string
	ActorLabel  string
	TargetType  string
	TargetID    string
	TargetLabel string
	Detail      map[string]any
	IP          string
}

// auditRequest tags req with a unique request id and removes the audit rows
// written under it when the test ends. The id is how a test finds its own rows
// among those of other tests in the shared database.
func auditRequest(t *testing.T, req *http.Request, prefix string) (*http.Request, string) {
	t.Helper()
	id := prefix + uuid.NewString()
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM audit_event WHERE request_id = $1`, id)
	})
	return req.WithContext(context.WithValue(req.Context(), chimw.RequestIDKey, id)), id
}

// auditEvents returns the rows written for action under requestID.
func auditEvents(t *testing.T, requestID, action string) []auditRow {
	t.Helper()
	rows, err := testPool.Query(context.Background(), `
		SELECT workspace_id::text, actor_type, actor_id, actor_label,
		       target_type, target_id, target_label, detail, ip
		FROM audit_event WHERE request_id = $1 AND action = $2`, requestID, action)
	if err != nil {
		t.Fatalf("query audit_event: %v", err)
	}
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var r auditRow
		var raw []byte
		if err := rows.Scan(&r.WorkspaceID, &r.ActorType, &r.ActorID, &r.ActorLabel,
			&r.TargetType, &r.TargetID, &r.TargetLabel, &raw, &r.IP); err != nil {
			t.Fatalf("scan audit_event: %v", err)
		}
		if err := json.Unmarshal(raw, &r.Detail); err != nil {
			t.Fatalf("decode detail: %v", err)
		}
		out = append(out, r)
	}
	return out
}

type auditWant struct {
	action      string
	workspaceID string // empty: the row must have a NULL workspace_id
	actorID     string
	actorLabel  string
	targetType  string
	targetID    string
	targetLabel string // empty: not checked
	detail      map[string]any
}

// wantOneAudit asserts exactly one row for want.action under requestID and
// that it carries the expected actor, target, workspace and detail.
func wantOneAudit(t *testing.T, requestID string, want auditWant) {
	t.Helper()
	rows := auditEvents(t, requestID, want.action)
	if len(rows) != 1 {
		t.Fatalf("%s: got %d audit rows, want 1", want.action, len(rows))
	}
	got := rows[0]
	if want.workspaceID == "" {
		if got.WorkspaceID != nil {
			t.Errorf("%s: workspace_id = %q, want NULL", want.action, *got.WorkspaceID)
		}
	} else if got.WorkspaceID == nil || *got.WorkspaceID != want.workspaceID {
		t.Errorf("%s: workspace_id = %v, want %s", want.action, got.WorkspaceID, want.workspaceID)
	}
	if got.ActorType != audit.ActorMember {
		t.Errorf("%s: actor_type = %q, want member", want.action, got.ActorType)
	}
	if got.ActorID == nil || *got.ActorID != want.actorID {
		t.Errorf("%s: actor_id = %v, want %s", want.action, got.ActorID, want.actorID)
	}
	if want.actorLabel != "" && got.ActorLabel != want.actorLabel {
		t.Errorf("%s: actor_label = %q, want %q", want.action, got.ActorLabel, want.actorLabel)
	}
	if got.TargetType != want.targetType || got.TargetID != want.targetID {
		t.Errorf("%s: target = %s/%s, want %s/%s", want.action, got.TargetType, got.TargetID, want.targetType, want.targetID)
	}
	if want.targetLabel != "" && got.TargetLabel != want.targetLabel {
		t.Errorf("%s: target_label = %q, want %q", want.action, got.TargetLabel, want.targetLabel)
	}
	wantDetail := want.detail
	if wantDetail == nil {
		wantDetail = map[string]any{}
	}
	if !reflect.DeepEqual(got.Detail, wantDetail) {
		t.Errorf("%s: detail = %v, want %v", want.action, got.Detail, wantDetail)
	}
}

// failAuditWrites makes every audit_event insert whose request id starts with
// auditFailPrefix raise an error, which is how these tests inject a failing
// audit write without a seam in the handlers.
func failAuditWrites(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE OR REPLACE FUNCTION audit_test_fail() RETURNS trigger AS $$
		 BEGIN
		   IF NEW.request_id LIKE '` + auditFailPrefix + `%' THEN
		     RAISE EXCEPTION 'injected audit failure';
		   END IF;
		   RETURN NEW;
		 END $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS audit_test_fail ON audit_event`,
		`CREATE TRIGGER audit_test_fail BEFORE INSERT ON audit_event
		 FOR EACH ROW EXECUTE FUNCTION audit_test_fail()`,
	} {
		if _, err := testPool.Exec(ctx, stmt); err != nil {
			t.Fatalf("install failing audit trigger: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DROP TRIGGER IF EXISTS audit_test_fail ON audit_event`)
		_, _ = testPool.Exec(context.Background(), `DROP FUNCTION IF EXISTS audit_test_fail()`)
	})
}

func auditTestUserEmail(t *testing.T, userID string) string {
	t.Helper()
	var email string
	dbfx.QueryRow(t, `SELECT email FROM "user" WHERE id = $1`, userID).Scan(&email)
	return email
}

func auditTestMemberID(t *testing.T, workspaceID, userID string) string {
	t.Helper()
	var id string
	dbfx.QueryRow(t, `SELECT id FROM member WHERE workspace_id = $1 AND user_id = $2`, workspaceID, userID).Scan(&id)
	return id
}

func auditMemberRequest(userID, method, memberRowID string, body any) *http.Request {
	req := newRequestAs(userID, method, "/api/workspaces/"+testWorkspaceID+"/members/"+memberRowID, body)
	return withURLParams(req, "id", testWorkspaceID, "memberId", memberRowID)
}

func TestAuditMemberEvents(t *testing.T) {
	requirePlatformTestDatabase(t)

	t.Run("role changed", func(t *testing.T) {
		userID := createTestUserAndMember(t, "member")
		memberRow := auditTestMemberID(t, testWorkspaceID, userID)
		req, reqID := auditRequest(t, auditMemberRequest(testUserID, "PATCH", memberRow, UpdateMemberRequest{Role: "admin"}), "audit-")
		testutil.Call(t, testHandler.UpdateMember, req).Want(http.StatusOK)
		wantOneAudit(t, reqID, auditWant{
			action: audit.ActionMemberRoleChanged, workspaceID: testWorkspaceID, actorID: testUserID,
			actorLabel: handlerTestEmail, targetType: "member", targetID: userID,
			targetLabel: auditTestUserEmail(t, userID), detail: map[string]any{"from": "member", "to": "admin"},
		})
	})

	t.Run("removed", func(t *testing.T) {
		userID := createTestUserAndMember(t, "member")
		memberRow := auditTestMemberID(t, testWorkspaceID, userID)
		req, reqID := auditRequest(t, auditMemberRequest(testUserID, "DELETE", memberRow, nil), "audit-")
		testutil.Call(t, testHandler.DeleteMember, req).Want(http.StatusNoContent)
		wantOneAudit(t, reqID, auditWant{
			action: audit.ActionMemberRemoved, workspaceID: testWorkspaceID, actorID: testUserID,
			actorLabel: handlerTestEmail, targetType: "member", targetID: userID,
			targetLabel: auditTestUserEmail(t, userID),
		})
	})

	t.Run("left", func(t *testing.T) {
		userID := createTestUserAndMember(t, "member")
		email := auditTestUserEmail(t, userID)
		req := withURLParam(newRequestAs(userID, "POST", "/api/workspaces/"+testWorkspaceID+"/leave", nil), "id", testWorkspaceID)
		req, reqID := auditRequest(t, req, "audit-")
		testutil.Call(t, testHandler.LeaveWorkspace, req).Want(http.StatusNoContent)
		wantOneAudit(t, reqID, auditWant{
			action: audit.ActionMemberLeft, workspaceID: testWorkspaceID, actorID: userID,
			actorLabel: email, targetType: "member", targetID: userID, targetLabel: email,
		})
	})
}

func TestAuditWorkspaceEvents(t *testing.T) {
	requirePlatformTestDatabase(t)

	t.Run("created", func(t *testing.T) {
		slug := "audit-ws-" + uuid.NewString()[:8]
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug) })
		req, reqID := auditRequest(t, newRequest("POST", "/api/workspaces", map[string]string{"name": "Audit Created", "slug": slug}), "audit-")
		var created WorkspaceResponse
		testutil.Call(t, testHandler.CreateWorkspace, req).Want(http.StatusCreated).JSON(&created)
		wantOneAudit(t, reqID, auditWant{
			action: audit.ActionWorkspaceCreated, workspaceID: created.ID, actorID: testUserID,
			actorLabel: handlerTestEmail, targetType: "workspace", targetID: created.ID, targetLabel: "Audit Created",
		})
	})

	t.Run("updated records field names only", func(t *testing.T) {
		wsID := dbfx.Workspace(t, "Audit Before", "audit-upd-"+uuid.NewString()[:8])
		dbfx.Member(t, wsID, testUserID, "owner")
		req := newRequest("PATCH", "/api/workspaces/"+wsID, map[string]any{"name": "Audit After", "description": "secret-looking value"})
		req = withURLParam(req, "id", wsID)
		req, reqID := auditRequest(t, req, "audit-")
		testutil.Call(t, testHandler.UpdateWorkspace, req).Want(http.StatusOK)
		rows := auditEvents(t, reqID, audit.ActionWorkspaceUpdated)
		if len(rows) != 1 {
			t.Fatalf("got %d rows, want 1", len(rows))
		}
		if strings.Contains(fmt.Sprint(rows[0].Detail), "Audit After") || strings.Contains(fmt.Sprint(rows[0].Detail), "secret-looking") {
			t.Fatalf("detail holds a field value: %v", rows[0].Detail)
		}
		wantOneAudit(t, reqID, auditWant{
			action: audit.ActionWorkspaceUpdated, workspaceID: wsID, actorID: testUserID,
			actorLabel: handlerTestEmail, targetType: "workspace", targetID: wsID, targetLabel: "Audit After",
			detail: map[string]any{"changed_fields": []any{"name", "description"}},
		})
	})

	t.Run("deleted", func(t *testing.T) {
		wsID := dbfx.Workspace(t, "Audit Doomed", "audit-del-"+uuid.NewString()[:8])
		dbfx.Member(t, wsID, testUserID, "owner")
		req := withURLParam(newRequest("DELETE", "/api/workspaces/"+wsID, nil), "id", wsID)
		req, reqID := auditRequest(t, req, "audit-")
		testutil.Call(t, testHandler.DeleteWorkspace, req).Want(http.StatusNoContent)
		wantOneAudit(t, reqID, auditWant{
			action: audit.ActionWorkspaceDeleted, workspaceID: wsID, actorID: testUserID,
			actorLabel: handlerTestEmail, targetType: "workspace", targetID: wsID, targetLabel: "Audit Doomed",
		})
	})
}

func auditCreateInvitation(t *testing.T, email string) InvitationResponse {
	t.Helper()
	clearInvitationsForTestWorkspace(t)
	req := withURLParam(newRequest("POST", "/api/workspaces/"+testWorkspaceID+"/members", CreateMemberRequest{Email: email, Role: "member"}), "id", testWorkspaceID)
	var inv InvitationResponse
	testutil.Call(t, testHandler.CreateInvitation, req).Want(http.StatusCreated).JSON(&inv)
	return inv
}

func TestAuditInvitationEvents(t *testing.T) {
	requirePlatformTestDatabase(t)

	t.Run("created", func(t *testing.T) {
		clearInvitationsForTestWorkspace(t)
		email := testutil.UniqueEmail("audit-invite")
		req := withURLParam(newRequest("POST", "/api/workspaces/"+testWorkspaceID+"/members", CreateMemberRequest{Email: email, Role: "member"}), "id", testWorkspaceID)
		req, reqID := auditRequest(t, req, "audit-")
		var inv InvitationResponse
		testutil.Call(t, testHandler.CreateInvitation, req).Want(http.StatusCreated).JSON(&inv)
		wantOneAudit(t, reqID, auditWant{
			action: audit.ActionInvitationCreated, workspaceID: testWorkspaceID, actorID: testUserID,
			actorLabel: handlerTestEmail, targetType: "invitation", targetID: inv.ID, targetLabel: email,
			detail: map[string]any{"role": "member"},
		})
	})

	t.Run("revoked", func(t *testing.T) {
		email := testutil.UniqueEmail("audit-revoke")
		inv := auditCreateInvitation(t, email)
		req := withURLParams(newRequest("DELETE", "/api/workspaces/"+testWorkspaceID+"/invitations/"+inv.ID, nil), "id", testWorkspaceID, "invitationId", inv.ID)
		req, reqID := auditRequest(t, req, "audit-")
		testutil.Call(t, testHandler.RevokeInvitation, req).Want(http.StatusNoContent)
		wantOneAudit(t, reqID, auditWant{
			action: audit.ActionInvitationRevoked, workspaceID: testWorkspaceID, actorID: testUserID,
			actorLabel: handlerTestEmail, targetType: "invitation", targetID: inv.ID, targetLabel: email,
		})
	})

	t.Run("accepted", func(t *testing.T) {
		email := testutil.UniqueEmail("audit-accept")
		inviteeID := dbfx.User(t, "Audit Invitee", email)
		dbfx.Cleanup(t, `DELETE FROM member WHERE user_id = $1`, inviteeID)
		inv := auditCreateInvitation(t, email)
		req := withURLParam(newRequestAs(inviteeID, "POST", "/api/invitations/"+inv.ID+"/accept", nil), "id", inv.ID)
		req, reqID := auditRequest(t, req, "audit-")
		testutil.Call(t, testHandler.AcceptInvitation, req).Want(http.StatusOK)
		wantOneAudit(t, reqID, auditWant{
			action: audit.ActionInvitationAccepted, workspaceID: testWorkspaceID, actorID: inviteeID,
			actorLabel: email, targetType: "invitation", targetID: inv.ID, targetLabel: email,
			detail: map[string]any{"role": "member"},
		})
	})

	t.Run("declined", func(t *testing.T) {
		email := testutil.UniqueEmail("audit-decline")
		inviteeID := dbfx.User(t, "Audit Decliner", email)
		inv := auditCreateInvitation(t, email)
		req := withURLParam(newRequestAs(inviteeID, "POST", "/api/invitations/"+inv.ID+"/decline", nil), "id", inv.ID)
		req, reqID := auditRequest(t, req, "audit-")
		testutil.Call(t, testHandler.DeclineInvitation, req).Want(http.StatusNoContent)
		wantOneAudit(t, reqID, auditWant{
			action: audit.ActionInvitationDeclined, workspaceID: testWorkspaceID, actorID: inviteeID,
			actorLabel: email, targetType: "invitation", targetID: inv.ID, targetLabel: email,
		})
	})
}

func TestAuditShareLinkEvents(t *testing.T) {
	requirePlatformTestDatabase(t)

	t.Run("created", func(t *testing.T) {
		clearShareLinksForTestWorkspace(t)
		req := withURLParam(newRequest("POST", "/api/workspaces/"+testWorkspaceID+"/share-links", CreateShareLinkRequest{Role: "member"}), "id", testWorkspaceID)
		req, reqID := auditRequest(t, req, "audit-")
		var link ShareLinkResponse
		testutil.Call(t, testHandler.CreateShareLink, req).Want(http.StatusCreated).JSON(&link)
		wantOneAudit(t, reqID, auditWant{
			action: audit.ActionShareLinkCreated, workspaceID: testWorkspaceID, actorID: testUserID,
			actorLabel: handlerTestEmail, targetType: "share_link", targetID: link.ID,
		})
	})

	t.Run("revoked", func(t *testing.T) {
		clearShareLinksForTestWorkspace(t)
		link := createTestShareLink(t, testWorkspaceID, "member", 0, 0)
		linkID := uuidToString(link.ID)
		req := withURLParams(newRequest("DELETE", "/api/workspaces/"+testWorkspaceID+"/share-links/"+linkID, nil), "id", testWorkspaceID, "linkId", linkID)
		req, reqID := auditRequest(t, req, "audit-")
		testutil.Call(t, testHandler.RevokeShareLink, req).Want(http.StatusNoContent)
		wantOneAudit(t, reqID, auditWant{
			action: audit.ActionShareLinkRevoked, workspaceID: testWorkspaceID, actorID: testUserID,
			actorLabel: handlerTestEmail, targetType: "share_link", targetID: linkID,
		})

		// Revoking it again changes nothing, so it writes nothing.
		again, againID := auditRequest(t, withURLParams(newRequest("DELETE", "/", nil), "id", testWorkspaceID, "linkId", linkID), "audit-")
		testutil.Call(t, testHandler.RevokeShareLink, again).Want(http.StatusNoContent)
		if n := len(auditEvents(t, againID, audit.ActionShareLinkRevoked)); n != 0 {
			t.Fatalf("repeat revoke wrote %d audit rows, want 0", n)
		}
	})

	t.Run("joined", func(t *testing.T) {
		clearShareLinksForTestWorkspace(t)
		link := createTestShareLink(t, testWorkspaceID, "member", 0, 0)
		joinerID := createTestUserAndMember(t, "")
		email := auditTestUserEmail(t, joinerID)
		req := newRequestAs(joinerID, "POST", "/api/share-links/join", JoinByShareLinkRequest{Code: link.Code})
		req, reqID := auditRequest(t, req, "audit-")
		testutil.Call(t, testHandler.JoinByShareLink, req).Want(http.StatusOK)
		wantOneAudit(t, reqID, auditWant{
			action: audit.ActionShareLinkJoined, workspaceID: testWorkspaceID, actorID: joinerID,
			actorLabel: email, targetType: "share_link", targetID: uuidToString(link.ID),
		})
	})
}

func TestAuditAccountEvents(t *testing.T) {
	requirePlatformTestDatabase(t)

	t.Run("sessions revoked", func(t *testing.T) {
		resetPlatformAccount(t)
		req, reqID := auditRequest(t, revokeAllRequest(), "audit-")
		testutil.Call(t, testHandler.RevokeAllSessions(nil), req).Want(http.StatusOK)
		wantOneAudit(t, reqID, auditWant{
			action: audit.ActionAuthSessionsRevoked, actorID: testUserID, actorLabel: handlerTestEmail,
			targetType: "user", targetID: testUserID, targetLabel: handlerTestEmail,
		})
	})

	t.Run("pat created then revoked", func(t *testing.T) {
		req, createID := auditRequest(t, newRequest("POST", "/api/tokens", CreatePATRequest{Name: "audit-pat"}), "audit-")
		var created CreatePATResponse
		testutil.Call(t, testHandler.CreatePersonalAccessToken, req).Want(http.StatusCreated).JSON(&created)
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM personal_access_token WHERE id = $1`, created.ID)
		})
		wantOneAudit(t, createID, auditWant{
			action: audit.ActionPATCreated, actorID: testUserID, actorLabel: handlerTestEmail,
			targetType: "personal_access_token", targetID: created.ID, targetLabel: "audit-pat",
		})
		for _, r := range auditEvents(t, createID, audit.ActionPATCreated) {
			if strings.Contains(fmt.Sprint(r), created.Token) {
				t.Fatal("audit row holds the token value")
			}
		}

		revoke, revokeID := auditRequest(t, withURLParam(newRequest("DELETE", "/api/tokens/"+created.ID, nil), "id", created.ID), "audit-")
		testutil.Call(t, testHandler.RevokePersonalAccessToken, revoke).Want(http.StatusNoContent)
		wantOneAudit(t, revokeID, auditWant{
			action: audit.ActionPATRevoked, actorID: testUserID, actorLabel: handlerTestEmail,
			targetType: "personal_access_token", targetID: created.ID, targetLabel: "audit-pat",
		})
	})
}

// Each of the three issueJWT call sites records auth.login with its method.
func TestAuditLoginAtEachCallSite(t *testing.T) {
	requirePlatformTestDatabase(t)

	loginWant := func(userID, email, method string) auditWant {
		return auditWant{
			action: audit.ActionAuthLogin, actorID: userID, actorLabel: email,
			targetType: "user", targetID: userID, targetLabel: email,
			detail: map[string]any{"method": method},
		}
	}

	t.Run("email code", func(t *testing.T) {
		email := testutil.UniqueEmail("audit-login-code")
		userID := dbfx.User(t, "Audit Login Code", email)
		dbfx.Cleanup(t, `DELETE FROM verification_code WHERE email = $1`, email)
		createVerificationCodeForTest(t, email, "123456")
		req := testutil.JSONRequest(http.MethodPost, "/auth/verify-code", map[string]string{"email": email, "code": "123456"})
		req, reqID := auditRequest(t, req, "audit-")
		testutil.Call(t, testHandler.VerifyCode, req).Want(http.StatusOK)
		wantOneAudit(t, reqID, loginWant(userID, email, "email_code"))
	})

	t.Run("google", func(t *testing.T) {
		t.Setenv("GOOGLE_CLIENT_ID", "test-client")
		t.Setenv("GOOGLE_CLIENT_SECRET", "test-secret")
		email := testutil.UniqueEmail("audit-login-google")
		userID := dbfx.User(t, "Audit Login Google", email)
		h := newTestHandler(Config{AllowSignup: true})
		h.Queries = testHandler.Queries
		h.googleOAuthHTTPClient = &http.Client{Transport: googleRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "oauth2.googleapis.com" {
				return googleResponse(req, http.StatusOK, `{"access_token":"test-token"}`), nil
			}
			return googleResponse(req, http.StatusOK, fmt.Sprintf(`{"email":%q}`, email)), nil
		})}
		req := httptest.NewRequest(http.MethodPost, "/auth/google", strings.NewReader(`{"code":"test-code","redirect_uri":"http://localhost/auth/callback"}`))
		req, reqID := auditRequest(t, req, "audit-")
		testutil.Call(t, h.GoogleLogin, req).Want(http.StatusOK)
		wantOneAudit(t, reqID, loginWant(userID, email, "google"))
	})

	t.Run("cli token", func(t *testing.T) {
		req, reqID := auditRequest(t, newRequest("POST", "/api/cli-token", nil), "audit-")
		testutil.Call(t, testHandler.IssueCliToken, req).Want(http.StatusOK)
		wantOneAudit(t, reqID, loginWant(testUserID, handlerTestEmail, "other"))
	})
}

// A failing audit write fails the request and leaves the change unsaved.
func TestAuditFailureRollsBackMemberAndWorkspaceChanges(t *testing.T) {
	requirePlatformTestDatabase(t)
	failAuditWrites(t)

	t.Run("update member", func(t *testing.T) {
		userID := createTestUserAndMember(t, "member")
		memberRow := auditTestMemberID(t, testWorkspaceID, userID)
		req, reqID := auditRequest(t, auditMemberRequest(testUserID, "PATCH", memberRow, UpdateMemberRequest{Role: "admin"}), auditFailPrefix)
		testutil.Call(t, testHandler.UpdateMember, req).Want(http.StatusInternalServerError)
		var role string
		dbfx.QueryRow(t, `SELECT role FROM member WHERE id = $1`, memberRow).Scan(&role)
		if role != "member" {
			t.Fatalf("role = %q after failed audit, want member", role)
		}
		if n := len(auditEvents(t, reqID, audit.ActionMemberRoleChanged)); n != 0 {
			t.Fatalf("%d audit rows after failed audit, want 0", n)
		}
	})

	t.Run("delete member", func(t *testing.T) {
		userID := createTestUserAndMember(t, "member")
		memberRow := auditTestMemberID(t, testWorkspaceID, userID)
		req, _ := auditRequest(t, auditMemberRequest(testUserID, "DELETE", memberRow, nil), auditFailPrefix)
		testutil.Call(t, testHandler.DeleteMember, req).Want(http.StatusInternalServerError)
		if n := dbfx.Count(t, `SELECT count(*) FROM member WHERE id = $1`, memberRow); n != 1 {
			t.Fatalf("member rows = %d after failed audit, want 1", n)
		}
	})

	t.Run("leave workspace", func(t *testing.T) {
		userID := createTestUserAndMember(t, "member")
		req := withURLParam(newRequestAs(userID, "POST", "/api/workspaces/"+testWorkspaceID+"/leave", nil), "id", testWorkspaceID)
		req, _ = auditRequest(t, req, auditFailPrefix)
		testutil.Call(t, testHandler.LeaveWorkspace, req).Want(http.StatusInternalServerError)
		if n := dbfx.Count(t, `SELECT count(*) FROM member WHERE user_id = $1 AND workspace_id = $2`, userID, testWorkspaceID); n != 1 {
			t.Fatalf("member rows = %d after failed audit, want 1", n)
		}
	})

	t.Run("create workspace", func(t *testing.T) {
		slug := "audit-fail-ws-" + uuid.NewString()[:8]
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug) })
		req, _ := auditRequest(t, newRequest("POST", "/api/workspaces", map[string]string{"name": "Audit Fail", "slug": slug}), auditFailPrefix)
		testutil.Call(t, testHandler.CreateWorkspace, req).Want(http.StatusInternalServerError)
		if n := dbfx.Count(t, `SELECT count(*) FROM workspace WHERE slug = $1`, slug); n != 0 {
			t.Fatalf("workspace rows = %d after failed audit, want 0", n)
		}
	})

	t.Run("update workspace", func(t *testing.T) {
		wsID := dbfx.Workspace(t, "Audit Keep", "audit-keep-"+uuid.NewString()[:8])
		dbfx.Member(t, wsID, testUserID, "owner")
		req := withURLParam(newRequest("PATCH", "/api/workspaces/"+wsID, map[string]any{"name": "Audit Changed"}), "id", wsID)
		req, reqID := auditRequest(t, req, auditFailPrefix)
		testutil.Call(t, testHandler.UpdateWorkspace, req).Want(http.StatusInternalServerError)
		var name string
		dbfx.QueryRow(t, `SELECT name FROM workspace WHERE id = $1`, wsID).Scan(&name)
		if name != "Audit Keep" {
			t.Fatalf("name = %q after failed audit, want unchanged", name)
		}
		if n := len(auditEvents(t, reqID, audit.ActionWorkspaceUpdated)); n != 0 {
			t.Fatalf("%d audit rows after failed audit, want 0", n)
		}
	})

	t.Run("delete workspace", func(t *testing.T) {
		wsID := dbfx.Workspace(t, "Audit Survives", "audit-surv-"+uuid.NewString()[:8])
		dbfx.Member(t, wsID, testUserID, "owner")
		req := withURLParam(newRequest("DELETE", "/api/workspaces/"+wsID, nil), "id", wsID)
		req, _ = auditRequest(t, req, auditFailPrefix)
		testutil.Call(t, testHandler.DeleteWorkspace, req).Want(http.StatusInternalServerError)
		if n := dbfx.Count(t, `SELECT count(*) FROM workspace WHERE id = $1`, wsID); n != 1 {
			t.Fatalf("workspace rows = %d after failed audit, want 1", n)
		}
	})
}
