-- Plan description (spec R6 section 3b.1, D88). Empty means no description. A
-- plain column with a default: no index, so no concurrent build.
ALTER TABLE IF EXISTS plan ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT '';
