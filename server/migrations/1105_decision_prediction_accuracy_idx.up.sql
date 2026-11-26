-- The accuracy of a person's predictions per kind, over the last resolved ones (spec 6.4).
CREATE INDEX CONCURRENTLY IF NOT EXISTS decision_prediction_accuracy_idx
    ON decision_prediction (workspace_id, member_id, decision_kind, resolved_at);
