-- The route the platform gives a Secretary chat turn: fact, judgment, decision or
-- clarify. Set in the same transaction that creates the turn, so the claim never
-- sees a turn without it. It guides the Secretary's first step; the reply gate is
-- what enforces the answer.
ALTER TABLE agent_task_queue ADD COLUMN IF NOT EXISTS chat_route TEXT;
