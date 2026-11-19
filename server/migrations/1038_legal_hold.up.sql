-- legal_hold: a hold on a tenant, a workspace or an account (spec R6 section 3c.7,
-- D104). A hold has no end until a system_admin releases it. The platform writes
-- it and Multica reads it at its deletion points. The one-live-hold index is
-- migration 1039. The table keeps no unique index on id: rows are found by scope.
CREATE TABLE IF NOT EXISTS legal_hold (
    id uuid NOT NULL,
    scope_kind text NOT NULL CHECK (scope_kind IN ('tenant', 'workspace', 'account')),
    scope_id uuid NOT NULL,
    reason text NOT NULL,
    case_ref text NOT NULL,
    placed_by uuid,
    placed_at timestamptz NOT NULL DEFAULT now(),
    released_by uuid,
    released_at timestamptz
);
