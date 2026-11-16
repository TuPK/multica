package staff

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/db"
)

// MFAQueries is the part of *db.Queries that MFAStore uses.
type MFAQueries interface {
	GetConfirmedUserMFA(ctx context.Context, userID pgtype.UUID) (db.GetConfirmedUserMFARow, error)
	GetAnyConfirmedUserMFA(ctx context.Context) ([]byte, error)
	UseUserMFAStep(ctx context.Context, arg db.UseUserMFAStepParams) (int64, error)
}

// MFAStore reads the second factor that Multica stores for a user, and takes a
// TOTP step once. It is the platformauth MFAStore and the mfa.Sampler.
type MFAStore struct{ q MFAQueries }

// NewMFAStore returns an MFAStore over q.
func NewMFAStore(q MFAQueries) *MFAStore { return &MFAStore{q: q} }

// ConfirmedMFA returns the sealed secret and the last used step of the user's
// confirmed row. found is false for no row or a row still pending.
func (s *MFAStore) ConfirmedMFA(ctx context.Context, userID string) (sealed []byte, lastStep int64, found bool, err error) {
	id, err := parseUUID(userID)
	if err != nil {
		return nil, 0, false, err
	}
	row, err := s.q.GetConfirmedUserMFA(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	return row.SecretEnc, row.LastUsedStep, true, nil
}

// UseMFAStep records step as used. It returns false when the user's last used
// step is already step or later.
func (s *MFAStore) UseMFAStep(ctx context.Context, userID string, step int64) (bool, error) {
	id, err := parseUUID(userID)
	if err != nil {
		return false, err
	}
	n, err := s.q.UseUserMFAStep(ctx, db.UseUserMFAStepParams{UserID: id, LastUsedStep: step})
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// AnyConfirmedSecret returns the sealed secret of one confirmed row, for the
// start check of the key.
func (s *MFAStore) AnyConfirmedSecret(ctx context.Context) (sealed []byte, found bool, err error) {
	sealed, err = s.q.GetAnyConfirmedUserMFA(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return sealed, true, nil
}
