-- name: ListSecretaryRequestsPastDeadline :many
-- Secretary request issues (parent issues in the Secretary desk project, held by
-- the Secretary agent) still open after their due date. The Secretary closes a
-- request when it publishes the assessment, so an open one past its due date has
-- no assessment. The digest reads them for the boss's brief (spec 13.7.3).
SELECT id, number, title, due_date
FROM issue
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND assignee_type = 'agent'
  AND status NOT IN ('done', 'cancelled')
  AND due_date IS NOT NULL
  AND due_date < sqlc.arg('now')::timestamptz
  -- projectaccess:scope
  AND (sqlc.arg(scope_mode)::text = 'all'
      OR (issue.project_id IS NULL AND sqlc.arg(scope_mode)::text <> 'guest')
      OR EXISTS (SELECT 1 FROM project_member scope_pm
                 WHERE scope_pm.project_id = issue.project_id AND scope_pm.user_id = sqlc.arg(scope_user_id)::uuid)
      OR (sqlc.arg(scope_mode)::text = 'member'
          AND EXISTS (SELECT 1 FROM project scope_p WHERE scope_p.id = issue.project_id AND scope_p.access = 'workspace')))
ORDER BY due_date ASC, id ASC
LIMIT 50;

-- name: CountOpenSecretaryRequests :one
-- Requests in the Secretary desk still open. A child of a request (a directive or
-- an addressee) has a parent, so only top-level issues count. Guardrail: spec 13.7.1.
SELECT count(*)::bigint
FROM issue
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND parent_issue_id IS NULL
  AND status NOT IN ('done', 'cancelled');
