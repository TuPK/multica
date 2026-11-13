-- Attach the CONCURRENTLY-built unique index as the table's primary key.
ALTER TABLE issue_workflow_draft
    ADD CONSTRAINT issue_workflow_draft_pkey PRIMARY KEY USING INDEX issue_workflow_draft_pkey_uidx;
