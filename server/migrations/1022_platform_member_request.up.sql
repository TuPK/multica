-- platform_member_request: a staff member's request to add a person to a
-- workspace (spec R6 section 4), or later to remove or re-role one (kind). The
-- platform writes the request and cancels it; Multica's member_provisioning job
-- writes state, error_code, attempts, user_id, member_id, user_created,
-- updated_at and completed_at. The unique indexes are migrations 1023 and 1024.
CREATE TABLE IF NOT EXISTS platform_member_request (
    id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    kind text NOT NULL DEFAULT 'add',
    target_user_id uuid,
    previous_role text NOT NULL DEFAULT '',
    email text NOT NULL,
    role text NOT NULL,
    state text NOT NULL DEFAULT 'pending',
    error_code text NOT NULL DEFAULT '',
    attempts int NOT NULL DEFAULT 0,
    user_id uuid,
    member_id uuid,
    user_created boolean NOT NULL DEFAULT false,
    requested_by uuid NOT NULL,
    requested_by_label text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);
