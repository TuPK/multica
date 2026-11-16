// Package mfa holds the pieces of TOTP multi-factor sign-in that do not touch
// HTTP or the database: the secret and its sealing, code matching, recovery
// codes, and the per-token attempt limit.
package mfa

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/multica-ai/multica/server/internal/util/secretbox"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	// EnvKey names the variable holding the base64 32-byte key that seals TOTP
	// secrets and keys the recovery-code hashes. With it unset MFA is off.
	EnvKey = "MULTICA_MFA_SECRET_KEY"

	// Issuer is the name an authenticator app shows for the entry.
	Issuer = "Multica"

	// Period is the TOTP time step in seconds.
	Period = 30

	// Skew is how many steps either side of now a code may come from, to
	// absorb clock drift between the phone and the server.
	Skew = 1

	RecoveryCodeCount  = 10
	RecoveryCodeLength = 10

	// MaxWrongCodes is how many wrong codes one pending token may spend.
	MaxWrongCodes = 5

	// recoveryAlphabet has 32 symbols, so a random byte maps onto it without
	// bias, and leaves out 0, 1, I and O, which are misread.
	recoveryAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

// ErrUnavailable means MFA is not configured: the key is unset or unusable.
var ErrUnavailable = errors.New("mfa: secret key not configured")

// Crypto seals TOTP secrets and hashes recovery codes under the configured key.
type Crypto struct {
	box *secretbox.Box
	key []byte
}

// LoadCrypto reads the key from EnvKey. It is read per call rather than held,
// so an operator who sets the key does not need a restart to turn MFA on.
func LoadCrypto() (*Crypto, error) {
	key, err := secretbox.LoadKey(EnvKey)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	box, err := secretbox.New(key)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	return &Crypto{box: box, key: key}, nil
}

// Seal encrypts a base32 TOTP secret for the secret_enc column.
func (c *Crypto) Seal(secret string) ([]byte, error) { return c.box.Seal([]byte(secret)) }

// Open reverses Seal.
func (c *Crypto) Open(sealed []byte) (string, error) {
	plain, err := c.box.Open(sealed)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// HashRecoveryCode returns the stored form of a recovery code: a keyed hash of
// its normalised text. Unkeyed, a leaked table of 50-bit codes could be
// searched offline.
func (c *Crypto) HashRecoveryCode(code string) string {
	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte("mfa-recovery-code:"))
	mac.Write([]byte(NormalizeRecoveryCode(code)))
	return hex.EncodeToString(mac.Sum(nil))
}

// NewSecret makes a TOTP secret for account and the otpauth:// URI an
// authenticator app imports.
func NewSecret(issuer, account string) (secret, uri string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: account,
		Period:      Period,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// MatchStep reports the time step whose code equals code, searching Skew steps
// either side of now. The step, not just a yes, lets the caller refuse a step
// that was already used.
func MatchStep(secret, code string, now time.Time) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != int(otp.DigitsSix) {
		return 0, false
	}
	current := now.Unix() / Period
	for step := current - Skew; step <= current+Skew; step++ {
		want, err := totp.GenerateCodeCustom(secret, time.Unix(step*Period, 0), totp.ValidateOpts{
			Period:    Period,
			Digits:    otp.DigitsSix,
			Algorithm: otp.AlgorithmSHA1,
		})
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

// NewRecoveryCodes returns RecoveryCodeCount distinct random codes.
func NewRecoveryCodes() ([]string, error) {
	codes := make([]string, 0, RecoveryCodeCount)
	seen := make(map[string]bool, RecoveryCodeCount)
	for len(codes) < RecoveryCodeCount {
		var buf [RecoveryCodeLength]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return nil, err
		}
		for i, b := range buf {
			buf[i] = recoveryAlphabet[b&31]
		}
		code := string(buf[:])
		if !seen[code] {
			seen[code] = true
			codes = append(codes, code)
		}
	}
	return codes, nil
}

// NormalizeRecoveryCode upper-cases a typed code and drops spaces and dashes.
func NormalizeRecoveryCode(code string) string {
	return strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(code))
}

// LooksLikeRecoveryCode reports whether code has the shape of a recovery code,
// which tells it apart from a six-digit TOTP code.
func LooksLikeRecoveryCode(code string) bool {
	return len(NormalizeRecoveryCode(code)) == RecoveryCodeLength
}

// AttemptLimiter lets a pending token spend at most MaxWrongCodes attempts and
// one success. State is in this process: with several API instances the cap is
// per instance, so a token can see up to MaxWrongCodes attempts on each.
type AttemptLimiter struct {
	mu      sync.Mutex
	entries map[string]*attemptEntry
}

type attemptEntry struct {
	attempts int
	spent    bool
	expires  time.Time
}

func NewAttemptLimiter() *AttemptLimiter {
	return &AttemptLimiter{entries: map[string]*attemptEntry{}}
}

// Attempts is the limiter the sign-in handler uses.
var Attempts = NewAttemptLimiter()

// Reserve claims one attempt for the token id and reports whether it may go
// ahead. The attempt is counted before the code is checked, so parallel
// requests cannot overshoot the cap. expires is when the token dies; the entry
// is dropped after it.
func (l *AttemptLimiter) Reserve(id string, expires, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now)
	e := l.entries[id]
	if e == nil {
		e = &attemptEntry{expires: expires}
		l.entries[id] = e
	}
	if e.spent || e.attempts >= MaxWrongCodes {
		return false
	}
	e.attempts++
	return true
}

// Spend retires a token after a successful verification.
func (l *AttemptLimiter) Spend(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e := l.entries[id]; e != nil {
		e.spent = true
	}
}

func (l *AttemptLimiter) sweep(now time.Time) {
	for id, e := range l.entries {
		if now.After(e.expires) {
			delete(l.entries, id)
		}
	}
}

func (l *AttemptLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}
