-- Which criterion a verification task was asked to check (durable spec 2.5, A3).
-- One row per (outcome, criterion) that an agent verifier takes on. The row is
-- written with the task in the same transaction, so a criterion with a row is
-- never assigned twice. Artifact and metric criteria, and checks that found no
-- verifier, have no row here. No foreign keys (project rule).
CREATE TABLE outcome_check_task (
    workspace_id UUID NOT NULL,
    outcome_id UUID NOT NULL,
    criterion_id UUID NOT NULL,
    task_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
