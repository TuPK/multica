-- Workflow routing as transitions (docs/engineering/workflow-tab.md §3).
--
-- Each step's next_status_key and back_status_key become a transitions list,
-- in the order the two-way routing applied them:
--   outcome approved -> next      (id "approved")
--   outcome rejected -> back      (id "rejected")
--   default          -> next      (id "default")
-- An empty key leaves its branch out. A step with no routing gets no
-- transitions key. A step that already has transitions is left as it is, so
-- running this again changes nothing. The legacy keys are removed: the server
-- derives them from the transitions on every response. The ids and labels match
-- issueworkflow.LegacyTransitions in Go; keep them in step.
--
-- One UPDATE of the workflow rows. It adds no index and no foreign key.
UPDATE issue_workflow AS w
SET steps = (
    SELECT COALESCE(jsonb_agg(
        CASE
            WHEN e.step->'transitions' IS NOT NULL THEN e.step
            WHEN jsonb_array_length(r.routes) = 0 THEN e.step - 'next_status_key' - 'back_status_key'
            ELSE (e.step - 'next_status_key' - 'back_status_key')
                || jsonb_build_object('transitions', r.routes)
        END
        ORDER BY e.n
    ), '[]'::jsonb)
    FROM jsonb_array_elements(w.steps) WITH ORDINALITY AS e(step, n)
    CROSS JOIN LATERAL (
        SELECT COALESCE(jsonb_agg(leg.route ORDER BY leg.ord), '[]'::jsonb) AS routes
        FROM (
            SELECT 1 AS ord,
                   jsonb_build_object(
                       'id', 'approved',
                       'label', 'Approved',
                       'when', jsonb_build_object('field', 'outcome', 'op', 'eq', 'value', 'approved'),
                       'to_status_key', e.step->>'next_status_key'
                   ) AS route
            WHERE NULLIF(e.step->>'next_status_key', '') IS NOT NULL
            UNION ALL
            SELECT 2,
                   jsonb_build_object(
                       'id', 'rejected',
                       'label', 'Rejected',
                       'when', jsonb_build_object('field', 'outcome', 'op', 'eq', 'value', 'rejected'),
                       'to_status_key', e.step->>'back_status_key'
                   )
            WHERE NULLIF(e.step->>'back_status_key', '') IS NOT NULL
            UNION ALL
            SELECT 3,
                   jsonb_build_object(
                       'id', 'default',
                       'label', 'Default',
                       'when', 'null'::jsonb,
                       'to_status_key', e.step->>'next_status_key'
                   )
            WHERE NULLIF(e.step->>'next_status_key', '') IS NOT NULL
        ) AS leg
    ) AS r
)
WHERE jsonb_typeof(w.steps) = 'array'
  AND EXISTS (
      SELECT 1 FROM jsonb_array_elements(w.steps) AS x(step)
      WHERE x.step->'transitions' IS NULL
        AND (x.step->'next_status_key' IS NOT NULL OR x.step->'back_status_key' IS NOT NULL)
  );
