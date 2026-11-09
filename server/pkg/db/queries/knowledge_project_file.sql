-- name: ListKnowledgeProjectFiles :many
SELECT * FROM knowledge_project_file WHERE workspace_id = $1 AND project_id = $2;

-- name: UpsertKnowledgeProjectFile :exec
INSERT INTO knowledge_project_file (workspace_id, project_id, path, knowledge_link_id, content_hash, archived_at, synced_at)
VALUES ($1, $2, $3, $4, $5, NULL, now())
ON CONFLICT (workspace_id, project_id, path) DO UPDATE SET
    knowledge_link_id = EXCLUDED.knowledge_link_id,
    content_hash = EXCLUDED.content_hash,
    archived_at = NULL,
    synced_at = now();

-- name: ArchiveKnowledgeProjectFile :exec
UPDATE knowledge_project_file SET archived_at = now(), knowledge_link_id = NULL
WHERE workspace_id = $1 AND project_id = $2 AND path = $3;

-- name: DeleteWorkspaceKnowledgeProjectFiles :exec
DELETE FROM knowledge_project_file WHERE workspace_id = @workspace_id;

-- name: ListProjectDocSources :many
-- Local directories attached to live projects of a workspace. The daemon picks
-- the ones pinned to it.
SELECT pr.id AS resource_id, pr.project_id, p.title AS project_title, pr.resource_ref
FROM project_resource pr
JOIN project p ON p.id = pr.project_id AND p.workspace_id = pr.workspace_id
WHERE pr.workspace_id = $1 AND pr.resource_type = 'local_directory' AND p.status <> 'cancelled'
ORDER BY pr.created_at;
