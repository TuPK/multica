-- The effect log (durable spec 4.4): one row per action that has an effect the
-- platform sees, written by the gate before the action runs and settled after.
-- Append-only except three moves: status (awaiting to a final state, executed to
-- undone), undo and undo_until (set when the forward call succeeds), and intent_id
-- (set once, when a rehearsal creates its intent). TestEffectEventAppendOnly. Reversibility is '' for reads, which have
-- nothing to take back. Parameters are not stored; summary is the classifier's
-- one-line text. No foreign keys (project rule).
CREATE TABLE effect_event (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    task_id UUID,
    agent_id UUID,
    member_id UUID,
    source TEXT NOT NULL CHECK (source IN
        ('connected_app', 'plugin_action', 'cli', 'channel', 'webhook', 'publish_channel', 'declared')),
    target TEXT NOT NULL CHECK (char_length(target) <= 300),
    effect_class TEXT NOT NULL CHECK (effect_class IN
        ('read', 'write_internal', 'write', 'send', 'share', 'delete', 'spend')),
    reversibility TEXT NOT NULL CHECK (reversibility IN
        ('reversible', 'compensable', 'irreversible', '')),
    decision TEXT NOT NULL CHECK (decision IN ('allow', 'confirm', 'rehearse', 'block')),
    status TEXT NOT NULL CHECK (status IN ('executed', 'blocked', 'awaiting', 'failed', 'undone', 'declared')),
    summary TEXT NOT NULL DEFAULT '' CHECK (char_length(summary) <= 500),
    undo JSONB,
    undo_until TIMESTAMPTZ,
    intent_id UUID,
    app_tool_call_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
