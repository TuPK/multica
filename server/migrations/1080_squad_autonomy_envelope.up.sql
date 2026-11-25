-- Autonomy envelope (durable spec 3.4, A7): the floor and the ceiling the boss sets for a
-- squad's autonomy level. The machine moves the level only inside it, on evidence. A squad
-- with no row has no envelope and never changes level by itself.
--
-- No foreign keys (project rule). The unique key is a separate index, built concurrently
-- in 1081.
CREATE TABLE squad_autonomy_envelope (
    workspace_id UUID NOT NULL,
    squad_id UUID NOT NULL,
    floor_level INTEGER NOT NULL CHECK (floor_level BETWEEN 0 AND 3),
    ceiling_level INTEGER NOT NULL CHECK (ceiling_level BETWEEN 0 AND 3),
    set_by UUID NOT NULL,
    reason TEXT NOT NULL DEFAULT '' CHECK (char_length(reason) <= 500),
    last_auto_change_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (floor_level <= ceiling_level)
);
