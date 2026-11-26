-- A prediction of what a person will decide on a question (durable spec 6.2, F5). A question is
-- admitted with a pending row; the Secretary fills in the prediction; the person's decision resolves
-- it. The row is kept after a profile is deleted, without its why and trait ids (spec 6.6).
CREATE TABLE decision_prediction (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    member_id UUID NOT NULL,
    subject_type TEXT NOT NULL CHECK (subject_type IN ('proposal', 'effect_intent', 'outcome_check', 'inquiry')),
    subject_id UUID NOT NULL,
    decision_kind TEXT NOT NULL,
    predicted TEXT NOT NULL DEFAULT '',
    confidence NUMERIC NOT NULL DEFAULT 0 CHECK (confidence >= 0 AND confidence <= 1),
    why TEXT NOT NULL DEFAULT '' CHECK (char_length(why) <= 1000),
    trait_ids UUID[] NOT NULL DEFAULT '{}',
    profile_version INTEGER NOT NULL DEFAULT 0,
    predictor_task_id UUID,
    actual TEXT,
    matched BOOLEAN,
    applied_by_delegation BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ
);
