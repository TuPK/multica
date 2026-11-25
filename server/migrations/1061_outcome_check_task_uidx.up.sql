CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS outcome_check_task_outcome_criterion_uidx
    ON outcome_check_task (outcome_id, criterion_id);
