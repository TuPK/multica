// Package session signs and verifies the console session token and builds the
// cookie that carries it. A token is
//
//	base64url(payload) "." base64url(HMAC-SHA256(secret, payload))
//
// with a JSON payload. No table backs it: the middleware re-reads the staff
// row and the account state on every request, so revocation needs no token
// list.
package session

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	// CookieName is the only credential the console reads.
	CookieName = "mpa_session"

	// MaxAge is how long a sign-in stays valid, counted from auth_time.
	MaxAge = 12 * time.Hour

	// MethodEmailCode is the amr value of a sign-in by emailed code.
	MethodEmailCode = "email_code"

	// clockSkew is how far auth_time may lie ahead of the verifying clock.
	clockSkew = time.Minute
)

// ErrInvalid is the only error Verify returns. It never says which check
// failed.
var ErrInvalid = errors.New("invalid session token")

// Claims is the token payload.
type Claims struct {
	UserID   string   `json:"uid"`
	AuthUnix int64    `json:"auth_time"`
	AMR      []string `json:"amr"`
	Exp      int64    `json:"exp"`
}

// AuthTime is when the person signed in.
func (c Claims) AuthTime() time.Time { return time.Unix(c.AuthUnix, 0) }

func sign(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Sign returns a token for userID who signed in at authTime by email code.
func Sign(secret, userID string, authTime time.Time) (string, error) {
	payload, err := json.Marshal(Claims{
		UserID:   userID,
		AuthUnix: authTime.Unix(),
		AMR:      []string{MethodEmailCode},
		Exp:      authTime.Add(MaxAge).Unix(),
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload) + "." + sign(secret, payload), nil
}

// Verify checks the signature, the expiry, and that auth_time is within MaxAge
// of now.
func Verify(secret, token string, now time.Time) (Claims, error) {
	encPayload, encSig, ok := strings.Cut(token, ".")
	if !ok || strings.Contains(encSig, ".") {
		return Claims{}, ErrInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(encPayload)
	if err != nil {
		return Claims{}, ErrInvalid
	}
	gotSig, err := base64.RawURLEncoding.DecodeString(encSig)
	if err != nil {
		return Claims{}, ErrInvalid
	}
	wantSig, _ := base64.RawURLEncoding.DecodeString(sign(secret, payload))
	if !hmac.Equal(gotSig, wantSig) {
		return Claims{}, ErrInvalid
	}

	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil || c.UserID == "" {
		return Claims{}, ErrInvalid
	}
	authTime := c.AuthTime()
	if authTime.After(now.Add(clockSkew)) {
		return Claims{}, ErrInvalid
	}
	if !now.Before(time.Unix(c.Exp, 0)) || !now.Before(authTime.Add(MaxAge)) {
		return Claims{}, ErrInvalid
	}
	return c, nil
}

// NewCookie returns the session cookie. It has no Domain, so only the origin
// that set it receives it back.
func NewCookie(token string, secure bool, now time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  now.Add(MaxAge),
		MaxAge:   int(MaxAge / time.Second),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	}
}

// ClearCookie returns a cookie that makes the browser drop the session cookie.
func ClearCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	}
}
