-- One running test per seat and subject: the weekly job and a person's start cannot both open one.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS necessity_test_running_uidx
    ON necessity_test (agent_id, subject_type, subject_ref) WHERE state = 'running';
