-- TOTP second factor, one row per user. secret_enc is sealed with
-- MULTICA_MFA_SECRET_KEY. The row is a pending enrolment until confirmed_at is
-- set. No primary key or foreign key: 999 adds the unique index on user_id.
CREATE TABLE user_mfa (
    user_id uuid NOT NULL,
    secret_enc bytea NOT NULL,
    confirmed_at timestamptz,
    recovery_hashes text[] NOT NULL DEFAULT '{}',
    last_used_step bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);
