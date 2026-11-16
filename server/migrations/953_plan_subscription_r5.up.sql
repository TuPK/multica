-- R5 columns of the platform's plan and subscription tables (platform
-- management spec 7b.1). Every default makes a row written before this
-- migration read as a seat plan open to any workspace. No index, so nothing
-- here needs a concurrent build.
ALTER TABLE plan
    ADD COLUMN IF NOT EXISTS audience text NOT NULL DEFAULT 'any',
    ADD COLUMN IF NOT EXISTS billing_model text NOT NULL DEFAULT 'seat',
    ADD COLUMN IF NOT EXISTS usage_rates jsonb NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS usage_terms jsonb NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 0;

ALTER TABLE workspace_subscription
    ADD COLUMN IF NOT EXISTS pending_change jsonb,
    ADD COLUMN IF NOT EXISTS unit_price bigint;
