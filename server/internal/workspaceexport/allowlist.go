package workspaceexport

// Export tables, in archive order. The archive file of a table is
// "<table>.ndjson".
const (
	TableWorkspace   = "workspace"
	TableMembers     = "members"
	TableProjects    = "projects"
	TableIssues      = "issues"
	TableComments    = "comments"
	TableLabels      = "labels"
	TableStatuses    = "statuses"
	TableAttachments = "attachments"
)

// Tables lists the exported tables in archive order.
var Tables = []string{
	TableWorkspace, TableMembers, TableProjects, TableIssues,
	TableComments, TableLabels, TableStatuses, TableAttachments,
}

// allowed lists, for each table, the only columns the export may write. A
// column that is not here is dropped from a row even if a source supplies it.
// The workspace's settings and repos, an issue's metadata and flow state, and
// anything holding agent environment variables, MCP configuration or tokens are
// not on a list. An attachment row has no url: it is metadata only.
var allowed = map[string][]string{
	TableWorkspace: {"id", "name", "slug", "description", "context", "issue_prefix", "avatar_url", "created_at", "updated_at"},
	TableMembers:   {"id", "workspace_id", "user_id", "name", "email", "role", "created_at"},
	TableProjects: {
		"id", "workspace_id", "title", "description", "icon", "status", "priority", "lead_type", "lead_id",
		"start_date", "due_date", "creator_type", "creator_id", "created_at", "updated_at",
	},
	TableIssues: {
		"id", "workspace_id", "number", "title", "description", "status", "priority",
		"assignee_type", "assignee_id", "creator_type", "creator_id", "parent_issue_id", "project_id",
		"start_date", "due_date", "done_at", "acceptance_criteria", "properties", "created_at", "updated_at",
	},
	TableComments: {
		"id", "workspace_id", "issue_id", "parent_id", "author_type", "author_id", "content", "type",
		"resolved_at", "created_at", "updated_at",
	},
	TableLabels: {"id", "workspace_id", "name", "color", "description", "resource_type", "created_at", "updated_at"},
	TableStatuses: {
		"id", "workspace_id", "key", "name", "description", "category", "color", "icon", "is_system",
		"position", "archived_at", "created_at", "updated_at",
	},
	TableAttachments: {"id", "workspace_id", "issue_id", "comment_id", "name", "content_type", "size_bytes", "created_at"},
}

// AllowedColumns returns a copy of the columns the export may write for table,
// or nil for a table that is not exported.
func AllowedColumns(table string) []string {
	cols, ok := allowed[table]
	if !ok {
		return nil
	}
	return append([]string(nil), cols...)
}

// workspaceKey is the column that ties a row to its workspace: the workspace's
// own id, and workspace_id everywhere else.
func workspaceKey(table string) string {
	if table == TableWorkspace {
		return "id"
	}
	return "workspace_id"
}
