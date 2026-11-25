ALTER TABLE workspace DROP CONSTRAINT IF EXISTS workspace_attention_split_check;
ALTER TABLE workspace DROP CONSTRAINT IF EXISTS workspace_attention_minutes_per_week_check;
ALTER TABLE workspace DROP COLUMN IF EXISTS attention_split;
ALTER TABLE workspace DROP COLUMN IF EXISTS attention_minutes_per_week;
DROP TABLE IF EXISTS attention_ledger;
