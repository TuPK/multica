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

ALTER TABLE issue_status_transition DROP COLUMN assignee_type, DROP COLUMN assignee_id, DROP COLUMN label_names;
