-- One ledger row per question. Admitting a question again (the check job runs each
-- minute) returns the row that exists, so it is never counted twice.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS attention_ledger_subject_uidx
    ON attention_ledger (subject_type, subject_id);
