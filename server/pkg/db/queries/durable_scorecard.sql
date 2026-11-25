-- Scorecard and delivery readings from issue_status_transition (durable spec 2.2,
-- 2.9, A5). Done and category tests here mirror LifecycleCategoryOf: a catalog row
-- decides, and a built-in key with no row maps by table. Only the built-in "done"
-- key is in that category, so the catalog-or-done test below is exact. Started and
-- closed keys are resolved in Go (durable_scorecard_history_fork.go), not here.

-- name: DurableScorecardDoneIssues :many
-- Issues of a scorecard scope that entered done in the window. done_at is the
-- first transition into done in the window; an issue with no such transition but
-- whose stored done time (done_at, else updated_at) falls in the window is kept
-- with has_history = false: it was done before the transitions were recorded, so
-- the scorecard marks the period history_partial. last_member_comment_at is the
-- newest member comment; a person came back after done when it is later than done_at.
SELECT
    i.id,
    i.created_at,
    COALESCE(d.done_at, i.done_at, i.updated_at)::timestamptz AS done_at,
    (d.done_at IS NOT NULL)::boolean AS has_history,
    i.reopen_count,
    (SELECT COUNT(*) FROM agent_task_queue r WHERE r.issue_id = i.id AND r.status = 'completed')::bigint AS completed_runs,
    (SELECT MAX(c.created_at) FROM comment c WHERE c.issue_id = i.id AND c.author_type = 'member')::timestamptz AS last_member_comment_at
FROM issue i
LEFT JOIN issue_status st ON st.workspace_id = i.workspace_id AND st.key = i.status
LEFT JOIN LATERAL (
    SELECT MIN(t.created_at) AS done_at
    FROM issue_status_transition t
    LEFT JOIN issue_status ts ON ts.workspace_id = t.workspace_id AND ts.key = t.to_status_key
    WHERE t.issue_id = i.id
      AND t.workspace_id = i.workspace_id
      AND t.created_at >= @period_start::timestamptz
      AND t.created_at < @period_end::timestamptz
      AND (ts.category = 'done' OR (ts.category IS NULL AND t.to_status_key = 'done'))
) d ON TRUE
WHERE i.workspace_id = @workspace_id
  AND (
    d.done_at IS NOT NULL
    OR (
      (i.status = 'done' OR st.category = 'done')
      AND COALESCE(i.done_at, i.updated_at) >= @period_start::timestamptz
      AND COALESCE(i.done_at, i.updated_at) < @period_end::timestamptz
      AND NOT EXISTS (
        SELECT 1 FROM issue_status_transition t2
        LEFT JOIN issue_status ts2 ON ts2.workspace_id = t2.workspace_id AND ts2.key = t2.to_status_key
        WHERE t2.issue_id = i.id
          AND (ts2.category = 'done' OR (ts2.category IS NULL AND t2.to_status_key = 'done'))
      )
    )
  )
  -- MUL-8472: stored aggregate
  -- Stored aggregate: any member reads it, so issues of restricted projects are left out.
  AND (i.project_id IS NULL
       OR EXISTS (SELECT 1 FROM project scope_p WHERE scope_p.id = i.project_id AND scope_p.visibility = 'workspace'))
  AND (sqlc.narg('agent_id')::uuid IS NULL OR (i.assignee_type = 'agent' AND i.assignee_id = sqlc.narg('agent_id')::uuid))
  AND (sqlc.narg('squad_id')::uuid IS NULL OR (
        (i.assignee_type = 'squad' AND i.assignee_id = sqlc.narg('squad_id')::uuid)
        OR (i.assignee_type = 'agent' AND EXISTS (
            SELECT 1 FROM squad_member sm
            WHERE sm.squad_id = sqlc.narg('squad_id')::uuid
              AND sm.member_type = 'agent'
              AND sm.member_id = i.assignee_id))));

-- name: DurableScorecardTransitions :many
-- Every transition, oldest first, of each scope issue that has any transition in
-- the window. The window bounds which issues are read, not which transitions: the
-- cycle time needs the first start, which may precede the window. Categories are
-- joined from issue_status and resolved in Go (LifecycleCategoryOf).
SELECT
    t.issue_id,
    t.from_status_key,
    t.to_status_key,
    t.source,
    t.created_at,
    fs.category AS from_catalog_category,
    ts.category AS to_catalog_category
FROM issue_status_transition t
LEFT JOIN issue_status fs ON fs.workspace_id = t.workspace_id AND fs.key = t.from_status_key
LEFT JOIN issue_status ts ON ts.workspace_id = t.workspace_id AND ts.key = t.to_status_key
WHERE t.workspace_id = @workspace_id
  AND t.issue_id IN (
    SELECT t2.issue_id
    FROM issue_status_transition t2
    JOIN issue i ON i.id = t2.issue_id AND i.workspace_id = t2.workspace_id
    WHERE t2.workspace_id = @workspace_id
      AND t2.created_at >= @period_start::timestamptz
      AND t2.created_at < @period_end::timestamptz
      AND (i.project_id IS NULL
           OR EXISTS (SELECT 1 FROM project scope_p WHERE scope_p.id = i.project_id AND scope_p.visibility = 'workspace'))
      AND (sqlc.narg('agent_id')::uuid IS NULL OR (i.assignee_type = 'agent' AND i.assignee_id = sqlc.narg('agent_id')::uuid))
      AND (sqlc.narg('squad_id')::uuid IS NULL OR (
            (i.assignee_type = 'squad' AND i.assignee_id = sqlc.narg('squad_id')::uuid)
            OR (i.assignee_type = 'agent' AND EXISTS (
                SELECT 1 FROM squad_member sm
                WHERE sm.squad_id = sqlc.narg('squad_id')::uuid
                  AND sm.member_type = 'agent'
                  AND sm.member_id = i.assignee_id))))
  )
ORDER BY t.issue_id, t.created_at;

-- name: DurableSquadTransitions :many
-- The delivery readings' scope (BuiltinIssueDeliveryStats): squad-assigned issues,
-- restricted to squad_ids when given. Same shape as DurableScorecardTransitions.
SELECT
    t.issue_id,
    t.from_status_key,
    t.to_status_key,
    t.source,
    t.created_at,
    fs.category AS from_catalog_category,
    ts.category AS to_catalog_category
FROM issue_status_transition t
LEFT JOIN issue_status fs ON fs.workspace_id = t.workspace_id AND fs.key = t.from_status_key
LEFT JOIN issue_status ts ON ts.workspace_id = t.workspace_id AND ts.key = t.to_status_key
WHERE t.workspace_id = @workspace_id
  AND t.issue_id IN (
    SELECT t2.issue_id
    FROM issue_status_transition t2
    JOIN issue i ON i.id = t2.issue_id AND i.workspace_id = t2.workspace_id
    WHERE t2.workspace_id = @workspace_id
      AND t2.created_at >= @period_start::timestamptz
      AND t2.created_at < @period_end::timestamptz
      AND i.assignee_type = 'squad'
      AND (sqlc.narg('squad_ids')::uuid[] IS NULL OR i.assignee_id = ANY(sqlc.narg('squad_ids')::uuid[]))
      AND (i.project_id IS NULL
           OR EXISTS (SELECT 1 FROM project scope_p WHERE scope_p.id = i.project_id AND scope_p.visibility = 'workspace'))
  )
ORDER BY t.issue_id, t.created_at;

-- name: DurableScorecardOutcomes :many
-- Work outcomes whose done transition is in the window, for the given issues. Each
-- row is one done entry with its outcome; verdict and sampled decide verification.
SELECT
    wo.issue_id,
    wo.verdict,
    wo.sampled,
    wo.fail_cause,
    (wo.settled_at IS NOT NULL)::boolean AS settled
FROM work_outcome wo
JOIN issue_status_transition t ON t.id = wo.done_transition_id
WHERE wo.workspace_id = @workspace_id
  AND wo.issue_id = ANY(@issue_ids::uuid[])
  AND t.created_at >= @period_start::timestamptz
  AND t.created_at < @period_end::timestamptz;
