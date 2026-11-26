-- One primary task gives at most one pair. A second claim of the same task (a retry, a
-- reclaim) finds the pair and enqueues nothing.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS shadow_pair_primary_task_uidx ON shadow_pair (primary_task_id);
