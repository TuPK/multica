CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS issue_status_transition_seq_uidx
    ON issue_status_transition (seq);
