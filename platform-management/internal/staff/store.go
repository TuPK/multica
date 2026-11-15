package staff

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/internal/db"
)

// Queries is the part of *db.Queries that this package uses.
type Queries interface {
	GetActiveStaffByUserID(ctx context.Context, userID pgtype.UUID) (db.PlatformStaff, error)
	GetUserIDByEmail(ctx context.Context, email string) (pgtype.UUID, error)
	GetUserEmailByID(ctx context.Context, id pgtype.UUID) (string, error)
	GetAccountState(ctx context.Context, userID pgtype.UUID) (db.GetAccountStateRow, error)
	InsertStaff(ctx context.Context, arg db.InsertStaffParams) (int64, error)
	ReactivateStaff(ctx context.Context, arg db.ReactivateStaffParams) (int64, error)
}

// Store reads staff, users and account state from the database. It is the
// platformauth Directory and the accountguard Store.
type Store struct{ q Queries }

// NewStore returns a Store over q.
func NewStore(q Queries) *Store { return &Store{q: q} }

func parseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid user id: %w", err)
	}
	return u, nil
}

func uuidString(u pgtype.UUID) string {
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// UserIDByEmail takes a lower-cased email.
func (s *Store) UserIDByEmail(ctx context.Context, email string) (string, bool, error) {
	id, err := s.q.GetUserIDByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return uuidString(id), true, nil
}

// ActiveStaff returns the role of the user's active row. A role this build
// does not know counts as no row.
func (s *Store) ActiveStaff(ctx context.Context, userID string) (Role, bool, error) {
	id, err := parseUUID(userID)
	if err != nil {
		return "", false, err
	}
	row, err := s.q.GetActiveStaffByUserID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	role, ok := ParseRole(row.Role)
	return role, ok, nil
}

func (s *Store) EmailByUserID(ctx context.Context, userID string) (string, error) {
	id, err := parseUUID(userID)
	if err != nil {
		return "", err
	}
	return s.q.GetUserEmailByID(ctx, id)
}

// AccountState implements accountguard.Store.
func (s *Store) AccountState(ctx context.Context, userID string) (accountguard.Record, bool, error) {
	id, err := parseUUID(userID)
	if err != nil {
		return accountguard.Record{}, false, err
	}
	row, err := s.q.GetAccountState(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return accountguard.Record{}, false, nil
	}
	if err != nil {
		return accountguard.Record{}, false, err
	}
	rec := accountguard.Record{Status: accountguard.State(row.Status)}
	if row.SessionsValidAfter.Valid {
		rec.SessionsValidAfter = row.SessionsValidAfter.Time
	}
	return rec, true, nil
}
