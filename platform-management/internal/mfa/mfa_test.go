package mfa

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

// The fixture was made by the Delegora server's own code
// (server/internal/mfa: LoadCrypto, Seal, and the pquerna TOTP library) with
// the key below, so the platform is shown to read what Delegora writes.
const (
	fixtureKey    = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	fixtureSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	fixtureSealed = "7702f26fb2eac2e9ef79f48a31ee8f16c04c417ded9d2446d0138e13c66ca2b1118b6409e6b495d94c120c8db2f6c1fd93ecd750990154d7b5c17500"
	fixtureCode   = "876629"
	fixtureStep   = 63333334
)

var fixtureTime = time.Unix(1_900_000_020, 0)

func fixtureCrypto(t *testing.T) *Crypto {
	t.Helper()
	key, err := ParseKey(fixtureKey)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewCrypto(key)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestOpenReadsAValueSealedByMultica(t *testing.T) {
	sealed, err := hex.DecodeString(fixtureSealed)
	if err != nil {
		t.Fatal(err)
	}
	got, err := fixtureCrypto(t).Open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if got != fixtureSecret {
		t.Fatalf("secret = %q, want %q", got, fixtureSecret)
	}
}

func TestOpenRefusesATamperedOrShortValue(t *testing.T) {
	sealed, _ := hex.DecodeString(fixtureSealed)
	sealed[len(sealed)-1] ^= 1
	if _, err := fixtureCrypto(t).Open(sealed); err == nil {
		t.Fatal("tampered value opened")
	}
	if _, err := fixtureCrypto(t).Open([]byte{1, 2, 3}); err == nil {
		t.Fatal("short value opened")
	}
}

func TestMatchStepAgreesWithMulticasCode(t *testing.T) {
	step, ok := MatchStep(fixtureSecret, fixtureCode, fixtureTime)
	if !ok || step != fixtureStep {
		t.Fatalf("MatchStep = %d, %v; want %d, true", step, ok, fixtureStep)
	}
}

func TestMatchStepWindow(t *testing.T) {
	// One step either side of now is accepted, two are not.
	for _, tc := range []struct {
		name  string
		shift time.Duration
		want  bool
	}{
		{"previous step", -Period * time.Second, true},
		{"next step", Period * time.Second, true},
		{"two steps back", -2 * Period * time.Second, false},
		{"two steps ahead", 2 * Period * time.Second, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := MatchStep(fixtureSecret, fixtureCode, fixtureTime.Add(tc.shift))
			if ok != tc.want {
				t.Fatalf("ok = %v, want %v", ok, tc.want)
			}
		})
	}
}

func TestMatchStepRefusesMalformedCodes(t *testing.T) {
	for _, code := range []string{"", "12345", "1234567", "abcdef", "87662"} {
		if _, ok := MatchStep(fixtureSecret, code, fixtureTime); ok {
			t.Errorf("code %q accepted", code)
		}
	}
	if _, ok := MatchStep("not base32 !!", fixtureCode, fixtureTime); ok {
		t.Error("a secret that is not base32 matched")
	}
}

// RFC 6238 appendix B, SHA-1, time 59: the 8-digit code 94287082 ends in 287082.
func TestCodeAtMatchesRFC6238(t *testing.T) {
	got, err := CodeAt("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", time.Unix(59, 0))
	if err != nil {
		t.Fatal(err)
	}
	if got != "287082" {
		t.Fatalf("code = %q, want 287082", got)
	}
}

func TestParseKey(t *testing.T) {
	if _, err := ParseKey(fixtureKey); err != nil {
		t.Fatalf("good key: %v", err)
	}
	short := base64.StdEncoding.EncodeToString([]byte("too short"))
	for name, raw := range map[string]string{"empty": "", "not base64": "%%%", "short": short} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseKey(raw)
			if err == nil {
				t.Fatal("expected an error")
			}
			if raw != "" && strings.Contains(err.Error(), raw) {
				t.Fatalf("error repeats the value: %v", err)
			}
		})
	}
}

type fakeSampler struct {
	sealed []byte
	found  bool
	err    error
}

func (s fakeSampler) AnyConfirmedSecret(context.Context) ([]byte, bool, error) {
	return s.sealed, s.found, s.err
}

func TestCheckKeyPassesWhenNoRowExists(t *testing.T) {
	if err := CheckKey(context.Background(), fixtureCrypto(t), fakeSampler{}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckKeyPassesWhenTheKeyOpensTheRow(t *testing.T) {
	sealed, _ := hex.DecodeString(fixtureSealed)
	if err := CheckKey(context.Background(), fixtureCrypto(t), fakeSampler{sealed: sealed, found: true}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckKeyNamesTheVariableOnAMismatch(t *testing.T) {
	other := make([]byte, 32)
	if _, err := rand.Read(other); err != nil {
		t.Fatal(err)
	}
	block, _ := aes.NewCipher(other)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	sealed := gcm.Seal(nonce, nonce, []byte(fixtureSecret), nil)

	err := CheckKey(context.Background(), fixtureCrypto(t), fakeSampler{sealed: sealed, found: true})
	if err == nil {
		t.Fatal("a key that does not open the row passed")
	}
	if !strings.Contains(err.Error(), EnvKey) {
		t.Fatalf("error does not name %s: %v", EnvKey, err)
	}
	if strings.Contains(err.Error(), fixtureKey) || strings.Contains(err.Error(), fixtureSecret) {
		t.Fatalf("error leaks a secret: %v", err)
	}
}

func TestCheckKeyReportsAReadFailure(t *testing.T) {
	err := CheckKey(context.Background(), fixtureCrypto(t), fakeSampler{err: errors.New("db down")})
	if err == nil {
		t.Fatal("expected an error")
	}
}
