-- The chat session a run may reply into with `multica chat reply`. A children-done
-- or deadline run of a Secretary request carries it, taken from the request's
-- chat_session_id metadata. It is kept apart from chat_session_id on purpose:
-- chat_session_id makes a run's output the outcome of a chat turn, which would
-- write the run's internal output into the boss's chat. No index: it is read
-- by task id only.
ALTER TABLE agent_task_queue ADD COLUMN IF NOT EXISTS reply_chat_session_id UUID;
