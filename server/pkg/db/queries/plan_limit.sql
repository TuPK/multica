-- name: LockPlanGate :exec
-- Serialises the creations a plan quota or an account limit counts. The lock is
-- held until the calling transaction ends, so the count that follows sees
-- everything an earlier holder committed. Scope is a workspace id for a
-- resource gate and a user id for the owner limit.
SELECT pg_advisory_xact_lock(hashtext(@scope::text), hashtext(@gate::text));

-- name: CountPlanLimitAgents :one
-- Agents that take a slot of the `agents` gate: not archived, user agents.
SELECT count(*)::bigint FROM agent
WHERE workspace_id = $1 AND archived_at IS NULL AND kind = 'user';

-- name: CountPlanLimitRuntimes :one
SELECT count(*)::bigint FROM agent_runtime WHERE workspace_id = $1;

-- name: SumPlanLimitAttachmentBytes :one
SELECT COALESCE(sum(size_bytes), 0)::bigint FROM attachment WHERE workspace_id = $1;

-- name: ListOwnedWorkspacePlanKeys :many
-- One row per workspace the user owns: the plan key of its subscription, or ''
-- when it has no subscription row (the free plan).
SELECT COALESCE(s.plan_key, '')::text AS plan_key
FROM member m
LEFT JOIN workspace_subscription s ON s.workspace_id = m.workspace_id
WHERE m.user_id = $1 AND m.role = 'owner';

-- name: OwnerHasUnpaidUsageInvoice :one
-- Whether any workspace the user owns has a usage invoice still pending: a debt
-- tied to the workspace (platform spec 7b.3, rule 13).
SELECT EXISTS (
    SELECT 1
    FROM member m
    JOIN billing_order o ON o.workspace_id = m.workspace_id
    WHERE m.user_id = $1 AND m.role = 'owner'
      AND o.kind = 'usage' AND o.status = 'pending'
);
