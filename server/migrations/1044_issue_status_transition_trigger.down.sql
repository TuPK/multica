DROP TRIGGER IF EXISTS issue_status_transition_ai ON issue;
DROP FUNCTION IF EXISTS issue_status_transition_record();
DROP FUNCTION IF EXISTS issue_status_transition_uuid(TEXT);
