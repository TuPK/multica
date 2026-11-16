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
	"github.com/multica-ai/multica/platform-management/internal/session"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

const (
	codeTTL     = 10 * time.Minute
	maxAttempts = 5

	limitWindow = 10 * time.Minute
	emailLimit  = 5
	ipLimit     = 20

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
	Log       *slog.Logger
	Now       func() time.Time
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

func (h *Handler) verifyCode(w http.ResponseWriter, r *http.Request) {
	userID, role, ok := h.tryVerifyCode(r.Context(), w, r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_code", "invalid code")
		return
	}
	now := h.Now()
	token, err := session.Sign(h.SessionSecret, userID, now)
	if err != nil {
		h.Log.Error("platform sign-in: token signing failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	http.SetCookie(w, session.NewCookie(token, h.SecureCookies, now))
	writeJSON(w, http.StatusOK, map[string]string{"user_id": userID, "role": string(role)})
}

func (h *Handler) tryVerifyCode(ctx context.Context, w http.ResponseWriter, r *http.Request) (string, staff.Role, bool) {
	var body struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if !decodeBody(w, r, &body) {
		return "", "", false
	}
	email, ok := normalizeEmail(body.Email)
	if !ok {
		return "", "", false
	}
	hash := emailhash.Of(email)

	if !h.devCodeMatches(body.Code) {
		if !h.checkStoredCode(ctx, hash, body.Code) {
			return "", "", false
		}
	}
	userID, found, err := h.Directory.UserIDByEmail(ctx, email)
	if err != nil || !found {
		return "", "", false
	}
	role, ok, err := h.eligible(ctx, userID)
	if err != nil || !ok {
		return "", "", false
	}
	return userID, role, true
}

// checkStoredCode counts an attempt against the code sent to the email and
// reports whether code is it. A right code is deleted: it works once.
func (h *Handler) checkStoredCode(ctx context.Context, hash, code string) bool {
	stored, attempt, found, err := h.Codes.Attempt(ctx, hash)
	if err != nil {
		h.Log.Warn("platform sign-in: code lookup failed", "email_hash", hash, "error", err)
		return false
	}
	if !found {
		return false
	}
	if !hmac.Equal(stored, h.macOf(code)) {
		if attempt >= maxAttempts {
			if err := h.Codes.Delete(ctx, hash); err != nil {
				h.Log.Warn("platform sign-in: spent code was not deleted", "email_hash", hash, "error", err)
			}
		}
		return false
	}
	// A code works once. If it cannot be deleted it must not be accepted.
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
