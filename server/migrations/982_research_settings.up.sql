-- Research Lab source settings: one row per workspace.
--
-- openalex_key_sealed and s2_key_sealed hold the OpenAlex and Semantic Scholar API keys sealed with
-- secretbox (MULTICA_RESEARCH_SECRET_KEY, or a key derived from JWT_SECRET when that is unset). A
-- NULL column means no key is stored. A key is never returned by the settings endpoint; only the
-- credentials endpoint, which agents and members call to run `multica research`, returns it.
-- contact_email is the address sent to Crossref and Unpaywall; it is not a secret.
--
-- No foreign keys: the writers check the workspace in code, and workspace deletion removes the row.
CREATE TABLE IF NOT EXISTS research_settings (
    workspace_id        UUID PRIMARY KEY,
    openalex_key_sealed BYTEA,
    s2_key_sealed       BYTEA,
    contact_email       TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
