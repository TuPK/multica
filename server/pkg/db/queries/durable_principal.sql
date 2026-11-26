-- The principal profile (durable spec 6.2, F5). Every query filters by workspace_id and
-- member_id; there are no foreign keys. Evidence checks read the decision, comment or chat
-- message the trait cites and ask whether the member wrote or decided it.

-- name: GetPrincipalProfile :one
SELECT * FROM principal_profile
WHERE workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id);

-- name: UpsertPrincipalProfile :one
-- Consent turns the profile on. Consenting again keeps the profile and its version moving.
INSERT INTO principal_profile (workspace_id, member_id, status, consent_at, share_with_owners)
VALUES (sqlc.arg(workspace_id), sqlc.arg(member_id), 'active', now(), sqlc.arg(share_with_owners))
ON CONFLICT (workspace_id, member_id) DO UPDATE SET
    status = 'active',
    consent_at = now(),
    share_with_owners = EXCLUDED.share_with_owners,
    version = principal_profile.version + 1,
    updated_at = now()
RETURNING *;

-- name: BumpPrincipalProfileVersion :execrows
UPDATE principal_profile SET version = version + 1, updated_at = now()
WHERE workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id);

-- name: DeletePrincipalProfile :execrows
DELETE FROM principal_profile
WHERE workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id);

-- name: DeletePrincipalTraits :execrows
DELETE FROM principal_trait
WHERE workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id);

-- name: InsertPrincipalTrait :one
INSERT INTO principal_trait (id, workspace_id, member_id, decision_kind, statement, evidence, origin)
VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(member_id), sqlc.arg(decision_kind),
    sqlc.arg(statement), sqlc.arg(evidence)::jsonb, sqlc.arg(origin)
)
RETURNING *;

-- name: GetPrincipalTrait :one
SELECT * FROM principal_trait
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id);

-- name: ListActivePrincipalTraits :many
SELECT * FROM principal_trait
WHERE workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id) AND status = 'active'
ORDER BY created_at, id;

-- name: CountActivePrincipalTraits :one
SELECT COUNT(*)::int FROM principal_trait
WHERE workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id) AND status = 'active';

-- name: RetirePrincipalTrait :execrows
UPDATE principal_trait SET status = 'retired', retired_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id)
  AND status = 'active';

-- name: EditPrincipalTraitStatement :one
-- The person's own edit: the sentence changes and the trait is marked edited, so a job never
-- retires or rewrites it (spec 6.3).
UPDATE principal_trait SET statement = sqlc.arg(statement), origin = 'edited'
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND member_id = sqlc.arg(member_id)
  AND status = 'active'
RETURNING *;

-- name: IsOwnProposalDecision :one
-- A proposal the member decided in this workspace.
SELECT EXISTS (
    SELECT 1 FROM proposal p
    WHERE p.id = sqlc.arg(id) AND p.workspace_id = sqlc.arg(workspace_id) AND p.decided_by = sqlc.arg(member_id)
) AS owned;

-- name: IsOwnComment :one
-- A comment the member wrote in this workspace's issues.
SELECT EXISTS (
    SELECT 1 FROM comment c
    WHERE c.id = sqlc.arg(id) AND c.workspace_id = sqlc.arg(workspace_id)
      AND c.author_type = 'member' AND c.author_id = sqlc.arg(member_id)
) AS owned;

-- name: IsOwnChatMessage :one
-- A message the member sent in one of their own chat sessions in this workspace.
SELECT EXISTS (
    SELECT 1 FROM chat_message m
    JOIN chat_session s ON s.id = m.chat_session_id
    WHERE m.id = sqlc.arg(id) AND s.workspace_id = sqlc.arg(workspace_id)
      AND s.creator_id = sqlc.arg(member_id) AND m.role = 'user'
) AS owned;

-- name: GetWorkspaceSecretaryAgentID :one
SELECT secretary_agent_id FROM workspace WHERE id = sqlc.arg(workspace_id);

-- name: GetPrincipalInterviewSession :one
-- The interview's chat with the Secretary, which the person started (durable spec 6.3).
SELECT * FROM chat_session
WHERE workspace_id = sqlc.arg(workspace_id) AND creator_id = sqlc.arg(member_id)
  AND agent_id = sqlc.arg(agent_id) AND title = sqlc.arg(title)
ORDER BY created_at DESC
LIMIT 1;

-- name: DeleteWorkspacePrincipalTraits :execrows
-- Workspace teardown (durable spec 6.2). The principal tables carry no foreign key, so the teardown
-- removes the workspace's rows itself, one table per query.
DELETE FROM principal_trait WHERE workspace_id = sqlc.arg(workspace_id);

-- name: DeleteWorkspacePrincipalProfiles :execrows
DELETE FROM principal_profile WHERE workspace_id = sqlc.arg(workspace_id);
