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
  -- MUL-8472: private projects the caller cannot see.
  AND (sqlc.narg('hidden_project_ids')::uuid[] IS NULL OR NOT (COALESCE(issue.project_id, '00000000-0000-0000-0000-000000000000'::uuid) = ANY(sqlc.narg('hidden_project_ids')::uuid[])))
ORDER BY due_date ASC, id ASC
LIMIT 50;
