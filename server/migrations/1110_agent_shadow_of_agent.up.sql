-- The seat a shadow agent stands for (durable spec 9.4, D27). NULL for every ordinary
-- agent. Shadow agents are hidden from agent lists and never assigned work by a person.
ALTER TABLE agent ADD COLUMN IF NOT EXISTS shadow_of_agent_id UUID;
