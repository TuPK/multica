-- Allow agent template Hub items (durable spec 13.3, F12) in hub_item and hub_activity.
ALTER TABLE hub_item DROP CONSTRAINT IF EXISTS hub_item_kind_check;
ALTER TABLE hub_item ADD CONSTRAINT hub_item_kind_check
    CHECK (kind IN ('skill', 'plugin', 'agent'));

ALTER TABLE hub_activity DROP CONSTRAINT IF EXISTS hub_activity_kind_check;
ALTER TABLE hub_activity ADD CONSTRAINT hub_activity_kind_check
    CHECK (kind IN ('skill', 'plugin', 'agent'));
