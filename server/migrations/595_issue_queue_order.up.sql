-- Per-issue `queue_order`: the position of a backlog issue in an automatic run
-- queue. Issues sharing a project (or sharing no project) and a number start
-- together; a number starts once every lower-numbered issue in the same scope
-- has finished. NULL = not queued. The value is kept after the issue leaves
-- backlog because later numbers keep waiting on it.
ALTER TABLE issue ADD COLUMN queue_order INTEGER CHECK (queue_order IS NULL OR queue_order >= 1);
