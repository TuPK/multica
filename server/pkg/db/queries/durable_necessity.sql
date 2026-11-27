-- Necessity tests of the scaffolding sunset (durable spec 10). A test is a shadow run of the seat
-- without the piece under test; its verdict is one row here.

-- name: InsertNecessityTest :one
INSERT INTO necessity_test (id, workspace_id, subject_type, subject_ref, agent_id, margin, min_pairs, created_by_type, created_by_id)
VALUES (@id, @workspace_id, @subject_type, @subject_ref, @agent_id, @margin, @min_pairs, @created_by_type, @created_by_id)
RETURNING *;

-- name: SetNecessityTestShadowConfig :execrows
UPDATE necessity_test SET shadow_config_id = @shadow_config_id
WHERE id = @id AND workspace_id = @workspace_id;

-- name: GetNecessityTest :one
SELECT * FROM necessity_test WHERE id = @id AND workspace_id = @workspace_id;

-- name: GetNecessityTestByShadowConfig :one
SELECT * FROM necessity_test WHERE shadow_config_id = @shadow_config_id AND workspace_id = @workspace_id;

-- name: ListNecessityTestsForAgent :many
SELECT * FROM necessity_test
WHERE workspace_id = @workspace_id AND agent_id = @agent_id
ORDER BY created_at DESC
LIMIT 50;

-- name: SetNecessityTestOutcome :execrows
-- Writes the verdict of a running test once. A test that is no longer running keeps its verdict.
UPDATE necessity_test
SET state = @state, result = @result, proposal_id = @proposal_id, completed_at = now()
WHERE id = @id AND workspace_id = @workspace_id AND state = 'running';

-- name: SetNecessityTestStopped :execrows
UPDATE necessity_test SET state = 'stopped', completed_at = now()
WHERE id = @id AND workspace_id = @workspace_id AND state = 'running';

-- name: CountRunningNecessityTestsBySquad :many
-- The running tests of a workspace per squad, for the two-at-a-time cap of the weekly job (spec 10.3).
SELECT a.home_squad_id AS squad_id, count(*)::bigint AS running
FROM necessity_test n
JOIN agent a ON a.id = n.agent_id
WHERE n.workspace_id = @workspace_id AND n.state = 'running'
GROUP BY a.home_squad_id;

-- name: ListNecessityCandidateSkills :many
-- Every skill a seat holds, with the seat's work in the last 30 days and when the skill was last
-- tested. The weekly job picks from these (spec 10.3).
SELECT a.id AS agent_id, a.home_squad_id AS squad_id, s.id AS skill_id, s.name AS skill_name,
       (SELECT count(*) FROM agent_task_queue t
        WHERE t.agent_id = a.id AND t.created_at >= now() - interval '30 days')::bigint AS tasks_30d,
       (SELECT max(n.created_at) FROM necessity_test n
        WHERE n.agent_id = a.id AND n.subject_type = 'skill' AND n.subject_ref = s.id::text)::timestamptz AS last_tested_at
FROM agent_skill ags
JOIN agent a ON a.id = ags.agent_id
JOIN skill s ON s.id = ags.skill_id
WHERE a.workspace_id = @workspace_id
  AND a.archived_at IS NULL
  AND a.shadow_of_agent_id IS NULL
  AND a.home_squad_id IS NOT NULL
  AND a.runtime_id IS NOT NULL;

-- name: ListFlowGateVerdictsSince :many
-- First-round gate verdicts per flow step and gate task since a time (spec 10.4). A task of round 0
-- is the step's first pass; a rejection there is a resend.
SELECT i.flow_step, i.flow_task,
       count(*)::bigint AS passes,
       count(*) FILTER (WHERE i.outcome = 'approved')::bigint AS approved_first,
       count(*) FILTER (WHERE i.outcome = 'rejected')::bigint AS resends
FROM issue i
WHERE i.workspace_id = @workspace_id
  AND i.flow_task IS NOT NULL
  AND i.flow_round = 0
  AND i.outcome IN ('approved', 'rejected')
  AND i.created_at >= @since
GROUP BY i.flow_step, i.flow_task
ORDER BY i.flow_step, i.flow_task;

-- name: CountAgentSkillLink :one
SELECT count(*)::bigint FROM agent_skill WHERE agent_id = @agent_id AND skill_id = @skill_id;

-- name: CopyAgentSkillsExcept :execrows
-- The shadow agent of a necessity test holds the seat's skills but the one under test (spec 10.2).
INSERT INTO agent_skill (agent_id, skill_id, enabled)
SELECT @shadow_agent_id, ags.skill_id, ags.enabled
FROM agent_skill ags
WHERE ags.agent_id = @seat_id AND ags.skill_id <> @except_skill_id
ON CONFLICT DO NOTHING;
