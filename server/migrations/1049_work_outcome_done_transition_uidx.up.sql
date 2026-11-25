CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS work_outcome_done_transition_uidx
    ON work_outcome (done_transition_id);
