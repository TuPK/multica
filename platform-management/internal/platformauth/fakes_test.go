package platformauth

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/internal/emailhash"
	"github.com/multica-ai/multica/platform-management/internal/mfa"
	"github.com/multica-ai/multica/platform-management/internal/session"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

const (
	secret        = "0123456789abcdef0123456789abcdef"
	origin        = "https://console.example.com"
	staffEmail    = "ops@example.com"
	staffUserID   = "11111111-2222-3333-4444-555555555555"
	otherUserID   = "99999999-2222-3333-4444-555555555555"
	otherEmail    = "other@example.com"
	secureDefault = true
)

// mfaSecret is the base32 TOTP secret the fake staff member enrolled.
const mfaSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

// testKey is the 32-byte MULTICA_MFA_SECRET_KEY of these tests.
func testKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

// seal encrypts a secret the way Multica does: AES-256-GCM, nonce first.
func seal(t *testing.T, key []byte, plain string) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	return gcm.Seal(nonce, nonce, []byte(plain), nil)
}

type mfaRow struct {
	sealed   []byte
	lastStep int64
}

// fakeMFA is the confirmed user_mfa rows in memory.
type fakeMFA struct {
	mu   sync.Mutex
	rows map[string]*mfaRow
	err  error
}

func (m *fakeMFA) ConfirmedMFA(_ context.Context, userID string) ([]byte, int64, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, 0, false, m.err
	}
	r, ok := m.rows[userID]
	if !ok {
		return nil, 0, false, nil
	}
	return r.sealed, r.lastStep, true, nil
}

func (m *fakeMFA) UseMFAStep(_ context.Context, userID string, step int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return false, m.err
	}
	r, ok := m.rows[userID]
	if !ok || r.lastStep >= step {
		return false, nil
	}
	r.lastStep = step
	return true, nil
}

var t0 = time.Date(2030, 5, 6, 7, 8, 9, 0, time.UTC)

// fakeDirectory is the staff store and the user table in memory.
type fakeDirectory struct {
	mu       sync.Mutex
	users    map[string]string     // email -> user id
	emails   map[string]string     // user id -> email
	active   map[string]staff.Role // user id -> role of the active row
	err      error
	staffErr error
}

func newDirectory() *fakeDirectory {
	d := &fakeDirectory{
		users:  map[string]string{},
		emails: map[string]string{},
		active: map[string]staff.Role{},
	}
	d.addUser(staffEmail, staffUserID)
	d.active[staffUserID] = staff.RoleAdmin
	return d
}

func (d *fakeDirectory) addUser(email, id string) {
	d.users[email] = id
	d.emails[id] = email
}

func (d *fakeDirectory) UserIDByEmail(_ context.Context, email string) (string, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return "", false, d.err
	}
	id, ok := d.users[email]
	return id, ok, nil
}

func (d *fakeDirectory) ActiveStaff(_ context.Context, userID string) (staff.Role, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.staffErr != nil {
		return "", false, d.staffErr
	}
	r, ok := d.active[userID]
	return r, ok, nil
}

func (d *fakeDirectory) EmailByUserID(_ context.Context, userID string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if e, ok := d.emails[userID]; ok {
		return e, nil
	}
	return "", errors.New("no such user")
}

func (d *fakeDirectory) revoke(userID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.active, userID)
}

// fakeSeeder records calls and, when grant is set, gives the email's user an
// admin row the way the real seeding does.
type fakeSeeder struct {
	dir   *fakeDirectory
	calls []string
	grant bool
	err   error
}

func (s *fakeSeeder) SeedEmail(_ context.Context, email string) error {
	s.calls = append(s.calls, email)
	if s.err != nil {
		return s.err
	}
	if s.grant {
		if id, ok := s.dir.users[email]; ok {
			s.dir.active[id] = staff.RoleAdmin
		}
	}
	return nil
}

type fakeGuard struct {
	recs map[string]accountguard.Record
	err  error
}

func (g *fakeGuard) State(_ context.Context, userID string) (accountguard.Record, error) {
	if g.err != nil {
		return accountguard.Record{}, g.err
	}
	if r, ok := g.recs[userID]; ok {
		return r, nil
	}
	return accountguard.Record{Status: accountguard.Active}, nil
}

type codeEntry struct {
	mac      []byte
	attempts int
	expires  time.Time
}

// fakeCodes is the Redis code store in memory, with a clock.
type fakeCodes struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]*codeEntry
	ttls    []time.Duration
	err     error
}

func newCodes(now func() time.Time) *fakeCodes {
	return &fakeCodes{now: now, entries: map[string]*codeEntry{}}
}

func (c *fakeCodes) Save(_ context.Context, emailHash string, mac []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.ttls = append(c.ttls, ttl)
	c.entries[emailHash] = &codeEntry{mac: append([]byte(nil), mac...), expires: c.now().Add(ttl)}
	return nil
}

func (c *fakeCodes) Attempt(_ context.Context, emailHash string) ([]byte, int, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return nil, 0, false, c.err
	}
	e, ok := c.entries[emailHash]
	if !ok || !c.now().Before(e.expires) {
		delete(c.entries, emailHash)
		return nil, 0, false, nil
	}
	e.attempts++
	return e.mac, e.attempts, true, nil
}

func (c *fakeCodes) Delete(_ context.Context, emailHash string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, emailHash)
	return nil
}

func (c *fakeCodes) has(email string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.entries[emailhash.Of(email)]
	return ok
}

type limitCall struct {
	key    string
	limit  int
	window time.Duration
}

// fakeLimiter counts per key and refuses once a key passes its limit.
type fakeLimiter struct {
	mu     sync.Mutex
	counts map[string]int
	calls  []limitCall
	err    error
}

func newLimiter() *fakeLimiter { return &fakeLimiter{counts: map[string]int{}} }

func (l *fakeLimiter) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, limitCall{key, limit, window})
	if l.err != nil {
		return false, l.err
	}
	l.counts[key]++
	return l.counts[key] <= limit, nil
}

type sentMail struct{ to, code string }

type fakeMailer struct {
	mu   sync.Mutex
	sent []sentMail
	err  error
}

func (m *fakeMailer) SendCode(_ context.Context, to, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, sentMail{to, code})
	return nil
}

func (m *fakeMailer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

func (m *fakeMailer) last() sentMail {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sent[len(m.sent)-1]
}

// env is one handler with every fake behind it.
type env struct {
	t       *testing.T
	h       *Handler
	dir     *fakeDirectory
	seeder  *fakeSeeder
	guard   *fakeGuard
	codes   *fakeCodes
	limiter *fakeLimiter
	mailer  *fakeMailer
	mfa     *fakeMFA
	logs    *bytes.Buffer
	now     time.Time
	cfg     Config
}

func newEnv(t *testing.T, mutate ...func(*Config)) *env {
	t.Helper()
	e := &env{t: t, dir: newDirectory(), guard: &fakeGuard{recs: map[string]accountguard.Record{}}, limiter: newLimiter(), mailer: &fakeMailer{}, logs: &bytes.Buffer{}, now: t0}
	e.mfa = &fakeMFA{rows: map[string]*mfaRow{staffUserID: {sealed: seal(t, testKey(), mfaSecret)}}}
	crypto, err := mfa.NewCrypto(testKey())
	if err != nil {
		t.Fatal(err)
	}
	e.seeder = &fakeSeeder{dir: e.dir}
	e.codes = newCodes(func() time.Time { return e.now })
	e.cfg = Config{
		SessionSecret: secret,
		ConsoleOrigin: origin,
		SecureCookies: secureDefault,
		AdminEmails:   []string{staffEmail},
	}
	for _, m := range mutate {
		m(&e.cfg)
	}
	e.h = New(Deps{
		Config:    e.cfg,
		Directory: e.dir,
		Seeder:    e.seeder,
		Guard:     e.guard,
		Codes:     e.codes,
		Limiter:   e.limiter,
		Mailer:    e.mailer,
		MFA:       e.mfa,
		Crypto:    crypto,
		Log:       slog.New(slog.NewTextHandler(e.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Now:       func() time.Time { return e.now },
	})
	return e
}

func withCIDRs(cidrs ...string) func(*Config) {
	return func(c *Config) {
		for _, s := range cidrs {
			c.AdminCIDRs = append(c.AdminCIDRs, netip.MustParsePrefix(s))
		}
	}
}

// request describes one request to the handler.
type request struct {
	method, path, body string
	headers            map[string]string
	cookies            []*http.Cookie
	remoteAddr         string
}

func (e *env) do(r request) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(r.method, r.path, strings.NewReader(r.body))
	if r.remoteAddr != "" {
		req.RemoteAddr = r.remoteAddr
	}
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}
	for _, c := range r.cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	e.h.Wait()
	return rec
}

func (e *env) post(path, body string) *httptest.ResponseRecorder {
	return e.do(request{method: "POST", path: path, body: body, headers: map[string]string{"Origin": origin}, remoteAddr: "203.0.113.9:4444"})
}

func (e *env) sendCode(email string) *httptest.ResponseRecorder {
	return e.post("/api/platform/auth/send-code", `{"email":"`+email+`"}`)
}

// verify submits the email code with the staff member's current TOTP code.
func (e *env) verify(email, code string) *httptest.ResponseRecorder {
	return e.verifyWith(email, code, e.totp())
}

// verifyWith submits the email code and the given TOTP code, which may be empty.
func (e *env) verifyWith(email, code, totp string) *httptest.ResponseRecorder {
	return e.post("/api/platform/auth/verify-code", `{"email":"`+email+`","code":"`+code+`","totp":"`+totp+`"}`)
}

// totp is the code the staff member's authenticator shows at the test clock.
func (e *env) totp() string {
	e.t.Helper()
	c, err := mfa.CodeAt(mfaSecret, e.now)
	if err != nil {
		e.t.Fatal(err)
	}
	return c
}

// token mints a valid session token for the user, signed at authTime.
func (e *env) token(userID string, authTime time.Time) string {
	e.t.Helper()
	tok, err := SignMFASession(secret, userID, authTime)
	if err != nil {
		e.t.Fatal(err)
	}
	return tok
}

func sessionCookie(tok string) *http.Cookie {
	return &http.Cookie{Name: session.CookieName, Value: tok}
}
