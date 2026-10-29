-- One row per "Quickstart" request from the web app. The server holds the
-- lifecycle (pending -> running -> completed|failed) so the request is shared
-- across API replicas. There are deliberately no foreign keys: the handler
-- checks runtime and workspace ownership.
CREATE TABLE IF NOT EXISTS runtime_quickstart_request (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    runtime_id UUID NOT NULL,
    workspace_id UUID NOT NULL,
    initiator_user_id UUID NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    steps JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(steps) = 'array'),
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id)
);
