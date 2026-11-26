-- The runtime_change type (durable spec 9.6). NOT VALID then VALIDATE, as migration 870
-- did: the new list is a superset of the old one, so existing rows pass.
ALTER TABLE org_change_application
    DROP CONSTRAINT IF EXISTS org_change_application_change_type_check,
    ADD CONSTRAINT org_change_application_change_type_check CHECK (change_type IN (
        'instruction_change', 'skill_change', 'charter_change', 'autonomy_change',
        'hire', 'retire', 'move', 'merge_squads', 'unretire', 'unmerge', 'runtime_change')) NOT VALID;
ALTER TABLE org_change_application VALIDATE CONSTRAINT org_change_application_change_type_check;
