-- The nightly track record job lists the agent pairs with a settled outcome in the last
-- 90 days (durable spec 3.2). Partial, so it covers settled agent outcomes only.
CREATE INDEX CONCURRENTLY IF NOT EXISTS work_outcome_settled_agent_at_idx
    ON work_outcome (settled_at)
    WHERE settled_at IS NOT NULL AND agent_id IS NOT NULL;
