SET LOCAL lock_timeout = '2s';

ALTER TABLE issue_status_transition ALTER COLUMN created_at SET DEFAULT now();
