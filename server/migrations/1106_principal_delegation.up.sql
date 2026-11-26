-- Delegation by prediction (durable spec 6.2, 6.5, F5): a person lets the predictor decide one kind
-- of question. A kind is re-enabled as a new row, so the history of enables and disables stays.
CREATE TABLE principal_delegation (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    member_id UUID NOT NULL,
    decision_kind TEXT NOT NULL,
    min_confidence NUMERIC NOT NULL DEFAULT 0.90 CHECK (min_confidence > 0 AND min_confidence <= 1),
    enabled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    disabled_at TIMESTAMPTZ,
    disabled_reason TEXT NOT NULL DEFAULT ''
);
