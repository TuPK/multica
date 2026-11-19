-- quota_request: a request to raise one plan gate of a workspace (spec R6 section
-- 3c.5, D102). The platform owns it; Multica writes a row when an owner asks. The
-- unique indexes are migrations 1036 (id) and 1037 (one pending per workspace and gate).
CREATE TABLE IF NOT EXISTS quota_request (
    id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    tenant_id uuid,
    gate text NOT NULL,
    current_value bigint,
    requested_value bigint NOT NULL,
    approved_value bigint,
    state text NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending', 'approved', 'denied', 'canceled')),
    reason text NOT NULL DEFAULT '',
    decision_reason text NOT NULL DEFAULT '',
    requested_by uuid,
    requested_by_label text NOT NULL DEFAULT '',
    decided_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz
);
