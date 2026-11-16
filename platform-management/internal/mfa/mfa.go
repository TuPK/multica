// Package mfa reads the TOTP secrets that the Multica server seals. The format
// is the one in server/internal/mfa and server/internal/util/secretbox:
// AES-256-GCM with the 12-byte nonce in front, under the key in EnvKey, and
// standard six-digit SHA-1 codes with a 30-second step. The platform lives in
// its own module and cannot import that code, so only the decryption and the
// code check are here; a test opens a value that Multica's code sealed.
package mfa

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// EnvKey names the variable holding the base64 32-byte key. It is the
	// variable the Multica server uses, so one key serves both services.
	EnvKey = "MULTICA_MFA_SECRET_KEY"

	// Period is the TOTP time step in seconds.
	Period = 30

	// Skew is how many steps either side of now a code may come from.
	Skew = 1

	keySize    = 32
	codeDigits = 6
)

// ParseKey decodes the base64 value of EnvKey. The error never repeats the
// value.
func ParseKey(raw string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.New("is not valid base64")
	}
	if len(key) != keySize {
		return nil, fmt.Errorf("decodes to %d bytes, expected %d", len(key), keySize)
	}
	return key, nil
}

// Crypto opens the sealed secrets.
type Crypto struct{ aead cipher.AEAD }

// NewCrypto returns a Crypto for a 32-byte key.
func NewCrypto(key []byte) (*Crypto, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("mfa: key must be %d bytes", keySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Crypto{aead: aead}, nil
}

// Open returns the base32 secret that Multica sealed.
func (c *Crypto) Open(sealed []byte) (string, error) {
	ns := c.aead.NonceSize()
	if len(sealed) < ns+c.aead.Overhead() {
		return "", errors.New("mfa: sealed value too short")
	}
	plain, err := c.aead.Open(nil, sealed[:ns], sealed[ns:], nil)
	if err != nil {
		return "", errors.New("mfa: sealed value does not open")
	}
	return string(plain), nil
}

// CodeAt returns the six-digit code for the secret at time t.
func CodeAt(secret string, t time.Time) (string, error) {
	return codeForStep(secret, t.Unix()/Period)
}

func codeForStep(secret string, step int64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).
		DecodeString(strings.TrimRight(strings.ToUpper(strings.TrimSpace(secret)), "="))
	if err != nil {
		return "", errors.New("mfa: secret is not base32")
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", codeDigits, value%1_000_000), nil
}

// MatchStep reports the time step whose code equals code, searching Skew steps
// either side of now. The step lets the caller refuse one already used.
func MatchStep(secret, code string, now time.Time) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != codeDigits {
		return 0, false
	}
	current := now.Unix() / Period
	for step := current - Skew; step <= current+Skew; step++ {
		want, err := codeForStep(secret, step)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

// Sampler returns the sealed secret of one confirmed user_mfa row.
type Sampler interface {
	AnyConfirmedSecret(ctx context.Context) (sealed []byte, found bool, err error)
}

// CheckKey proves that c opens what Multica sealed, by opening one confirmed
// row. With no row there is nothing to prove and it passes. A mismatch names
// EnvKey, never the key or the secret.
func CheckKey(ctx context.Context, c *Crypto, s Sampler) error {
	sealed, found, err := s.AnyConfirmedSecret(ctx)
	if err != nil {
		return fmt.Errorf("check %s against user_mfa: %w", EnvKey, err)
	}
	if !found {
		return nil
	}
	if _, err := c.Open(sealed); err != nil {
		return fmt.Errorf("%s does not open the second-factor secrets Multica stored; it must be the same value as in the Multica server", EnvKey)
	}
	return nil
}
