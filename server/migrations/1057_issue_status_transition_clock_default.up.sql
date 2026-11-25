-- created_at of a transition is the time the row was written, not the start of the
-- writing transaction (durable spec 2.4, A2). now() would order a long transaction
-- before rows committed earlier, and the outcome job reads by created_at, so a late
-- commit could land behind the cursor. The trigger does not set created_at, so the
-- column default applies to its rows.
--
-- Rows written before this migration keep their values. Migration 1041 created the
-- column, so this four-digit migration only touches an earlier table (D32).
SET LOCAL lock_timeout = '2s';

ALTER TABLE issue_status_transition ALTER COLUMN created_at SET DEFAULT clock_timestamp();
