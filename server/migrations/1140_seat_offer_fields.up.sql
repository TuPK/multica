-- Extend seat_offer with spec 13.3 fields: work class, proposed budget, proposed autonomy
-- envelope, and making required runtime optional ("runtime yêu cầu (nếu có)").
ALTER TABLE seat_offer
    ALTER COLUMN runtime_id DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS work_class TEXT NOT NULL DEFAULT '' CHECK (char_length(work_class) <= 200),
    ADD COLUMN IF NOT EXISTS proposed_budget_micros BIGINT CHECK (proposed_budget_micros IS NULL OR proposed_budget_micros >= 0),
    ADD COLUMN IF NOT EXISTS proposed_budget_period TEXT NOT NULL DEFAULT '' CHECK (proposed_budget_period IN ('', 'week', 'month', 'quarter')),
    ADD COLUMN IF NOT EXISTS proposed_envelope_floor INTEGER CHECK (proposed_envelope_floor IS NULL OR proposed_envelope_floor BETWEEN 0 AND 3),
    ADD COLUMN IF NOT EXISTS proposed_envelope_ceiling INTEGER CHECK (proposed_envelope_ceiling IS NULL OR proposed_envelope_ceiling BETWEEN 0 AND 3),
    DROP CONSTRAINT IF EXISTS seat_offer_proposed_envelope_bounds_chk,
    DROP CONSTRAINT IF EXISTS seat_offer_proposed_budget_period_chk,
    ADD CONSTRAINT seat_offer_proposed_envelope_bounds_chk CHECK (
        (proposed_envelope_floor IS NULL) = (proposed_envelope_ceiling IS NULL)
        AND (proposed_envelope_floor IS NULL OR proposed_envelope_floor <= proposed_envelope_ceiling)
    ),
    ADD CONSTRAINT seat_offer_proposed_budget_period_chk CHECK (
        (proposed_budget_micros IS NULL AND proposed_budget_period = '')
        OR (proposed_budget_micros IS NOT NULL AND proposed_budget_period <> '')
    );
