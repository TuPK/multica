-- Fails while applications by the shadow exist, by design: those cannot be reinterpreted under
-- the four-actor list.
ALTER TABLE org_change_application
    DROP CONSTRAINT IF EXISTS org_change_application_applied_by_type_check,
    ADD CONSTRAINT org_change_application_applied_by_type_check CHECK (applied_by_type IN (
        'member', 'agent', 'system', 'policy'));
