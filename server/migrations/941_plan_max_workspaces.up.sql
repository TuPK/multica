-- Adds the max_workspaces limit to the catalogue plans (spec R6 section 5.6): a
-- whole number of workspaces per tenant, or an explicit JSON null for no limit.
-- A plan that already has the key keeps it, so a value staff set is not
-- overwritten and a second run changes nothing. This runs before 953 on a new
-- database, so it uses only the key and limits columns of 670_plan.
UPDATE plan AS p
SET limits = jsonb_build_object('max_workspaces', v.max_workspaces) || p.limits
FROM (VALUES
    ('free', 1),
    ('pro', 3),
    ('business', 10),
    ('payg', 5),
    ('enterprise', NULL),
    ('unlimited', NULL)
) AS v(key, max_workspaces)
WHERE p.key = v.key AND NOT (p.limits ? 'max_workspaces');
