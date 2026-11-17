CREATE INDEX CONCURRENTLY IF NOT EXISTS einvoice_issuance_due_idx ON einvoice_issuance (next_attempt_at, id) WHERE state = 'pending';
