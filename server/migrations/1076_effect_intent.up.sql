-- Rehearsed actions (durable spec 5.2, F4). A rehearse decision stores the exact call
-- the agent made and a preview of its result; a person approves the preview, and the
-- platform runs that call. Rows are not deleted: an intent leaves pending by one of
-- its state moves. No foreign keys (project rule).
CREATE TABLE effect_intent (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    task_id UUID,
    agent_id UUID,
    issue_id UUID,
    source TEXT NOT NULL CHECK (source IN
        ('connected_app', 'plugin_action', 'cli', 'channel', 'webhook', 'publish_channel', 'declared')),
    target TEXT NOT NULL CHECK (char_length(target) <= 300),
    effect_class TEXT NOT NULL CHECK (effect_class IN
        ('read', 'write_internal', 'write', 'send', 'share', 'delete', 'spend')),
    reversibility TEXT NOT NULL CHECK (reversibility IN
        ('reversible', 'compensable', 'irreversible', '')),
    call JSONB NOT NULL,
    call_sha256 TEXT NOT NULL,
    preview JSONB NOT NULL,
    rationale TEXT NOT NULL DEFAULT '' CHECK (char_length(rationale) <= 2000),
    predicted JSONB,
    state TEXT NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending', 'approved', 'rejected', 'expired', 'executed', 'failed', 'withdrawn')),
    decided_by UUID,
    decided_at TIMESTAMPTZ,
    decision_note TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    batch_key TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
