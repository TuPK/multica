-- The shadow run applies an auto-promoted runtime change, so its application names the shadow
-- as the actor (durable spec 9.6). NOT VALID then VALIDATE, as migration 870 did: the new list
-- is a superset of the old one, so existing rows pass.
ALTER TABLE org_change_application
    DROP CONSTRAINT IF EXISTS org_change_application_applied_by_type_check,
    ADD CONSTRAINT org_change_application_applied_by_type_check CHECK (applied_by_type IN (
        'member', 'agent', 'system', 'policy', 'shadow')) NOT VALID;
ALTER TABLE org_change_application VALIDATE CONSTRAINT org_change_application_applied_by_type_check;
