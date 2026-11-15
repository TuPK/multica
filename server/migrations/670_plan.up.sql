CREATE TABLE IF NOT EXISTS plan (
    key text NOT NULL,
    name text NOT NULL,
    limits jsonb NOT NULL DEFAULT '{}',
    features jsonb NOT NULL DEFAULT '{}',
    currency text NOT NULL,
    seat_price_month bigint,
    seat_price_year bigint,
    seats_fixed int,
    min_seats int,
    trial_days int NOT NULL DEFAULT 0,
    is_public boolean NOT NULL DEFAULT false,
    sort int NOT NULL DEFAULT 0,
    archived_at timestamptz
);
