-- The traits of a member for one decision kind, read by the predictor (spec 6.4).
CREATE INDEX CONCURRENTLY IF NOT EXISTS principal_trait_member_kind_idx
    ON principal_trait (workspace_id, member_id, decision_kind);
