-- name: CountDirectiveIssuesForMessage :one
-- Directives already filed from one boss chat message. The handler compares
-- this against the per-message cap before it creates another.
SELECT count(*)::bigint FROM issue
WHERE workspace_id = $1 AND origin_type = 'directive' AND origin_id = $2;

-- name: ListBlockedDirectivesPastDue :many
-- Directives the leader has marked blocked after their due date. The digest
-- reads them so the boss sees a stuck directive without opening each one.
SELECT id, number, title, due_date
FROM issue
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
  AND origin_type = 'directive'
  AND status = 'blocked'
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
