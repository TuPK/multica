-- Enforced squad workflow engine: the state the server keeps for a request that
-- runs on a flow, and the flow's own configuration.
-- issue.outcome: the verdict a handler reported when it closed the issue.
-- issue.flow: the request's flow state, written only by the flow engine.
-- issue.flow_step, flow_task, flow_round: the identity of a task the engine opened
-- under a request; the unique index of 986 keeps one issue per identity.
-- No foreign keys, no CHECK: relationships and values are validated in code.
ALTER TABLE issue ADD COLUMN IF NOT EXISTS outcome TEXT;
ALTER TABLE issue ADD COLUMN IF NOT EXISTS flow JSONB;
ALTER TABLE issue ADD COLUMN IF NOT EXISTS flow_step TEXT;
ALTER TABLE issue ADD COLUMN IF NOT EXISTS flow_task TEXT;
ALTER TABLE issue ADD COLUMN IF NOT EXISTS flow_round INTEGER;
-- issue_workflow.flow_config: set only on a workflow created from a catalog flow.
-- template_ref: the catalog flow and revision that workflow came from; such a
-- workflow is read-only. flow_auto: the background runner may advance its requests.
ALTER TABLE issue_workflow ADD COLUMN IF NOT EXISTS flow_config JSONB;
ALTER TABLE issue_workflow ADD COLUMN IF NOT EXISTS template_ref TEXT;
ALTER TABLE issue_workflow ADD COLUMN IF NOT EXISTS flow_auto BOOLEAN NOT NULL DEFAULT false;
