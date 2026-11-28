DELETE FROM hub_item WHERE kind = 'agent';
ALTER TABLE hub_item DROP CONSTRAINT IF EXISTS hub_item_kind_check;
ALTER TABLE hub_item ADD CONSTRAINT hub_item_kind_check
    CHECK (kind IN ('skill', 'plugin'));

UPDATE hub_activity SET kind = NULL WHERE kind = 'agent';
ALTER TABLE hub_activity DROP CONSTRAINT IF EXISTS hub_activity_kind_check;
ALTER TABLE hub_activity ADD CONSTRAINT hub_activity_kind_check
    CHECK (kind IN ('skill', 'plugin'));
