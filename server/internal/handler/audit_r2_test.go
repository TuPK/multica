package handler

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Audit events of the R2 slice (platform spec 5.14). Each action has the three
// checks of the contract: one row with the right actor and target, a failed
// audit write that rolls the change back, and a detail that carries no planted
// secret. Database-backed: they run only against the dedicated *_test database
// named by PLATFORM_TEST_DATABASE_URL. They were written without a database
// available and have not been run.

const auditR2Prefix = "audit-r2-"

// r2Request tags req so its audit rows can be found, and removes them when the
// test ends.
func r2Request(t *testing.T, req *http.Request) (*http.Request, string) {
	t.Helper()
	return auditRequest(t, req, auditR2Prefix)
}

// r2FailingRequest tags req so that its audit insert raises (see failAuditWrites).
func r2FailingRequest(t *testing.T, req *http.Request) *http.Request {
	t.Helper()
	req, _ = auditRequest(t, req, auditFailPrefix)
	return req
}

// oneAuditRow returns the single row written for action under reqID and checks
// the actor (the shared test user, as a member), the workspace and the target.
func oneAuditRow(t *testing.T, reqID, action, workspaceID, targetType, targetID string) auditRow {
	t.Helper()
	rows := auditEvents(t, reqID, action)
	if len(rows) != 1 {
		t.Fatalf("%s: %d audit rows, want 1", action, len(rows))
	}
	row := rows[0]
	if row.ActorType != audit.ActorMember || row.ActorID == nil || *row.ActorID != testUserID {
		t.Errorf("%s: actor = %s/%v, want member/%s", action, row.ActorType, row.ActorID, testUserID)
	}
	if row.ActorLabel != auditTestUserEmail(t, testUserID) {
		t.Errorf("%s: actor_label = %q, want the actor's email", action, row.ActorLabel)
	}
	if workspaceID == "" {
		if row.WorkspaceID != nil {
			t.Errorf("%s: workspace_id = %q, want NULL", action, *row.WorkspaceID)
		}
	} else if row.WorkspaceID == nil || *row.WorkspaceID != workspaceID {
		t.Errorf("%s: workspace_id = %v, want %s", action, row.WorkspaceID, workspaceID)
	}
	if row.TargetType != targetType || row.TargetID != targetID {
		t.Errorf("%s: target = %s/%s, want %s/%s", action, row.TargetType, row.TargetID, targetType, targetID)
	}
	return row
}

// noAuditRow asserts that no row of action exists under reqID.
func noAuditRow(t *testing.T, reqID, action string) {
	t.Helper()
	if n := len(auditEvents(t, reqID, action)); n != 0 {
		t.Fatalf("%s: %d audit rows after a refused change, want 0", action, n)
	}
}

// detailHasNo fails when any secret appears in the row's detail or labels.
func detailHasNo(t *testing.T, row auditRow, secrets ...string) {
	t.Helper()
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range secrets {
		if strings.Contains(string(raw), s) {
			t.Errorf("audit row carries the planted secret %q: %s", s, raw)
		}
	}
}

func r2OwnedWorkspace(t *testing.T, name string) string {
	t.Helper()
	wsID := dbfx.Workspace(t, name, "auditr2-"+uuid.NewString()[:8])
	dbfx.Member(t, wsID, testUserID, "owner")
	dbfx.Cleanup(t, `DELETE FROM platform_workspace WHERE workspace_id = $1::uuid`, wsID)
	return wsID
}

// --- workspace deletion and export

func TestAuditR2WorkspaceDeletionScheduledAndCancelled(t *testing.T) {
	requirePlatformTestDatabase(t)
	failAuditWrites(t)
	setDeletionDays(t, 14)

	t.Run("scheduled and cancelled", func(t *testing.T) {
		wsID := r2OwnedWorkspace(t, "Audit schedule")
		req, reqID := r2Request(t, deleteRequest(wsID))
		var out WorkspaceDeletionScheduledResponse
		testutil.Call(t, testHandler.DeleteWorkspace, req).Want(http.StatusAccepted).JSON(&out)
		row := oneAuditRow(t, reqID, audit.ActionWorkspaceDeletionScheduled, wsID, "workspace", wsID)
		if row.Detail["deletion_scheduled_at"] != out.DeletionScheduledAt {
			t.Errorf("detail = %v, want deletion_scheduled_at %s", row.Detail, out.DeletionScheduledAt)
		}

		req, reqID = r2Request(t, cancelRequest(wsID))
		testutil.Call(t, testHandler.CancelWorkspaceDeletion, req).Want(http.StatusNoContent)
		oneAuditRow(t, reqID, audit.ActionWorkspaceDeletionCancelled, wsID, "workspace", wsID)
	})

	t.Run("a repeated request writes no second row", func(t *testing.T) {
		wsID := r2OwnedWorkspace(t, "Audit repeat")
		req, reqID := r2Request(t, deleteRequest(wsID))
		testutil.Call(t, testHandler.DeleteWorkspace, req).Want(http.StatusAccepted)
		req2, reqID2 := r2Request(t, deleteRequest(wsID))
		testutil.Call(t, testHandler.DeleteWorkspace, req2).Want(http.StatusAccepted)
		oneAuditRow(t, reqID, audit.ActionWorkspaceDeletionScheduled, wsID, "workspace", wsID)
		noAuditRow(t, reqID2, audit.ActionWorkspaceDeletionScheduled)
	})

	t.Run("a failed audit write rolls the schedule back", func(t *testing.T) {
		wsID := r2OwnedWorkspace(t, "Audit schedule fails")
		testutil.Call(t, testHandler.DeleteWorkspace, r2FailingRequest(t, deleteRequest(wsID))).Want(http.StatusInternalServerError)
		if row := platformRow(t, wsID); row != nil && row.status == "pending_deletion" {
			t.Fatalf("the schedule survived a failed audit write: %+v", row)
		}
	})

	t.Run("a failed audit write rolls the cancel back", func(t *testing.T) {
		wsID := r2OwnedWorkspace(t, "Audit cancel fails")
		seedPending(t, wsID, time.Now().Add(24*time.Hour))
		testutil.Call(t, testHandler.CancelWorkspaceDeletion, r2FailingRequest(t, cancelRequest(wsID))).Want(http.StatusInternalServerError)
		if row := platformRow(t, wsID); row == nil || row.status != "pending_deletion" {
			t.Fatalf("the cancel survived a failed audit write: %+v", row)
		}
	})
}

func TestAuditR2WorkspaceExportRequested(t *testing.T) {
	requirePlatformTestDatabase(t)
	failAuditWrites(t)

	exportRequest := func(wsID string) *http.Request {
		return withURLParam(newRequest(http.MethodPost, "/api/workspaces/"+wsID+"/exports", nil), "id", wsID)
	}

	t.Run("one row naming the export", func(t *testing.T) {
		wsID := r2OwnedWorkspace(t, "Audit export")
		dbfx.Cleanup(t, `DELETE FROM workspace_export WHERE workspace_id = $1::uuid`, wsID)
		req, reqID := r2Request(t, exportRequest(wsID))
		var out WorkspaceExportResponse
		testutil.Call(t, testHandler.CreateWorkspaceExport, req).Want(http.StatusAccepted).JSON(&out)
		oneAuditRow(t, reqID, audit.ActionWorkspaceExportRequested, wsID, "workspace_export", out.ID)
	})

	t.Run("a failed audit write queues no export", func(t *testing.T) {
		wsID := r2OwnedWorkspace(t, "Audit export fails")
		dbfx.Cleanup(t, `DELETE FROM workspace_export WHERE workspace_id = $1::uuid`, wsID)
		testutil.Call(t, testHandler.CreateWorkspaceExport, r2FailingRequest(t, exportRequest(wsID))).Want(http.StatusInternalServerError)
		if n := dbfx.Count(t, `SELECT count(*) FROM workspace_export WHERE workspace_id = $1::uuid`, wsID); n != 0 {
			t.Fatalf("%d export rows after a failed audit write, want 0", n)
		}
	})
}

// --- integrations

func TestAuditR2GitHubConnectAndDisconnect(t *testing.T) {
	requirePlatformTestDatabase(t)
	failAuditWrites(t)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM github_installation WHERE workspace_id = $1`, testWorkspaceID)
	})

	setup := func(installationID string) *http.Request {
		state, err := signState(testWorkspaceID)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, "/api/github/setup?installation_id="+installationID+"&state="+state, nil)
		req.Header.Set("X-User-ID", testUserID)
		return req
	}
	count := func() int {
		return dbfx.Count(t, `SELECT count(*) FROM github_installation WHERE workspace_id = $1`, testWorkspaceID)
	}

	t.Run("connect writes one row without a secret", func(t *testing.T) {
		req, reqID := r2Request(t, setup("7700001"))
		testutil.Call(t, testHandler.GitHubSetupCallback, req).Want(http.StatusFound)
		var id string
		dbfx.QueryRow(t, `SELECT id::text FROM github_installation WHERE workspace_id = $1 AND installation_id = 7700001`, testWorkspaceID).Scan(&id)
		row := oneAuditRow(t, reqID, audit.ActionIntegrationConnected, testWorkspaceID, "integration", id)
		if row.Detail["provider"] != "github" {
			t.Errorf("detail = %v, want provider github", row.Detail)
		}
	})

	t.Run("a failed audit write saves no installation", func(t *testing.T) {
		before := count()
		req := r2FailingRequest(t, setup("7700002"))
		testutil.Call(t, testHandler.GitHubSetupCallback, req).Want(http.StatusFound)
		if got := count(); got != before {
			t.Fatalf("%d installations after a failed audit write, want %d", got, before)
		}
	})

	disconnect := func(id string) *http.Request {
		return withURLParams(newRequest(http.MethodDelete, "/api/workspaces/"+testWorkspaceID+"/github/installations/"+id, nil),
			"id", testWorkspaceID, "installationId", id)
	}

	t.Run("disconnect writes one row", func(t *testing.T) {
		inst, err := testHandler.Queries.CreateGitHubInstallation(context.Background(), db.CreateGitHubInstallationParams{
			WorkspaceID: parseUUID(testWorkspaceID), InstallationID: 7700003, AccountLogin: "audit-r2", AccountType: "User",
		})
		if err != nil {
			t.Fatal(err)
		}
		id := uuidToString(inst.ID)
		req, reqID := r2Request(t, disconnect(id))
		testutil.Call(t, testHandler.DeleteGitHubInstallation, req).Want(http.StatusNoContent)
		row := oneAuditRow(t, reqID, audit.ActionIntegrationDisconnected, testWorkspaceID, "integration", id)
		if row.Detail["provider"] != "github" || row.TargetLabel != "audit-r2" {
			t.Errorf("row = %+v, want provider github and label audit-r2", row)
		}
	})

	t.Run("a failed audit write keeps the installation", func(t *testing.T) {
		inst, err := testHandler.Queries.CreateGitHubInstallation(context.Background(), db.CreateGitHubInstallationParams{
			WorkspaceID: parseUUID(testWorkspaceID), InstallationID: 7700004, AccountLogin: "audit-r2-keep", AccountType: "User",
		})
		if err != nil {
			t.Fatal(err)
		}
		testutil.Call(t, testHandler.DeleteGitHubInstallation, r2FailingRequest(t, disconnect(uuidToString(inst.ID)))).Want(http.StatusInternalServerError)
		if n := dbfx.Count(t, `SELECT count(*) FROM github_installation WHERE id = $1::uuid`, uuidToString(inst.ID)); n != 1 {
			t.Fatalf("installation rows = %d after a failed audit write, want 1", n)
		}
	})

	t.Run("a missing installation writes no row", func(t *testing.T) {
		id := uuid.NewString()
		req, reqID := r2Request(t, disconnect(id))
		testutil.Call(t, testHandler.DeleteGitHubInstallation, req).Want(http.StatusNoContent)
		noAuditRow(t, reqID, audit.ActionIntegrationDisconnected)
	})
}

func TestAuditR2VCSConnectAndDisconnect(t *testing.T) {
	requirePlatformTestDatabase(t)
	failAuditWrites(t)
	ctx := context.Background()
	box := withVCSBox(t)
	t.Cleanup(func() { cleanupVCS(context.Background(), "") })

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"login":"vcs-audit-user"}`))
	}))
	defer provider.Close()
	testHandler.cfg.VCSIntegrationEnabled = true

	const plantedToken = "planted-vcs-access-token"
	connect := func(prefixed bool) (*http.Request, string) {
		req := vcsHandlerRequest(http.MethodPost, "/api/workspaces/"+testWorkspaceID+"/vcs/connections", map[string]any{
			"provider": "forgejo", "instance_url": provider.URL, "access_token": plantedToken,
		}, "")
		if prefixed {
			return r2Request(t, req)
		}
		return r2FailingRequest(t, req), ""
	}

	t.Run("a failed audit write saves no connection", func(t *testing.T) {
		req, _ := connect(false)
		testutil.Call(t, testHandler.ConnectVCS, req).Want(http.StatusInternalServerError)
		if n := dbfx.Count(t, `SELECT count(*) FROM vcs_connection WHERE workspace_id = $1 AND instance_url = $2`, testWorkspaceID, provider.URL); n != 0 {
			t.Fatalf("%d connections after a failed audit write, want 0", n)
		}
	})

	t.Run("connect writes one row without the token", func(t *testing.T) {
		req, reqID := connect(true)
		var out VCSConnectResponse
		testutil.Call(t, testHandler.ConnectVCS, req).Want(http.StatusOK).JSON(&out)
		row := oneAuditRow(t, reqID, audit.ActionIntegrationConnected, testWorkspaceID, "integration", out.ID)
		detailHasNo(t, row, plantedToken, out.WebhookSecret)
		if row.Detail["provider"] != "vcs" {
			t.Errorf("detail = %v, want provider vcs", row.Detail)
		}
	})

	deleteReq := func(id string) *http.Request {
		return vcsHandlerRequest(http.MethodDelete, "/api/workspaces/"+testWorkspaceID+"/vcs/connections/"+id, nil, id)
	}

	t.Run("disconnect writes one row", func(t *testing.T) {
		id := seedVCSConnection(t, ctx, box, "forgejo", "https://forgejo-audit.test")
		req, reqID := r2Request(t, deleteReq(id))
		testutil.Call(t, testHandler.DeleteVCSConnection, req).Want(http.StatusNoContent)
		oneAuditRow(t, reqID, audit.ActionIntegrationDisconnected, testWorkspaceID, "integration", id)
	})

	t.Run("a failed audit write keeps the connection", func(t *testing.T) {
		id := seedVCSConnection(t, ctx, box, "forgejo", "https://forgejo-audit-keep.test")
		testutil.Call(t, testHandler.DeleteVCSConnection, r2FailingRequest(t, deleteReq(id))).Want(http.StatusInternalServerError)
		if n := dbfx.Count(t, `SELECT count(*) FROM vcs_connection WHERE id = $1::uuid`, id); n != 1 {
			t.Fatalf("connection rows = %d after a failed audit write, want 1", n)
		}
	})
}

// The five channel integrations share one revoke path. The Lark handler runs it
// here; the source test below holds the other four to the same call.
func TestAuditR2ChannelRevokeWritesDisconnected(t *testing.T) {
	requirePlatformTestDatabase(t)
	failAuditWrites(t)
	wireLarkInstallServices(t)

	seed := func(t *testing.T, appID string) string {
		t.Helper()
		agentID := createHandlerTestAgent(t, "audit-r2-lark-"+uuid.NewString()[:8], nil)
		var id string
		if err := testPool.QueryRow(context.Background(), `
			INSERT INTO channel_installation (workspace_id, agent_id, channel_type, config, installer_user_id, status)
			VALUES ($1, $2, 'feishu', jsonb_build_object('app_id', $3::text, 'app_secret_encrypted', 'planted-lark-secret'), $4, 'active')
			RETURNING id::text`, testWorkspaceID, agentID, appID, testUserID).Scan(&id); err != nil {
			t.Fatalf("seed installation: %v", err)
		}
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM channel_installation WHERE id = $1::uuid`, id)
		})
		return id
	}
	revoke := func(id string) *http.Request {
		return withURLParams(newRequest(http.MethodDelete, "/api/workspaces/"+testWorkspaceID+"/lark/installations/"+id, nil),
			"id", testWorkspaceID, "installationId", id)
	}
	status := func(id string) string {
		var s string
		dbfx.QueryRow(t, `SELECT status FROM channel_installation WHERE id = $1::uuid`, id).Scan(&s)
		return s
	}

	t.Run("one row, no config", func(t *testing.T) {
		id := seed(t, "cli_audit_r2")
		req, reqID := r2Request(t, revoke(id))
		testutil.Call(t, testHandler.RevokeLarkInstallation, req).Want(http.StatusNoContent)
		row := oneAuditRow(t, reqID, audit.ActionIntegrationDisconnected, testWorkspaceID, "integration", id)
		detailHasNo(t, row, "planted-lark-secret")
		if row.Detail["provider"] != "lark" {
			t.Errorf("detail = %v, want provider lark", row.Detail)
		}
		if status(id) != "revoked" {
			t.Errorf("status = %q, want revoked", status(id))
		}
	})

	t.Run("a failed audit write leaves the installation active", func(t *testing.T) {
		id := seed(t, "cli_audit_r2_keep")
		testutil.Call(t, testHandler.RevokeLarkInstallation, r2FailingRequest(t, revoke(id))).Want(http.StatusInternalServerError)
		if status(id) != "active" {
			t.Fatalf("status = %q after a failed audit write, want active", status(id))
		}
	})
}

func TestAuditR2QoderConnectAndDisconnect(t *testing.T) {
	requirePlatformTestDatabase(t)
	failAuditWrites(t)
	withVCSBox(t)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM daemon_token WHERE id IN (SELECT token_id FROM cloud_agent_connection WHERE provider='qoder_cloud' AND workspace_id=$1)`, testWorkspaceID)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM cloud_agent_connection WHERE provider='qoder_cloud' AND workspace_id=$1`, testWorkspaceID)
	})
	connections := func() int {
		return dbfx.Count(t, `SELECT count(*) FROM cloud_agent_connection WHERE provider='qoder_cloud' AND workspace_id=$1`, testWorkspaceID)
	}

	const plantedQoderToken, plantedGitHubToken = "planted-qoder-token", "planted-github-token"
	body := map[string]any{
		"name": "Audit QCA", "base_url": "https://api.qoder.com/api/v1/cloud", "environment_id": "env_audit",
		"qoder_token": plantedQoderToken, "github_tokens": map[string]string{"https://github.com/owner/repo": plantedGitHubToken},
	}
	save := func() *http.Request { return vcsHandlerRequest(http.MethodPut, "/qoder", body, "") }

	testutil.Call(t, testHandler.SaveQoderConnection, r2FailingRequest(t, save())).Want(http.StatusInternalServerError)
	if connections() != 0 {
		t.Fatal("a connection was saved although the audit write failed")
	}

	req, reqID := r2Request(t, save())
	testutil.Call(t, testHandler.SaveQoderConnection, req).Want(http.StatusOK)
	row := oneAuditRow(t, reqID, audit.ActionIntegrationConnected, testWorkspaceID, "integration", testWorkspaceID)
	detailHasNo(t, row, plantedQoderToken, plantedGitHubToken, "env_audit", "mdt_")
	if row.Detail["provider"] != "qoder" {
		t.Errorf("detail = %v, want provider qoder", row.Detail)
	}

	disconnect := func() *http.Request {
		return vcsHandlerRequest(http.MethodDelete, "/qoder", map[string]any{"expected_active_agent_ids": []string{}}, "")
	}
	testutil.Call(t, testHandler.DisconnectQoderConnection, r2FailingRequest(t, disconnect())).Want(http.StatusInternalServerError)
	if connections() != 1 {
		t.Fatal("the connection was removed although the audit write failed")
	}
	req, reqID = r2Request(t, disconnect())
	testutil.Call(t, testHandler.DisconnectQoderConnection, req).Want(http.StatusOK)
	oneAuditRow(t, reqID, audit.ActionIntegrationDisconnected, testWorkspaceID, "integration", testWorkspaceID)
}

// --- plugins

func TestAuditR2PluginLifecycle(t *testing.T) {
	requirePlatformTestDatabase(t)
	failAuditWrites(t)
	withPluginsV1Flag(t, testHandler, true)
	cleanupPluginInstallations(t)
	versionID := withLocalPluginSource(t, handlerTestManifest)

	const plantedSecret = "sk-planted-plugin-secret"
	call := func(h http.HandlerFunc, method string, body any, params map[string]string, failing bool) (*httptest.ResponseRecorder, string) {
		raw, _ := json.Marshal(body)
		req := pluginHandlerRequest(method, "/plugins", raw, params)
		var reqID string
		if failing {
			req = r2FailingRequest(t, req)
		} else {
			req, reqID = r2Request(t, req)
		}
		w := httptest.NewRecorder()
		h(w, req)
		return w, reqID
	}
	wsParams := map[string]string{"id": testWorkspaceID}
	install := map[string]any{"version_id": versionID, "granted_scopes": []string{"issues:read", "comments:write", "storage:user"}}

	w, _ := call(testHandler.InstallPlugin, http.MethodPost, install, wsParams, true)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("install with a failing audit write: status %d, want 500", w.Code)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM plugin_installation WHERE workspace_id = $1`, testWorkspaceID); n != 0 {
		t.Fatalf("%d installations after a failed audit write, want 0", n)
	}

	w, reqID := call(testHandler.InstallPlugin, http.MethodPost, install, wsParams, false)
	if w.Code != http.StatusCreated {
		t.Fatalf("install: status %d: %s", w.Code, w.Body.String())
	}
	var installed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &installed); err != nil {
		t.Fatal(err)
	}
	row := oneAuditRow(t, reqID, audit.ActionPluginInstalled, testWorkspaceID, "plugin", installed.ID)
	if row.Detail["plugin"] != "com.example.hello" {
		t.Errorf("install detail = %v, want plugin com.example.hello", row.Detail)
	}
	params := map[string]string{"id": testWorkspaceID, "installationId": installed.ID}

	// configure: names the fields, never a value, and a failed write keeps the old config.
	cfg := map[string]any{"values": map[string]any{"repo": "multica-ai/multica", "token": plantedSecret}}
	if w, _ = call(testHandler.ConfigurePlugin, http.MethodPut, cfg, params, true); w.Code != http.StatusInternalServerError {
		t.Fatalf("configure with a failing audit write: status %d, want 500", w.Code)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM plugin_secret WHERE installation_id = $1::uuid`, installed.ID); n != 0 {
		t.Fatalf("%d secrets after a failed audit write, want 0", n)
	}
	w, reqID = call(testHandler.ConfigurePlugin, http.MethodPut, cfg, params, false)
	if w.Code != http.StatusOK {
		t.Fatalf("configure: status %d: %s", w.Code, w.Body.String())
	}
	row = oneAuditRow(t, reqID, audit.ActionPluginConfigured, testWorkspaceID, "plugin", installed.ID)
	detailHasNo(t, row, plantedSecret, "multica-ai/multica")

	// token rotation: the token is shown once to the caller and never recorded.
	var hashBefore *string
	dbfx.QueryRow(t, `SELECT token_hash FROM plugin_installation WHERE id = $1::uuid`, installed.ID).Scan(&hashBefore)
	if w, _ = call(testHandler.RotatePluginToken, http.MethodPost, nil, params, true); w.Code != http.StatusInternalServerError {
		t.Fatalf("rotate with a failing audit write: status %d, want 500", w.Code)
	}
	var hashAfter *string
	dbfx.QueryRow(t, `SELECT token_hash FROM plugin_installation WHERE id = $1::uuid`, installed.ID).Scan(&hashAfter)
	if (hashBefore == nil) != (hashAfter == nil) || (hashBefore != nil && *hashBefore != *hashAfter) {
		t.Fatal("the token hash changed although the audit write failed")
	}
	w, reqID = call(testHandler.RotatePluginToken, http.MethodPost, nil, params, false)
	if w.Code != http.StatusOK {
		t.Fatalf("rotate: status %d: %s", w.Code, w.Body.String())
	}
	var rotated pluginTokenResponse
	if err := json.Unmarshal(w.Body.Bytes(), &rotated); err != nil {
		t.Fatal(err)
	}
	row = oneAuditRow(t, reqID, audit.ActionPluginTokenRotated, testWorkspaceID, "plugin", installed.ID)
	detailHasNo(t, row, rotated.Token, rotated.SigningSecret)

	// disable and enable
	if w, _ = call(testHandler.DisablePlugin, http.MethodPost, nil, params, true); w.Code != http.StatusInternalServerError {
		t.Fatalf("disable with a failing audit write: status %d, want 500", w.Code)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM plugin_installation WHERE id = $1::uuid AND enabled`, installed.ID); n != 1 {
		t.Fatal("the plugin was disabled although the audit write failed")
	}
	w, reqID = call(testHandler.DisablePlugin, http.MethodPost, nil, params, false)
	if w.Code != http.StatusOK {
		t.Fatalf("disable: status %d", w.Code)
	}
	oneAuditRow(t, reqID, audit.ActionPluginDisabled, testWorkspaceID, "plugin", installed.ID)
	w, reqID = call(testHandler.EnablePlugin, http.MethodPost, nil, params, false)
	if w.Code != http.StatusOK {
		t.Fatalf("enable: status %d", w.Code)
	}
	oneAuditRow(t, reqID, audit.ActionPluginEnabled, testWorkspaceID, "plugin", installed.ID)
	// A second enable changes nothing and records nothing.
	w, reqID = call(testHandler.EnablePlugin, http.MethodPost, nil, params, false)
	if w.Code != http.StatusOK {
		t.Fatalf("repeated enable: status %d", w.Code)
	}
	noAuditRow(t, reqID, audit.ActionPluginEnabled)

	// uninstall
	if w, _ = call(testHandler.UninstallPlugin, http.MethodDelete, nil, params, true); w.Code != http.StatusInternalServerError {
		t.Fatalf("uninstall with a failing audit write: status %d, want 500", w.Code)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM plugin_installation WHERE id = $1::uuid`, installed.ID); n != 1 {
		t.Fatal("the plugin was uninstalled although the audit write failed")
	}
	w, reqID = call(testHandler.UninstallPlugin, http.MethodDelete, nil, params, false)
	if w.Code != http.StatusNoContent {
		t.Fatalf("uninstall: status %d", w.Code)
	}
	oneAuditRow(t, reqID, audit.ActionPluginUninstalled, testWorkspaceID, "plugin", installed.ID)
}

// --- MCP library and runtime profiles

func TestAuditR2WorkspaceMcpServerChanges(t *testing.T) {
	requirePlatformTestDatabase(t)
	failAuditWrites(t)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM workspace_mcp_server WHERE workspace_id = $1`, testWorkspaceID)
	})

	const plantedToken = "planted-mcp-bearer-token"
	cfg := map[string]any{"url": "https://mcp-audit.example", "headers": map[string]string{"Authorization": "Bearer " + plantedToken}}
	mcpReq := func(method string, body any, serverID string) *http.Request {
		path := "/api/workspaces/" + testWorkspaceID + "/mcp-servers"
		if serverID == "" {
			return withURLParam(newRequest(method, path, body), "id", testWorkspaceID)
		}
		return withURLParams(newRequest(method, path+"/"+serverID, body), "id", testWorkspaceID, "serverId", serverID)
	}
	mcpCount := func() int {
		return dbfx.Count(t, `SELECT count(*) FROM workspace_mcp_server WHERE workspace_id = $1`, testWorkspaceID)
	}

	before := mcpCount()
	testutil.Call(t, testHandler.CreateWorkspaceMcpServer, r2FailingRequest(t, mcpReq(http.MethodPost, map[string]any{"name": "audit-mcp", "config": cfg}, ""))).
		Want(http.StatusInternalServerError)
	if mcpCount() != before {
		t.Fatal("a server was saved although the audit write failed")
	}

	req, reqID := r2Request(t, mcpReq(http.MethodPost, map[string]any{"name": "audit-mcp", "config": cfg}, ""))
	var created WorkspaceMcpServerResponse
	testutil.Call(t, testHandler.CreateWorkspaceMcpServer, req).Want(http.StatusCreated).JSON(&created)
	row := oneAuditRow(t, reqID, audit.ActionMCPServerCreated, testWorkspaceID, "mcp_server", created.ID)
	detailHasNo(t, row, plantedToken, "mcp-audit.example")
	if row.TargetLabel != "audit-mcp" {
		t.Errorf("target_label = %q, want the server name", row.TargetLabel)
	}

	renamed := map[string]any{"name": "audit-mcp-2", "config": cfg}
	testutil.Call(t, testHandler.UpdateWorkspaceMcpServer, r2FailingRequest(t, mcpReq(http.MethodPut, renamed, created.ID))).Want(http.StatusInternalServerError)
	var name string
	dbfx.QueryRow(t, `SELECT name FROM workspace_mcp_server WHERE id = $1::uuid`, created.ID).Scan(&name)
	if name != "audit-mcp" {
		t.Fatalf("name = %q after a failed audit write, want audit-mcp", name)
	}
	req, reqID = r2Request(t, mcpReq(http.MethodPut, renamed, created.ID))
	testutil.Call(t, testHandler.UpdateWorkspaceMcpServer, req).Want(http.StatusOK)
	row = oneAuditRow(t, reqID, audit.ActionMCPServerUpdated, testWorkspaceID, "mcp_server", created.ID)
	detailHasNo(t, row, plantedToken)
	if row.TargetLabel != "audit-mcp-2" {
		t.Errorf("target_label = %q, want the new name", row.TargetLabel)
	}

	testutil.Call(t, testHandler.DeleteWorkspaceMcpServer, r2FailingRequest(t, mcpReq(http.MethodDelete, nil, created.ID))).Want(http.StatusInternalServerError)
	if n := dbfx.Count(t, `SELECT count(*) FROM workspace_mcp_server WHERE id = $1::uuid`, created.ID); n != 1 {
		t.Fatal("the server was deleted although the audit write failed")
	}
	req, reqID = r2Request(t, mcpReq(http.MethodDelete, nil, created.ID))
	testutil.Call(t, testHandler.DeleteWorkspaceMcpServer, req).Want(http.StatusNoContent)
	oneAuditRow(t, reqID, audit.ActionMCPServerDeleted, testWorkspaceID, "mcp_server", created.ID)
}

func TestAuditR2RuntimeProfileChanges(t *testing.T) {
	requirePlatformTestDatabase(t)
	failAuditWrites(t)
	ctx := context.Background()

	profileReq := func(method string, body any, profileID string) *http.Request {
		path := "/api/workspaces/" + testWorkspaceID + "/runtime-profiles"
		if profileID == "" {
			return withURLParam(newRequest(method, path, body), "id", testWorkspaceID)
		}
		return withURLParams(newRequest(method, path+"/"+profileID, body), "id", testWorkspaceID, "profileId", profileID)
	}
	create := map[string]any{"display_name": "Audit Profile", "protocol_family": "codex", "command_name": "audit-r2-codex", "fixed_args": []string{"--planted-arg-secret"}}
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM runtime_profile WHERE workspace_id = $1 AND command_name = 'audit-r2-codex'`, testWorkspaceID)
	})

	testutil.Call(t, testHandler.CreateRuntimeProfile, r2FailingRequest(t, profileReq(http.MethodPost, create, ""))).Want(http.StatusInternalServerError)
	if n := dbfx.Count(t, `SELECT count(*) FROM runtime_profile WHERE workspace_id = $1 AND command_name = 'audit-r2-codex'`, testWorkspaceID); n != 0 {
		t.Fatal("a profile was saved although the audit write failed")
	}

	req, reqID := r2Request(t, profileReq(http.MethodPost, create, ""))
	var created RuntimeProfileResponse
	testutil.Call(t, testHandler.CreateRuntimeProfile, req).Want(http.StatusCreated).JSON(&created)
	row := oneAuditRow(t, reqID, audit.ActionRuntimeProfileCreated, testWorkspaceID, "runtime_profile", created.ID)
	detailHasNo(t, row, "planted-arg-secret")

	patch := map[string]any{"display_name": "Audit Profile 2"}
	testutil.Call(t, testHandler.UpdateRuntimeProfile, r2FailingRequest(t, profileReq(http.MethodPatch, patch, created.ID))).Want(http.StatusInternalServerError)
	var name string
	dbfx.QueryRow(t, `SELECT display_name FROM runtime_profile WHERE id = $1::uuid`, created.ID).Scan(&name)
	if name != "Audit Profile" {
		t.Fatalf("display_name = %q after a failed audit write, want the old name", name)
	}
	req, reqID = r2Request(t, profileReq(http.MethodPatch, patch, created.ID))
	testutil.Call(t, testHandler.UpdateRuntimeProfile, req).Want(http.StatusOK)
	row = oneAuditRow(t, reqID, audit.ActionRuntimeProfileUpdated, testWorkspaceID, "runtime_profile", created.ID)
	if fields, _ := row.Detail["fields"].([]any); len(fields) != 1 || fields[0] != "display_name" {
		t.Errorf("detail = %v, want the changed field names [display_name]", row.Detail)
	}

	testutil.Call(t, testHandler.DeleteRuntimeProfile, r2FailingRequest(t, profileReq(http.MethodDelete, nil, created.ID))).Want(http.StatusInternalServerError)
	if n := dbfx.Count(t, `SELECT count(*) FROM runtime_profile WHERE id = $1::uuid`, created.ID); n != 1 {
		t.Fatal("the profile was deleted although the audit write failed")
	}
	req, reqID = r2Request(t, profileReq(http.MethodDelete, nil, created.ID))
	testutil.Call(t, testHandler.DeleteRuntimeProfile, req).Want(http.StatusNoContent)
	oneAuditRow(t, reqID, audit.ActionRuntimeProfileDeleted, testWorkspaceID, "runtime_profile", created.ID)
}

// --- agents

func TestAuditR2AgentCreateArchiveAndPermission(t *testing.T) {
	requirePlatformTestDatabase(t)
	failAuditWrites(t)
	ctx := context.Background()
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM agent WHERE workspace_id = $1 AND name LIKE 'audit-r2-agent-%'`, testWorkspaceID)
	})

	const plantedEnv = "planted-agent-env-value"
	newAgent := func(name string) map[string]any {
		return map[string]any{
			"name": name, "runtime_id": handlerTestRuntimeID(t), "visibility": "private",
			"max_concurrent_tasks": 1, "custom_env": map[string]string{"AUDIT_R2_TOKEN": plantedEnv},
		}
	}
	createReq := func(body any) *http.Request { return newRequest(http.MethodPost, "/api/agents", body) }

	failingName := "audit-r2-agent-" + uuid.NewString()[:8]
	testutil.Call(t, testHandler.CreateAgent, r2FailingRequest(t, createReq(newAgent(failingName)))).Want(http.StatusInternalServerError)
	if n := dbfx.Count(t, `SELECT count(*) FROM agent WHERE workspace_id = $1 AND name = $2`, testWorkspaceID, failingName); n != 0 {
		t.Fatal("an agent was saved although the audit write failed")
	}

	name := "audit-r2-agent-" + uuid.NewString()[:8]
	req, reqID := r2Request(t, createReq(newAgent(name)))
	var created AgentResponse
	testutil.Call(t, testHandler.CreateAgent, req).Want(http.StatusCreated).JSON(&created)
	row := oneAuditRow(t, reqID, audit.ActionAgentCreated, testWorkspaceID, "agent", created.ID)
	detailHasNo(t, row, plantedEnv)
	if row.TargetLabel != name {
		t.Errorf("target_label = %q, want %q", row.TargetLabel, name)
	}

	// permission change: old and new mode, no target ids of a private list
	updateReq := func(body any) *http.Request {
		return withURLParam(newRequest(http.MethodPut, "/api/agents/"+created.ID, body), "id", created.ID)
	}
	var modeBefore string
	dbfx.QueryRow(t, `SELECT permission_mode FROM agent WHERE id = $1::uuid`, created.ID).Scan(&modeBefore)
	next := "public_to"
	if modeBefore == next {
		next = "private"
	}
	testutil.Call(t, testHandler.UpdateAgent, r2FailingRequest(t, updateReq(map[string]any{"permission_mode": next}))).Want(http.StatusInternalServerError)
	var modeAfter string
	dbfx.QueryRow(t, `SELECT permission_mode FROM agent WHERE id = $1::uuid`, created.ID).Scan(&modeAfter)
	if modeAfter != modeBefore {
		t.Fatalf("permission_mode = %q after a failed audit write, want %q", modeAfter, modeBefore)
	}
	req, reqID = r2Request(t, updateReq(map[string]any{"permission_mode": next}))
	testutil.Call(t, testHandler.UpdateAgent, req).Want(http.StatusOK)
	row = oneAuditRow(t, reqID, audit.ActionAgentPermissionChanged, testWorkspaceID, "agent", created.ID)
	perm, _ := row.Detail["permission_mode"].(map[string]any)
	if perm["from"] != modeBefore || perm["to"] != next {
		t.Errorf("detail = %v, want permission_mode from %s to %s", row.Detail, modeBefore, next)
	}
	// Resubmitting the same mode changes nothing and records nothing.
	req, reqID = r2Request(t, updateReq(map[string]any{"permission_mode": next}))
	testutil.Call(t, testHandler.UpdateAgent, req).Want(http.StatusOK)
	noAuditRow(t, reqID, audit.ActionAgentPermissionChanged)

	// archive
	archiveReq := func() *http.Request {
		return withURLParam(newRequest(http.MethodPost, "/api/agents/"+created.ID+"/archive", nil), "id", created.ID)
	}
	testutil.Call(t, testHandler.ArchiveAgent, r2FailingRequest(t, archiveReq())).Want(http.StatusInternalServerError)
	if n := dbfx.Count(t, `SELECT count(*) FROM agent WHERE id = $1::uuid AND archived_at IS NULL`, created.ID); n != 1 {
		t.Fatal("the agent was archived although the audit write failed")
	}
	req, reqID = r2Request(t, archiveReq())
	testutil.Call(t, testHandler.ArchiveAgent, req).Want(http.StatusOK)
	oneAuditRow(t, reqID, audit.ActionAgentArchived, testWorkspaceID, "agent", created.ID)
}

func TestAuditR2AgentEnvUpdated(t *testing.T) {
	requirePlatformTestDatabase(t)
	failAuditWrites(t)
	agentID := createHandlerTestAgent(t, "audit-r2-env-"+uuid.NewString()[:8], nil)
	dbfx.Exec(t, `UPDATE agent SET custom_env = '{"KEEP_ME":"old-env-value"}' WHERE id = $1::uuid`, agentID)

	const plantedValue = "planted-new-env-value"
	envReq := func() *http.Request {
		return withURLParam(newRequest(http.MethodPut, "/api/agents/"+agentID+"/env",
			map[string]any{"custom_env": map[string]string{"KEEP_ME": "old-env-value", "NEW_NAME": plantedValue}}), "id", agentID)
	}
	envOf := func() string {
		var s string
		dbfx.QueryRow(t, `SELECT custom_env::text FROM agent WHERE id = $1::uuid`, agentID).Scan(&s)
		return s
	}
	before := envOf()
	testutil.Call(t, testHandler.UpdateAgentEnv, r2FailingRequest(t, envReq())).Want(http.StatusInternalServerError)
	if envOf() != before {
		t.Fatal("the env changed although the audit write failed")
	}

	req, reqID := r2Request(t, envReq())
	testutil.Call(t, testHandler.UpdateAgentEnv, req).Want(http.StatusOK)
	row := oneAuditRow(t, reqID, audit.ActionAgentEnvUpdated, testWorkspaceID, "agent", agentID)
	detailHasNo(t, row, plantedValue, "old-env-value")
	added, _ := row.Detail["added"].([]any)
	if len(added) != 1 || added[0] != "NEW_NAME" {
		t.Errorf("detail = %v, want the added variable names [NEW_NAME]", row.Detail)
	}
}

// --- runtimes

func TestAuditR2RuntimeVisibilityAndDelete(t *testing.T) {
	requirePlatformTestDatabase(t)
	failAuditWrites(t)

	visibility := func(runtimeID string) string {
		var v string
		dbfx.QueryRow(t, `SELECT visibility FROM agent_runtime WHERE id = $1::uuid`, runtimeID).Scan(&v)
		return v
	}
	patch := func(runtimeID string) *http.Request {
		return withURLParam(newRequest(http.MethodPatch, "/api/runtimes/"+runtimeID, map[string]any{"visibility": "public"}), "runtimeId", runtimeID)
	}
	remove := func(runtimeID string) *http.Request {
		return withURLParam(newRequest(http.MethodDelete, "/api/runtimes/"+runtimeID, nil), "runtimeId", runtimeID)
	}

	t.Run("visibility", func(t *testing.T) {
		runtimeID := dbfx.Runtime(t, "Audit visibility runtime")
		testutil.Call(t, testHandler.UpdateAgentRuntime, r2FailingRequest(t, patch(runtimeID))).Want(http.StatusInternalServerError)
		if visibility(runtimeID) != "private" {
			t.Fatal("the visibility changed although the audit write failed")
		}
		req, reqID := r2Request(t, patch(runtimeID))
		testutil.Call(t, testHandler.UpdateAgentRuntime, req).Want(http.StatusOK)
		row := oneAuditRow(t, reqID, audit.ActionRuntimeVisibilityChanged, testWorkspaceID, "runtime", runtimeID)
		vis, _ := row.Detail["visibility"].(map[string]any)
		if vis["from"] != "private" || vis["to"] != "public" {
			t.Errorf("detail = %v, want visibility from private to public", row.Detail)
		}
		// The same value again changes nothing and records nothing.
		req, reqID = r2Request(t, patch(runtimeID))
		testutil.Call(t, testHandler.UpdateAgentRuntime, req).Want(http.StatusOK)
		noAuditRow(t, reqID, audit.ActionRuntimeVisibilityChanged)
	})

	t.Run("delete", func(t *testing.T) {
		runtimeID := dbfx.Runtime(t, "Audit delete runtime")
		testutil.Call(t, testHandler.DeleteAgentRuntime, r2FailingRequest(t, remove(runtimeID))).Want(http.StatusInternalServerError)
		if n := dbfx.Count(t, `SELECT count(*) FROM agent_runtime WHERE id = $1::uuid`, runtimeID); n != 1 {
			t.Fatal("the runtime was deleted although the audit write failed")
		}
		req, reqID := r2Request(t, remove(runtimeID))
		testutil.Call(t, testHandler.DeleteAgentRuntime, req).Want(http.StatusOK)
		oneAuditRow(t, reqID, audit.ActionRuntimeDeleted, testWorkspaceID, "runtime", runtimeID)
	})

	t.Run("confirmed delete with unbinding", func(t *testing.T) {
		runtimeID := dbfx.Runtime(t, "Audit unbind runtime")
		req := withURLParam(newRequest(http.MethodPost, "/api/runtimes/"+runtimeID+"/unbind-agents-and-delete",
			map[string]any{"expected_active_agent_ids": []string{}}), "runtimeId", runtimeID)
		req, reqID := r2Request(t, req)
		testutil.Call(t, testHandler.UnbindAgentsAndDeleteRuntime, req).Want(http.StatusOK)
		oneAuditRow(t, reqID, audit.ActionRuntimeDeleted, testWorkspaceID, "runtime", runtimeID)
	})
}

// functionBodies returns the source of every function in file, by name.
func functionBodies(t *testing.T, file string) map[string]string {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, src, 0)
	if err != nil {
		t.Fatal(err)
	}
	bodies := map[string]string{}
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Body != nil {
			bodies[fn.Name.Name] = string(src[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset])
		}
	}
	return bodies
}

// Source check: every handler that writes an R2 action reaches the audit
// writer. The accepted forms are a call to a handler audit helper or to the
// service hook that runs inside the transaction.
func TestAuditR2HandlersRecordAuditEvents(t *testing.T) {
	want := map[string][]string{
		"workspace_deletion.go": {"scheduleWorkspaceDeletion", "CancelWorkspaceDeletion"},
		"workspace_export.go":   {"CreateWorkspaceExport"},
		"github.go":             {"GitHubSetupCallback", "DeleteGitHubInstallation"},
		"vcs.go":                {"ConnectVCS", "DeleteVCSConnection"},
		"slack.go":              {"RevokeSlackInstallation"},
		"lark.go":               {"RevokeLarkInstallation"},
		"telegram.go":           {"RevokeTelegramInstallation"},
		"wecom_web.go":          {"RevokeWecomInstallation"},
		"dingtalk.go":           {"RevokeDingTalkInstallation"},
		"qoder_connection.go":   {"saveQoderConnection"},
		"qoder_disconnect.go":   {"DisconnectQoderConnection"},
		"plugin.go":             {"InstallPlugin", "ConfigurePlugin", "setPluginEnabled", "UninstallPlugin"},
		"plugin_hook.go":        {"RotatePluginToken"},
		"workspace_mcp_api.go":  {"CreateWorkspaceMcpServer", "UpdateWorkspaceMcpServer", "DeleteWorkspaceMcpServer"},
		"runtime_profile.go":    {"CreateRuntimeProfile", "UpdateRuntimeProfile", "DeleteRuntimeProfile"},
		"agent.go":              {"CreateAgent", "UpdateAgent", "ArchiveAgent"},
		"squad_pack.go":         {"installPack"},
		"agent_env.go":          {"UpdateAgentEnv"},
		"runtime.go":            {"updateRuntimeAccessIfNoDependencies", "DeleteAgentRuntime", "UnbindAgentsAndDeleteRuntime"},
	}
	for file, names := range want {
		bodies := functionBodies(t, file)
		for _, name := range names {
			body, ok := bodies[name]
			if !ok {
				t.Errorf("%s: function %s not found", file, name)
				continue
			}
			if !strings.Contains(body, "recordAudit") && !strings.Contains(body, "recordAgentCreated(") &&
				!strings.Contains(body, "revokeChannelInstallation(") && !strings.Contains(body, "pluginAudit(") &&
				!strings.Contains(body, "auditTx(") {
				t.Errorf("%s: %s has no audit call", file, name)
			}
		}
	}
}
