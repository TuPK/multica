-- sqlc only. Migration 1030 adds plan.description, but sorts before the table's
-- creation (670) when sqlc reads the migrations in name order, so sqlc needs this
-- copy of the same column. Keep it identical to server/migrations/1030.
ALTER TABLE plan ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT '';
