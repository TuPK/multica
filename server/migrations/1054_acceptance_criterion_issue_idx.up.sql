CREATE INDEX CONCURRENTLY IF NOT EXISTS acceptance_criterion_issue_position_idx
    ON acceptance_criterion (issue_id, position);
