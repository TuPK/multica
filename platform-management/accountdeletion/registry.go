// Package accountdeletion lists every column that can hold a user id and says
// what account deletion does with it. The server test that reads the
// migrations fails when a new such column is missing here, so a new table
// cannot be added without deciding how account deletion treats it.
package accountdeletion

// Action is what account deletion does with the rows that name the user.
type Action string

const (
	// ActionDelete: no row naming the user remains once the job has run.
	ActionDelete Action = "delete"
	// ActionAnonymize: the row stays and the personal value is overwritten.
	ActionAnonymize Action = "anonymize"
	// ActionKeep: rows remain and point at the anonymised user row. Reason says why.
	ActionKeep Action = "keep"
	// ActionNotAUser: the column matches the naming rule but does not hold a user id.
	ActionNotAUser Action = "not_a_user"
)

// Column is one registry entry. Reason is required for every action except delete.
type Column struct {
	Table  string
	Column string
	Action Action
	Reason string
}

const refReason = "authorship or ownership reference in a workspace that continues; the user row is anonymised, so the id resolves to a deleted user"

// Columns is the registry, sorted by table then column. A "delete" entry means
// that after the job no row names the user in that column; the database test
// of the job checks it.
var Columns = []Column{
	{"agent", "owner_id", ActionKeep, refReason},
	{"agent_invocation_target", "created_by", ActionKeep, refReason},
	{"agent_runtime", "owner_id", ActionKeep, "member removal makes the runtime unusable and archives its agents; the row stays as history"},
	{"agent_task_queue", "accountable_user_id", ActionKeep, refReason},
	{"agent_task_queue", "initiator_user_id", ActionKeep, refReason},
	{"agent_task_queue", "originator_user_id", ActionKeep, refReason},
	{"approval_policy", "created_by", ActionKeep, refReason},
	{"autopilot_collaborator", "user_id", ActionDelete, "job removes the rows"},
	{"autopilot_subscriber", "user_id", ActionDelete, "removed with the membership, or with the workspace"},
	{"billing_order", "created_by", ActionKeep, "a financial record that must outlive the user; it is the invoice basis"},
	{"budget", "created_by", ActionKeep, refReason},
	{"channel_chat_context_generation", "initiator_user_id", ActionKeep, refReason},
	{"channel_installation", "installer_user_id", ActionKeep, refReason},
	{"channel_user_binding", "multica_user_id", ActionDelete, "job removes the rows"},
	{"chat_pinned_agent", "user_id", ActionDelete, "job removes the rows"},
	{"chat_session", "creator_id", ActionKeep, "chats in a workspace that continues stay; removing them needs a chat teardown path this slice does not add (known gap)"},
	{"client_usage_daily", "user_id", ActionDelete, "job removes the rows"},
	{"cloud_agent_connection", "owner_id", ActionKeep, "a workspace-level connection that stays in a workspace that continues"},
	{"daemon_token", "owner_id", ActionDelete, "job removes the rows"},
	{"feedback", "user_id", ActionDelete, "job removes the rows"},
	{"floor_chief", "created_by", ActionKeep, refReason},
	{"goal", "owner_id", ActionKeep, refReason},
	{"hub_item_version_blob", "created_by", ActionKeep, refReason},
	{"hub_source", "created_by", ActionKeep, refReason},
	{"initiative", "sponsor_user_id", ActionKeep, refReason},
	{"issue", "creator_id", ActionKeep, refReason},
	{"issue_source_context", "captured_by_user_id", ActionKeep, refReason},
	{"issue_subscriber", "user_id", ActionDelete, "removed with the membership, or with the workspace"},
	{"issue_unread", "user_id", ActionDelete, "job removes the rows"},
	{"issue_view", "owner_id", ActionKeep, "private views are deleted on member removal; shared views stay as workspace content"},
	{"issue_view_preference", "user_id", ActionDelete, "removed with the membership, or with the workspace"},
	{"issue_wakeup", "created_by", ActionKeep, refReason},
	{"issue_workflow", "owner_id", ActionKeep, refReason},
	{"issue_workflow_draft", "created_by", ActionKeep, refReason},
	{"knowledge_owner_account", "user_id", ActionDelete, "job removes the rows"},
	{"lark_installation", "installer_user_id", ActionKeep, refReason},
	{"lark_user_binding", "multica_user_id", ActionDelete, "job removes the rows"},
	{"member", "user_id", ActionDelete, "removed with the membership, or with the workspace"},
	{"member_delegation_day", "user_id", ActionDelete, "job removes the rows"},
	{"member_group", "created_by", ActionKeep, "the group creator is kept as workspace history; the group is workspace content"},
	{"member_group_member", "added_by", ActionKeep, "the adder is kept as workspace history on the membership row"},
	{"member_group_member", "user_id", ActionDelete, "removed with the membership, or with the workspace"},
	{"member_removal", "user_id", ActionKeep, "the record of a removal from a workspace that continues; the user row is anonymised, so the id resolves to a deleted user"},
	{"metric_webhook_token", "created_by", ActionKeep, refReason},
	{"notification_preference", "user_id", ActionDelete, "job removes the rows"},
	{"personal_access_token", "user_id", ActionDelete, "job removes the rows, which revokes the tokens"},
	{"pinned_item", "user_id", ActionDelete, "job removes the rows"},
	{"plan_price_change", "created_by", ActionKeep, "the staff record of a plan price change; it stays as platform history"},
	{"platform_account", "user_id", ActionKeep, "the account state row stays and records status deleted"},
	{"platform_account_request", "created_by", ActionKeep, "the staff who asked for the account; the request row stays as the record of who asked"},
	{"platform_account_request", "user_id", ActionKeep, "the request row stays and points at the anonymised user row; the deletion job overwrites its email and display name"},
	{"platform_member_request", "target_user_id", ActionKeep, "the member a remove or change_role request names; the request row stays and its email is anonymised with the account"},
	{"platform_member_request", "user_id", ActionKeep, "the request row stays and points at the anonymised user row; the deletion job overwrites its email"},
	{"platform_staff", "created_by", ActionKeep, "the staff row records who added it; it stays as history"},
	{"platform_staff", "user_id", ActionKeep, "the row stays as history; the account guard refuses a deleted account, so the row grants nothing"},
	{"plugin_binding", "created_by", ActionKeep, refReason},
	{"plugin_installation_config", "created_by", ActionKeep, refReason},
	{"plugin_package", "created_by", ActionKeep, refReason},
	{"plugin_remote_mcp_secret", "created_by", ActionKeep, refReason},
	{"project", "creator_id", ActionKeep, refReason},
	{"project_member", "added_by", ActionKeep, refReason},
	{"project_member", "user_id", ActionDelete, "job removes the rows"},
	{"project_resource", "created_by", ActionKeep, refReason},
	{"runtime_profile", "created_by", ActionKeep, refReason},
	{"runtime_quickstart_request", "initiator_user_id", ActionKeep, refReason},
	{"seat_capacity_outbox", "user_id", ActionKeep, "billing outbox row; it must outlive the user to settle the seat"},
	{"skill", "created_by", ActionKeep, refReason},
	{"skill_version", "created_by", ActionKeep, refReason},
	{"squad", "creator_id", ActionKeep, refReason},
	{"squad_plugin", "created_by", ActionKeep, refReason},
	{"squad_skill", "created_by", ActionKeep, refReason},
	{"sso_connection", "created_by", ActionKeep, refReason},
	{"task_token", "user_id", ActionDelete, "job removes the rows"},
	{"tenant", "created_by", ActionKeep, "the user who created the tenant; the tenant stays as the container of its workspaces, and the id resolves to the anonymised user"},
	{"user_composio_connection", "user_id", ActionDelete, "job removes the rows"},
	{"user_consent", "user_id", ActionDelete, "job removes the rows"},
	{"user_mfa", "user_id", ActionDelete, "job removes the row, with the secret and recovery codes"},
	{"wecom_install_session", "initiator_user_id", ActionKeep, refReason},
	{"workspace_invitation", "invitee_user_id", ActionDelete, "job removes the rows"},
	{"workspace_invitation", "inviter_id", ActionKeep, "an invitation record in a workspace that continues"},
	{"workspace_mcp_server", "created_by", ActionKeep, refReason},
	{"workspace_share_link", "created_by", ActionKeep, refReason},
}

// DeletedColumns returns the entries whose action is delete.
func DeletedColumns() []Column {
	var out []Column
	for _, c := range Columns {
		if c.Action == ActionDelete {
			out = append(out, c)
		}
	}
	return out
}
