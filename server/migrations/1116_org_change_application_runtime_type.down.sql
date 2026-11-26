-- Fails while runtime_change applications exist, by design: those cannot be reinterpreted
-- under the ten-type list.
ALTER TABLE org_change_application
    DROP CONSTRAINT IF EXISTS org_change_application_change_type_check,
    ADD CONSTRAINT org_change_application_change_type_check CHECK (change_type IN (
        'instruction_change', 'skill_change', 'charter_change', 'autonomy_change',
        'hire', 'retire', 'move', 'merge_squads', 'unretire', 'unmerge'));
