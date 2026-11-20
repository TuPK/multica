-- Plan description (spec R6 section 3b.1, D88). Empty means no description. A
-- plain column with a default: no index, so no concurrent build.
-- Numbered 938 so it sorts after 670_plan, which creates the table: migrations
-- run in lexical order, so a 1xxx name would run first on a new database.
ALTER TABLE IF EXISTS plan ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT '';
