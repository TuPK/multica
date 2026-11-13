-- One task issue per (request, step, task, round): concurrent engine runs cannot open the same task twice.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_flow_task_once
    ON issue (parent_issue_id, flow_step, flow_task, flow_round)
    WHERE flow_task IS NOT NULL;
