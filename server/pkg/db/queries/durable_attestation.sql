-- name: AttestationOutcomeCounts :many
-- The scored outcomes of one agent settled in [window_start, window_end), by work class: the
-- verdicts that make the score (pass, partial, fail) and how each was verified (spec 13.2).
-- An unverified outcome is not scored and is not counted here.
SELECT
    o.work_class,
    COUNT(*) FILTER (WHERE o.verdict = 'pass')::bigint AS n_pass,
    COUNT(*) FILTER (WHERE o.verdict = 'partial')::bigint AS n_partial,
    COUNT(*) FILTER (WHERE o.verdict = 'fail')::bigint AS n_fail,
    COUNT(*) FILTER (WHERE o.verification_mode = 'full')::bigint AS n_full,
    COUNT(*) FILTER (WHERE o.sampled)::bigint AS n_sampled,
    COUNT(*) FILTER (WHERE EXISTS (
        SELECT 1 FROM jury_panel j WHERE j.workspace_id = o.workspace_id AND j.outcome_id = o.id
    ))::bigint AS n_jury
FROM work_outcome o
WHERE o.workspace_id = sqlc.arg(workspace_id)
    AND o.agent_id = sqlc.arg(agent_id)
    AND o.verdict IN ('pass', 'partial', 'fail')
    AND o.settled_at >= sqlc.arg(window_start)::timestamptz
    AND o.settled_at < sqlc.arg(window_end)::timestamptz
GROUP BY o.work_class;

-- name: AttestationOutcomeModels :many
-- The models that did the scored work in the window, for the attestation's model list.
SELECT DISTINCT o.model
FROM work_outcome o
WHERE o.workspace_id = sqlc.arg(workspace_id)
    AND o.agent_id = sqlc.arg(agent_id)
    AND o.verdict IN ('pass', 'partial', 'fail')
    AND o.model <> ''
    AND o.settled_at >= sqlc.arg(window_start)::timestamptz
    AND o.settled_at < sqlc.arg(window_end)::timestamptz
ORDER BY o.model;
