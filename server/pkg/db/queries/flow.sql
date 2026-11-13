-- Enforced squad workflow engine. A request that runs on a flow keeps its state in
-- issue.flow; a task the engine opened under it carries flow_step, flow_task and
-- flow_round. Only the engine writes these columns.

-- name: LockIssueForFlow :one
-- Serializes concurrent engine runs on one request (the same pattern as LockSquadForWorkflowChange).
SELECT * FROM issue
WHERE id = sqlc.arg('id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid
FOR UPDATE;

-- name: InitIssueFlow :one
-- Starts a request on a flow. Returns no row when the issue already has a flow.
UPDATE issue SET flow = sqlc.arg('flow')::jsonb
WHERE id = sqlc.arg('id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid
  AND flow IS NULL
RETURNING *;

-- name: SetIssueFlow :one
-- Moves the request's flow state. The condition on the step and the rework count
-- the caller decided from makes two concurrent decisions lose to one. Returns no
-- row when the state moved under the caller.
UPDATE issue SET flow = sqlc.arg('flow')::jsonb
WHERE id = sqlc.arg('id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid
  AND flow->>'step' = sqlc.arg('expected_step')::text
  AND COALESCE((flow->>'rework')::int, 0) = sqlc.arg('expected_rework')::int
RETURNING *;

-- name: ClearIssueFlow :exec
-- Detaches a request from its flow.
UPDATE issue SET flow = NULL
WHERE id = sqlc.arg('id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid;

-- name: ClearIssueFlowTasks :exec
-- Turns the tasks of a detached request into ordinary sub-issues.
UPDATE issue SET flow = NULL, flow_step = NULL, flow_task = NULL, flow_round = NULL
WHERE parent_issue_id = sqlc.arg('parent_issue_id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid
  AND flow_task IS NOT NULL;

-- name: SetIssueOutcome :exec
-- Stores the verdict a handler reported when it closed the issue; NULL clears it.
UPDATE issue SET outcome = sqlc.narg('outcome')::text
WHERE id = sqlc.arg('id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid;

-- name: ListFlowTasks :many
-- Every task the engine opened under a request, oldest first.
SELECT * FROM issue
WHERE parent_issue_id = sqlc.arg('parent_issue_id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid
  AND flow_task IS NOT NULL
ORDER BY created_at, id;

-- name: ListOpenFlowRequests :many
-- Requests that run on a flow and are not closed, in id order after the cursor.
-- Served by idx_issue_flow_open.
SELECT * FROM issue
WHERE flow IS NOT NULL
  AND NOT (flow ? 'closed_at')
  AND (workspace_id, id) > (sqlc.arg('after_workspace_id')::uuid, sqlc.arg('after_id')::uuid)
ORDER BY workspace_id, id
LIMIT sqlc.arg('page_limit')::int;

-- name: ListOpenFlowRequestIDsByWorkflow :many
-- The open requests pinned to a workflow; they block its deletion.
SELECT id FROM issue
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND flow IS NOT NULL
  AND NOT (flow ? 'closed_at')
  AND flow->>'workflow_id' = sqlc.arg('workflow_id')::text
ORDER BY id
LIMIT sqlc.arg('page_limit')::int;

-- name: CountOpenFlowRequestsByWorkflow :many
-- Open requests per workflow, for the workflow list.
SELECT COALESCE(flow->>'workflow_id', '')::text AS workflow_id, COUNT(*)::bigint AS open_count
FROM issue
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND flow IS NOT NULL
  AND NOT (flow ? 'closed_at')
GROUP BY COALESCE(flow->>'workflow_id', '');

-- name: SetIssueWorkflowFlowAuto :one
-- The runner switch of a workflow; only this query writes flow_auto.
UPDATE issue_workflow SET flow_auto = sqlc.arg('flow_auto')::boolean, updated_at = now()
WHERE id = sqlc.arg('id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid
RETURNING *;

-- name: CountAttachmentsOnIssueByUploader :one
-- The files one uploader (an agent or a member) attached to an issue, in its
-- description or in a comment (a file linked to a comment keeps the issue's id).
-- A flow task that requires an attachment counts the ones its own assignee
-- uploaded, and the ones the member who closes it for the assignee uploaded.
SELECT COUNT(*)::bigint FROM attachment
WHERE issue_id = sqlc.arg('issue_id')::uuid
  AND workspace_id = sqlc.arg('workspace_id')::uuid
  AND uploader_type = sqlc.arg('uploader_type')::text
  AND uploader_id = sqlc.arg('uploader_id')::uuid;

-- name: ListFlowManualActions :many
-- The latest close or reopen a member made for the assignee of each task, to say
-- who closed a task that is closed now.
SELECT DISTINCT ON (issue_id) issue_id, action, actor_type, actor_id
FROM activity_log
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND issue_id = ANY(sqlc.arg('issue_ids')::uuid[])
  AND action IN ('flow_manual_close', 'flow_manual_reopen')
ORDER BY issue_id, created_at DESC, id DESC;

-- name: IsFlowRequestAuto :one
-- Whether the request runs on a flow whose workflow lets the background runner
-- move it. Computed where it is read and never stored, so turning flow_auto off
-- takes effect at once. No row: the issue is not an open flow request.
SELECT COALESCE((
  SELECT w.flow_auto FROM issue_workflow w
  WHERE w.id = NULLIF(i.flow->>'workflow_id', '')::uuid AND w.workspace_id = i.workspace_id
), false)::boolean AS auto
FROM issue i
WHERE i.id = sqlc.arg('id')::uuid
  AND i.flow IS NOT NULL
  AND NOT (i.flow ? 'closed_at');

-- name: ListAutoFlowRequests :many
-- Open requests whose flow workflow lets the runner move them, in id order after
-- the cursor. The sweep reads a page per pass.
SELECT i.* FROM issue i
JOIN issue_workflow w ON w.id = NULLIF(i.flow->>'workflow_id', '')::uuid AND w.workspace_id = i.workspace_id
WHERE i.flow IS NOT NULL
  AND NOT (i.flow ? 'closed_at')
  AND w.flow_auto
  AND (i.workspace_id, i.id) > (sqlc.arg('after_workspace_id')::uuid, sqlc.arg('after_id')::uuid)
ORDER BY i.workspace_id, i.id
LIMIT sqlc.arg('page_limit')::int;

-- name: ListUnstartedFlowRequests :many
-- Requests given to a squad whose active workflow is an auto flow but that have
-- no flow state yet: the write that created them did not reach the engine. A request
-- a person detached (the flow_detached entry) stays off its flow. Only a request a member
-- filed qualifies: an agent files work for a squad through the paths that start the flow
-- themselves (a handoff), and the company planning job files its issues in the leader's name.
SELECT i.* FROM issue i
JOIN squad s ON s.id = i.assignee_id AND s.workspace_id = i.workspace_id
JOIN issue_workflow w ON w.id = s.active_workflow_id AND w.workspace_id = s.workspace_id
WHERE i.assignee_type = 'squad'
  AND i.flow IS NULL
  AND i.flow_task IS NULL
  AND i.parent_issue_id IS NULL
  AND i.creator_type = 'member'
  AND i.status NOT IN ('done', 'cancelled')
  AND s.archived_at IS NULL
  AND w.flow_config IS NOT NULL
  AND w.flow_auto
  AND NOT EXISTS (SELECT 1 FROM activity_log a WHERE a.issue_id = i.id AND a.action = 'flow_detached')
ORDER BY i.created_at
LIMIT sqlc.arg('page_limit')::int;

-- name: FlowRunStats :one
-- The runs of an issue since a moment: how many were made and when the issue last had activity. The
-- active runs and the one running longest are counted whenever they began: a run that started before
-- the step's entered_at (a person's answer woke the lead, then Resume moved the step) is still the
-- issue's run. The sweep uses it to retry a task nobody works on and to cap a run that never ends.
SELECT
  (COUNT(*) FILTER (WHERE created_at >= sqlc.arg('since')::timestamptz))::int AS attempts,
  (COUNT(*) FILTER (WHERE status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')))::int AS active,
  (MIN(started_at) FILTER (WHERE status = 'running'))::timestamptz AS running_since,
  (MAX(COALESCE(completed_at, started_at, created_at)) FILTER (WHERE created_at >= sqlc.arg('since')::timestamptz))::timestamptz AS last_activity
FROM agent_task_queue
WHERE issue_id = sqlc.arg('issue_id')::uuid;

-- name: LastAgentCommentOnIssue :one
-- The newest comment an agent wrote on an issue: a blocked task says there what it could not do.
SELECT content FROM comment
WHERE issue_id = $1 AND author_type = 'agent'
ORDER BY created_at DESC, id DESC
LIMIT 1;
