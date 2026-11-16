ALTER TABLE workspace_subscription
    DROP COLUMN IF EXISTS pending_change,
    DROP COLUMN IF EXISTS unit_price;

ALTER TABLE plan
    DROP COLUMN IF EXISTS audience,
    DROP COLUMN IF EXISTS billing_model,
    DROP COLUMN IF EXISTS usage_rates,
    DROP COLUMN IF EXISTS usage_terms,
    DROP COLUMN IF EXISTS version;
