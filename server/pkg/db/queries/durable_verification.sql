-- Acceptance criteria, verifications and the verification tasks of work outcomes
-- (durable spec 2.3, 2.5 to 2.7, A3). outcome_verification is append-only: no
-- statement here updates or deletes it (TestOutcomeAppendOnlyVerifications).

-- name: LockIssueForCriterionWrite :one
-- Serializes the open-criteria count and the insert of one issue. FOR NO KEY UPDATE
-- does not conflict with the FOR KEY SHARE that task enqueue takes on the issue, so
-- a criterion write never waits on an enqueue.
SELECT id FROM issue
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
FOR NO KEY UPDATE;

-- name: ListIssueAcceptanceCriteria :many
SELECT * FROM acceptance_criterion
WHERE workspace_id = sqlc.arg(workspace_id) AND issue_id = sqlc.arg(issue_id)
ORDER BY position, created_at, id;

-- name: CountOpenAcceptanceCriteria :one
SELECT count(*)::bigint FROM acceptance_criterion
WHERE workspace_id = sqlc.arg(workspace_id) AND issue_id = sqlc.arg(issue_id)
  AND archived_at IS NULL;

-- name: GetAcceptanceCriterion :one
SELECT * FROM acceptance_criterion
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND issue_id = sqlc.arg(issue_id);

-- name: InsertAcceptanceCriterion :one
INSERT INTO acceptance_criterion (
    id, workspace_id, issue_id, position, text, check_kind, check_spec, source, created_by_type, created_by_id
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(issue_id), sqlc.arg(position), sqlc.arg(text),
    sqlc.arg(check_kind), sqlc.arg(check_spec), sqlc.arg(source), sqlc.arg(created_by_type), sqlc.narg(created_by_id)
)
RETURNING *;

-- name: UpdateAcceptanceCriterionText :one
UPDATE acceptance_criterion SET text = sqlc.arg(text)
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND archived_at IS NULL
RETURNING *;

-- name: ArchiveAcceptanceCriterion :one
UPDATE acceptance_criterion SET archived_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id) AND archived_at IS NULL
RETURNING *;

-- name: CountCriterionChecksOnCurrentOutcome :one
-- Checks already written against the criterion's pending outcome. Such a criterion
-- cannot be edited: the verifier has judged the text it had.
SELECT count(*)::bigint
FROM outcome_verification v
JOIN work_outcome o ON o.id = v.outcome_id
WHERE v.criterion_id = sqlc.arg(criterion_id)
  AND o.workspace_id = sqlc.arg(workspace_id)
  AND o.superseded_by IS NULL
  AND o.settled_at IS NULL;

-- name: GetCurrentWorkOutcomeForIssue :one
SELECT * FROM work_outcome
WHERE workspace_id = sqlc.arg(workspace_id) AND issue_id = sqlc.arg(issue_id)
  AND superseded_by IS NULL
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetWorkOutcomeInWorkspace :one
SELECT * FROM work_outcome
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id);

-- name: ListWorkOutcomes :many
-- Newest first. The cursor is (created_at, id) of the last row of the previous page.
SELECT * FROM work_outcome
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(agent_id)::uuid IS NULL OR agent_id = sqlc.narg(agent_id)::uuid)
  AND (sqlc.narg(squad_id)::uuid IS NULL OR squad_id = sqlc.narg(squad_id)::uuid)
  AND (sqlc.narg(work_class)::text IS NULL OR work_class = sqlc.narg(work_class)::text)
  AND (sqlc.narg(verdict)::text IS NULL OR verdict = sqlc.narg(verdict)::text)
  AND (sqlc.narg(since)::timestamptz IS NULL OR created_at >= sqlc.narg(since)::timestamptz)
  AND (sqlc.narg(before_at)::timestamptz IS NULL
       OR (created_at, id) < (sqlc.narg(before_at)::timestamptz, sqlc.narg(before_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: SummarizeWorkOutcomes :many
-- One row per group. group_by is agent, squad or work_class; the caller checks it.
SELECT
    CASE sqlc.arg(group_by)::text
        WHEN 'agent' THEN COALESCE(agent_id::text, '')
        WHEN 'squad' THEN COALESCE(squad_id::text, '')
        ELSE work_class
    END::text AS group_key,
    count(*)::bigint AS total,
    count(*) FILTER (WHERE verdict = 'pass')::bigint AS passed,
    count(*) FILTER (WHERE verdict = 'partial')::bigint AS partial,
    count(*) FILTER (WHERE verdict = 'fail')::bigint AS failed,
    count(*) FILTER (WHERE verdict = 'unverified')::bigint AS unverified,
    count(*) FILTER (WHERE verdict = 'pending')::bigint AS pending
FROM work_outcome
WHERE workspace_id = sqlc.arg(workspace_id)
  AND created_at >= sqlc.arg(since)::timestamptz
GROUP BY group_key
ORDER BY total DESC, group_key;

-- name: ListOutcomeVerifications :many
SELECT * FROM outcome_verification
WHERE outcome_id = sqlc.arg(outcome_id) AND workspace_id = sqlc.arg(workspace_id)
ORDER BY created_at, id;

-- name: InsertOutcomeVerification :one
INSERT INTO outcome_verification (
    id, workspace_id, outcome_id, criterion_id, verifier_type, verifier_id, task_id,
    result, severity, note, evidence
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(outcome_id), sqlc.narg(criterion_id),
    sqlc.arg(verifier_type), sqlc.narg(verifier_id), sqlc.narg(task_id),
    sqlc.arg(result), sqlc.arg(severity), sqlc.arg(note), sqlc.arg(evidence)
)
RETURNING *;

-- name: ListLatestCriterionVerifications :many
-- The newest check of each criterion of one outcome. Criterion-less checks are the
-- conclusion of a person and are not read here.
SELECT DISTINCT ON (criterion_id) *
FROM outcome_verification
WHERE outcome_id = sqlc.arg(outcome_id) AND criterion_id IS NOT NULL
ORDER BY criterion_id, created_at DESC, id DESC;

-- name: ListOutcomeCheckTaskCriteria :many
-- Criteria of one outcome already assigned to a verification task.
SELECT criterion_id FROM outcome_check_task
WHERE outcome_id = sqlc.arg(outcome_id) AND workspace_id = sqlc.arg(workspace_id);

-- name: InsertOutcomeCheckTask :exec
INSERT INTO outcome_check_task (workspace_id, outcome_id, criterion_id, task_id)
VALUES (sqlc.arg(workspace_id), sqlc.arg(outcome_id), sqlc.arg(criterion_id), sqlc.arg(task_id))
ON CONFLICT (outcome_id, criterion_id) DO NOTHING;

-- name: GetOutcomeCheckTaskForCriterion :one
-- The verification task a criterion was assigned to. An agent may check the
-- criterion only through this task.
SELECT task_id FROM outcome_check_task
WHERE workspace_id = sqlc.arg(workspace_id) AND outcome_id = sqlc.arg(outcome_id)
  AND criterion_id = sqlc.arg(criterion_id);

-- name: ListSchedulableOutcomes :many
-- Current outcomes still open for checks, oldest first, after a (created_at, id) key.
-- The job pages through all of them: a page that stopped at the first rows would
-- starve the outcomes behind it.
SELECT * FROM work_outcome
WHERE settled_at IS NULL AND superseded_by IS NULL AND sampled
  AND (sqlc.narg(after_at)::timestamptz IS NULL
       OR (created_at, id) > (sqlc.narg(after_at)::timestamptz, sqlc.narg(after_id)::uuid))
ORDER BY created_at, id
LIMIT sqlc.arg(row_limit);

-- name: ListOpenSystemCriteria :many
-- Artifact or metric criteria of current open outcomes that no verification has
-- decided yet. The system checks read these, hourly for metrics.
SELECT c.id AS criterion_id, c.workspace_id, c.issue_id, c.check_kind, c.check_spec, c.text,
       c.created_at AS criterion_created_at,
       o.id AS outcome_id, o.settle_at
FROM acceptance_criterion c
JOIN work_outcome o ON o.issue_id = c.issue_id AND o.workspace_id = c.workspace_id
WHERE c.check_kind = sqlc.arg(check_kind)
  AND c.archived_at IS NULL
  AND o.superseded_by IS NULL
  AND o.settled_at IS NULL
  AND o.sampled
  AND NOT EXISTS (
      SELECT 1 FROM outcome_verification v WHERE v.outcome_id = o.id AND v.criterion_id = c.id
  )
ORDER BY o.created_at, c.id
LIMIT sqlc.arg(row_limit);

-- name: HasPublishedIssueKnowledge :one
-- An artifact criterion passes once a document of its kind, published from this issue,
-- exists (knowledge_link, migrations 620 and 751).
SELECT EXISTS (
    SELECT 1 FROM knowledge_link
    WHERE workspace_id = sqlc.arg(workspace_id)
      AND source_type = 'issue'
      AND source_id = sqlc.arg(issue_id)
      AND kind = sqlc.arg(kind)
      AND state = 'published'
);

-- name: LatestMetricReadingSince :one
-- The best company-scope reading of a metric since the criterion was written.
-- Returns NULL when no reading in the window reaches the value.
SELECT max(value)::numeric AS best FROM metric_reading
WHERE workspace_id = sqlc.arg(workspace_id)
  AND metric_key = sqlc.arg(metric_key)
  AND scope_type = 'company'
  AND observed_at >= sqlc.arg(since)::timestamptz;

-- name: SetWorkOutcomeOverride :execrows
-- A person's conclusion on a settled outcome, within the override window (spec 2.6).
UPDATE work_outcome
SET verdict = sqlc.arg(verdict), severity = sqlc.arg(severity), fail_cause = sqlc.arg(fail_cause),
    updated_at = now()
WHERE id = sqlc.arg(id) AND workspace_id = sqlc.arg(workspace_id)
  AND settled_at IS NOT NULL AND settled_at >= sqlc.arg(since)::timestamptz;

-- name: GetPendingAgentTaskOnIssue :one
SELECT * FROM agent_task_queue
WHERE issue_id = sqlc.arg(issue_id) AND agent_id = sqlc.arg(agent_id)
  AND status IN ('queued', 'dispatched', 'deferred')
ORDER BY created_at DESC
LIMIT 1;

-- name: SetAgentTaskDurableContext :execrows
-- Marks a verification task in its context, so the claim, the run class and the
-- briefing know what it is. Only a task not yet claimed is marked.
UPDATE agent_task_queue
SET context = COALESCE(context, '{}'::jsonb)
    || jsonb_build_object('durable_kind', 'verification', 'outcome_id', sqlc.arg(outcome_id)::text)
WHERE id = sqlc.arg(id)
  AND status IN ('queued', 'dispatched', 'deferred');
