-- Runs once, on first init of a fresh data volume (docker-entrypoint-initdb.d).
-- pg_stat_statements is preloaded via the server command in docker-compose.yml;
-- the extension itself must be created in each database we want to inspect.
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
