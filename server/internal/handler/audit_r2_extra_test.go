package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/integrations/wecom"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util/secretbox"
	"github.com/multica-ai/multica/server/internal/workspaceexport"
)

// Audit events of the R2 slice that no request writes (the export job and the
// channel install services), and the separate-transaction permission path of
// UpdateAgent. Database-backed like audit_r2_test.go: they run only against the
// dedicated *_test database named by PLATFORM_TEST_DATABASE_URL. They were
// written without a database available and have not been run.

// failAuditWhen makes every audit_event insert that satisfies cond raise. cond
// is a SQL expression over NEW and is built by the caller from generated ids
// only. Rows written by a job or a service carry no request id, so the
// request-tagged trigger of failAuditWrites does not reach them.
func failAuditWhen(t *testing.T, cond string) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		"CREATE OR REPLACE FUNCTION audit_test_fail_when() RETURNS trigger AS $$\n" +
			" BEGIN\n IF " + cond + " THEN\n RAISE EXCEPTION 'injected audit failure';\n END IF;\n RETURN NEW;\n" +
			" END $$ LANGUAGE plpgsql",
		"DROP TRIGGER IF EXISTS audit_test_fail_when ON audit_event",
		"CREATE TRIGGER audit_test_fail_when BEFORE INSERT ON audit_event" +
			" FOR EACH ROW EXECUTE FUNCTION audit_test_fail_when()",
	} {
		if _, err := testPool.Exec(ctx, stmt); err != nil {
			t.Fatalf("install failing audit trigger: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), "DROP TRIGGER IF EXISTS audit_test_fail_when ON audit_event")
		_, _ = testPool.Exec(context.Background(), "DROP FUNCTION IF EXISTS audit_test_fail_when()")
	})
}

// auditRowsFor returns the rows of action about targetID, whatever request
// wrote them, and removes them when the test ends.
func auditRowsFor(t *testing.T, action, targetID string) []auditRow {
	t.Helper()
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM audit_event WHERE action = $1 AND target_id = $2`, action, targetID)
	})
	rows, err := testPool.Query(context.Background(), `
		SELECT workspace_id::text, actor_type, actor_id, actor_label,
		       target_type, target_id, target_label, detail, ip
		FROM audit_event WHERE action = $1 AND target_id = $2`, action, targetID)
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

func TestAuditR2WorkspaceExportCompletedAndFailed(t *testing.T) {
	requirePlatformTestDatabase(t)
	store := workspaceexport.NewDBStore(testHandler.Queries, testHandler.TxStarter)
	ctx := context.Background()

	running := func(t *testing.T, wsID string) string {
		t.Helper()
		id := uuid.NewString()
		dbfx.Exec(t, `INSERT INTO workspace_export (id, workspace_id, requested_by, status, started_at)
			VALUES ($1::uuid, $2::uuid, $3::uuid, 'running', now())`, id, wsID, testUserID)
		dbfx.Cleanup(t, `DELETE FROM workspace_export WHERE id = $1::uuid`, id)
		return id
	}
	status := func(id string) string {
		var s string
		dbfx.QueryRow(t, `SELECT status FROM workspace_export WHERE id = $1::uuid`, id).Scan(&s)
		return s
	}
	const plantedKey, plantedError = "workspace-exports/planted-storage-key.zip", "planted-provider-error-secret"
	now := time.Now()

	t.Run("completed writes one system row without the storage key", func(t *testing.T) {
		wsID := r2OwnedWorkspace(t, "Audit export done")
		id := running(t, wsID)
		ok, err := store.MarkDone(ctx, id, now, now.Add(time.Hour), plantedKey, 1234)
		if err != nil || !ok {
			t.Fatalf("MarkDone = %v, %v", ok, err)
		}
		rows := auditRowsFor(t, audit.ActionWorkspaceExportCompleted, id)
		if len(rows) != 1 {
			t.Fatalf("%d audit rows, want 1", len(rows))
		}
		row := rows[0]
		if row.ActorType != audit.ActorSystem || row.ActorID != nil {
			t.Errorf("actor = %s/%v, want system with no id", row.ActorType, row.ActorID)
		}
		if row.WorkspaceID == nil || *row.WorkspaceID != wsID || row.TargetType != "workspace_export" {
			t.Errorf("row = %+v, want workspace %s and target workspace_export", row, wsID)
		}
		if row.Detail["size_bytes"] != float64(1234) {
			t.Errorf("detail = %v, want size_bytes 1234", row.Detail)
		}
		detailHasNo(t, row, plantedKey)
		// A repeat finds no running export and writes no second row.
		if ok, err := store.MarkDone(ctx, id, now, now.Add(time.Hour), plantedKey, 1234); err != nil || ok {
			t.Fatalf("second MarkDone = %v, %v, want false", ok, err)
		}
		if n := len(auditRowsFor(t, audit.ActionWorkspaceExportCompleted, id)); n != 1 {
			t.Fatalf("%d audit rows after a repeat, want 1", n)
		}
	})

	t.Run("a failed audit write leaves the export running", func(t *testing.T) {
		wsID := r2OwnedWorkspace(t, "Audit export done fails")
		id := running(t, wsID)
		failAuditWhen(t, "NEW.target_id = '"+id+"'")
		if ok, err := store.MarkDone(ctx, id, now, now.Add(time.Hour), plantedKey, 1); err == nil || ok {
			t.Fatalf("MarkDone = %v, %v, want an error", ok, err)
		}
		if got := status(id); got != "running" {
			t.Fatalf("status = %q after a failed audit write, want running", got)
		}
	})

	t.Run("failed writes one row without the error text", func(t *testing.T) {
		wsID := r2OwnedWorkspace(t, "Audit export failed")
		id := running(t, wsID)
		ok, err := store.MarkFailed(ctx, id, now, plantedError)
		if err != nil || !ok {
			t.Fatalf("MarkFailed = %v, %v", ok, err)
		}
		rows := auditRowsFor(t, audit.ActionWorkspaceExportFailed, id)
		if len(rows) != 1 || rows[0].ActorType != audit.ActorSystem {
			t.Fatalf("rows = %+v, want one system row", rows)
		}
		detailHasNo(t, rows[0], plantedError)
		if status(id) != "failed" {
			t.Errorf("status = %q, want failed", status(id))
		}
	})

	t.Run("a failed audit write leaves the failing export running", func(t *testing.T) {
		wsID := r2OwnedWorkspace(t, "Audit export failed fails")
		id := running(t, wsID)
		failAuditWhen(t, "NEW.target_id = '"+id+"'")
		if ok, err := store.MarkFailed(ctx, id, now, plantedError); err == nil || ok {
			t.Fatalf("MarkFailed = %v, %v, want an error", ok, err)
		}
		if got := status(id); got != "running" {
			t.Fatalf("status = %q after a failed audit write, want running", got)
		}
	})
}

// okProbe accepts every credential, so the install path runs without a socket.
type okProbe struct{}

func (okProbe) Probe(context.Context, string, string) error { return nil }

func TestAuditR2WecomConnectWritesIntegrationConnected(t *testing.T) {
	requirePlatformTestDatabase(t)
	ctx := context.Background()
	box, err := secretbox.New(make([]byte, secretbox.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := wecom.NewInstallationService(testHandler.Queries, testPool, box, wecom.WithCredentialProbe(okProbe{}))
	if err != nil {
		t.Fatal(err)
	}
	const plantedSecret = "planted-wecom-bot-secret"
	install := func(agentID, botID string) (wecom.Installation, error) {
		return svc.Upsert(ctx, wecom.InstallationParams{
			WorkspaceID: parseUUID(testWorkspaceID), AgentID: parseUUID(agentID), InstallerUserID: parseUUID(testUserID),
			BotID: botID, Secret: plantedSecret,
		})
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM channel_installation WHERE workspace_id = $1 AND channel_type = 'wecom'`, testWorkspaceID)
	})

	t.Run("connect writes one row without the secret", func(t *testing.T) {
		agentID := createHandlerTestAgent(t, "audit-r2-wecom-"+uuid.NewString()[:8], nil)
		botID := "audit-r2-bot-" + uuid.NewString()[:8]
		inst, err := install(agentID, botID)
		if err != nil {
			t.Fatal(err)
		}
		rows := auditRowsFor(t, audit.ActionIntegrationConnected, uuidToString(inst.ID))
		if len(rows) != 1 {
			t.Fatalf("%d audit rows, want 1", len(rows))
		}
		row := rows[0]
		if row.ActorType != audit.ActorMember || row.ActorID == nil || *row.ActorID != testUserID ||
			row.ActorLabel != auditTestUserEmail(t, testUserID) {
			t.Errorf("actor = %s/%v/%s, want the installer", row.ActorType, row.ActorID, row.ActorLabel)
		}
		if row.WorkspaceID == nil || *row.WorkspaceID != testWorkspaceID || row.TargetType != "integration" {
			t.Errorf("row = %+v, want workspace %s and target integration", row, testWorkspaceID)
		}
		if row.Detail["provider"] != "wecom" {
			t.Errorf("detail = %v, want provider wecom", row.Detail)
		}
		detailHasNo(t, row, plantedSecret, botID)
	})

	t.Run("a failed audit write saves no installation", func(t *testing.T) {
		agentID := createHandlerTestAgent(t, "audit-r2-wecom-fail-"+uuid.NewString()[:8], nil)
		failAuditWhen(t, "NEW.action = 'integration.connected' AND NEW.workspace_id = '"+testWorkspaceID+
			"'::uuid AND NEW.detail->>'provider' = 'wecom'")
		if _, err := install(agentID, "audit-r2-bot-"+uuid.NewString()[:8]); err == nil {
			t.Fatal("Upsert returned no error for a failed audit write")
		}
		if n := dbfx.Count(t, `SELECT count(*) FROM channel_installation WHERE agent_id = $1::uuid AND channel_type = 'wecom'`, agentID); n != 0 {
			t.Fatalf("%d installations after a failed audit write, want 0", n)
		}
	})
}

// UpdateAgent opens its own transaction for a permission change when the
// request carries no runtime and no instructions. The invocation targets, the
// agent row and the audit row commit together there.
func TestAuditR2AgentPermissionTargetsChangeInTheirOwnTransaction(t *testing.T) {
	requirePlatformTestDatabase(t)
	failAuditWrites(t)
	const plantedEnv = "planted-permission-env-value"
	agentID := createHandlerTestAgent(t, "audit-r2-perm-"+uuid.NewString()[:8], nil)
	dbfx.Exec(t, `UPDATE agent SET owner_id = $2::uuid, custom_env = jsonb_build_object('AUDIT_R2_TOKEN', $3::text) WHERE id = $1::uuid`,
		agentID, testUserID, plantedEnv)

	update := func() *http.Request {
		return withURLParam(newRequest(http.MethodPut, "/api/agents/"+agentID, map[string]any{
			"permission_mode":    "public_to",
			"invocation_targets": []map[string]any{{"target_type": "member", "target_id": testUserID}},
		}), "id", agentID)
	}
	targets := func() []string {
		rows, err := testPool.Query(context.Background(),
			`SELECT target_type || ':' || target_id::text FROM agent_invocation_target WHERE agent_id = $1::uuid ORDER BY 1`, agentID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		return out
	}

	// The audit write fails: the targets keep the workspace target.
	testutil.Call(t, testHandler.UpdateAgent, r2FailingRequest(t, update())).Want(http.StatusInternalServerError)
	if got := targets(); len(got) != 1 || got[0] != "workspace:"+testWorkspaceID {
		t.Fatalf("targets = %v after a failed audit write, want the workspace target only", got)
	}

	// The change commits: the targets are replaced and one row records it.
	req, reqID := r2Request(t, update())
	testutil.Call(t, testHandler.UpdateAgent, req).Want(http.StatusOK)
	if got := targets(); len(got) != 1 || got[0] != "member:"+testUserID {
		t.Fatalf("targets = %v, want [member:%s]", got, testUserID)
	}
	row := oneAuditRow(t, reqID, audit.ActionAgentPermissionChanged, testWorkspaceID, "agent", agentID)
	detailHasNo(t, row, plantedEnv)
	if row.Detail["target_count"] != float64(1) {
		t.Errorf("detail = %v, want target_count 1", row.Detail)
	}
}
