package staff

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/emailhash"
)

const (
	uidText = "11111111-2222-3333-4444-555555555555"
)

func uid(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatal(err)
	}
	return u
}

// fakeQueries is the part of db.Queries the staff package uses, in memory.
type fakeQueries struct {
	users      map[string]pgtype.UUID // email -> id
	emails     map[pgtype.UUID]string
	active     map[pgtype.UUID]db.PlatformStaff
	revoked    map[pgtype.UUID]int
	inserted   []db.InsertStaffParams
	reactivate []db.ReactivateStaffParams
	state      map[pgtype.UUID]db.GetAccountStateRow

	userErr, activeErr, insertErr, reactivateErr error
}

func newFakeQueries() *fakeQueries {
	return &fakeQueries{
		users:   map[string]pgtype.UUID{},
		emails:  map[pgtype.UUID]string{},
		active:  map[pgtype.UUID]db.PlatformStaff{},
		revoked: map[pgtype.UUID]int{},
		state:   map[pgtype.UUID]db.GetAccountStateRow{},
	}
}

func (q *fakeQueries) addUser(t *testing.T, email string) pgtype.UUID {
	id := uid(t, uidText)
	if len(q.users) > 0 {
		id = uid(t, "aaaaaaaa-2222-3333-4444-"+strings.Repeat("0", 11)+string(rune('0'+len(q.users))))
	}
	q.users[email] = id
	q.emails[id] = email
	return id
}

func (q *fakeQueries) GetUserIDByEmail(_ context.Context, email string) (pgtype.UUID, error) {
	if q.userErr != nil {
		return pgtype.UUID{}, q.userErr
	}
	id, ok := q.users[email]
	if !ok {
		return pgtype.UUID{}, pgx.ErrNoRows
	}
	return id, nil
}

func (q *fakeQueries) GetUserEmailByID(_ context.Context, id pgtype.UUID) (string, error) {
	e, ok := q.emails[id]
	if !ok {
		return "", pgx.ErrNoRows
	}
	return e, nil
}

func (q *fakeQueries) GetActiveStaffByUserID(_ context.Context, id pgtype.UUID) (db.PlatformStaff, error) {
	if q.activeErr != nil {
		return db.PlatformStaff{}, q.activeErr
	}
	row, ok := q.active[id]
	if !ok {
		return db.PlatformStaff{}, pgx.ErrNoRows
	}
	return row, nil
}

func (q *fakeQueries) InsertStaff(_ context.Context, arg db.InsertStaffParams) (int64, error) {
	if q.insertErr != nil {
		return 0, q.insertErr
	}
	q.inserted = append(q.inserted, arg)
	q.active[arg.UserID] = db.PlatformStaff{UserID: arg.UserID, Role: arg.Role}
	return 1, nil
}

func (q *fakeQueries) ReactivateStaff(_ context.Context, arg db.ReactivateStaffParams) (int64, error) {
	if q.reactivateErr != nil {
		return 0, q.reactivateErr
	}
	q.reactivate = append(q.reactivate, arg)
	if q.revoked[arg.UserID] == 0 {
		return 0, nil
	}
	q.revoked[arg.UserID]--
	q.active[arg.UserID] = db.PlatformStaff{UserID: arg.UserID, Role: arg.Role}
	return 1, nil
}

func (q *fakeQueries) GetAccountState(_ context.Context, id pgtype.UUID) (db.GetAccountStateRow, error) {
	row, ok := q.state[id]
	if !ok {
		return db.GetAccountStateRow{}, pgx.ErrNoRows
	}
	return row, nil
}

func logger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestSeedGivesAnAccountHolderAnAdminRow(t *testing.T) {
	q := newFakeQueries()
	id := q.addUser(t, "ops@example.com")
	log, _ := logger()
	if err := NewSeeder(q, []string{"ops@example.com"}, log).SeedAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(q.inserted) != 1 || q.inserted[0].UserID != id || q.inserted[0].Role != "admin" || q.inserted[0].CreatedBy.Valid {
		t.Fatalf("inserted = %+v", q.inserted)
	}
}

func TestSeedReactivatesARevokedRowAsAdmin(t *testing.T) {
	q := newFakeQueries()
	id := q.addUser(t, "ops@example.com")
	q.revoked[id] = 1
	log, _ := logger()
	if err := NewSeeder(q, []string{"ops@example.com"}, log).SeedEmail(context.Background(), "ops@example.com"); err != nil {
		t.Fatal(err)
	}
	if len(q.reactivate) != 1 || q.reactivate[0].Role != "admin" {
		t.Fatalf("reactivate = %+v", q.reactivate)
	}
	if len(q.inserted) != 0 {
		t.Errorf("inserted = %+v, want none", q.inserted)
	}
	if _, ok := q.active[id]; !ok {
		t.Error("no active row after seeding")
	}
}

func TestSeedLeavesAnActiveRowAlone(t *testing.T) {
	q := newFakeQueries()
	id := q.addUser(t, "ops@example.com")
	q.active[id] = db.PlatformStaff{UserID: id, Role: "support"}
	log, _ := logger()
	if err := NewSeeder(q, nil, log).SeedEmail(context.Background(), "ops@example.com"); err != nil {
		t.Fatal(err)
	}
	if len(q.inserted)+len(q.reactivate) != 0 {
		t.Fatalf("changed an active row: %+v %+v", q.inserted, q.reactivate)
	}
	if q.active[id].Role != "support" {
		t.Errorf("role = %q", q.active[id].Role)
	}
}

func TestSeedSkipsAnEmailWithoutAnAccountAndKeepsGoing(t *testing.T) {
	q := newFakeQueries()
	q.addUser(t, "second@example.com")
	log, buf := logger()
	err := NewSeeder(q, []string{"first@example.com", "second@example.com"}, log).SeedAll(context.Background())
	if err != nil {
		t.Fatalf("a missing account must not fail the seeding: %v", err)
	}
	if len(q.inserted) != 1 {
		t.Fatalf("inserted = %+v, want only the second", q.inserted)
	}
	logs := buf.String()
	if !strings.Contains(logs, emailhash.Of("first@example.com")) {
		t.Errorf("the skipped email is not logged by hash: %s", logs)
	}
	if strings.Contains(logs, "first@example.com") {
		t.Errorf("the log has the email address: %s", logs)
	}
}

func TestSeedReturnsADatabaseError(t *testing.T) {
	for name, set := range map[string]func(q *fakeQueries){
		"user lookup": func(q *fakeQueries) { q.userErr = errors.New("db down") },
		"staff read":  func(q *fakeQueries) { q.activeErr = errors.New("db down") },
		"insert":      func(q *fakeQueries) { q.insertErr = errors.New("db down") },
		"reactivate":  func(q *fakeQueries) { q.reactivateErr = errors.New("db down") },
	} {
		t.Run(name, func(t *testing.T) {
			q := newFakeQueries()
			q.addUser(t, "ops@example.com")
			set(q)
			log, _ := logger()
			if err := NewSeeder(q, []string{"ops@example.com"}, log).SeedAll(context.Background()); err == nil {
				t.Fatal("SeedAll returned nil")
			}
		})
	}
}

func TestSeedTreatsALostRaceAsDone(t *testing.T) {
	q := newFakeQueries()
	q.addUser(t, "ops@example.com")
	q.reactivateErr = &pgconn.PgError{Code: "23505"}
	log, _ := logger()
	if err := NewSeeder(q, []string{"ops@example.com"}, log).SeedAll(context.Background()); err != nil {
		t.Fatalf("a unique violation means another instance seeded the row: %v", err)
	}
}

// --- Store ---

func TestStoreActiveStaff(t *testing.T) {
	q := newFakeQueries()
	id := q.addUser(t, "ops@example.com")
	s := NewStore(q)
	ctx := context.Background()

	if _, found, err := s.ActiveStaff(ctx, uidText); err != nil || found {
		t.Fatalf("no row: found=%v err=%v", found, err)
	}
	q.active[id] = db.PlatformStaff{UserID: id, Role: "billing"}
	role, found, err := s.ActiveStaff(ctx, uidText)
	if err != nil || !found || role != RoleBilling {
		t.Fatalf("role=%q found=%v err=%v", role, found, err)
	}
	q.active[id] = db.PlatformStaff{UserID: id, Role: "owner"}
	if _, found, err := s.ActiveStaff(ctx, uidText); err != nil || found {
		t.Fatalf("unknown role: found=%v err=%v", found, err)
	}
	q.activeErr = errors.New("db down")
	if _, _, err := s.ActiveStaff(ctx, uidText); err == nil {
		t.Fatal("a database error was swallowed")
	}
	if _, _, err := s.ActiveStaff(ctx, "not-a-uuid"); err == nil {
		t.Fatal("a bad user id was accepted")
	}
}

func TestStoreUserLookups(t *testing.T) {
	q := newFakeQueries()
	id := q.addUser(t, "ops@example.com")
	s := NewStore(q)
	ctx := context.Background()

	got, found, err := s.UserIDByEmail(ctx, "ops@example.com")
	if err != nil || !found || got != uidText {
		t.Fatalf("got=%q found=%v err=%v", got, found, err)
	}
	if _, found, err := s.UserIDByEmail(ctx, "nobody@example.com"); err != nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	email, err := s.EmailByUserID(ctx, uidText)
	if err != nil || email != "ops@example.com" {
		t.Fatalf("email=%q err=%v", email, err)
	}
	_ = id
}

func TestStoreAccountState(t *testing.T) {
	q := newFakeQueries()
	id := q.addUser(t, "ops@example.com")
	s := NewStore(q)
	ctx := context.Background()

	if _, found, err := s.AccountState(ctx, uidText); err != nil || found {
		t.Fatalf("no row: found=%v err=%v", found, err)
	}
	valid := pgtype.Timestamptz{Time: testTime, Valid: true}
	q.state[id] = db.GetAccountStateRow{Status: "suspended", SessionsValidAfter: valid}
	rec, found, err := s.AccountState(ctx, uidText)
	if err != nil || !found || string(rec.Status) != "suspended" || !rec.SessionsValidAfter.Equal(testTime) {
		t.Fatalf("rec=%+v found=%v err=%v", rec, found, err)
	}
	q.state[id] = db.GetAccountStateRow{Status: "active"}
	rec, _, _ = s.AccountState(ctx, uidText)
	if !rec.SessionsValidAfter.IsZero() {
		t.Errorf("SessionsValidAfter = %v, want zero", rec.SessionsValidAfter)
	}
}

var testTime = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
