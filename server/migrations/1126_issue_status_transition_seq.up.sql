-- Add a monotonic sequence to issue_status_transition so the outcome cursor
-- advances by sequence instead of created_at timestamp (V2.2).
ALTER TABLE issue_status_transition ADD COLUMN IF NOT EXISTS seq BIGSERIAL NOT NULL;
