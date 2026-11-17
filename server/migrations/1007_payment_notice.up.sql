CREATE TABLE IF NOT EXISTS payment_notice (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    provider text NOT NULL,
    provider_event_id text NOT NULL,
    state text NOT NULL,
    review_reason text NOT NULL DEFAULT '',
    transfer_code text NOT NULL DEFAULT '',
    amount bigint NOT NULL DEFAULT 0,
    currency text NOT NULL DEFAULT '',
    order_id text NOT NULL DEFAULT '',
    body_sha256 text NOT NULL DEFAULT '',
    received_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz,
    decided_by text NOT NULL DEFAULT '',
    decision_reason text NOT NULL DEFAULT ''
);
