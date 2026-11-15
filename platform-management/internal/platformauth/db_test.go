package platformauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/session"
	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

type noCache struct{}

func (noCache) Get(context.Context, string) ([]byte, bool, error)        { return nil, false, nil }
func (noCache) Set(context.Context, string, []byte, time.Duration) error { return nil }
func (noCache) Del(context.Context, string) error                        { return nil }

// Database test: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. It runs the middleware over the real
// staff store and checks that a revocation applies to the next request.
func TestDBRevocationTakesEffectOnTheNextRequest(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	q := db.New(pool)
	store := staff.NewStore(q)

	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	email := "revoke-" + hex.EncodeToString(b) + "@example.test"
	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO "user" (name, email) VALUES ('Revoke Test', $1) RETURNING id::text`, email).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM platform_staff WHERE user_id = $1::uuid`, userID)
		pool.Exec(ctx, `DELETE FROM "user" WHERE id = $1::uuid`, userID)
	})
	if err := staff.NewSeeder(q, []string{email}, nil).SeedAll(ctx); err != nil {
		t.Fatal(err)
	}

	h := New(Deps{
		Config:    Config{SessionSecret: secret, ConsoleOrigin: origin, SecureCookies: true},
		Directory: store,
		Seeder:    staff.NewSeeder(q, nil, nil),
		Guard:     accountguard.New(store, noCache{}),
		Codes:     newCodes(time.Now),
		Limiter:   newLimiter(),
		Mailer:    &fakeMailer{},
	})
	tok, err := session.Sign(secret, userID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	status := func() int {
		req, _ := http.NewRequest("GET", "/api/platform/me", nil)
		req.RemoteAddr = "203.0.113.9:1"
		req.AddCookie(sessionCookie(tok))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := status(); got != http.StatusOK {
		t.Fatalf("before revocation: %d", got)
	}
	if _, err := pool.Exec(ctx, `UPDATE platform_staff SET revoked_at = now() WHERE user_id = $1::uuid`, userID); err != nil {
		t.Fatal(err)
	}
	if got := status(); got != http.StatusNotFound {
		t.Fatalf("after revocation: %d", got)
	}
}
