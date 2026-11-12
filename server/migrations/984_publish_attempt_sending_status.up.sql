-- An attempt is 'sending' from the moment it is claimed until its result is
-- recorded. The status check gains that value. Dropping the check by name first
-- keeps this migration re-runnable.
ALTER TABLE publish_attempt DROP CONSTRAINT IF EXISTS publish_attempt_status_check;
ALTER TABLE publish_attempt ADD CONSTRAINT publish_attempt_status_check
    CHECK (status IN ('queued', 'sending', 'posted', 'failed'));
