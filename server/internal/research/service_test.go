package research

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestNewBoxPrefersTheExplicitKey(t *testing.T) {
	t.Setenv("JWT_SECRET", "some-jwt-secret")
	t.Setenv(SecretKeyEnv, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	box, fromJWT, err := NewBox()
	if err != nil || box == nil || fromJWT {
		t.Fatalf("box=%v fromJWT=%v err=%v", box, fromJWT, err)
	}
}

func TestNewBoxFallsBackToTheJWTSecretAndSealsDeterministically(t *testing.T) {
	t.Setenv(SecretKeyEnv, "")
	t.Setenv("JWT_SECRET", "some-jwt-secret")
	a, fromJWT, err := NewBox()
	if err != nil || a == nil || !fromJWT {
		t.Fatalf("box=%v fromJWT=%v err=%v", a, fromJWT, err)
	}
	sealed, err := a.Seal([]byte("OA-KEY"))
	if err != nil {
		t.Fatal(err)
	}
	// The same secret on another start opens what the first start sealed.
	b, _, _ := NewBox()
	if got, err := b.Open(sealed); err != nil || string(got) != "OA-KEY" {
		t.Fatalf("%q %v", got, err)
	}
	// Another secret does not.
	t.Setenv("JWT_SECRET", "another-secret")
	c, _, _ := NewBox()
	if _, err := c.Open(sealed); err == nil {
		t.Fatal("a different JWT_SECRET opened the sealed key")
	}
}

func TestNewBoxWithNoSecretIsNilAndAnInvalidKeyIsAnError(t *testing.T) {
	t.Setenv(SecretKeyEnv, "")
	t.Setenv("JWT_SECRET", "")
	if box, _, err := NewBox(); box != nil || err != nil {
		t.Fatalf("box=%v err=%v", box, err)
	}
	t.Setenv(SecretKeyEnv, "not base64 !!")
	if _, _, err := NewBox(); err == nil {
		t.Fatal("an invalid explicit key must be an error, not a silent fallback")
	}
}

func TestInputValidation(t *testing.T) {
	for _, bad := range []string{"has space", "tab\there", "new\nline", "naïve", strings.Repeat("k", 257)} {
		if err := validKey("key", bad); err == nil {
			t.Errorf("key %q accepted", bad)
		}
	}
	if err := validKey("key", "sk-OK_123.abc"); err != nil {
		t.Errorf("a normal key was rejected: %v", err)
	}
	for _, bad := range []string{"nope", "a@", "Lab <lab@example.org>", "lab@example.org, b@example.org", strings.Repeat("a", 250) + "@example.org"} {
		if err := validEmail(bad); err == nil {
			t.Errorf("email %q accepted", bad)
		}
	}
	for _, ok := range []string{"", "lab@example.org"} {
		if err := validEmail(ok); err != nil {
			t.Errorf("email %q rejected: %v", ok, err)
		}
	}
	var ve *ValidationError
	if err := validEmail("nope"); !errors.As(err, &ve) {
		t.Fatal("a rejected value must be a ValidationError so the handler answers 400")
	}
}
