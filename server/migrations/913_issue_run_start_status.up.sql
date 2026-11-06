-- The status a queued backlog issue moves to when its waits are over. todo keeps
-- the previous behavior; a planning issue names the status of its owner (for
-- example analysis), so the queue starts every kind of work, not only the build.
-- Plain column, no foreign key: the key is checked against the workflow when the
-- issue is queued.
ALTER TABLE issue ADD COLUMN run_start_status TEXT NOT NULL DEFAULT 'todo';
