package staff

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/emailhash"
)

const uniqueViolation = "23505"

// Seeder gives the people in PLATFORM_ADMIN_EMAILS an active system_admin row. It is
// how the first administrator gets in, and it cannot lock itself out: a
// revoked row of such a person is re-activated.
type Seeder struct {
	q      Queries
	emails []string
	log    *slog.Logger
}

// NewSeeder returns a Seeder for the lower-cased emails. A nil logger means
// slog.Default.
func NewSeeder(q Queries, emails []string, log *slog.Logger) *Seeder {
	if log == nil {
		log = slog.Default()
	}
	return &Seeder{q: q, emails: emails, log: log}
}

// SeedAll seeds every configured email. An email with no Multica account is
// logged and skipped. It returns the first database error.
func (s *Seeder) SeedAll(ctx context.Context) error {
	for _, email := range s.emails {
		if err := s.SeedEmail(ctx, email); err != nil {
			return err
		}
	}
	return nil
}

// SeedEmail seeds one email, which the caller has taken from the configured
// list. An active row of any role is left as it is.
func (s *Seeder) SeedEmail(ctx context.Context, email string) error {
	hash := emailhash.Of(email)
	userID, err := s.q.GetUserIDByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		s.log.Info("platform admin has no Multica account yet; skipped", "email_hash", hash)
		return nil
	}
	if err != nil {
		return fmt.Errorf("seed staff: look up user: %w", err)
	}

	_, err = s.q.GetActiveStaffByUserID(ctx, userID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("seed staff: read staff row: %w", err)
	}

	n, err := s.q.ReactivateStaff(ctx, db.ReactivateStaffParams{UserID: userID, Role: string(RoleSystemAdmin)})
	if isUniqueViolation(err) {
		// Another instance gave this user an active row between the two steps.
		return nil
	}
	if err != nil {
		return fmt.Errorf("seed staff: reactivate: %w", err)
	}
	if n > 0 {
		s.log.Info("platform admin re-activated", "email_hash", hash)
		return nil
	}

	n, err = s.q.InsertStaff(ctx, db.InsertStaffParams{UserID: userID, Role: string(RoleSystemAdmin), CreatedBy: pgtype.UUID{}})
	if err != nil {
		return fmt.Errorf("seed staff: insert: %w", err)
	}
	if n > 0 {
		s.log.Info("platform admin seeded", "email_hash", hash)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == uniqueViolation
}
