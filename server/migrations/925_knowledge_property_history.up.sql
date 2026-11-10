-- Who changed which property of a knowledge document, and when (spec 7.1). One row per
-- change. No primary key and no foreign keys: an implicit index would not be built
-- CONCURRENTLY, and the link is checked in the application, which also removes its history.
CREATE TABLE IF NOT EXISTS knowledge_property_history (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    link_id UUID NOT NULL,
    actor_type TEXT NOT NULL CHECK (actor_type IN ('agent', 'member', 'system')),
    actor_id UUID,
    changed_keys TEXT[] NOT NULL DEFAULT '{}',
    changes JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
