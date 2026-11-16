-- plan_price_change: a plan's scheduled, applied and canceled price changes
-- (platform management spec 7b.4, decision D46). `old` and `new` hold the whole
-- price fields the change touches. A plan has at most one scheduled row: the
-- partial unique index is migration 965. The table has no other index; it
-- holds one row per price change, so a lookup by plan_key scans a few rows.
CREATE TABLE IF NOT EXISTS plan_price_change (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    plan_key text NOT NULL,
    state text NOT NULL DEFAULT 'scheduled',
    effective_from timestamptz NOT NULL,
    old jsonb NOT NULL DEFAULT '{}',
    new jsonb NOT NULL DEFAULT '{}',
    reason text NOT NULL DEFAULT '',
    notice_sent_at timestamptz,
    created_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    applied_at timestamptz,
    canceled_by uuid,
    canceled_at timestamptz
);
