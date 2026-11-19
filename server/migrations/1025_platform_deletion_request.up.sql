-- platform_deletion_request: a staff member's request to delete a workspace or a
-- tenant now (spec R6 section 6), later an account or a runtime (kind). The
-- platform writes the request and cancels or retries it; Multica's
-- staff_deletion job writes state, error_code, attempts, target_label,
-- updated_at and completed_at. The unique indexes are migrations 1026 and 1027.
CREATE TABLE IF NOT EXISTS platform_deletion_request (
    id uuid NOT NULL,
    kind text NOT NULL,
    target_id uuid NOT NULL,
    target_label text NOT NULL DEFAULT '',
    reason text NOT NULL,
    state text NOT NULL DEFAULT 'pending',
    error_code text NOT NULL DEFAULT '',
    attempts int NOT NULL DEFAULT 0,
    approved_by uuid,
    approved_at timestamptz,
    export_first boolean NOT NULL DEFAULT false,
    notice_sent_at timestamptz,
    resolutions jsonb NOT NULL DEFAULT '{}'::jsonb,
    parent_id uuid,
    requested_by uuid NOT NULL,
    requested_by_label text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);
