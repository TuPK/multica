-- usage_charge: the charge of one workspace, UTC day and meter on a pay as you
-- go plan (platform management spec 7b.3, rules 7 to 10), written by the
-- platform's usage_rating job. One row per (workspace, day, meter) is enforced
-- by the unique index in migration 962. order_id stays empty until the day is
-- invoiced; its partial index is migration 963. Each row keeps the price it was
-- rated at (unit_price, per, discount_pct, plan_key), so a later price change
-- never moves a charged day.
CREATE TABLE IF NOT EXISTS usage_charge (
    workspace_id uuid NOT NULL,
    day date NOT NULL,
    meter text NOT NULL,
    quantity bigint NOT NULL DEFAULT 0,
    plan_key text NOT NULL,
    unit_price bigint NOT NULL DEFAULT 0,
    per bigint NOT NULL DEFAULT 1,
    discount_pct integer NOT NULL DEFAULT 0,
    net_amount bigint NOT NULL DEFAULT 0,
    note text NOT NULL DEFAULT '',
    order_id uuid,
    rated_at timestamptz NOT NULL DEFAULT now()
);
