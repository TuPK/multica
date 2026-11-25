-- name: SetIssueActorSession :exec
-- Sets the actor that the issue status trigger records (migration 1044). The
-- values end with the transaction, so run this inside the transaction that
-- changes the issue. Use service.WithActorSession, which also checks that.
SELECT
    set_config('multica.actor_type', sqlc.arg(actor_type)::text, true),
    set_config('multica.actor_id', sqlc.arg(actor_id)::text, true),
    set_config('multica.task_id', sqlc.arg(task_id)::text, true);

-- name: CurrentIssueActorType :one
SELECT COALESCE(current_setting('multica.actor_type', true), '')::text AS actor_type;

-- name: ListIssueStatusTransitions :many
-- Categories are joined from issue_status here, not stored in the row (D32). A
-- built-in key with no catalog row comes back NULL; the Go side maps it
-- (LifecycleCategoryOf).
SELECT
    t.id, t.workspace_id, t.issue_id, t.from_status_key, t.to_status_key,
    t.actor_type, t.actor_id, t.task_id, t.source, t.created_at,
    fs.category AS from_catalog_category,
    ts.category AS to_catalog_category
FROM issue_status_transition t
LEFT JOIN issue_status fs ON fs.workspace_id = t.workspace_id AND fs.key = t.from_status_key
LEFT JOIN issue_status ts ON ts.workspace_id = t.workspace_id AND ts.key = t.to_status_key
WHERE t.workspace_id = sqlc.arg(workspace_id) AND t.issue_id = sqlc.arg(issue_id)
ORDER BY t.created_at;

-- name: LatestDoneTransition :one
-- A transition into done: the catalog category is done, or there is no catalog
-- row and the key is the built-in done (the only built-in key in that category).
SELECT t.id, t.workspace_id, t.issue_id, t.from_status_key, t.to_status_key,
    t.actor_type, t.actor_id, t.task_id, t.source, t.created_at
FROM issue_status_transition t
LEFT JOIN issue_status ts ON ts.workspace_id = t.workspace_id AND ts.key = t.to_status_key
WHERE t.issue_id = sqlc.arg(issue_id)
    AND (ts.category = 'done' OR (ts.category IS NULL AND t.to_status_key = 'done'))
ORDER BY t.created_at DESC
LIMIT 1;
