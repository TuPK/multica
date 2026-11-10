-- name: SetKnowledgeLinkProperties :one
-- Replaces the typed properties of a document. The caller has merged and validated them
-- against the kind's fields; the document's content and state are not touched.
UPDATE knowledge_link SET properties = @properties::jsonb
WHERE id = @id::uuid AND workspace_id = @workspace_id::uuid
RETURNING *;

-- name: InsertKnowledgePropertyHistory :exec
INSERT INTO knowledge_property_history (workspace_id, link_id, actor_type, actor_id, changed_keys, changes)
VALUES (@workspace_id::uuid, @link_id::uuid, @actor_type, sqlc.narg('actor_id')::uuid, @changed_keys::text[], @changes::jsonb);

-- name: ListKnowledgePropertyHistory :many
-- The changes to one document, newest first. Bounded: a history page is for reading, not export.
SELECT * FROM knowledge_property_history
WHERE workspace_id = @workspace_id::uuid AND link_id = @link_id::uuid
ORDER BY created_at DESC, id DESC
LIMIT 50;

-- name: LastKnowledgePropertyActorType :one
-- Who last changed a property key of a document: a person or an agent. Used to stop an agent
-- from moving a stage backwards after a person set it.
SELECT actor_type FROM knowledge_property_history
WHERE workspace_id = @workspace_id::uuid AND link_id = @link_id::uuid AND @key::text = ANY(changed_keys)
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: ListKnowledgeCollection :many
-- The published documents of a workspace that carry properties, for the Collections view.
-- Each filter is optional: a kind, a base, a containment match on the properties (a JSON
-- object such as {"stage": "qualified"}), and a date key that must be on or before a day.
-- Dates are ISO YYYY-MM-DD text, so a text comparison orders them. Newest first, bounded.
SELECT * FROM knowledge_link
WHERE workspace_id = @workspace_id::uuid
  AND state = 'published'
  AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind')::text)
  AND (sqlc.narg('knowledge_base_id')::uuid IS NULL OR knowledge_base_id = sqlc.narg('knowledge_base_id')::uuid)
  AND (sqlc.narg('match')::jsonb IS NULL OR properties @> sqlc.narg('match')::jsonb)
  AND (sqlc.narg('due_key')::text IS NULL OR (properties ->> sqlc.narg('due_key')::text) <= sqlc.narg('due_on')::text)
ORDER BY created_at DESC, id DESC
LIMIT @max_rows::int;

-- name: SetKnowledgeKindFields :execrows
-- Brings a kind's declared fields up to the default. Only a kind with no fields yet is set, so
-- a field an admin changed is kept.
UPDATE knowledge_kind SET fields = @fields::jsonb, updated_at = now()
WHERE workspace_id = @workspace_id::uuid AND key = @key AND fields = '[]'::jsonb;
