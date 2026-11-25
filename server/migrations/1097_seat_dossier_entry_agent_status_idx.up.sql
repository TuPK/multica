-- The briefing reads a seat's active entries, and the cap counts them (spec 9.2).
CREATE INDEX CONCURRENTLY IF NOT EXISTS seat_dossier_entry_agent_status_idx
    ON seat_dossier_entry (workspace_id, agent_id, status);
