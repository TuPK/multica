-- Usage invoices have no expires_at, so the previous schema cannot hold them.
-- Refuse rollback rather than invent an expiry or destroy unpaid invoices.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM billing_order WHERE expires_at IS NULL) THEN
        RAISE EXCEPTION 'cannot roll back 969: billing orders without an expiry still exist'
            USING HINT = 'Pay or cancel every usage invoice before retrying; this migration will not delete billing data.';
    END IF;
END
$$;

ALTER TABLE billing_order ALTER COLUMN expires_at SET NOT NULL;
