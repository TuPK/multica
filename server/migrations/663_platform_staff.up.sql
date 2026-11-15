CREATE TABLE IF NOT EXISTS platform_staff (
    user_id uuid NOT NULL,
    role text NOT NULL,
    created_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);
