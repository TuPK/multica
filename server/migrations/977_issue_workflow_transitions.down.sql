-- Reverses 977: each step's next_status_key is the target of its default
-- transition and back_status_key the target of its outcome-rejected transition.
-- Branches beyond those two are lost; a workflow with custom routing does not
-- survive a downgrade unchanged.
UPDATE issue_workflow AS w
SET steps = (
    SELECT COALESCE(jsonb_agg(
        CASE
            WHEN e.step->'transitions' IS NULL THEN e.step
            ELSE (e.step - 'transitions')
                || COALESCE((
                    SELECT jsonb_build_object('next_status_key', t->>'to_status_key')
                    FROM jsonb_array_elements(e.step->'transitions') AS t
                    WHERE t->'when' = 'null'::jsonb
                    LIMIT 1
                ), '{}'::jsonb)
                || COALESCE((
                    SELECT jsonb_build_object('back_status_key', t->>'to_status_key')
                    FROM jsonb_array_elements(e.step->'transitions') AS t
                    WHERE t->'when'->>'field' = 'outcome'
                      AND t->'when'->>'op' = 'eq'
                      AND t->'when'->>'value' = 'rejected'
                    LIMIT 1
                ), '{}'::jsonb)
        END
        ORDER BY e.n
    ), '[]'::jsonb)
    FROM jsonb_array_elements(w.steps) WITH ORDINALITY AS e(step, n)
)
WHERE jsonb_typeof(w.steps) = 'array'
  AND EXISTS (
      SELECT 1 FROM jsonb_array_elements(w.steps) AS x(step)
      WHERE x.step->'transitions' IS NOT NULL
  );
