-- Workspace overrides of the effect policy (durable spec 4.3, B1). Saves upsert
-- one cell; the decision is validated in Go (ValidateOverride) before it reaches
-- here.

-- name: ListEffectPolicyOverrides :many
SELECT workspace_id, autonomy_level, effect_class, reversibility, decision, set_by, created_at
FROM effect_policy_override
WHERE workspace_id = sqlc.arg('workspace_id')::uuid
ORDER BY autonomy_level, effect_class, reversibility;

-- name: UpsertEffectPolicyOverride :exec
INSERT INTO effect_policy_override (workspace_id, autonomy_level, effect_class, reversibility, decision, set_by)
VALUES (
    sqlc.arg('workspace_id')::uuid,
    sqlc.arg('autonomy_level')::integer,
    sqlc.arg('effect_class')::text,
    sqlc.arg('reversibility')::text,
    sqlc.arg('decision')::text,
    sqlc.arg('set_by')::uuid
)
ON CONFLICT (workspace_id, autonomy_level, effect_class, reversibility) DO UPDATE SET
    decision = EXCLUDED.decision,
    set_by = EXCLUDED.set_by,
    created_at = now();
