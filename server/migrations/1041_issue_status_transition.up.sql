-- Issue status transition history (durable spec 2.2, D3). One row per change of
-- issue.status, written by the trigger in migration 1044 so that every writer,
-- including plain SQL from sweepers, leaves exactly one row.
--
-- The row stores status keys only. The lifecycle category of a key is derived
-- when the row is read (queries join issue_status; the Go fallback covers
-- built-in keys without a catalog row). A migration sorted before the table that
-- holds categories must not reference it (D32), so no category is stored here.
--
-- No foreign keys (project rule). Rows are not removed when an issue is deleted;
-- the application owns that cleanup.
CREATE TABLE issue_status_transition (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    issue_id UUID NOT NULL,
    from_status_key TEXT,
    to_status_key TEXT NOT NULL,
    actor_type TEXT NOT NULL CHECK (actor_type IN ('member', 'agent', 'system')),
    actor_id UUID,
    task_id UUID,
    source TEXT NOT NULL DEFAULT 'live' CHECK (source IN ('live', 'activity_backfill')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
