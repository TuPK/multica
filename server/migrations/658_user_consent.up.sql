CREATE TABLE user_consent (
    user_id uuid NOT NULL,
    document text NOT NULL,
    version text NOT NULL,
    accepted_at timestamptz NOT NULL DEFAULT now(),
    ip text NOT NULL DEFAULT ''
);
