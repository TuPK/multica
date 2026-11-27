-- Allow the transient undoing state on effect_event so concurrent undo requests
-- claim the row before invoking an external inverse tool (W1-2).
ALTER TABLE effect_event DROP CONSTRAINT IF EXISTS effect_event_status_check;
ALTER TABLE effect_event ADD CONSTRAINT effect_event_status_check
    CHECK (status IN ('executed', 'blocked', 'awaiting', 'failed', 'undoing', 'undone', 'declared'));
