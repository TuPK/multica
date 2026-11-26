-- One prediction per question and person. Built CONCURRENTLY, one statement per file (project rule).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS decision_prediction_subject_uidx
    ON decision_prediction (subject_type, subject_id, member_id);
