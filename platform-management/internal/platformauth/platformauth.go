// Package platformauth serves the console's sign-in routes and guards the rest
// of /api/platform/ with RequirePlatformStaff.
//
// The routes answer without revealing who is staff: send-code always returns
// 204 and verify-code returns one error for every failure. The middleware
// answers 404 to every failed check, with the body of an unknown route, so the
// console's existence is not probed through the API.
package platformauth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/mail"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/internal/emailhash"
	"github.com/multica-ai/multica/platform-management/internal/mfa"
	"github.com/multica-ai/multica/platform-management/internal/session"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

const (
	codeTTL     = 10 * time.Minute
	maxAttempts = 5

	limitWindow = 10 * time.Minute
	emailLimit  = 5
	ipLimit     = 20

	// methodMFA is the amr value of a session that passed the TOTP step.
	methodMFA = "mfa"

	maxBodyBytes = 4 << 10
	mailTimeout  = 30 * time.Second
	maxEmailLen  = 254
)

// Directory is the user table and the staff table, read-only.
type Directory interface {
	// UserIDByEmail takes a lower-cased email.
	UserIDByEmail(ctx context.Context, email string) (userID string, found bool, err error)
	// ActiveStaff returns the role of the user's active staff row. A user with
	// no active row, or with a role this build does not know, is not found.
	ActiveStaff(ctx context.Context, userID string) (role staff.Role, found bool, err error)
	EmailByUserID(ctx context.Context, userID string) (string, error)
}

// MFAStore reads the second factor a staff member enrolled in the Multica web
// app (the user_mfa table).
type MFAStore interface {
	// ConfirmedMFA returns the sealed secret and the last used time step of the
	// user's confirmed row. found is false when there is none.
	ConfirmedMFA(ctx context.Context, userID string) (sealed []byte, lastStep int64, found bool, err error)
	// UseMFAStep records step as used, and returns false when step is not after
	// the last used step.
	UseMFAStep(ctx context.Context, userID string, step int64) (bool, error)
}

// Seeder gives a configured administrator their staff row.
type Seeder interface {
	SeedEmail(ctx context.Context, email string) error
}

// Guard reads a Multica account's state. *accountguard.Guard satisfies it.
type Guard interface {
	State(ctx context.Context, userID string) (accountguard.Record, error)
}

// CodeStore keeps the hashed sign-in codes, one per email hash.
type CodeStore interface {
	// Save replaces any code for the email hash and resets its attempts.
	Save(ctx context.Context, emailHash string, mac []byte, ttl time.Duration) error
	// Attempt counts one verification attempt and returns the stored value and
	// the attempt number. found is false when no live code exists.
	Attempt(ctx context.Context, emailHash string) (mac []byte, attempt int, found bool, err error)
	Delete(ctx context.Context, emailHash string) error
}

// Limiter counts requests in a fixed window.
type Limiter interface {
	// Allow counts one request under key and reports whether it is within limit.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// Mailer sends the code.
type Mailer interface {
	SendCode(ctx context.Context, to, code string) error
}

// Config is the part of the platform configuration this package uses.
type Config struct {
	SessionSecret string
	ConsoleOrigin string
	SecureCookies bool
	// AdminCIDRs limits the middleware to these networks; empty means no limit.
	AdminCIDRs []netip.Prefix
	// AdminEmails are the lower-cased PLATFORM_ADMIN_EMAILS.
	AdminEmails []string
	// DevVerificationCode is an extra code verify-code accepts; empty means
	// none. With it set and a nil Mailer, send-code sends nothing.
	DevVerificationCode string
}

// Deps are the Handler's dependencies. Now defaults to time.Now and Log to
// slog.Default.
type Deps struct {
	Config
	Directory Directory
	Seeder    Seeder
	Guard     Guard
	Codes     CodeStore
	Limiter   Limiter
	Mailer    Mailer
	MFA       MFAStore
	// Crypto opens the secrets in MFA. Without it, or without MFA, no one
	// signs in.
	Crypto *mfa.Crypto
	Log    *slog.Logger
	Now    func() time.Time
}

// Handler serves /api/platform/.
type Handler struct {
	Deps
	admins map[string]bool
	// devMAC is the HMAC of the dev code, nil when none is set. Comparing
	// HMACs keeps the comparison the same length for any submitted code.
	devMAC []byte
	mux    *http.ServeMux
	mails  sync.WaitGroup
}

// New builds the handler.
func New(d Deps) *Handler {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	h := &Handler{Deps: d, admins: map[string]bool{}, mux: http.NewServeMux()}
	if d.DevVerificationCode != "" {
		h.devMAC = h.macOf(d.DevVerificationCode)
	}
	for _, e := range d.AdminEmails {
		h.admins[strings.ToLower(strings.TrimSpace(e))] = true
	}

	h.mux.Handle("POST /api/platform/auth/send-code", h.RequireOrigin(http.HandlerFunc(h.sendCode)))
	h.mux.Handle("POST /api/platform/auth/verify-code", h.RequireOrigin(http.HandlerFunc(h.verifyCode)))
	h.mux.Handle("POST /api/platform/auth/logout", h.RequireOrigin(http.HandlerFunc(h.logout)))
	h.mux.Handle("GET /api/platform/me", h.RequirePlatformStaff()(http.HandlerFunc(h.me)))
	h.mux.HandleFunc("/api/platform/", http.NotFound)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }

// Wait blocks until every mail started by a request has finished.
func (h *Handler) Wait() { h.mails.Wait() }

// RequireOrigin refuses a request whose Origin header is not the console's.
// Without an Origin header the request passes only when it carries no cookie.
func (h *Handler) RequireOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if values, present := r.Header["Origin"]; present {
			if len(values) != 1 || !strings.EqualFold(values[0], h.ConsoleOrigin) {
				writeError(w, http.StatusForbidden, "bad_origin", "origin not allowed")
				return
			}
		} else if len(r.Header.Values("Cookie")) > 0 {
			writeError(w, http.StatusForbidden, "bad_origin", "origin not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	return json.NewDecoder(r.Body).Decode(v) == nil
}

// normalizeEmail returns the lower-cased plain address, or false.
func normalizeEmail(raw string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > maxEmailLen {
		return "", false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", false
	}
	return email, true
}

func (h *Handler) allow(ctx context.Context, key string, limit int) bool {
	ok, err := h.Limiter.Allow(ctx, key, limit, limitWindow)
	if err != nil {
		// Without a count the request cannot be shown to be within the limit.
		h.Log.Warn("platform sign-in: rate limiter unavailable", "error", err)
		return false
	}
	return ok
}

// eligible reports whether the user may sign in: an active staff row and an
// active Multica account.
func (h *Handler) eligible(ctx context.Context, userID string) (staff.Role, bool, error) {
	role, found, err := h.Directory.ActiveStaff(ctx, userID)
	if err != nil || !found {
		return "", false, err
	}
	rec, err := h.Guard.State(ctx, userID)
	if err != nil || rec.Status != accountguard.Active {
		return "", false, err
	}
	return role, true, nil
}

func (h *Handler) sendCode(w http.ResponseWriter, r *http.Request) {
	h.trySendCode(r.Context(), w, r)
	w.WriteHeader(http.StatusNoContent)
}

// trySendCode does the work of send-code. The caller answers 204 whatever
// happens here.
func (h *Handler) trySendCode(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	ip, _ := clientIP(r)
	if !h.allow(ctx, "mpa:rl:ip:"+ipKey(ip), ipLimit) {
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	email, ok := normalizeEmail(body.Email)
	if !ok {
		return
	}
	hash := emailhash.Of(email)
	if !h.allow(ctx, "mpa:rl:email:"+hash, emailLimit) {
		return
	}
	devOnly := h.Mailer == nil
	if devOnly {
		h.Log.Info("platform sign-in: no email provider, code not sent; the dev code applies", "email_hash", hash)
	}

	if h.admins[email] {
		if err := h.Seeder.SeedEmail(ctx, email); err != nil {
			h.Log.Warn("platform sign-in: seeding failed", "email_hash", hash, "error", err)
			return
		}
	}
	userID, found, err := h.Directory.UserIDByEmail(ctx, email)
	if err != nil {
		h.Log.Warn("platform sign-in: user lookup failed", "email_hash", hash, "error", err)
		return
	}
	if !found {
		return
	}
	if _, ok, err := h.eligible(ctx, userID); err != nil {
		h.Log.Warn("platform sign-in: eligibility check failed", "email_hash", hash, "error", err)
		return
	} else if !ok {
		return
	}

	if devOnly {
		return
	}
	code, err := newCode()
	if err != nil {
		h.Log.Error("platform sign-in: code generation failed", "error", err)
		return
	}
	if err := h.Codes.Save(ctx, hash, h.macOf(code), codeTTL); err != nil {
		h.Log.Warn("platform sign-in: code was not stored", "email_hash", hash, "error", err)
		return
	}

	// The mail goes out after the response, so the answer takes the same time
	// whether or not the address belongs to staff.
	h.mails.Add(1)
	go func() {
		defer h.mails.Done()
		mctx, cancel := context.WithTimeout(context.Background(), mailTimeout)
		defer cancel()
		if err := h.Mailer.SendCode(mctx, email, code); err != nil {
			h.Log.Warn("platform sign-in: mail failed", "email_hash", hash, "error", err)
		}
	}()
}

// newCode returns six random decimal digits.
func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// devCodeMatches reports whether code is the configured dev code.
func (h *Handler) devCodeMatches(code string) bool {
	return h.devMAC != nil && subtle.ConstantTimeCompare(h.devMAC, h.macOf(code)) == 1
}

func (h *Handler) macOf(code string) []byte {
	m := hmac.New(sha256.New, []byte(h.SessionSecret))
	m.Write([]byte(code))
	return m.Sum(nil)
}

// SignMFASession returns a session token for a user who signed in with the
// email code and then the TOTP code, so its amr names both.
func SignMFASession(secret, userID string, authTime time.Time) (string, error) {
	payload, err := json.Marshal(session.Claims{
		UserID:   userID,
		AuthUnix: authTime.Unix(),
		AMR:      []string{session.MethodEmailCode, methodMFA},
		Exp:      authTime.Add(session.MaxAge).Unix(),
	})
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

type verifyResult int

const (
	verifyRefused verifyResult = iota
	// verifyNeedsTOTP means the email code is right and no TOTP code came yet.
	verifyNeedsTOTP
	verifyOK
)

func (h *Handler) verifyCode(w http.ResponseWriter, r *http.Request) {
	userID, role, result := h.tryVerifyCode(r.Context(), w, r)
	switch result {
	case verifyRefused:
		writeError(w, http.StatusBadRequest, "invalid_code", "invalid code")
		return
	case verifyNeedsTOTP:
		writeJSON(w, http.StatusOK, map[string]bool{"mfa_required": true})
		return
	}
	now := h.Now()
	token, err := SignMFASession(h.SessionSecret, userID, now)
	if err != nil {
		h.Log.Error("platform sign-in: token signing failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	http.SetCookie(w, session.NewCookie(token, h.SecureCookies, now))
	writeJSON(w, http.StatusOK, map[string]string{"user_id": userID, "role": string(role)})
}

// tryVerifyCode checks the email code, then the TOTP code of the staff member's
// confirmed second factor. A request that carries the right email code and no
// TOTP code does not spend the email code, so the console can ask for the TOTP
// code next. Every request counts as one attempt on the email code, so wrong
// TOTP codes are limited by the same five attempts.
func (h *Handler) tryVerifyCode(ctx context.Context, w http.ResponseWriter, r *http.Request) (string, staff.Role, verifyResult) {
	var body struct {
		Email string `json:"email"`
		Code  string `json:"code"`
		TOTP  string `json:"totp"`
	}
	if !decodeBody(w, r, &body) {
		return "", "", verifyRefused
	}
	email, ok := normalizeEmail(body.Email)
	if !ok {
		return "", "", verifyRefused
	}
	hash := emailhash.Of(email)

	// attempt is the number of this attempt on the stored email code, and 0
	// when the dev code stood in for it.
	attempt := 0
	if !h.devCodeMatches(body.Code) {
		if attempt, ok = h.matchStoredCode(ctx, hash, body.Code); !ok {
			return "", "", verifyRefused
		}
	}

	userID, role, step, result := h.checkSecondFactor(ctx, hash, email, body.TOTP)
	switch result {
	case verifyNeedsTOTP:
		return userID, role, result
	case verifyOK:
		// A step works once, and an email code works once. If either cannot be
		// recorded the sign-in must not be accepted.
		if h.claimStep(ctx, hash, userID, step) && (attempt == 0 || h.spendCode(ctx, hash)) {
			return userID, role, verifyOK
		}
	}
	if attempt >= maxAttempts {
		h.deleteCode(ctx, hash)
	}
	return "", "", verifyRefused
}

// checkSecondFactor finds the staff member and checks their TOTP code against
// the secret they enrolled in Multica. It refuses a staff member with no
// confirmed second factor, and whenever the secret cannot be read or opened.
func (h *Handler) checkSecondFactor(ctx context.Context, hash, email, totp string) (string, staff.Role, int64, verifyResult) {
	userID, found, err := h.Directory.UserIDByEmail(ctx, email)
	if err != nil || !found {
		return "", "", 0, verifyRefused
	}
	role, ok, err := h.eligible(ctx, userID)
	if err != nil || !ok {
		return "", "", 0, verifyRefused
	}
	if h.MFA == nil || h.Crypto == nil {
		h.Log.Error("platform sign-in: the second factor is not configured; nobody can sign in")
		return "", "", 0, verifyRefused
	}
	sealed, _, found, err := h.MFA.ConfirmedMFA(ctx, userID)
	if err != nil {
		h.Log.Warn("platform sign-in: second factor lookup failed", "email_hash", hash, "error", err)
		return "", "", 0, verifyRefused
	}
	if !found {
		h.Log.Info("platform sign-in refused: the staff member has no confirmed second factor", "email_hash", hash)
		return "", "", 0, verifyRefused
	}
	if strings.TrimSpace(totp) == "" {
		return userID, role, 0, verifyNeedsTOTP
	}
	secretText, err := h.Crypto.Open(sealed)
	if err != nil {
		h.Log.Error("platform sign-in: the stored second-factor secret does not open; check MULTICA_MFA_SECRET_KEY", "email_hash", hash)
		return "", "", 0, verifyRefused
	}
	step, ok := mfa.MatchStep(secretText, totp, h.Now())
	if !ok {
		return "", "", 0, verifyRefused
	}
	return userID, role, step, verifyOK
}

// claimStep records the TOTP step as used, and reports false when it was used
// already or cannot be recorded.
func (h *Handler) claimStep(ctx context.Context, hash, userID string, step int64) bool {
	used, err := h.MFA.UseMFAStep(ctx, userID, step)
	if err != nil {
		h.Log.Warn("platform sign-in: TOTP step was not recorded", "email_hash", hash, "error", err)
		return false
	}
	return used
}

// matchStoredCode counts an attempt against the code sent to the email and
// reports whether code is it, with the attempt number. A right code is kept
// until the whole sign-in succeeds.
func (h *Handler) matchStoredCode(ctx context.Context, hash, code string) (int, bool) {
	stored, attempt, found, err := h.Codes.Attempt(ctx, hash)
	if err != nil {
		h.Log.Warn("platform sign-in: code lookup failed", "email_hash", hash, "error", err)
		return 0, false
	}
	if !found {
		return 0, false
	}
	if attempt > maxAttempts || !hmac.Equal(stored, h.macOf(code)) {
		if attempt >= maxAttempts {
			h.deleteCode(ctx, hash)
		}
		return 0, false
	}
	return attempt, true
}

func (h *Handler) deleteCode(ctx context.Context, hash string) {
	if err := h.Codes.Delete(ctx, hash); err != nil {
		h.Log.Warn("platform sign-in: spent code was not deleted", "email_hash", hash, "error", err)
	}
}

// spendCode deletes the email code after a sign-in. A code that cannot be
// deleted must not be accepted.
func (h *Handler) spendCode(ctx context.Context, hash string) bool {
	if err := h.Codes.Delete(ctx, hash); err != nil {
		h.Log.Warn("platform sign-in: used code was not deleted", "email_hash", hash, "error", err)
		return false
	}
	return true
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, session.ClearCookie(h.SecureCookies))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	id, ok := IdentityFrom(r.Context())
	if !ok {
		http.NotFound(w, r)
		return
	}
	email, err := h.Directory.EmailByUserID(r.Context(), id.UserID)
	if err != nil {
		h.Log.Warn("platform me: email lookup failed", "user_id", id.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"user_id": id.UserID, "email": email, "role": string(id.Role)})
}
