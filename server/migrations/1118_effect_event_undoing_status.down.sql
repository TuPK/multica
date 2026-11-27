UPDATE effect_event SET status = 'executed' WHERE status = 'undoing';
ALTER TABLE effect_event DROP CONSTRAINT IF EXISTS effect_event_status_check;
ALTER TABLE effect_event ADD CONSTRAINT effect_event_status_check
    CHECK (status IN ('executed', 'blocked', 'awaiting', 'failed', 'undone', 'declared'));
