-- platform_runtime: a runtime staff disabled. The platform writes it; Delegora only
-- reads it, in the daemon guard and the claim gate (spec R6 section 3b.5, D98).

-- name: GetPlatformRuntimeDisabled :one
SELECT EXISTS (SELECT 1 FROM platform_runtime WHERE runtime_id = $1);
