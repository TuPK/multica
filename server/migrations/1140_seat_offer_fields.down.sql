ALTER TABLE seat_offer
    DROP CONSTRAINT IF EXISTS seat_offer_proposed_budget_period_chk,
    DROP CONSTRAINT IF EXISTS seat_offer_proposed_envelope_bounds_chk,
    DROP COLUMN IF EXISTS proposed_envelope_ceiling,
    DROP COLUMN IF EXISTS proposed_envelope_floor,
    DROP COLUMN IF EXISTS proposed_budget_period,
    DROP COLUMN IF EXISTS proposed_budget_micros,
    DROP COLUMN IF EXISTS work_class;

DELETE FROM seat_offer WHERE runtime_id IS NULL;
ALTER TABLE seat_offer ALTER COLUMN runtime_id SET NOT NULL;
