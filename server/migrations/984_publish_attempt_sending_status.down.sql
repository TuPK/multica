-- Attempts still sending go back to queued, so the narrower check holds.
UPDATE publish_attempt SET status = 'queued' WHERE status = 'sending';
ALTER TABLE publish_attempt DROP CONSTRAINT IF EXISTS publish_attempt_status_check;
ALTER TABLE publish_attempt ADD CONSTRAINT publish_attempt_status_check
    CHECK (status IN ('queued', 'posted', 'failed'));
