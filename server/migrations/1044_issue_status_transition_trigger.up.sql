-- Writes issue_status_transition for every change of issue.status (durable spec
-- 2.2, D26). A trigger rather than a call in each writer: status is written from
-- many paths (handlers, services, batch updates, runtime sweepers), and a missed
-- path would silently lose a transition.
--
-- The actor comes from session settings that the application sets with
-- set_config(..., true) inside the transaction that changes the issue
-- (WithActorSession). Without them the row is recorded as system.
--
-- This migration references only issue and issue_status_transition. Both exist
-- before it runs in sort order on a fresh database (D32).
--
-- SET LOCAL lock_timeout: this trigger's DDL takes a lock on the issue table, so
-- it must not queue behind long transactions (same as migration 492).
SET LOCAL lock_timeout = '2s';

-- Malformed or empty input is NULL, not an error, so a bad session value never
-- fails the status write that fired the trigger.
CREATE OR REPLACE FUNCTION issue_status_transition_uuid(p TEXT) RETURNS UUID
LANGUAGE plpgsql IMMUTABLE AS $$
BEGIN
    RETURN NULLIF(p, '')::uuid;
EXCEPTION WHEN invalid_text_representation THEN
    RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION issue_status_transition_record() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    v_actor_type TEXT := NULLIF(current_setting('multica.actor_type', true), '');
    v_actor_id UUID := issue_status_transition_uuid(current_setting('multica.actor_id', true));
    v_task_id UUID := issue_status_transition_uuid(current_setting('multica.task_id', true));
BEGIN
    -- Only a member or agent with a valid id is an actor. Anything else is
    -- recorded as system with no actor id.
    IF v_actor_type IS NULL OR v_actor_type NOT IN ('member', 'agent') OR v_actor_id IS NULL THEN
        v_actor_type := 'system';
        v_actor_id := NULL;
    END IF;

    INSERT INTO issue_status_transition (
        workspace_id, issue_id, from_status_key, to_status_key,
        actor_type, actor_id, task_id
    ) VALUES (
        NEW.workspace_id, NEW.id, OLD.status, NEW.status,
        v_actor_type, v_actor_id, v_task_id
    );
    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS issue_status_transition_ai ON issue;
CREATE TRIGGER issue_status_transition_ai
AFTER UPDATE OF status ON issue
FOR EACH ROW
WHEN (OLD.status IS DISTINCT FROM NEW.status)
EXECUTE FUNCTION issue_status_transition_record();
