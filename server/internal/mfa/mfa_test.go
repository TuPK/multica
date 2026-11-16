package mfa

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const testKeyB64 = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" // 32 bytes

func TestLoadCryptoUnavailableWithoutKey(t *testing.T) {
	t.Setenv(EnvKey, "")
	if _, err := LoadCrypto(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("LoadCrypto with no key = %v, want ErrUnavailable", err)
	}
	t.Setenv(EnvKey, "not-base64-!!")
	if _, err := LoadCrypto(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("LoadCrypto with a malformed key = %v, want ErrUnavailable", err)
	}
	t.Setenv(EnvKey, base64.StdEncoding.EncodeToString([]byte("short")))
	if _, err := LoadCrypto(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("LoadCrypto with a short key = %v, want ErrUnavailable", err)
	}
}

func TestSecretIsSealedNotEncoded(t *testing.T) {
	t.Setenv(EnvKey, testKeyB64)
	c, err := LoadCrypto()
	if err != nil {
		t.Fatalf("LoadCrypto: %v", err)
	}
	secret, _, err := NewSecret("Multica", "a@example.com")
	if err != nil {
		t.Fatalf("NewSecret: %v", err)
	}
	sealed, err := c.Seal(secret)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed, []byte(secret)) {
		t.Fatal("sealed value contains the secret in clear")
	}
	if string(sealed) == base64.StdEncoding.EncodeToString([]byte(secret)) {
		t.Fatal("sealed value is only the base64 of the secret")
	}
	opened, err := c.Open(sealed)
	if err != nil || opened != secret {
		t.Fatalf("Open = %q, %v; want the secret back", opened, err)
	}
}

func TestNewSecretBuildsAnOTPAuthURI(t *testing.T) {
	secret, uri, err := NewSecret("Multica", "a@example.com")
	if err != nil {
		t.Fatalf("NewSecret: %v", err)
	}
	if secret == "" || !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Fatalf("secret %q, uri %q", secret, uri)
	}
	if !strings.Contains(uri, "secret="+secret) || !strings.Contains(uri, "issuer=Multica") {
		t.Fatalf("uri %q does not carry the secret and issuer", uri)
	}
}

func TestMatchStep(t *testing.T) {
	secret, _, err := NewSecret("Multica", "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_010, 0)
	code, err := totp.GenerateCodeCustom(secret, now, totp.ValidateOpts{Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		t.Fatal(err)
	}

	step, ok := MatchStep(secret, code, now)
	if !ok || step != now.Unix()/30 {
		t.Fatalf("MatchStep = %d, %v; want step %d", step, ok, now.Unix()/30)
	}
	// One step of skew either way.
	if step, ok := MatchStep(secret, code, now.Add(30*time.Second)); !ok || step != now.Unix()/30 {
		t.Fatalf("code one step old: MatchStep = %d, %v", step, ok)
	}
	if _, ok := MatchStep(secret, code, now.Add(90*time.Second)); ok {
		t.Fatal("code three steps old was accepted")
	}
	if _, ok := MatchStep(secret, "000000x", now); ok {
		t.Fatal("malformed code was accepted")
	}
	if _, ok := MatchStep(secret, "", now); ok {
		t.Fatal("empty code was accepted")
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, err := NewRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 10 {
		t.Fatalf("got %d codes, want 10", len(codes))
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if len(c) != 10 {
			t.Fatalf("code %q has length %d, want 10", c, len(c))
		}
		if strings.ContainsAny(c, "01OI") {
			t.Fatalf("code %q uses an ambiguous character", c)
		}
		if seen[c] {
			t.Fatalf("duplicate code %q", c)
		}
		seen[c] = true
	}

	t.Setenv(EnvKey, testKeyB64)
	crypto, err := LoadCrypto()
	if err != nil {
		t.Fatal(err)
	}
	h := crypto.HashRecoveryCode(codes[0])
	if h == codes[0] || h == "" {
		t.Fatalf("hash %q is not a hash", h)
	}
	if crypto.HashRecoveryCode(strings.ToLower(codes[0][:5])+"-"+codes[0][5:]) != h {
		t.Fatal("case and a dash must not change the hash")
	}
	if crypto.HashRecoveryCode(codes[1]) == h {
		t.Fatal("two codes hash alike")
	}
}

func TestAttemptLimiterFailsTheTokenOnTheSixthAttempt(t *testing.T) {
	l := NewAttemptLimiter()
	now := time.Now()
	exp := now.Add(5 * time.Minute)
	for i := 1; i <= 5; i++ {
		if !l.Reserve("tok", exp, now) {
			t.Fatalf("attempt %d refused", i)
		}
	}
	if l.Reserve("tok", exp, now) {
		t.Fatal("sixth attempt allowed")
	}
	if !l.Reserve("other", exp, now) {
		t.Fatal("another token was refused")
	}
}

func TestAttemptLimiterSpendsAToken(t *testing.T) {
	l := NewAttemptLimiter()
	now := time.Now()
	exp := now.Add(5 * time.Minute)
	if !l.Reserve("tok", exp, now) {
		t.Fatal("first attempt refused")
	}
	l.Spend("tok")
	if l.Reserve("tok", exp, now) {
		t.Fatal("a spent token was accepted again")
	}
}

func TestAttemptLimiterForgetsExpiredTokens(t *testing.T) {
	l := NewAttemptLimiter()
	now := time.Now()
	l.Reserve("old", now.Add(time.Minute), now)
	l.Reserve("new", now.Add(time.Hour), now.Add(2*time.Minute))
	if n := l.size(); n != 1 {
		t.Fatalf("limiter holds %d tokens, want 1 after the first expired", n)
	}
}
