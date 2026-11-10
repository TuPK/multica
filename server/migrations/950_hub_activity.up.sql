-- Audit trail of skill and plugin hub actions (spec 7.4). Append-only: rows are
-- written by the hub handlers and read by the activity feed. No primary key and
-- no foreign keys, for the same reason as hub_source (927): an implicit
-- primary-key index would not be built CONCURRENTLY, and ids are checked in the
-- application. Deleting a source or item leaves its history in place.
CREATE TABLE IF NOT EXISTS hub_activity (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    actor_type TEXT NOT NULL DEFAULT 'member' CHECK (actor_type IN ('member', 'agent', 'system')),
    actor_id UUID,
    action TEXT NOT NULL CHECK (char_length(action) BETWEEN 1 AND 64),
    kind TEXT CHECK (kind IN ('skill', 'plugin')),
    ref_id UUID,
    item_name TEXT NOT NULL DEFAULT '' CHECK (char_length(item_name) <= 512),
    source_id UUID,
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
