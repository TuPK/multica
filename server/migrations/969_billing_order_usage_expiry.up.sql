-- A usage invoice (platform spec 7b.3, rule 11) does not expire: it stays
-- pending until it is paid or written off. Such an order has no expires_at.
-- Idempotent: dropping NOT NULL on a column that is already nullable is a no-op.
ALTER TABLE billing_order ALTER COLUMN expires_at DROP NOT NULL;
