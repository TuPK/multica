package subscription

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/db"
)

// PgxProfileRunner is the ProfileRunner over a Postgres pool.
type PgxProfileRunner struct{ pool Beginner }

// NewPgxProfileRunner returns a ProfileRunner that opens one transaction per call.
func NewPgxProfileRunner(pool Beginner) *PgxProfileRunner { return &PgxProfileRunner{pool: pool} }

// InTx implements ProfileRunner. It returns fn's error as it is, so a caller
// can still recognise an *Error.
func (r *PgxProfileRunner) InTx(ctx context.Context, fn func(ProfileTx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op once committed
	if err := fn(&pgxProfileTx{pgxTx: &pgxTx{q: db.New(tx)}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type pgxProfileTx struct{ *pgxTx }

// LockedProfile implements ProfileTx. The advisory lock stands in for the
// unique index of migration 955, which is not applied yet: it keeps two first
// writes of one workspace from inserting two rows.
func (t *pgxProfileTx) LockedProfile(ctx context.Context, workspaceID string) (BillingProfile, bool, error) {
	uid, err := parseUUID(workspaceID)
	if err != nil {
		return BillingProfile{}, false, err
	}
	if err := t.q.LockBillingProfileWrites(ctx, uid.String()); err != nil {
		return BillingProfile{}, false, err
	}
	return readProfile(ctx, t.q, uid)
}

// SaveProfile implements ProfileTx.
func (t *pgxProfileTx) SaveProfile(ctx context.Context, p BillingProfile) error {
	uid, err := parseUUID(p.WorkspaceID)
	if err != nil {
		return err
	}
	verifiedBy, err := optionalUUID(p.VerifiedBy)
	if err != nil {
		return err
	}
	updatedBy, err := optionalUUID(p.UpdatedBy)
	if err != nil {
		return err
	}
	_, err = t.q.UpdateBillingProfile(ctx, db.UpdateBillingProfileParams{
		WorkspaceID: uid, AccountKind: p.AccountKind, LegalName: p.LegalName, TaxCode: p.TaxCode,
		Address: p.Address, InvoiceEmail: p.InvoiceEmail, VerifiedAt: timestamptz(p.VerifiedAt),
		VerifiedBy: verifiedBy, UpdatedBy: updatedBy,
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	_, err = t.q.InsertBillingProfile(ctx, db.InsertBillingProfileParams{
		WorkspaceID: uid, AccountKind: p.AccountKind, LegalName: p.LegalName, TaxCode: p.TaxCode,
		Address: p.Address, InvoiceEmail: p.InvoiceEmail, VerifiedAt: timestamptz(p.VerifiedAt),
		VerifiedBy: verifiedBy, UpdatedBy: updatedBy,
	})
	return err
}

func optionalUUID(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, nil
	}
	return parseUUID(s)
}

func profileFromRow(r db.BillingProfile) BillingProfile {
	return BillingProfile{
		WorkspaceID: uuidText(r.WorkspaceID), AccountKind: r.AccountKind, LegalName: r.LegalName,
		TaxCode: r.TaxCode, Address: r.Address, InvoiceEmail: r.InvoiceEmail,
		VerifiedAt: timePtr(r.VerifiedAt), VerifiedBy: uuidText(r.VerifiedBy),
		UpdatedBy: uuidText(r.UpdatedBy), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
}

func readProfile(ctx context.Context, q *db.Queries, id pgtype.UUID) (BillingProfile, bool, error) {
	row, err := q.GetBillingProfile(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return BillingProfile{}, false, nil
	}
	if err != nil {
		return BillingProfile{}, false, err
	}
	return profileFromRow(row), true, nil
}

// PgxProfiles is the KindReader over a Postgres pool.
type PgxProfiles struct{ q *db.Queries }

// NewPgxProfiles returns a KindReader over pool.
func NewPgxProfiles(pool db.DBTX) *PgxProfiles { return &PgxProfiles{q: db.New(pool)} }

// Profile implements KindReader.
func (r *PgxProfiles) Profile(ctx context.Context, workspaceID string) (BillingProfile, bool, error) {
	uid, err := parseUUID(workspaceID)
	if err != nil {
		return BillingProfile{}, false, err
	}
	return readProfile(ctx, r.q, uid)
}
