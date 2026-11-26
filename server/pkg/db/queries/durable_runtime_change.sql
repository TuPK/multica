-- name: SetAgentRuntimeAndModel :one
-- A runtime change (durable spec 9.6) or its rollback. The seat keeps its other fields;
-- the caller has checked the workspace and what the change may name.
UPDATE agent
SET runtime_id = @runtime_id, model = sqlc.narg('model'), updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;
