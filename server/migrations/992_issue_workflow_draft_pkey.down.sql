-- Dropping the constraint drops the index it took over, leaving 991's down
-- direction a no-op via IF EXISTS.
ALTER TABLE issue_workflow_draft DROP CONSTRAINT IF EXISTS issue_workflow_draft_pkey;
