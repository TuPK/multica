package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// newTestHandler builds a handler with only the given config, for tests that
// never reach the database.
func newTestHandler(cfg Config) *Handler {
	return &Handler{
		cfg: cfg,
	}
}

// loginHandler returns the shared test handler with email delivery kept local,
// even when the developer has configured a provider.
func loginHandler(t *testing.T) *Handler {
	t.Helper()
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("SMTP_HOST", "")
	h := *testHandler
	h.EmailService = service.NewEmailService()
	return &h
}

// loginAccount inserts a Delegora user. A non-empty status also writes its
// platform_account row; an empty status leaves no row, which counts as active.
func loginAccount(t *testing.T, email, status string) {
	t.Helper()
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
	dbfx.Cleanup(t, `DELETE FROM platform_account WHERE user_id IN (SELECT id FROM "user" WHERE email = $1)`, email)
	dbfx.Cleanup(t, `DELETE FROM verification_code WHERE email = $1`, email)
	userID := dbfx.Insert(t, "user", testutil.Cols{"name": "Login Test", "email": email})
	if status != "" {
		dbfx.Exec(t, `INSERT INTO platform_account (user_id, status) VALUES ($1, $2)`, userID, status)
	}
}

func sendCodeBody(email string) map[string]string {
	return map[string]string{"email": email}
}

// Login only sends a code to an email platform management registered and left
// active. The refusal happens before any code is stored or emailed.
func TestSendCodeRefusesUnregisteredEmail(t *testing.T) {
	email := testutil.UniqueEmail("unregistered")
	dbfx.Cleanup(t, `DELETE FROM verification_code WHERE email = $1`, email)

	resp := testutil.Call(t, loginHandler(t).SendCode, testutil.JSONRequest(http.MethodPost, "/auth/send-code", sendCodeBody(email))).Want(http.StatusForbidden).Map()
	if resp["error"] != ErrAccountNotRegistered.Error() {
		t.Fatalf("error = %v, want %q", resp["error"], ErrAccountNotRegistered.Error())
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM verification_code WHERE email = $1`, email); n != 0 {
		t.Fatalf("a code was stored for an unregistered email: %d rows", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE email = $1`, email); n != 0 {
		t.Fatalf("send-code created a user: %d rows", n)
	}
}

// A code that already exists for an unregistered email must not sign anyone in,
// and verifying it must not create the account.
func TestVerifyCodeRefusesUnregisteredEmailWithoutCreatingAccount(t *testing.T) {
	email := testutil.UniqueEmail("unregistered-verify")
	dbfx.Cleanup(t, `DELETE FROM "user" WHERE email = $1`, email)
	dbfx.Cleanup(t, `DELETE FROM verification_code WHERE email = $1`, email)
	h := loginHandler(t)
	const code = "424242"
	if _, err := h.Queries.CreateVerificationCode(context.Background(), db.CreateVerificationCodeParams{
		Email:     email,
		Code:      code,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(10 * time.Minute), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	resp := testutil.Call(t, h.VerifyCode, testutil.JSONRequest(http.MethodPost, "/auth/verify-code", map[string]string{"email": email, "code": code})).Want(http.StatusForbidden).Map()
	if resp["error"] != ErrAccountNotRegistered.Error() {
		t.Fatalf("error = %v, want %q", resp["error"], ErrAccountNotRegistered.Error())
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE email = $1`, email); n != 0 {
		t.Fatalf("verify-code created a user: %d rows", n)
	}
}

// Only an active platform account, or no platform account row at all, may sign
// in. Suspended, pending deletion and deleted accounts get no code.
func TestSendCodeAccountStatusGate(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   int
	}{
		{"no platform row counts as active", "", http.StatusOK},
		{"active", "active", http.StatusOK},
		{"suspended", "suspended", http.StatusForbidden},
		{"pending deletion", "pending_deletion", http.StatusForbidden},
		{"deleted", "deleted", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email := testutil.UniqueEmail("status-" + tt.status)
			loginAccount(t, email, tt.status)
			resp := testutil.Call(t, loginHandler(t).SendCode, testutil.JSONRequest(http.MethodPost, "/auth/send-code", sendCodeBody(email))).Want(tt.want).Map()

			sent := dbfx.Count(t, `SELECT count(*) FROM verification_code WHERE email = $1`, email)
			if tt.want == http.StatusOK && sent != 1 {
				t.Fatalf("codes stored = %d, want 1", sent)
			}
			if tt.want != http.StatusOK {
				if sent != 0 {
					t.Fatalf("codes stored for a %s account: %d", tt.name, sent)
				}
				if resp["error"] != ErrAccountNotActive.Error() {
					t.Fatalf("error = %v, want %q", resp["error"], ErrAccountNotActive.Error())
				}
			}
		})
	}
}

// mockDB answers every query with getUserErr. Login only reads the user row
// before it refuses or continues, so a fake that fails that read is enough.
type mockDB struct {
	db.DBTX
	getUserErr error
}

func (m *mockDB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return &mockRow{err: m.getUserErr}
}

func (m *mockDB) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("INSERT 1"), nil
}

type mockRow struct {
	pgx.Row
	err error
}

func (m *mockRow) Scan(dest ...interface{}) error {
	return m.err
}
