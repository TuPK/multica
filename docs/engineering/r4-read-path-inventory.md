# R4 read-path inventory (P41.1)

Every place that reads the `issue` or `project` table, or a table derived from them,
and the state of its visibility check. Spec: `platform-management-spec.md` section 7.1.
The test `server/internal/projectaccess/inventory_test.go` reads this file.

Status values:

- `todo`: not yet filtered by the shared visibility predicate. The only status in use at P41.1.
- `scoped`: goes through the `projectaccess` predicate.
- `exempt`: needs no filter. The Note column must give the reason (for example owner-only).

How the test reads it: it scans `server/pkg/db/queries/*.sql` and the non-test Go files
under `server/internal` (no database), builds one unit per (file, function), and fails
when a unit has no row below, when a Status is not one of the three values, or when an
`exempt` row has an empty Note. Rows of Kind `manual` are readers the scanner cannot
see; the test checks only their Status.

To add a read path: add a row in the same commit that adds the path.

## Scanned readers

Count: 423 scanned units: 150 sql-query, 247 go-call, 20 raw-sql, 6 both go-call and raw-sql (counted once; listed in its own table).
Produced by running the test against an empty table and counting the failure lines:
`cd server && go test -count=1 ./internal/projectaccess 2>&1 | grep -c 'not listed'`.

How units are found:

- SQL: a query in `server/pkg/db/queries/*.sql` whose body matches `(from|join) (issue|project)` as whole words.
- Go call: a function under `server/internal` (non-test files) that calls a method named like one of those queries. The match is by name only, so a same-named method on another type is also listed.
- Raw SQL: a function with a string literal that matches the same pattern.

### SQL queries

| File | Function | Kind | Status | Note |
| --- | --- | --- | --- | --- |
| `server/pkg/db/queries/agent.sql` | `CreateAgentTask` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/agent.sql` | `CreateDeferredChannelIssueTask` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/agent.sql` | `ListActiveTasksByIssueFamily` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/agent.sql` | `ListPendingDelegatedFailureRecoveries` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/agent.sql` | `ListWorkspaceWorkingAgents` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/approval_policy.sql` | `ListPolicyAutoApprovals` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/approval_policy.sql` | `ListPolicyReversals` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/approval_policy.sql` | `ListRecentPolicyApprovals` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/brief_delivery.sql` | `ListBriefPendingDecisions` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/budget.sql` | `GetBudgetTaskScope` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/budget.sql` | `ListBudgetFinishedTaskCosts` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/budget.sql` | `ListBudgetInflightTasks` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/budget.sql` | `ListBudgetTaskCostsSince` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/budget.sql` | `LockBudgetTaskForAccounting` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/comment.sql` | `LockCommentForDelete` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/comment.sql` | `LockLiveComment` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/comment.sql` | `UpdateComment` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/digest.sql` | `DigestCharterActivity` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/digest.sql` | `DigestPendingProposals` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/digest.sql` | `DigestRecentDoneIssues` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/digest.sql` | `DigestSquadStats` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/directive.sql` | `CountDirectiveIssuesForMessage` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/directive.sql` | `ListBlockedDirectivesPastDue` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/flow.sql` | `CountOpenFlowRequestsByWorkflow` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/flow.sql` | `IsFlowRequestAuto` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/flow.sql` | `ListAutoFlowRequests` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/flow.sql` | `ListFlowInsightRequests` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/flow.sql` | `ListFlowManualCloses` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/flow.sql` | `ListFlowTasks` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/flow.sql` | `ListOpenFlowRequestIDsByWorkflow` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/flow.sql` | `ListOpenFlowRequests` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/flow.sql` | `ListUnstartedFlowRequests` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/flow.sql` | `LockIssueForFlow` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/inbox.sql` | `ArchiveCompletedInbox` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/inbox.sql` | `ListArchivedInboxItems` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/inbox.sql` | `ListInboxItems` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/inbox_archive.sql` | `ArchivedInboxFacets` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/inbox_archive.sql` | `ListArchivedInboxPage` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/initiative.sql` | `ListGateProposalsForInitiative` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/initiative.sql` | `MinIssuePositionInStatus` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `ChildIssueProgress` | sql-query | scoped | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `CountCreatedIssueAssignees` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `CountIssues` | sql-query | scoped | SQL carries the marker; no Go caller today (ListIssues builds its own SQL) |
| `server/pkg/db/queries/issue.sql` | `CountIssuesInTriage` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `CountIssuesUpTo` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `CountUnfinishedIssues` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `DeleteIssue` | sql-query | exempt | (a) delete by id from deleteIssuesAndCollectAttachmentURLs only, after DeleteIssue (loadIssueForUser) or BatchDeleteIssues (Scope.Allows) |
| `server/pkg/db/queries/issue.sql` | `FindActiveDuplicateIssue` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `FindRecentAutopilotDuplicateIssue` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `GetIssue` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `GetIssueByNumber` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `GetIssueByOrigin` | sql-query | exempt | (b) internal worker, no viewer: TaskService quick-create completion and SquadHandoff service look up the issue a task produced |
| `server/pkg/db/queries/issue.sql` | `GetIssueGCStatus` | sql-query | exempt | (b) internal worker, no viewer: daemon GC check (daemon token, Handler.GetIssueGCCheck) |
| `server/pkg/db/queries/issue.sql` | `GetIssueInWorkspace` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `GetIssueMetadataInWorkspace` | sql-query | exempt | (a) reload after a conditional write; runs only after loadIssueForUser in SetIssueMetadataKey and DeleteIssueMetadataKey |
| `server/pkg/db/queries/issue.sql` | `GetIssueRefInWorkspace` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `GetIssueTriageState` | sql-query | exempt | (b) internal worker, no viewer: TaskService queue door checks the issue before it inserts a task |
| `server/pkg/db/queries/issue.sql` | `IssueHasDuplicates` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `ListChildIssues` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `ListChildrenByParents` | sql-query | scoped | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `ListIssueDuplicates` | sql-query | scoped | SQL carries the marker; test TestRestrictedIssueSingleReadsAnswerMissing |
| `server/pkg/db/queries/issue.sql` | `ListIssueGCStatuses` | sql-query | exempt | (b) internal worker, no viewer: daemon GC check (daemon token, Handler.BatchIssueGCCheck) |
| `server/pkg/db/queries/issue.sql` | `ListIssueRefsInWorkspace` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `ListIssues` | sql-query | scoped | SQL carries the marker; no Go caller today (Handler.ListIssues builds its own SQL with Fragment) |
| `server/pkg/db/queries/issue.sql` | `ListIssuesByIDsInWorkspace` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `ListOpenIssues` | sql-query | scoped | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue.sql` | `ListReleasableQueuedIssues` | sql-query | exempt | (b) internal worker, no viewer: run-queue promoter (scheduler job, queue listener, and the promote pass after a queue write) |
| `server/pkg/db/queries/issue.sql` | `ListRunQueueWaits` | sql-query | scoped | SQL carries the marker for both the waiting issue and its predecessor; test TestRestrictedIssueListsOmitHidden |
| `server/pkg/db/queries/issue.sql` | `LockIssueForAttachmentWrite` | sql-query | exempt | (a) lock only, called from attachmentOwnerLockAttempt for DeleteAttachment, which runs attachmentInViewerScope first; test TestRestrictedIssueAttachmentsAnswerMissing |
| `server/pkg/db/queries/issue.sql` | `LockIssueForChannelMediaBind` | sql-query | exempt | (b) internal worker, no viewer: channel engine BindMediaRefs binds media of an external channel message to the issue the channel session is attached to |
| `server/pkg/db/queries/issue.sql` | `LockIssueForDelete` | sql-query | exempt | (a) lock by id from deleteIssuesAndCollectAttachmentURLs only, after DeleteIssue (loadIssueForUser) or BatchDeleteIssues (Scope.Allows) |
| `server/pkg/db/queries/issue.sql` | `LockIssueForDescriptionUpdate` | sql-query | todo | Handler callers are checked: updateIssueAtomically runs after loadIssueForUser or Scope.Allows. Open: the source-context callers (service IssueService.Create, TaskService.EnqueueQuickCreateTask) take the issue from a capture built in handler/source_context.go, outside the issue write group |
| `server/pkg/db/queries/issue.sql` | `LockIssuesForDuplicateMark` | sql-query | exempt | (a) lock by id from lockAndCheckDuplicateMark only, inside UpdateIssue after loadIssueForUser and the Scope.Allows check of the target |
| `server/pkg/db/queries/issue.sql` | `UpdateIssue` | sql-query | todo | Issue handlers call it after loadIssueForUser or Scope.Allows (UpdateIssue, BatchUpdateIssues, promoteQueuedIssue is a worker). Open: viewer-triggered service callers outside the issue write group (proposal.go, initiative.go, flow engine through UpdateIssueStatus) are classified with their own rows |
| `server/pkg/db/queries/issue.sql` | `UpdateIssueStatus` | sql-query | todo | Open: viewer-triggered callers outside the issue write group (service/proposal.go, service/initiative.go, handler/flow_engine*.go); workers (runtime_sweeper, proposal_expiry, qoder lifecycle, TaskService) are internal and need no viewer |
| `server/pkg/db/queries/issue_label.sql` | `AttachLabelToIssue` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue_label.sql` | `DetachLabelFromIssue` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue_property.sql` | `CountIssuesUsingPropertyOptions` | sql-query | exempt | Owner and admin only (Scope mode all): UpdateProperty gates on requirePropertyAdmin; the guard must count every issue |
| `server/pkg/db/queries/issue_property.sql` | `ListIssueProperties` | sql-query | scoped | SQL carries the marker on the usage_count subquery; test TestRestrictedIssueListsOmitHidden |
| `server/pkg/db/queries/issue_status.sql` | `CountIssuesUsingStatusKey` | sql-query | exempt | Owner and admin only (Scope mode all): ArchiveIssueStatus gates on loadIssueStatusForAdmin; the guard must count every issue |
| `server/pkg/db/queries/issue_unread.sql` | `MarkIssueUnreadForSubscribers` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/issue_workflow.sql` | `CountProjectIssuesByStatus` | sql-query | scoped | Marker predicate on i.project_id; test TestRestrictedWorkflowQueriesApplyScope |
| `server/pkg/db/queries/issue_workflow.sql` | `CountSquadIssuesByStatus` | sql-query | todo | Not scoped: callers squad_workflow.go and squad_project.go (squad group) would not compile with scope params; UpdateIssueWorkflow also calls it |
| `server/pkg/db/queries/issue_workflow.sql` | `ListProjectsUsingDefaultIssueWorkflow` | sql-query | scoped | Marker predicate on project.id; test TestRestrictedWorkflowQueriesApplyScope |
| `server/pkg/db/queries/issue_workflow.sql` | `ListProjectsUsingIssueWorkflow` | sql-query | scoped | Marker predicate on project.id; test TestRestrictedWorkflowQueriesApplyScope |
| `server/pkg/db/queries/issue_workflow.sql` | `LockProjectForWorkflowChange` | sql-query | exempt | (a) lock by id after the project passed Scope.Allows: SetProjectWorkflow (loadProjectForWorkflowWrite). saveHandoffOnProjectCopy and AssignSquadProject apply Scope.Allows to the returned row before anything else; tests TestRestrictedWorkflowRoutesAnswerMissing, TestRestrictedAssignSquadProjectAnswersMissing |
| `server/pkg/db/queries/issue_workflow.sql` | `RemapProjectIssueStatus` | sql-query | scoped | Marker predicate on i.project_id; test TestRestrictedWorkflowRemapSkipsNothingVisibleToOwner |
| `server/pkg/db/queries/issue_workflow.sql` | `RemapSquadIssueStatus` | sql-query | todo | Not scoped: caller remapSquadIssues (squad_workflow.go, squad group) would not compile with scope params |
| `server/pkg/db/queries/knowledge_project_file.sql` | `ListProjectDocSources` | sql-query | exempt | (b) the daemon project-docs sync (ListDaemonProjectDocSources, daemon token, no viewer) |
| `server/pkg/db/queries/metric.sql` | `BuiltinIssueDeliveryStats` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/metric.sql` | `BuiltinIssuesReopened` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/org_change.sql` | `CountIssuesWithTitle` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/org_map.sql` | `ListOrgMapSquads` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/platform_usage_daily.sql` | `CountUsageIssuesCreated` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/platform_usage_daily.sql` | `CountUsageIssuesTotal` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/pr_automation.sql` | `MoveIssueFromPullRequests` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/project.sql` | `CountIssuesByProject` | sql-query | scoped | SQL carries the marker; test TestRestrictedProjectCountsOmitHiddenIssues |
| `server/pkg/db/queries/project.sql` | `DeleteProject` | sql-query | exempt | (a) a write by id that Handler.DeleteProject runs only after its Scope.Allows check; test TestRestrictedProjectRoutesAnswerMissing |
| `server/pkg/db/queries/project.sql` | `GetOpenSquadProject` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/project.sql` | `GetProjectInWorkspace` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/project.sql` | `GetProjectIssueStats` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/project.sql` | `ListProjects` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/project.sql` | `ListProjectsForSquad` | sql-query | scoped | SQL carries the marker; test TestRestrictedProjectCountsOmitHiddenIssues |
| `server/pkg/db/queries/project.sql` | `LockProjectForChatSessionCreate` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/project.sql` | `LockProjectForDelete` | sql-query | exempt | (a) a lock by id that Handler.DeleteProject takes only after its Scope.Allows check; test TestRestrictedProjectRoutesAnswerMissing |
| `server/pkg/db/queries/project_access.sql` | `CountProjectIssues` | sql-query | exempt | Owner and admin only; runs after the handler checked the Scope; counts for the audit row |
| `server/pkg/db/queries/proposal.sql` | `GetProposalWithIssue` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/proposal.sql` | `ListProposals` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/proposal.sql` | `ListRecentProposalsForPriorArt` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/qoder_issue_lifecycle.sql` | `LockQoderTaskIssue` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/request_usage.sql` | `GetRequestUsageSummary` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/scorecard.sql` | `ScorecardDoneIssues` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/search_index.sql` | `GetSearchIndexManifest` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/search_index.sql` | `ListSearchIndexIssuesByIDs` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/search_index.sql` | `ListSearchIndexIssuesPage` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/search_index.sql` | `ListSearchIndexProjects` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/search_index.sql` | `ListSearchIndexProjectsByIDs` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/secretary_request.sql` | `CountOpenSecretaryRequests` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/secretary_request.sql` | `ListSecretaryRequestsPastDeadline` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/squad.sql` | `ListSquadMemberStatusRows` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/squad_directory.sql` | `ListOverdueSquadRequests` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/squad_directory.sql` | `ListSquadDirectory` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/squad_handoff.sql` | `GetSquadOpenProject` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/subscriber.sql` | `AddDelegatedSubscriber` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/subscriber.sql` | `HasAncestorOptOut` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/subscriber.sql` | `ListSubscribedIssueIDs` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/subscriber.sql` | `UnsubscribeFromIssueSubtree` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/system_wakeup.sql` | `CountCustomizedSystemWakeups` | sql-query | scoped | SQL carries the marker; test TestSystemWakeupsOmitHiddenIssues |
| `server/pkg/db/queries/system_wakeup.sql` | `ListParentsWithoutSystemWakeup` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/task_usage.sql` | `ListDashboardAgentRunTime` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/task_usage.sql` | `ListDashboardFailuresByAgent` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/task_usage.sql` | `ListDashboardFailuresDaily` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/task_usage.sql` | `ListDashboardRunTimeDaily` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/wakeup.sql` | `CreateWakeupTask` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/wakeup.sql` | `ListPausedWakeupIssues` | sql-query | scoped | SQL carries the marker; test TestRestrictedIssueListsOmitHidden |
| `server/pkg/db/queries/wakeup.sql` | `ListWorkspaceWakeupSummaryRows` | sql-query | scoped | SQL carries the marker; test TestRestrictedIssueListsOmitHidden |
| `server/pkg/db/queries/wakeup.sql` | `LockWakeupIssue` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/workspace_delete.sql` | `DeleteWorkspaceIssueRoots` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/workspace_delete.sql` | `DeleteWorkspaceLeafData` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/workspace_delete.sql` | `DeleteWorkspaceRuntimesAndProjects` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/workspace_delete.sql` | `ListWorkspaceIssueIDFirstPage` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/workspace_delete.sql` | `ListWorkspaceIssueIDPage` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/workspace_delete.sql` | `LockWorkspaceTaskOwnerIssues` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/workspace_export.sql` | `ListIssuesForExport` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/workspace_export.sql` | `ListProjectsForExport` | sql-query | todo | Reads issue or project; no visibility filter today |
| `server/pkg/db/queries/workspace_wakeup.sql` | `ListWorkspaceWakeups` | sql-query | scoped | SQL carries the marker in both branches of the union (member rules and system rules); test TestRestrictedIssueListsOmitHidden |

### Go call sites

Grouped by file; a function appears once per file.

| File | Function | Kind | Status | Note |
| --- | --- | --- | --- | --- |
| `server/internal/daemon/project_docs.go` | `Daemon.runProjectDocsSync` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/activity.go` | `Handler.GetAssigneeFrequency` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/agent.go` | `Handler.ListWorkspaceWorkingAgents` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/pkg/db/queries/project_access.sql` | `LockProjectAccess` | sql-query | exempt | Owner and admin write; runs after the handler checked the Scope |
| `server/pkg/db/queries/project_access.sql` | `ListViewerProjectAccess` | sql-query | exempt | Source of the Scope itself; reads only the viewer own grants and restricted project ids |
| `server/pkg/db/queries/project_access.sql` | `GrantProjectMembersFromProject` | sql-query | exempt | Owner and admin write; reads the project and its issue assignees to grant them |
| `server/pkg/db/queries/project_access.sql` | `GetProjectAccess` | sql-query | scoped | Handler.load applies Scope.Allows to the row before use |
| `server/internal/handler/autopilot.go` | `Handler.parseAutopilotProjectID` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/channel_action.go` | `ChannelActions.apply` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/channel_command.go` | `ChannelActions.pendingDecisions` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/chat.go` | `Handler.CreateChatSession` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/chat.go` | `Handler.UpdateChatSession` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/comment.go` | `Handler.DeleteComment` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/comment.go` | `Handler.UpdateComment` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/comment.go` | `Handler.deleteComment` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/comment.go` | `Handler.withLiveCommentLock` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/comment_invocation_notice.go` | `Handler.createTriggerFailureComment` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/daemon.go` | `Handler.BatchIssueGCCheck` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/daemon.go` | `Handler.GetIssueGCCheck` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/daemon.go` | `Handler.ListTasksByIssue` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/daemon.go` | `Handler.ReportTaskProgress` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/daemon.go` | `Handler.buildClaimedTaskResponse` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/daemon.go` | `Handler.loadClaimParentIssue` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/daemon.go` | `Handler.reconcileCommentsOnCompletion` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/daemon.go` | `Handler.repairStaleCommentPlanIfNeeded` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/dashboard.go` | `Handler.GetDashboardAgentRunTime` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/dashboard.go` | `Handler.GetDashboardFailuresByAgent` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/dashboard.go` | `Handler.GetDashboardFailuresDaily` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/dashboard.go` | `Handler.GetDashboardRunTimeDaily` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/file.go` | `Handler.UploadFile` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/file.go` | `Handler.attachmentInViewerScope` | go-call | scoped | Scope.Allows on the issue of the attachment, directly or through its comment, answers like a missing attachment; test TestRestrictedIssueAttachmentsAnswerMissing |
| `server/internal/handler/file.go` | `Handler.attachmentOwnerLockAttempt` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/file.go` | `Handler.createAttachmentWithinQuota` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_api.go` | `Handler.StartIssueFlow` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_api.go` | `Handler.flowRequestFor` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_api_run.go` | `Handler.CloseIssueFlowTask` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_api_run.go` | `Handler.SetIssueFlowVar` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_engine.go` | `Handler.advanceOnce` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_engine.go` | `Handler.flowAdvanceResponse` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_engine.go` | `Handler.loadFlowTasks` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_engine.go` | `Handler.reopenCancelledWork` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_engine.go` | `Handler.stallFlow` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_engine.go` | `cancelFlowTasks` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_engine.go` | `moveFlowRequest` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_engine.go` | `supersedeFlowTasks` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_engine_ops.go` | `Handler.DetachFlow` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_engine_ops.go` | `Handler.ResumeFlow` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_engine_ops.go` | `Handler.lockFlowRun` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_guards.go` | `Handler.checkFlowChildCreate` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_guards.go` | `Handler.claimFlowBrief` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_guards.go` | `Handler.flowTaskRun` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_insights.go` | `Handler.GetFlowInsights` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_runner.go` | `Handler.advanceFlowAfterRun` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_runner.go` | `Handler.advanceFlowRequests` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_runner.go` | `Handler.isAutoFlowRequest` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_state.go` | `Handler.flowStateResponse` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_sweep.go` | `Handler.SweepFlowRequests` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_sweep.go` | `Handler.rescueFlowTasks` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_sweep.go` | `Handler.sweepFlowRequest` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/flow_sweep.go` | `Handler.sweepManualHumanTasks` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/github.go` | `Handler.lookupIssueByIdentifier` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/github.go` | `Handler.resolvePRLinkPolicy` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/handler.go` | `Handler.loadIssueForUser` | go-call | scoped | Calls a reader query; no visibility filter today |
| `server/internal/handler/handler.go` | `Handler.resolveIssueByIdentifier` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/inbox.go` | `Handler.ArchiveCompletedInbox` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/inbox.go` | `Handler.ListArchivedInbox` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/inbox.go` | `Handler.ListInbox` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/inbox.go` | `Handler.enrichInboxResponse` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/inbox_archive.go` | `Handler.GetArchivedInboxFacets` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/inbox_archive.go` | `Handler.ListArchivedInboxPage` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/initiative.go` | `Handler.initiativeDetail` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/initiative.go` | `initiativeReads.view` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/issue.go` | `Handler.BatchDeleteIssues` | go-call | scoped | Each id is read and kept only when Scope.Allows, so a hidden id is skipped like a missing one; test TestRestrictedIssueWritesAnswerMissing (BatchDeleteIssues) |
| `server/internal/handler/issue.go` | `Handler.BatchUpdateIssues` | go-call | scoped | Scope.Allows on every issue, parent and destination project; a move drops assignee and watchers who cannot see it; tests TestRestrictedIssueWritesAnswerMissing, TestUpdateIssueMovesDropWatchers, TestBatchUpdateAssigneeMustSeeProject |
| `server/internal/handler/issue.go` | `Handler.ChildIssueProgress` | go-call | scoped | Calls a reader query; no visibility filter today |
| `server/internal/handler/issue.go` | `Handler.CreateIssue` | go-call | scoped | A project or parent the creator cannot see answers 404 project not found or 400 like a missing parent; test TestCreateIssueChecksCreatorAndProject |
| `server/internal/handler/issue.go` | `Handler.ListChildIssues` | go-call | scoped | Calls a reader query; no visibility filter today |
| `server/internal/handler/issue.go` | `Handler.ListChildrenByParents` | go-call | scoped | Calls a reader query; no visibility filter today |
| `server/internal/handler/issue.go` | `Handler.QueryIssues` | go-call | scoped | Delegates to ListIssues; test TestRestrictedIssueListsOmitHidden |
| `server/internal/handler/issue.go` | `Handler.QuickCreateIssue` | go-call | scoped | A project or parent the requester cannot see is refused before the task is enqueued; test TestQuickCreateIssueRespectsProject |
| `server/internal/handler/issue.go` | `Handler.UpdateIssue` | go-call | scoped | loadIssueForUser, then Scope.Allows for the parent, duplicate target and destination project; a move drops assignee and watchers who cannot see it; tests TestRestrictedIssueWritesAnswerMissing, TestUpdateIssueMovesDropWatchers |
| `server/internal/handler/issue.go` | `Handler.deleteIssuesAndCollectAttachmentURLs` | go-call | exempt | (a) delete by id after the caller checked visibility: DeleteIssue (loadIssueForUser) and BatchDeleteIssues (Scope.Allows per id); it returns nothing to the viewer |
| `server/internal/handler/issue.go` | `Handler.loadDuplicateOriginals` | go-call | scoped | With a viewer scope an original that fails Scope.Allows stays nil, so the duplicate mark is left off; test TestRestrictedIssueSingleReadsAnswerMissing |
| `server/internal/handler/issue.go` | `Handler.updateIssueAtomically` | go-call | exempt | (a) locks and re-reads by id after UpdateIssue (loadIssueForUser) or BatchUpdateIssues (Scope.Allows) accepted the issue |
| `server/internal/handler/issue.go` | `updateIssueStoppingWakeups` | go-call | scoped | On a project move it clears an assignee and unsubscribes watchers who fail memberSeesProject in the same transaction; test TestUpdateIssueMovesDropWatchers |
| `server/internal/handler/issue_duplicate.go` | `Handler.ListIssueDuplicates` | go-call | scoped | loadIssueForUser, then the original through Scope.Allows and the duplicates through the SQL marker; test TestRestrictedIssueSingleReadsAnswerMissing |
| `server/internal/handler/issue_duplicate.go` | `lockAndCheckDuplicateMark` | go-call | exempt | (a) lock by id inside UpdateIssue, which loaded the issue with loadIssueForUser and refused a target that fails Scope.Allows first; test TestRestrictedIssueWritesAnswerMissing (duplicate mark target) |
| `server/internal/handler/issue_metadata.go` | `Handler.DeleteIssueMetadataKey` | go-call | scoped | loadIssueForUser answers a hidden issue like a missing one before the write; test TestRestrictedIssueWritesAnswerMissing |
| `server/internal/handler/issue_metadata.go` | `Handler.SetIssueMetadataKey` | go-call | scoped | loadIssueForUser answers a hidden issue like a missing one before the write; test TestRestrictedIssueWritesAnswerMissing |
| `server/internal/handler/issue_queue.go` | `Handler.BatchSetRunQueue` | go-call | scoped | decodeRunQueueBatch keeps only issues that pass Scope.Allows; test TestRestrictedIssueWritesAnswerMissing |
| `server/internal/handler/issue_queue.go` | `Handler.ListRunQueueWaits` | go-call | scoped | viewerScope passed to the SQL marker; test TestRestrictedIssueListsOmitHidden |
| `server/internal/handler/issue_queue.go` | `Handler.PreviewRunQueue` | go-call | scoped | decodeRunQueueBatch keeps only issues that pass Scope.Allows; test TestRestrictedIssueWritesAnswerMissing |
| `server/internal/handler/issue_queue.go` | `Handler.PromoteQueuedIssues` | go-call | exempt | (b) internal worker, no viewer: the issue queue listener (cmd/server/issue_queue_listeners.go) and the issue_queue_sweep scheduler job; SetIssueRunQueue and BatchSetRunQueue call it and discard the count |
| `server/internal/handler/issue_queue.go` | `Handler.SetIssueRunQueue` | go-call | scoped | loadIssueForUser, then validateRunAfter with the viewer Scope; test TestRestrictedIssueWritesAnswerMissing (run queue wait) |
| `server/internal/handler/issue_queue.go` | `Handler.decodeRunQueueBatch` | go-call | scoped | Reads the ids and keeps only issues that pass Scope.Allows; test TestRestrictedIssueWritesAnswerMissing |
| `server/internal/handler/issue_queue.go` | `Handler.runQueueWouldCycle` | go-call | scoped | A path is not followed through an issue that fails Scope.Allows; test TestRunQueueWaitsIgnoreHiddenIssues |
| `server/internal/handler/issue_queue.go` | `Handler.validateRunAfter` | go-call | scoped | A wait that fails Scope.Allows counts as missing; test TestRestrictedIssueWritesAnswerMissing (run queue wait) |
| `server/internal/handler/issue_status.go` | `Handler.ArchiveIssueStatus` | go-call | exempt | Owner and admin only (Scope mode all): loadIssueStatusForAdmin gates the route, and the issue count is the one an owner or admin sees |
| `server/internal/handler/issue_subissue_close.go` | `Handler.openSubIssuesBlockingClose` | go-call | scoped | A sub-issue that fails Scope.Allows is neither counted nor named; test TestRestrictedIssueWritesDoNotNameHiddenProject (openSubIssuesBlockingClose) |
| `server/internal/handler/issue_system_wakeup.go` | `Handler.childDoneSystemWakeup` | go-call | scoped | Sub-issues are filtered with Scope.Allows before they are counted or named; test TestSystemWakeupsOmitHiddenIssues |
| `server/internal/handler/issue_system_wakeup.go` | `Handler.workspaceSystemWakeups` | go-call | scoped | The customized count passes the Scope to the SQL marker; test TestSystemWakeupsOmitHiddenIssues |
| `server/internal/handler/issue_triage_guard.go` | `Handler.validateBatchTriageLocks` | go-call | scoped | Counts only triage entries that pass Scope.Allows, so a hidden entry is skipped like a missing id; test TestBatchTriageLocksRespectVisibility |
| `server/internal/handler/issue_trigger.go` | `Handler.PreviewIssueTrigger` | go-call | scoped | A hidden issue or project contributes no trigger, like a missing one; test TestRestrictedIssueListsOmitHidden |
| `server/internal/handler/issue_unmoved_run.go` | `Handler.reconcileUnmovedWorkflowRun` | go-call | exempt | (b) internal worker, no viewer: CompleteTask (daemon token) re-runs the step handler after a run ends; it returns nothing to a caller |
| `server/internal/handler/issue_view.go` | `Handler.CreateIssueView` | go-call | scoped | A project the viewer cannot see answers 404 like a missing one; test TestRestrictedProjectViewsAnswerMissing |
| `server/internal/handler/issue_view_preference.go` | `Handler.resolvePreferenceScope` | go-call | scoped | A project the viewer cannot see answers 404 like a missing one; test TestRestrictedProjectViewsAnswerMissing |
| `server/internal/handler/issue_wakeup.go` | `Handler.ListPausedWakeups` | go-call | scoped | viewerScope passed to the SQL marker; test TestRestrictedIssueListsOmitHidden |
| `server/internal/handler/issue_wakeup.go` | `Handler.ListWorkspaceWakeupSummaries` | go-call | scoped | viewerScope passed to the SQL marker; test TestRestrictedIssueListsOmitHidden |
| `server/internal/handler/issue_wakeup.go` | `Handler.ListWorkspaceWakeups` | go-call | scoped | viewerScope passed to the SQL marker; test TestRestrictedIssueListsOmitHidden |
| `server/internal/handler/issue_workflow.go` | `Handler.DeleteIssueWorkflow` | go-call | scoped | A workflow used by a project the caller cannot see answers 404 workflow not found; ListProjectsUsingIssueWorkflow carries the marker; test TestRestrictedWorkflowRoutesAnswerMissing/DeleteIssueWorkflow. Open flow request ids still come from ListOpenFlowRequestIDsByWorkflow (flow.sql row) |
| `server/internal/handler/issue_workflow.go` | `Handler.PreviewIssueWorkflowBrief` | go-call | scoped | A project the caller cannot see is not named in the brief, like a missing one; tests TestRestrictedIssueWritesDoNotNameHiddenProject, TestRestrictedWorkflowRoutesAnswerMissing/PreviewIssueWorkflowBrief |
| `server/internal/handler/issue_workflow.go` | `Handler.PreviewWorkflowHandoff` | go-call | scoped | loadIssueForUser, then the project param passes Scope.Allows and answers 404 like a missing project; tests TestRestrictedIssueWritesDoNotNameHiddenProject, TestRestrictedWorkflowRoutesAnswerMissing/PreviewWorkflowHandoff |
| `server/internal/handler/issue_workflow.go` | `Handler.SetProjectWorkflow` | go-call | scoped | loadProjectForWorkflowWrite answers 404 project not found for a hidden project; census and remap carry the marker; test TestRestrictedWorkflowRoutesAnswerMissing/SetProjectWorkflow |
| `server/internal/handler/issue_workflow.go` | `Handler.UpdateIssueWorkflow` | go-call | todo | Project part scoped (non-manager refused with 404 when a hidden project uses the workflow; test TestRestrictedWorkflowRoutesAnswerMissing/UpdateIssueWorkflow); squad census and remap still unscoped, see CountSquadIssuesByStatus |
| `server/internal/handler/issue_workflow.go` | `Handler.loadProjectForWorkflowWrite` | go-call | scoped | Scope.Allows after GetProjectInWorkspace answers 404 like a missing project; test TestRestrictedWorkflowRoutesAnswerMissing/SetProjectWorkflow |
| `server/internal/handler/issue_workflow.go` | `Handler.openFlowRequestCounts` | go-call | todo | Per-workflow count from CountOpenFlowRequestsByWorkflow (flow.sql row) and also called by squad_workflow.go (squad group); scope it with that query |
| `server/internal/handler/issue_workflow.go` | `Handler.projectIDsByWorkflow` | go-call | scoped | Scope.Allows on each loaded project; test TestRestrictedWorkflowListsOmitHidden |
| `server/internal/handler/issue_workflow.go` | `Handler.saveHandoffOnProjectCopy` | go-call | scoped | Scope.Allows on the locked project answers 404 like a missing project; test TestRestrictedWorkflowRoutesAnswerMissing/SetIssueWorkflowHandoff_copy |
| `server/internal/handler/issue_workflow.go` | `remapIssues` | go-call | scoped | Passes the viewer scope to RemapProjectIssueStatus (marker); test TestRestrictedWorkflowRemapSkipsNothingVisibleToOwner |
| `server/internal/handler/knowledge_context.go` | `Handler.issueKnowledgeContext` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/label.go` | `Handler.AttachLabel` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/label.go` | `Handler.DetachLabel` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/org_map.go` | `Handler.GetCompanyRequestUsage` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/pin.go` | `Handler.CreatePin` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/plugin_action.go` | `Handler.resolvePluginIssue` | go-call | exempt | (a) only caller is pluginIssueForUser, which applies Scope.Allows for the caller's user before the issue is used; test TestRestrictedPluginIssueAnswersMissing |
| `server/internal/handler/pr_auto_complete.go` | `Handler.maybeAutoCompleteIssue` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/project.go` | `Handler.DeleteProject` | go-call | scoped | viewerScope and Scope.Allows answer 404 like a missing project; test TestRestrictedProjectRoutesAnswerMissing |
| `server/internal/handler/project.go` | `Handler.GetProject` | go-call | scoped | Calls a reader query; no visibility filter today |
| `server/internal/handler/project.go` | `Handler.ListProjects` | go-call | scoped | Calls a reader query; no visibility filter today |
| `server/internal/handler/project.go` | `Handler.SearchProjects` | go-call | scoped | Calls a reader query; no visibility filter today |
| `server/internal/handler/project.go` | `Handler.UpdateProject` | go-call | scoped | viewerScope and Scope.Allows answer 404 like a missing project; test TestRestrictedProjectRoutesAnswerMissing |
| `server/internal/handler/project.go` | `Handler.loadProjectIssueStats` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/project_resource.go` | `Handler.loadProjectForResource` | go-call | scoped | viewerScope and Scope.Allows answer 404 like a missing project for every project resource route; test TestRestrictedProjectRoutesAnswerMissing |
| `server/internal/handler/project_resource.go` | `Handler.resolveClaimProjectContext` | go-call | exempt | (b) the daemon task claim (ClaimTask paths in daemon.go) loads the project of the claimed task; no viewer |
| `server/internal/handler/property.go` | `Handler.ListProperties` | go-call | scoped | viewerScope passed to the ListIssueProperties marker; test TestRestrictedIssueListsOmitHidden |
| `server/internal/handler/property.go` | `Handler.UpdateProperty` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/proposal.go` | `Handler.GetProposal` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/proposal.go` | `Handler.ListProposals` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/reaction.go` | `Handler.AddReaction` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/search_index.go` | `Handler.GetSearchIndexManifest` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/search_index.go` | `Handler.GetSearchIndexSnapshot` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/search_index.go` | `Handler.ListSearchIndexChanges` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/source_context.go` | `Handler.CreateCommentSubIssue` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/source_context.go` | `Handler.prepareAgentCommentSubIssue` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/squad.go` | `Handler.ListSquadMemberStatus` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/squad_briefing.go` | `secretaryDirectiveBriefing` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/squad_directory.go` | `Handler.ListSquadDirectory` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/squad_project.go` | `Handler.AssignSquadProject` | go-call | scoped | Scope.Allows after LockProjectForWorkflowChange answers 404 like a missing project; test TestRestrictedAssignSquadProjectAnswersMissing |
| `server/internal/handler/squad_workflow.go` | `Handler.squadWorkflowPlan` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/squad_workflow.go` | `remapSquadIssues` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/subscriber.go` | `Handler.ListSubscribedIssueIDs` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/subscriber.go` | `Handler.unsubscribeSubtreeSerialized` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/workspace.go` | `Handler.deleteWorkspaceData` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/workspace.go` | `deleteWorkspaceTasks` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/handler/workspace.go` | `lockWorkspaceTaskOwners` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/integrations/channel/engine/session.go` | `ChatSession.bindMediaRefs` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/integrations/channel/engine/session.go` | `dbSessionQueries.LockIssueForChannelMediaBind` | go-call | exempt | (b) internal worker, no viewer: the channel engine media binder locks the bound issue and discards the row |
| `server/internal/issueguard/duplicate.go` | `LockAndFindActiveDuplicate` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/issueguard/duplicate.go` | `LockAndFindRecentAutopilotDuplicate` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/issueworkflow/issueworkflow.go` | `ForIssue` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/projectaccess/handler.go` | `Handler.load` | go-call | scoped | Computes the Scope, then applies Scope.Allows to the loaded project; hidden and missing both 404 |
| `server/internal/service/approval_policy.go` | `ApprovalPolicyService.AutoApprovals` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/approval_policy.go` | `ApprovalPolicyService.List` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/approval_policy.go` | `ApprovalPolicyService.ListRecentAutoApprovals` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/approval_policy.go` | `ApprovalPolicyService.ListReversedAutoApprovals` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/autopilot.go` | `AutopilotService.ensureWebhookCreateIssueTask` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/autopilot.go` | `AutopilotService.loadDispatchChain` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/brief_inquiry.go` | `AskBriefInquiry` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/brief_push.go` | `BriefPushService.composeMessage` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/budget_accounting.go` | `accountTaskInTx` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/budget_accounting.go` | `seedBudgetPeriod` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/budget_gate.go` | `BudgetGate.Admit` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/budget_gate.go` | `BudgetGate.finishedCosts` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/budget_gate.go` | `budgetScopeOfTask` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/company_starter_skills.go` | `ensureSecretaryDesk` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/company_starter_skills.go` | `findSecretaryDesk` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/digest.go` | `DigestService.Compute` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/digest.go` | `DigestService.attachRoutines` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/digest.go` | `DigestService.computeBlockedDirectives` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/digest.go` | `DigestService.computeOverdueRequests` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/digest.go` | `DigestService.computeRequestsPastDeadline` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/directive_authority.go` | `AuthorizeDirective` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/directive_authority.go` | `checkDirectiveTarget` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/flow_start.go` | `startFlow` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/initiative.go` | `InitiativeService.closeLocked` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/initiative.go` | `InitiativeService.moveTo` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/initiative_build.go` | `InitiativeService.minTopPosition` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/initiative_gate.go` | `InitiativeService.fileDueOperateGates` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/initiative_gate.go` | `InitiativeService.gateIsDue` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/issue.go` | `IssueService.Create` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/issue.go` | `IssueService.PublishAttachmentsChanged` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/issue.go` | `IssueService.createWorkflow` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/issue_limit.go` | `AllocateIssueNumber` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/issue_limit.go` | `CountIssueUsage` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/issue_public.go` | `IssueService.UpdateContent` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/issue_wakeup.go` | `IssueWakeupService.dispatch` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/issue_wakeup.go` | `IssueWakeupService.systemRuleRunWaiting` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/issue_wakeup_join.go` | `IssueWakeupService.JoinWaitingWakeups` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/issue_wakeup_system.go` | `IssueWakeupService.BackfillChildDoneRules` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/issue_wakeup_system.go` | `IssueWakeupService.UpdateChildDoneRule` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/issue_wakeup_system.go` | `IssueWakeupService.autoFlowRequest` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/issue_wakeup_system.go` | `IssueWakeupService.dispatchSystem` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/issue_wakeup_system.go` | `IssueWakeupService.processChildEvents` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/knowledge.go` | `KnowledgeService.Status` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/knowledge_audit.go` | `KnowledgeService.requestReview` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/knowledge_autoenable.go` | `KnowledgeService.RunAutoEnable` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/knowledge_project_docs.go` | `KnowledgeService.ProjectDocSources` | go-call | exempt | (b) the daemon project-docs sync (ListDaemonProjectDocSources, daemon token, no viewer) |
| `server/internal/service/knowledge_project_docs.go` | `KnowledgeService.projectForDocs` | go-call | exempt | (b) the daemon project-docs sync (PostDaemonProjectDocsManifest, PostDaemonProjectDocs; daemon token, no viewer) |
| `server/internal/service/metrics_builtin.go` | `MetricService.windowReadings` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/org_map.go` | `LoadOrgMap` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/proposal.go` | `ProposalService.Resubmit` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/proposal.go` | `ProposalService.askForChanges` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/proposal.go` | `ProposalService.resumeWaitingIssues` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/proposal.go` | `ProposalService.revertDecision` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/proposal.go` | `ProposalService.settleInTx` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/proposal.go` | `ProposalService.snooze` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/proposal_expiry.go` | `ProposalService.expireOne` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/proposal_prior_art.go` | `ProposalService.RelatedProposalsByTitle` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/proposal_prior_art.go` | `ProposalService.relatedProposalsByTitle` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/qoder_issue_lifecycle.go` | `syncQoderIssue` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/review.go` | `ReviewService.FileMonthlyReviews` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/scorecard.go` | `ScorecardInputsFor` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/secretary_reply_gate.go` | `checkAssessmentReply` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/secretary_request_caps.go` | `CheckSecretaryRequestCaps` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/secretary_request_caps.go` | `requestTreePosition` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/secretary_request_reply.go` | `CheckSecretaryRequestAnswered` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/secretary_request_reply.go` | `IssueInSecretaryTree` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/secretary_request_reply.go` | `isSecretaryRequest` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/source_context.go` | `BuildSourceIssueSnapshot` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/squad_handoff.go` | `SquadHandoffService.perform` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/squad_handoff.go` | `SquadHandoffService.resultOf` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/squad_handoff.go` | `SquadHandoffService.targetRequest` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/task.go` | `TaskService.HandleFailedTasks` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/task.go` | `TaskService.RecoverPendingDelegatedFailures` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/task.go` | `TaskService.RerunIssue` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/task.go` | `TaskService.ResolveTaskScopeChecked` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/task.go` | `TaskService.createAgentComment` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/task.go` | `TaskService.dispatchDelegatedFailureRecovery` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/task.go` | `TaskService.enqueueIssueTaskWithCommentPlan` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/task.go` | `TaskService.enqueueMentionTaskWithCommentPlan` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/task.go` | `TaskService.enqueueQuickCreateTask` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/task.go` | `TaskService.notifyQuickCreateCompleted` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/task.go` | `TaskService.taskAnalyticsContext` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/task.go` | `loadDelegatedFailureRecoveryTarget` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/task.go` | `resolveDuplicateOf` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/service/task_triage_guard.go` | `guardIssueNotInTriage` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/usagesnapshot/dbstore.go` | `DBStore.Counts` | go-call | todo | Calls a reader query; no visibility filter today |
| `server/internal/workspaceexport/dbsource.go` | `DBSource.Rows` | go-call | todo | Calls a reader query; no visibility filter today |

### Raw SQL in Go

| File | Function | Kind | Status | Note |
| --- | --- | --- | --- | --- |
| `server/internal/handler/issue.go` | `Handler.ListGroupedIssues` | raw-sql | scoped | Inline SQL reads issue or project; no visibility filter today |
| `server/internal/handler/issue.go` | `buildSearchQuery` | raw-sql | scoped | scopeCondition on the issue_matches CTE; comments reach the result only through it; test TestRestrictedIssueListsOmitHidden |
| `server/internal/handler/issue_move.go` | `Handler.issueMoveAnchorPosition` | raw-sql | scoped | The anchor query carries scopeCondition, so a hidden anchor answers like a missing one; test TestRestrictedIssueWritesAnswerMissing (move anchor) |
| `server/internal/handler/issue_table_advanced.go` | `issueTableFilterCompiler.fieldPredicate` | raw-sql | scoped | Predicate is AND-ed onto the scopeCondition that compileIssueTableQuery puts first, so it sees visible rows only; tests TestRestrictedIssueListsOmitHidden |
| `server/internal/handler/issue_table_facets.go` | `Handler.ListIssueTableFacets` | raw-sql | scoped | Counts run on compileIssueTableQuery output, which carries scopeCondition; test TestRestrictedIssueListsOmitHidden |
| `server/internal/projectaccess/service.go` | `Service.SwitchAccess` | go-call | exempt | Owner and admin only; the handler checked the Scope before calling |
| `server/internal/projectaccess/service.go` | `Service.Revoke` | go-call | exempt | Owner and admin only; the handler checked the Scope before calling |
| `server/internal/projectaccess/service.go` | `Service.Grant` | go-call | exempt | Owner and admin only; the handler checked the Scope before calling |
| `server/internal/projectaccess/scope.go` | `ForViewer` | go-call | exempt | Computes the Scope; reads the viewer own member row and grants |
| `server/internal/handler/issue_table_facets.go` | `Handler.issueTableBaseFacetQuery` | raw-sql | scoped | Runs on compileIssueTableQuery output, which carries scopeCondition; test TestRestrictedIssueListsOmitHidden |
| `server/internal/handler/issue_table_facets.go` | `Handler.issueTableFacetQuery` | raw-sql | scoped | Runs on compileIssueTableQuery output, which carries scopeCondition; test TestRestrictedIssueListsOmitHidden |
| `server/internal/handler/issue_table_group.go` | `Handler.ListIssueTableGroups` | raw-sql | scoped | Groups run on compileIssueTableQuery output; the parent sort key and context use scopeCondition; tests TestRestrictedIssueListsOmitHidden, TestParentLookupDoesNotLeakHiddenParent |
| `server/internal/handler/issue_table_group.go` | `Handler.resolveIssueTableGroup` | raw-sql | scoped | Project title sort reads the project of a row that already passed the scope; the parent sort key moved to sortExpression, which applies scopeCondition; test TestParentLookupDoesNotLeakHiddenParent |
| `server/internal/handler/issue_table_group.go` | `resolvedIssueTableGroup.contextExpression` | raw-sql | scoped | Parent lookup applies scopeCondition to the parent row; hidden parent gives an empty context; test TestParentLookupDoesNotLeakHiddenParent |
| `server/internal/handler/issue_table_group.go` | `resolvedIssueTableGroup.sortExpression` | raw-sql | scoped | Parent sort key applies scopeCondition to the parent row; hidden parent sorts like a missing one; test TestParentLookupDoesNotLeakHiddenParent |
| `server/internal/handler/issue_table_query.go` | `Handler.compileIssueTableQuery` | raw-sql | scoped | Puts scopeCondition into every compiled WHERE; tests TestRestrictedIssueListsOmitHidden |
| `server/internal/handler/issue_table_rows.go` | `Handler.queryIssueTableRows` | raw-sql | scoped | Rows and hierarchy come from compileIssueTableQuery output; duplicate originals go through the viewer filler; test TestRestrictedIssueListsOmitHidden |
| `server/internal/handler/project.go` | `buildProjectSearchQuery` | raw-sql | scoped | Inline SQL reads issue or project; no visibility filter today |
| `server/internal/issueposition/position.go` | `NextTopPosition` | raw-sql | todo | Inline SQL reads issue or project; no visibility filter today |
| `server/internal/projectaccess/scope.go` | `<file-level>` | raw-sql | exempt | Defines the shared predicate text (Fragment); not a read path |
| `server/internal/service/issue_wakeup_condition.go` | `IssueWakeupService.evaluateCondition` | raw-sql | todo | Inline SQL reads issue or project; no visibility filter today |
| `server/internal/service/issue_wakeup_condition.go` | `loadSubIssues` | raw-sql | todo | Inline SQL reads issue or project; no visibility filter today |
| `server/internal/service/issue_wakeup_condition.go` | `validateCondition` | raw-sql | todo | Inline SQL reads issue or project; no visibility filter today |
| `server/internal/testutil/db.go` | `Fixture.Issue` | raw-sql | todo | Inline SQL reads issue or project; no visibility filter today |

### Go call sites with raw SQL

| File | Function | Kind | Status | Note |
| --- | --- | --- | --- | --- |
| `server/internal/handler/issue.go` | `Handler.ListIssues` | go-call+raw-sql | scoped | Calls a reader query and has inline SQL; no visibility filter today |
| `server/internal/handler/issue_move.go` | `Handler.MoveIssue` | go-call+raw-sql | scoped | loadIssueForUser for the issue, Scope.Allows for the destination, scopeCondition for the anchors; tests TestMoveIssueNeedsBothProjects and TestRestrictedIssueWritesAnswerMissing |
| `server/internal/service/issue_wakeup.go` | `IssueWakeupService.Disable` | go-call+raw-sql | todo | Calls a reader query and has inline SQL; no visibility filter today |
| `server/internal/service/issue_wakeup.go` | `IssueWakeupService.EditInstruction` | go-call+raw-sql | todo | Calls a reader query and has inline SQL; no visibility filter today |
| `server/internal/service/issue_wakeup.go` | `IssueWakeupService.save` | go-call+raw-sql | todo | Calls a reader query and has inline SQL; no visibility filter today |
| `server/internal/service/issue_wakeup_manage.go` | `lockManagedWakeup` | go-call+raw-sql | todo | Calls a reader query and has inline SQL; no visibility filter today |

## Manual rows

Readers the scanner cannot see. The test checks only that Status is a known value.

| File | Function | Kind | Status | Note |
| --- | --- | --- | --- | --- |
| `server/pkg/db/queries/comment.sql` | table `comment` (issue_id) | manual | todo | Derived table; follows its issue |
| `server/pkg/db/queries/attachment.sql` | table `attachment` (issue_id) | manual | todo | Derived table; follows its issue |
| `server/pkg/db/queries/activity.sql` | table `activity_log` (issue_id) | manual | todo | Derived table; follows its issue |
| `server/pkg/db/queries/issue_reaction.sql` | table `issue_reaction` (issue_id) | manual | todo | Derived table; follows its issue |
| `server/pkg/db/queries/subscriber.sql` | table `issue_subscriber` (issue_id) | manual | todo | Derived table; follows its issue |
| `server/pkg/db/queries/inbox.sql` | table `inbox_item` (issue_id) | manual | todo | Derived table; follows its issue |
| `server/pkg/db/queries/agent.sql` | table `agent_task_queue` (issue_id) | manual | todo | Task queue rows; follows its issue |
| `server/pkg/db/queries/knowledge_project_file.sql` | table `knowledge_project_file` (project_id) | manual | todo | Project knowledge documents; follow their project |
| `server/pkg/db/queries/project_resource.sql` | table `project_resource` (project_id) | manual | todo | Project resources; follow their project |
| `server/pkg/db/queries/issue_label.sql` | table `issue_to_label` (issue_id) | manual | todo | Derived table; follows its issue |
| `server/pkg/db/queries/issue_unread.sql` | table `issue_unread` (issue_id) | manual | todo | Derived table; follows its issue |
| `server/migrations` | other tables with issue_id or project_id | manual | todo | issue_dependency, issue_pull_request, issue_wakeup, proposal, planning_cycle, pipeline, autopilot, chat_session and similar; list taken from CREATE TABLE blocks in the migrations |
| `server/cmd/server/listeners.go` | registerListeners | manual | todo | Issue events go to ScopeWorkspace, so every workspace member receives them |
| `server/cmd/server` | other `*listeners.go` files | manual | todo | Activity, subscriber, notification, brief and unread listeners read issue data on events |
| `server/internal/realtime` | broadcaster.go and redis_relay.go fan-out | manual | todo | Workspace-wide rooms; no per-person audience for issue events |
| `server/internal/handler/search.go` | server-side search | manual | scoped | SearchIssues and SearchProjects apply the Scope (scopeCondition); test TestSearchServerSideOmitsHidden |
| `server/internal/handler/search_index.go` | manifest, snapshot, changes | manual | todo | Client index sync covers the whole workspace |
| `server/pkg/db/queries/search_index.sql` | search index queries | manual | todo | Queries behind the index sync |
| `packages/core/search-index` | client search index | manual | todo | Local copy of workspace issues; cannot revoke single rows |
| `server/internal/handler/hub_sync.go` | hub sync | manual | exempt | (b) hub source sync (SyncHubSourceAuto worker, SyncHubSource admin trigger); reads no issue or project row, hub items are skills and plugin packages |
| `server/internal/handler/knowledge_project_docs.go` | project knowledge docs | manual | exempt | (b) the daemon project-docs sync routes under /api/daemon (daemon token, no viewer) |
| `server/internal/handler/squad_project.go` | squad project | manual | todo | Reads project data |
| `server/internal/workspaceexport` | workspace export | manual | todo | Exports every issue and project of the workspace |
| `server/internal/usagesnapshot` | usage snapshot | manual | todo | Aggregates across issues and projects |
| `server/internal/handler/daemon.go` | daemon and mat_ token routes | manual | todo | Agent reads any issue of the workspace today |
| `server/internal/service` | task claim, budget gate (`claimRefusalFor`) and other services | manual | todo | Agent and daemon read paths; no originator visibility check |
| `server/internal/handler/mcp_overlay.go` | MCP overlay | manual | todo | Task-level MCP config; confirm whether it exposes issue data |
| `server/internal/daemon/remote_mcp_broker.go` | MCP broker | manual | todo | MCP tools reach the API with the agent token; confirm at P41.8 |
| `server/internal/handler/dashboard.go` | dashboard aggregates | manual | todo | Counts and sums across all issues |
| `server/pkg/db/queries/task_usage.sql` | usage rollups by project_id | manual | todo | Aggregates keyed by project |

## Not yet in the visibility set

Every row above is `todo` except the `projectaccess` package itself and its `project_access.sql` queries (P41.2). No issue or project read path calls `projectaccess` yet; P41.3 changes them.
