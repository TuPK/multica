CREATE TABLE IF NOT EXISTS platform_usage_daily (
    workspace_id uuid NOT NULL,
    day date NOT NULL,
    members int NOT NULL DEFAULT 0,
    agents int NOT NULL DEFAULT 0,
    runtimes int NOT NULL DEFAULT 0,
    issues_total int NOT NULL DEFAULT 0,
    issues_created int NOT NULL DEFAULT 0,
    tasks_run int NOT NULL DEFAULT 0,
    storage_bytes bigint NOT NULL DEFAULT 0,
    computed_at timestamptz NOT NULL DEFAULT now()
);
