-- Workspace overrides of the effect policy table (durable spec 4.3). One row per
-- cell the workspace changed; the code default stands for every other cell. The
-- cell is (autonomy level, effect class, reversibility), unique in 1071. created_at
-- is when the cell's current decision was set. No foreign keys (project rule).
CREATE TABLE effect_policy_override (
    workspace_id UUID NOT NULL,
    autonomy_level INTEGER NOT NULL CHECK (autonomy_level BETWEEN 0 AND 3),
    effect_class TEXT NOT NULL CHECK (effect_class IN
        ('read', 'write_internal', 'write', 'send', 'share', 'delete', 'spend')),
    reversibility TEXT NOT NULL CHECK (reversibility IN
        ('reversible', 'compensable', 'irreversible')),
    decision TEXT NOT NULL CHECK (decision IN ('allow', 'confirm', 'rehearse', 'block')),
    set_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
