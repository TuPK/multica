-- The platform's ledger of seat operations (spec 5.6, Appendix B). One row per
-- operation token. No primary key and no foreign key: the unique index on
-- (workspace_id, operation_token) is built in 675.
CREATE TABLE IF NOT EXISTS platform_seat_operation (
    workspace_id    uuid        NOT NULL,
    operation_token uuid        NOT NULL,
    kind            text        NOT NULL,
    subject_id      uuid,
    state           text        NOT NULL,
    reason          text        NOT NULL DEFAULT '',
    expires_at      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
