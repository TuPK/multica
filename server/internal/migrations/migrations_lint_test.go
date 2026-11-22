package migrations

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// legacyDuplicateMigrationStems are migrations 595–616, and 713–722 (fork and
// upstream numbered them independently), that already shipped with
// a shared prefix. They are applied on live databases keyed by their full name,
// so they cannot be renamed. Any new duplicate is still an error.
var legacyDuplicateMigrationStems = map[string]bool{
	"713_doc_collab":                              true,
	"713_proposal_revision_workspace_index":       true,
	"714_brief_inquiry":                           true,
	"714_doc_collab_state_pkey_index":             true,
	"715_brief_inquiry_pkey_index":                true,
	"715_doc_collab_state_pkey":                   true,
	"716_brief_inquiry_primary_key":               true,
	"716_doc_revision_pkey_index":                 true,
	"717_brief_inquiry_workspace_day_index":       true,
	"717_doc_revision_pkey":                       true,
	"718_auto_archive_prompt":                     true,
	"718_squad_attention":                         true,
	"719_remote_app_connection":                   true,
	"719_squad_attention_pkey_index":              true,
	"720_remote_app_oauth_client_idx":             true,
	"720_squad_attention_primary_key":             true,
	"721_brief_push_preference":                   true,
	"721_remote_app_connection_owner_idx":         true,
	"722_auto_archive_agent_trigger_prompt":       true,
	"722_brief_push_preference_pkey_index":        true,
	"600_agent_office_room":                       true,
	"600_wecom_install_session":                   true,
	"601_office_floor":                            true,
	"601_wecom_install_session_due_idx":           true,
	"602_office_floor_pkey_uidx":                  true,
	"602_wecom_install_session_request_idx":       true,
	"603_office_floor_primary_key":                true,
	"603_wecom_install_session_agent_pending_idx": true,
	"604_office_floor_workspace_position_idx":     true,
	"604_wecom_install_session_begin_rate_idx":    true,
	"605_agent_runtime_max_concurrent_tasks":      true,
	"605_squad_office_seat":                       true,
	"606_agent_task_queue_runtime_active_index":   true,
	"606_squad_office_seat_uidx":                  true,
	"607_agent_home_squad":                        true,
	"607_inbox_item_section":                      true,
	"608_drop_agent_office_room":                  true,
	"608_task_usage_credits":                      true,
	"610_proposal":                                true,
	"610_runtime_profile_add_zcode":               true,
	"611_proposal_pkey_index":                     true,
	"611_user_whats_new_seen_version":             true,
	"612_proposal_primary_key":                    true,
	"612_skill_version":                           true,
	"613_proposal_workspace_state_index":          true,
	"613_skill_version_index":                     true,
	"614_project_creator":                         true,
	"614_proposal_squad_state_index":              true,
	"615_attachment_image_dimensions":             true,
	"615_proposal_issue_uidx":                     true,
	"616_issue_origin_proposal":                   true,
	"616_member_pinned":                           true,

	"595_issue_queue_order":                      true,
	"595_issue_view_preference_sidebar":          true,
	"596_issue_queue_order_index":                true,
	"596_issue_view_preference_sidebar_projects": true,
	"597_agent_task_branch_merge":                true,
	"597_issue_unread":                           true,
	"598_issue_unread_unique_index":              true,
	"598_runtime_quickstart_request":             true,
	"599_issue_unread_primary_key":               true,
	"599_runtime_quickstart_request_index":       true,

	// Collisions between the fork's migrations and upstream's, both numbered in
	// the same range. A migration version is its whole name, so neither renames.
	"617_connected_apps":                        true,
	"617_issue_origin_proposal_validate":        true,
	"620_agent_app_agent_toolkit_idx":           true,
	"620_knowledge_tables":                      true,
	"621_agent_app_workspace_toolkit_idx":       true,
	"621_workspace_knowledge_pkey_index":        true,
	"622_app_action_rule_scope_idx":             true,
	"622_knowledge_base_pkey_index":             true,
	"623_app_action_request_issue_idx":          true,
	"623_knowledge_link_pkey_index":             true,
	"624_app_action_request_pending_idx":        true,
	"624_knowledge_primary_keys":                true,
	"625_app_tool_call_workspace_idx":           true,
	"625_knowledge_base_workspace_key_index":    true,
	"626_app_action_request_task_idx":           true,
	"626_knowledge_link_idempotency_index":      true,
	"627_app_tool_call_issue_idx":               true,
	"627_knowledge_link_base_state_index":       true,
	"628_app_action_request_chat_idx":           true,
	"628_knowledge_link_source_index":           true,
	"630_project_visibility":                    true,
	"630_workspace_secretary_agent":             true,
	"631_brief":                                 true,
	"631_project_member":                        true,
	"632_brief_pkey_index":                      true,
	"632_project_member_user_idx":               true,
	"633_brief_primary_key":                     true,
	"633_issue_environment":                     true,
	"634_brief_workspace_created_index":         true,
	"634_issue_environment_task_key_idx":        true,
	"650_knowledge_link_write_path":             true,
	"650_task_context_receipt_workspace_idx":    true,
	"651_platform_account":                      true,
	"651_task_context_receipt_issue_idx":        true,
	"652_platform_account_user_idx":             true,
	"652_runtime_profile_add_droid":             true,
	"653_chat_submission_receipt":               true,
	"653_platform_account_status_idx":           true,
	"654_audit_event":                           true,
	"654_chat_submission_receipt_key":           true,
	"655_audit_event_id_idx":                    true,
	"655_chat_submission_receipt_workspace":     true,
	"656_audit_event_workspace_created_idx":     true,
	"656_runtime_profile_add_amp":               true,
	"657_audit_event_actor_created_idx":         true,
	"657_inbox_item_snoozed_until":              true,
	"658_project_archive":                       true,
	"658_user_consent":                          true,
	"659_issue_view_preference_projects_page":   true,
	"659_user_consent_uidx":                     true,
	"660_audit_event_created_idx":               true,
	"660_inbox_item_snoozed_at":                 true,
	"663_chat_task_supplement_task":             true,
	"663_platform_staff":                        true,
	"664_chat_task_supplement_workspace":        true,
	"664_platform_staff_user_idx":               true,
	"665_platform_workspace":                    true,
	"665_weixin_state":                          true,
	"666_platform_workspace_uidx":               true,
	"666_weixin_login_expiry_idx":               true,
	"667_issue_origin_weixin_chat":              true,
	"667_platform_workspace_status_idx":         true,
	"668_issue_origin_weixin_chat_validate":     true,
	"668_platform_note":                         true,
	"669_platform_note_workspace_idx":           true,
	"669_weixin_login_session_unique_index":     true,
	"670_plan":                                  true,
	"670_weixin_login_session_primary_key":      true,
	"671_plan_key_uidx":                         true,
	"671_weixin_account_state_unique_index":     true,
	"672_weixin_account_state_primary_key":      true,
	"672_workspace_subscription":                true,
	"673_runtime_rate_limit":                    true,
	"673_workspace_subscription_uidx":           true,
	"674_platform_seat_operation":               true,
	"674_runtime_profile_daemon":                true,
	"675_chat_wakeup":                           true,
	"675_platform_seat_operation_uidx":          true,
	"676_chat_wakeup_due_idx":                   true,
	"676_platform_seat_operation_state_idx":     true,
	"677_billing_order":                         true,
	"677_chat_wakeup_session_idx":               true,
	"678_billing_order_id_uidx":                 true,
	"678_chat_wakeup_requester_idx":             true,
	"679_billing_order_idem_uidx":               true,
	"679_entitlement_limit_overage":             true,
	"680_billing_order_transfer_uidx":           true,
	"680_entitlement_limit_overage_scope_idx":   true,
	"681_billing_order_status_idx":              true,
	"681_project_folder":                        true,
	"682_billing_order_workspace_idx":           true,
	"682_project_folder_name_idx":               true,
	"683_billing_notice":                        true,
	"683_entitlement_feature_attempt":           true,
	"684_billing_notice_key_uidx":               true,
	"684_entitlement_feature_attempt_scope_idx": true,
	"685_docs":                                   true,
	"685_workspace_export":                       true,
	"686_doc_folder_pkey_index":                  true,
	"686_workspace_subscription_status_idx":      true,
	"687_doc_folder_pkey":                        true,
	"687_workspace_export_running_uidx":          true,
	"688_doc_pkey_index":                         true,
	"688_platform_note_id_uidx":                  true,
	"689_doc_pkey":                               true,
	"689_platform_usage_daily":                   true,
	"690_doc_version_pkey_index":                 true,
	"690_platform_usage_daily_uidx":              true,
	"691_doc_version_pkey":                       true,
	"691_workspace_policy":                       true,
	"692_doc_folder_workspace_idx":               true,
	"692_workspace_policy_uidx":                  true,
	"693_doc_workspace_idx":                      true,
	"693_sso_domain":                             true,
	"694_doc_version_workspace_idx":              true,
	"694_sso_domain_verified_domain_uidx":        true,
	"695_inbox_item_returned_at":                 true,
	"695_sso_domain_workspace_domain_uidx":       true,
	"696_inbox_item_active_shown_idx":            true,
	"696_sso_connection":                         true,
	"697_autopilot_run_idempotency_key":          true,
	"697_sso_connection_workspace_uidx":          true,
	"698_autopilot_run_idempotency_key_idx":      true,
	"698_personal_access_token_restrictions":     true,
	"699_member_guest_role":                      true,
	"699_workspace_viewer_role":                  true,
	"700_member_search_access":                   true,
	"700_squad_charter_and_routine":              true,
	"701_member_search_access_pkey_index":        true,
	"701_squad_charter_pkey_index":               true,
	"702_member_search_access_pkey":              true,
	"702_squad_routine_pkey_index":               true,
	"703_doc_comment":                            true,
	"703_squad_charter_and_routine_primary_keys": true,
	"704_doc_comment_pkey_index":                 true,
	"704_squad_routine_squad_index":              true,
	"707_doc_comment_workspace_idx":              true,
	"707_proposal_lifecycle":                     true,
	"708_attachment_workspace_size_idx":          true,
	"708_proposal_expiry_index":                  true,
	"709_drop_attachment_workspace_index":        true,
	"709_proposal_revision":                      true,
	"710_autopilot_quota_limit_notice":           true,
	"710_proposal_revision_pkey_index":           true,
	"711_dashboard_range_view":                   true,
	"711_proposal_revision_primary_key":          true,
	"712_dashboard_range_view_scope_idx":         true,
	"712_proposal_revision_number_index":         true,
	"998_issue_origin_union":                     true,
	"998_member_role_viewer_guest":               true,
	"998_project_access_to_visibility":           true,
	"998_user_mfa":                               true,
}

const (
	forkSyncDuplicateFirst = 713
	forkSyncDuplicateLast  = 899
)

func TestMigrationNumericPrefixesAreUnique(t *testing.T) {
	files := migrationFilesForLint(t, "*.up.sql")

	// Migrations through 128 contain historical duplicate numeric prefixes.
	// From 129 onward, keep the numeric sequence unique so release tooling and
	// operators can identify one schema change unambiguously by its number.
	const firstUniqueMigrationNumber = 129
	stemByNumber := make(map[int]string)
	for _, file := range files {
		stem, _, ok := splitMigrationFilename(filepath.Base(file))
		if !ok {
			continue
		}
		prefix, _, ok := strings.Cut(stem, "_")
		if !ok {
			continue
		}
		number, err := strconv.Atoi(prefix)
		if err != nil || number < firstUniqueMigrationNumber {
			continue
		}
		if previous, exists := stemByNumber[number]; exists {
			if legacyDuplicateMigrationStems[previous] && legacyDuplicateMigrationStems[stem] {
				continue
			}
			// The fork and upstream number their migrations independently in this
			// range; the runner keys applied migrations by full name.
			if number >= forkSyncDuplicateFirst && number <= forkSyncDuplicateLast {
				continue
			}
			t.Errorf("migrations %s and %s share numeric prefix %s", previous, stem, prefix)
			continue
		}
		stemByNumber[number] = stem
	}
}

func TestMigrationFilesHaveMatchingDirections(t *testing.T) {
	files := migrationFilesForLint(t, "*.sql")

	directionsByStem := make(map[string]map[string]bool)
	for _, file := range files {
		stem, direction, ok := splitMigrationFilename(filepath.Base(file))
		if !ok {
			continue
		}
		if directionsByStem[stem] == nil {
			directionsByStem[stem] = make(map[string]bool)
		}
		directionsByStem[stem][direction] = true
	}

	for stem, directions := range directionsByStem {
		if !directions["up"] || !directions["down"] {
			t.Errorf("migration %s must have both .up.sql and .down.sql files", stem)
		}
	}
}

func migrationFilesForLint(t *testing.T, pattern string) []string {
	t.Helper()

	dir := realMigrationsDir(t)
	files, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no migration files matched %s in %s", pattern, dir)
	}
	sort.Strings(files)
	return files
}

func realMigrationsDir(t *testing.T) string {
	t.Helper()

	// Relative to the package directory go test runs in; runtime.Caller
	// would return a module path under -trimpath.
	dir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	return dir
}

func splitMigrationFilename(name string) (stem, direction string, ok bool) {
	for _, candidateDirection := range []string{"up", "down"} {
		suffix := fmt.Sprintf(".%s.sql", candidateDirection)
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix), candidateDirection, true
		}
	}
	return "", "", false
}
