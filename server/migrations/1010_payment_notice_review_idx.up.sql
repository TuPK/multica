CREATE INDEX CONCURRENTLY IF NOT EXISTS payment_notice_review_idx ON payment_notice (received_at, id) WHERE state = 'needs_review';
