package handler

import (
	"context"
	"sort"
	"testing"

	"github.com/google/uuid"
)

type workspaceDeleteAction string

const (
	workspaceDelete       workspaceDeleteAction = "delete"
	workspaceDeleteDetach workspaceDeleteAction = "detach"
	workspaceDeleteKeep   workspaceDeleteAction = "keep"
	workspaceDeleteSettle workspaceDeleteAction = "settle"
)

// workspaceDeletionManifest is the schema coverage contract for workspace
// teardown. Adding a table requires an explicit ownership decision here; the
// handler deletion graph must then implement that decision before CI passes.
var workspaceDeletionManifest = map[string]workspaceDeleteAction{
	"cloud_agent_connection":             workspaceDelete,
	"hub_activity":                       workspaceDelete,
	"hub_item":                           workspaceDelete,
	"hub_item_version_blob":              workspaceDelete,
	"hub_source":                         workspaceDelete,
	"knowledge_property_history":         workspaceDelete,
	"activity_log":                       workspaceDelete,
	"agent":                              workspaceDelete,
	"agent_builder_draft":                workspaceDelete,
	"agent_invocation_target":            workspaceDelete,
	"agent_runtime":                      workspaceDelete,
	"agent_skill":                        workspaceDelete,
	"agent_task_queue":                   workspaceDelete,
	"agent_to_label":                     workspaceDelete,
	"attachment":                         workspaceDelete,
	"autopilot":                          workspaceDelete,
	"autopilot_collaborator":             workspaceDelete,
	"autopilot_quota_period":             workspaceDelete,
	"autopilot_quota_reservation":        workspaceDelete,
	"autopilot_rule_version":             workspaceDelete,
	"autopilot_run":                      workspaceDelete,
	"autopilot_subscriber":               workspaceDelete,
	"autopilot_trigger":                  workspaceDelete,
	"channel_binding_token":              workspaceDelete,
	"channel_chat_context_generation":    workspaceDelete,
	"channel_chat_session_binding":       workspaceDelete,
	"channel_inbound_audit":              workspaceDelete,
	"channel_inbound_message_dedup":      workspaceDelete,
	"channel_installation":               workspaceDelete,
	"channel_media_pending_object":       workspaceDeleteSettle,
	"channel_outbound_card_message":      workspaceDelete,
	"channel_outbound_message":           workspaceDelete,
	"channel_reply_delivery":             workspaceDelete,
	"channel_task_delivery":              workspaceDelete,
	"channel_typing_reaction":            workspaceDeleteSettle, // Source deletion makes retained anchors eligible for the cleanup worker.
	"channel_user_binding":               workspaceDelete,
	"chat_draft_restore":                 workspaceDelete,
	"chat_message":                       workspaceDelete,
	"chat_pinned_agent":                  workspaceDelete,
	"chat_session":                       workspaceDelete,
	"client_usage_daily":                 workspaceDeleteDetach,
	"comment":                            workspaceDelete,
	"comment_reaction":                   workspaceDelete,
	"contact_sales_inquiry":              workspaceDeleteKeep,
	"daemon_connection":                  workspaceDelete,
	"daemon_token":                       workspaceDelete,
	"dingtalk_group_presence":            workspaceDelete,
	"dingtalk_bot_identity":              workspaceDelete,
	"dingtalk_group_route":               workspaceDelete,
	"feedback":                           workspaceDeleteDetach,
	"github_installation":                workspaceDelete,
	"github_pending_check_suite":         workspaceDelete,
	"github_pending_installation":        workspaceDeleteKeep,
	"github_pull_request":                workspaceDelete,
	"github_pull_request_check_run":      workspaceDelete,
	"github_pull_request_check_suite":    workspaceDelete,
	"inbox_item":                         workspaceDelete,
	"instance_telemetry_state":           workspaceDeleteKeep,
	"issue":                              workspaceDelete,
	"issue_wakeup":                       workspaceDelete,
	"issue_wakeup_receipt":               workspaceDelete,
	"issue_child_event":                  workspaceDelete,
	"issue_view":                         workspaceDelete,
	"issue_view_preference":              workspaceDelete,
	"issue_dependency":                   workspaceDelete,
	"issue_label":                        workspaceDelete,
	"issue_property":                     workspaceDelete,
	"issue_pr_automation":                workspaceDelete,
	"issue_pull_request":                 workspaceDelete,
	"issue_pull_request_exclusion":       workspaceDelete,
	"issue_reaction":                     workspaceDelete,
	"issue_source_context":               workspaceDelete,
	"issue_source_context_object_intent": workspaceDeleteSettle,
	"issue_status":                       workspaceDelete,
	"issue_workflow":                     workspaceDelete,
	"issue_workflow_draft":               workspaceDelete,
	"issue_subscriber":                   workspaceDelete,
	"issue_to_label":                     workspaceDelete,
	"issue_unread":                       workspaceDelete,
	"issue_vcs_pull_request":             workspaceDelete,
	"lark_binding_token":                 workspaceDelete,
	"lark_chat_session_binding":          workspaceDelete,
	"lark_inbound_audit":                 workspaceDelete,
	"lark_inbound_message_dedup":         workspaceDelete,
	"lark_installation":                  workspaceDelete,
	"lark_outbound_card_message":         workspaceDelete,
	"lark_user_binding":                  workspaceDelete,
	"maintenance_job":                    workspaceDeleteKeep, // Global maintenance audit history, not workspace-owned.
	"job_status":                         workspaceDeleteKeep, // Global scheduler state (one row per job), not workspace-owned.
	"member":                             workspaceDelete,
	"member_delegation_day":              workspaceDeleteKeep, // user-scoped telemetry ledger, no workspace_id
	"member_group":                       workspaceDelete,     // workspace-owned account group (spec 7b.2); its members go with it
	"member_group_member":                workspaceDelete,     // workspace-owned group membership (spec 7b.2)
	"member_removal":                     workspaceDelete,
	"agent_mcp_server":                   workspaceDelete,
	"brief":                              workspaceDelete,
	"brief_feedback":                     workspaceDelete,
	"brief_delivery":                     workspaceDelete,
	"brief_inquiry":                      workspaceDelete,
	"brief_push_preference":              workspaceDelete,
	"proposal":                           workspaceDelete,
	"proposal_revision":                  workspaceDelete,
	"initiative":                         workspaceDelete,
	"initiative_artifact":                workspaceDelete,
	"initiative_stage_event":             workspaceDelete,
	"governance_event":                   workspaceDelete,
	"scorecard":                          workspaceDelete,
	"review":                             workspaceDelete,
	"agent_instruction_revision":         workspaceDelete,
	"org_change_application":             workspaceDelete,
	"budget":                             workspaceDelete,
	"budget_period":                      workspaceDelete,
	"budget_hold":                        workspaceDelete,
	"goal":                               workspaceDelete,
	"key_result":                         workspaceDelete,
	"metric":                             workspaceDelete,
	"metric_reading":                     workspaceDelete,
	"metric_webhook_token":               workspaceDelete,
	"retro_request":                      workspaceDelete,
	"planning_cycle":                     workspaceDelete,
	"approval_policy":                    workspaceDelete,
	"publish_channel":                    workspaceDelete,
	"publish_profile":                    workspaceDelete,
	"publish_attempt":                    workspaceDelete,
	"publish_settings":                   workspaceDelete,
	"research_settings":                  workspaceDelete,
	"retro_trigger":                      workspaceDelete,
	"floor_chief":                        workspaceDelete,
	"pipeline":                           workspaceDelete,
	"workspace_mcp_server":               workspaceDelete,
	"workspace_knowledge":                workspaceDelete, // the WeKnora tenant is not removed here
	"knowledge_base":                     workspaceDelete,
	"knowledge_link":                     workspaceDelete,
	"knowledge_acl":                      workspaceDelete,
	"knowledge_owner_account":            workspaceDelete,
	"knowledge_tenant_deprovision":       workspaceDeleteSettle, // queued for WeKnora tenant removal after the hold (plan 10.14)
	"knowledge_kind":                     workspaceDelete,
	"knowledge_publish_policy":           workspaceDelete,
	"knowledge_eval_case":                workspaceDelete,
	"knowledge_eval_run":                 workspaceDelete,
	"knowledge_health_run":               workspaceDelete,
	"knowledge_repo_sync":                workspaceDelete,
	"knowledge_repo_file":                workspaceDelete,
	"knowledge_project_file":             workspaceDelete,
	"notification_preference":            workspaceDelete,
	"company_floor":                      workspaceDelete,
	"personal_access_token":              workspaceDelete, // workspace-bound PATs (migration 698) are revoked with the workspace, as account deletion revokes PATs
	"pinned_item":                        workspaceDelete,
	"plugin_installation":                workspaceDelete,
	"plugin_hook_schedule":               workspaceDelete,
	"plugin_invocation":                  workspaceDelete,
	"plugin_storage":                     workspaceDelete,
	"plugin_secret":                      workspaceDelete,
	"plugin_package":                     workspaceDelete,
	"plugin_package_version":             workspaceDelete,
	"plugin_package_file":                workspaceDelete,
	"project":                            workspaceDelete,
	"project_resource":                   workspaceDelete,
	"quick_action":                       workspaceDelete,
	"runtime_profile":                    workspaceDelete,
	"runtime_quickstart_request":         workspaceDelete,
	"schema_migrations":                  workspaceDeleteKeep,
	"search_index_change":                workspaceDelete,
	"search_index_prune_mark":            workspaceDeleteKeep,
	"seat_capacity_outbox":               workspaceDeleteSettle,
	"skill":                              workspaceDelete,
	"skill_file":                         workspaceDelete,
	"skill_to_label":                     workspaceDelete,
	"skill_version":                      workspaceDelete,
	"squad":                              workspaceDelete,
	"squad_attention":                    workspaceDelete,
	"squad_charter":                      workspaceDelete,
	"squad_member":                       workspaceDelete,
	"squad_pack_install":                 workspaceDelete,
	"squad_plugin":                       workspaceDelete,
	"squad_routine":                      workspaceDelete,
	"squad_skill":                        workspaceDelete,
	"squad_handoff_rule":                 workspaceDelete,
	"squad_handoff":                      workspaceDelete,
	"sys_cron_executions":                workspaceDeleteKeep,
	"task_message":                       workspaceDelete,
	"task_supplement":                    workspaceDelete,
	"task_supplement_capability":         workspaceDelete,
	"task_token":                         workspaceDelete,
	"sso_connection":                     workspaceDelete,
	"sso_domain":                         workspaceDelete,
	"task_usage":                         workspaceDelete,
	"task_usage_hourly":                  workspaceDelete,
	"task_usage_hourly_dirty":            workspaceDelete,
	"task_usage_hourly_rollup_state":     workspaceDeleteKeep,
	"user":                               workspaceDeleteKeep,
	"user_composio_connection":           workspaceDeleteKeep,
	"user_mfa":                           workspaceDeleteKeep, // user-level: no workspace_id; the account deletion job removes the row
	"vcs_commit_status":                  workspaceDelete,
	"vcs_connection":                     workspaceDelete,
	"vcs_pull_request":                   workspaceDelete,
	"verification_code":                  workspaceDeleteKeep,
	"webhook_delivery":                   workspaceDelete,
	"wecom_install_session":              workspaceDelete,
	"workspace":                          workspaceDelete,
	"workspace_invitation":               workspaceDelete,
	"workspace_share_link":               workspaceDelete,
	// Platform-management tables (gate G-R2 decisions).
	"audit_event":              workspaceDeleteDetach, // rows survive with a NULL workspace_id; the retention job removes them
	"billing_notice":           workspaceDelete,
	"billing_order":            workspaceDeleteSettle, // financial record kept for accounting; buyer details are an open item
	"billing_profile":          workspaceDelete,       // buyer identity goes with the workspace (spec 7b.1); keeping it for invoices is an owner decision, same open item as billing_order
	"plan":                     workspaceDeleteKeep,
	"plan_price_change":        workspaceDeleteKeep, // plan-level: plan_key, no workspace_id (spec 7b.4)
	"platform_account":         workspaceDeleteKeep,
	"platform_account_request": workspaceDeleteDetach, // the request row survives as the record; workspace teardown clears its workspace_id
	"platform_note":            workspaceDelete,
	"platform_seat_operation":  workspaceDelete,
	"platform_staff":           workspaceDeleteKeep,
	"platform_usage_daily":     workspaceDelete,
	"platform_workspace":       workspaceDelete,
	"usage_charge":             workspaceDelete, // per-workspace usage charges, rated from platform_usage_daily (spec 7b.3)
	"user_consent":             workspaceDeleteKeep,
	"workspace_policy":         workspaceDelete, // per-workspace configuration, like sso_connection (spec 6.4)
	"workspace_export":         workspaceDelete, // storage archives are an open item
	"workspace_subscription":   workspaceDelete,
}

// workspaceDeletionColumns classifies a workspace_id column on a table whose
// rows the manifest keeps (table.column keys).
var workspaceDeletionColumns = map[string]workspaceDeleteAction{
	"personal_access_token.workspace_id": workspaceDeleteKeep, // user-level token; a binding to a deleted workspace matches nothing, so the token is refused everywhere
}

func TestWorkspaceDeletionManifestCoversPublicSchema(t *testing.T) {
	if testPool == nil {
		t.Skip("database not available")
	}

	rows, err := testPool.Query(context.Background(), `
SELECT tablename
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY tablename
`)
	if err != nil {
		t.Fatalf("list public tables: %v", err)
	}
	defer rows.Close()

	actual := make(map[string]struct{}, len(workspaceDeletionManifest))
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatalf("scan public table: %v", err)
		}
		actual[table] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate public tables: %v", err)
	}

	var unclassified, missing []string
	for table := range actual {
		if _, ok := workspaceDeletionManifest[table]; !ok {
			unclassified = append(unclassified, table)
		}
	}
	for table := range workspaceDeletionManifest {
		if _, ok := actual[table]; !ok {
			missing = append(missing, table)
		}
	}
	sort.Strings(unclassified)
	sort.Strings(missing)
	if len(unclassified) > 0 || len(missing) > 0 {
		t.Fatalf("workspace deletion manifest drift: unclassified=%v missing=%v", unclassified, missing)
	}

	workspaceColumns, err := testPool.Query(context.Background(), `
SELECT table_name
FROM information_schema.columns
WHERE table_schema = 'public'
  AND column_name = 'workspace_id'
`)
	if err != nil {
		t.Fatalf("list workspace_id columns: %v", err)
	}
	defer workspaceColumns.Close()

	withWorkspaceID := make(map[string]struct{})
	for workspaceColumns.Next() {
		var table string
		if err := workspaceColumns.Scan(&table); err != nil {
			t.Fatalf("scan workspace_id table: %v", err)
		}
		withWorkspaceID[table] = struct{}{}
	}
	if err := workspaceColumns.Err(); err != nil {
		t.Fatalf("iterate workspace_id tables: %v", err)
	}

	for table, action := range workspaceDeletionManifest {
		_, hasWorkspaceID := withWorkspaceID[table]
		switch action {
		case workspaceDeleteKeep:
			if _, classified := workspaceDeletionColumns[table+".workspace_id"]; hasWorkspaceID && !classified {
				t.Errorf("KEEP table %s gained workspace_id; classify its teardown behavior", table)
			}
		case workspaceDeleteDetach, workspaceDeleteSettle:
			if !hasWorkspaceID {
				t.Errorf("%s table %s lost workspace_id; update its teardown selector", action, table)
			}
		}
	}
}

// TestWorkspaceTeardownAppliesManifestDecisions runs the workspace teardown over
// one row of each class and checks the result: Delete rows are gone, audit rows
// keep existing with a NULL workspace_id, and the billing order is kept.
func TestWorkspaceTeardownAppliesManifestDecisions(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	wsID := dbfx.Workspace(t, "manifest teardown", "wsman-"+uuid.NewString()[:8])
	dbfx.Member(t, wsID, testUserID, "owner")

	dbfx.Exec(t, `INSERT INTO billing_notice (workspace_id, notice_key, kind) VALUES ($1::uuid, 'manifest', 'test')`, wsID)
	dbfx.Exec(t, `INSERT INTO platform_note (id, workspace_id, author_id, body) VALUES (gen_random_uuid(), $1::uuid, $2::uuid, 'manifest')`, wsID, testUserID)
	dbfx.Exec(t, `INSERT INTO platform_seat_operation (workspace_id, operation_token, kind, state) VALUES ($1::uuid, gen_random_uuid(), 'test', 'done')`, wsID)
	dbfx.Exec(t, `INSERT INTO platform_usage_daily (workspace_id, day) VALUES ($1::uuid, '2026-11-15')`, wsID)
	dbfx.Exec(t, `INSERT INTO platform_workspace (workspace_id, status, reason) VALUES ($1::uuid, 'active', '')`, wsID)
	dbfx.Exec(t, `INSERT INTO workspace_export (id, workspace_id, requested_by, status) VALUES (gen_random_uuid(), $1::uuid, $2::uuid, 'queued')`, wsID, testUserID)
	dbfx.Exec(t, `INSERT INTO workspace_subscription (workspace_id, plan_key, status) VALUES ($1::uuid, 'free', 'active')`, wsID)
	dbfx.Exec(t, `INSERT INTO billing_profile (workspace_id) VALUES ($1::uuid)`, wsID)
	dbfx.Exec(t, `INSERT INTO workspace_policy (workspace_id) VALUES ($1::uuid)`, wsID)
	dbfx.Exec(t, `INSERT INTO billing_order (id, workspace_id, kind, plan_key, "interval", amount, currency, status, provider, transfer_code, expires_at)
		VALUES (gen_random_uuid(), $1::uuid, 'subscription', 'free', 'month', 0, 'VND', 'pending', 'bank_transfer', 'manifest-' || $1, now() + interval '1 day')`, wsID)

	var auditID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO audit_event (workspace_id, actor_type, action) VALUES ($1::uuid, 'system', 'manifest.test') RETURNING id::text`,
		wsID).Scan(&auditID); err != nil {
		t.Fatalf("insert audit row: %v", err)
	}
	dbfx.Cleanup(t, `DELETE FROM audit_event WHERE id = $1::uuid`, auditID)
	dbfx.Cleanup(t, `DELETE FROM billing_order WHERE workspace_id = $1::uuid`, wsID)

	tx, err := testHandler.TxStarter.Begin(ctx)
	if err != nil {
		t.Fatalf("begin teardown: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := testHandler.deleteWorkspaceData(ctx, tx, parseUUID(wsID)); err != nil {
		t.Fatalf("teardown: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit teardown: %v", err)
	}

	for _, table := range []string{"billing_notice", "billing_profile", "platform_note", "platform_seat_operation", "platform_usage_daily", "platform_workspace", "workspace_export", "workspace_policy", "workspace_subscription"} {
		if n := dbfx.Count(t, `SELECT count(*) FROM `+table+` WHERE workspace_id = $1::uuid`, wsID); n != 0 {
			t.Errorf("%s: %d rows left for the deleted workspace; want 0", table, n)
		}
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM audit_event WHERE id = $1::uuid AND workspace_id IS NULL`, auditID); n != 1 {
		t.Errorf("audit_event row %s: want it kept with a NULL workspace_id", auditID)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM billing_order WHERE workspace_id = $1::uuid`, wsID); n != 1 {
		t.Errorf("billing_order: %d rows for the deleted workspace; want 1 (kept for accounting)", n)
	}
}
