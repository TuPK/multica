package service

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// testSeed is a fixed 32-byte Ed25519 seed for tests only. Production seeds come
// from MULTICA_ATTESTATION_KEY and are never committed.
var testSeed = []byte("delegora-attest-test-seed-000001")

func testKey(t *testing.T, seed []byte) ed25519.PrivateKey {
	t.Helper()
	if len(seed) != ed25519.SeedSize {
		t.Fatalf("test seed has %d bytes, want %d", len(seed), ed25519.SeedSize)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func testAttestation() map[string]any {
	return map[string]any{
		"format": "ocf-attestation/1",
		"subject": map[string]any{
			"template_ref": "hub:flow-reviewer@1.2.0",
			"seat_label":   "Reviewer",
		},
		"issuer": map[string]any{"host": "ocf.example.test"},
		"period": map[string]any{"from": "2026-08-01", "to": "2026-11-01"},
		"work_classes": []map[string]any{{
			"class":        "area:flow:flow_kv_pr",
			"n":            64,
			"pass":         58,
			"partial":      3,
			"fail":         3,
			"score_lower":  0.86,
			"verification": map[string]any{"full": 20, "sampled": 44, "jury": 12},
		}},
		"models":    []string{"anthropic/claude-sonnet-5-5"},
		"issued_at": "2026-11-02T10:00:00Z",
	}
}

func testKeySet(host string, keys map[string]ed25519.PrivateKey) KeySet {
	set := KeySet{Host: host, Keys: map[string]ed25519.PublicKey{}}
	for id, priv := range keys {
		set.Keys[id] = priv.Public().(ed25519.PublicKey)
	}
	return set
}

func TestSignVerifyRoundTrip(t *testing.T) {
	priv := testKey(t, testSeed)
	doc, err := Sign(testAttestation(), priv, "k1")
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(doc, &got); err != nil {
		t.Fatalf("signed document is not JSON: %v", err)
	}
	issuer, _ := got["issuer"].(map[string]any)
	if issuer["key_id"] != "k1" {
		t.Fatalf("issuer.key_id = %v, want k1", issuer["key_id"])
	}
	if sig, _ := got["signature"].(string); sig == "" {
		t.Fatalf("signature missing from signed document")
	}

	keys := testKeySet("ocf.example.test", map[string]ed25519.PrivateKey{"k1": priv})
	if err := Verify(doc, keys); err != nil {
		t.Fatalf("Verify of untouched document: %v", err)
	}
}

func TestSignDoesNotMutateInput(t *testing.T) {
	priv := testKey(t, testSeed)
	in := testAttestation()
	if _, err := Sign(in, priv, "k1"); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if _, ok := in["signature"]; ok {
		t.Fatalf("Sign added signature to the caller's map")
	}
	if _, ok := in["issuer"].(map[string]any)["key_id"]; ok {
		t.Fatalf("Sign added issuer.key_id to the caller's map")
	}
}

func TestTamperFails(t *testing.T) {
	priv := testKey(t, testSeed)
	doc, err := Sign(testAttestation(), priv, "k1")
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	keys := testKeySet("ocf.example.test", map[string]ed25519.PrivateKey{"k1": priv})

	tampered := []byte(strings.Replace(string(doc), `"n":64`, `"n":65`, 1))
	if string(tampered) == string(doc) {
		t.Fatalf("tamper did not change the document; canonical form has no \"n\":64")
	}
	if err := Verify(tampered, keys); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("Verify(tampered) = %v, want ErrBadSignature", err)
	}
}

func TestRotatedKeyStillVerifies(t *testing.T) {
	oldPriv := testKey(t, testSeed)
	newPriv := testKey(t, []byte("delegora-attest-test-seed-000002"))

	oldDoc, err := Sign(testAttestation(), oldPriv, "k1")
	if err != nil {
		t.Fatalf("Sign with old key: %v", err)
	}
	newDoc, err := Sign(testAttestation(), newPriv, "k2")
	if err != nil {
		t.Fatalf("Sign with new key: %v", err)
	}

	both := testKeySet("ocf.example.test", map[string]ed25519.PrivateKey{"k1": oldPriv, "k2": newPriv})
	if err := Verify(oldDoc, both); err != nil {
		t.Fatalf("old-key document after rotation: %v", err)
	}
	if err := Verify(newDoc, both); err != nil {
		t.Fatalf("new-key document: %v", err)
	}

	onlyNew := testKeySet("ocf.example.test", map[string]ed25519.PrivateKey{"k2": newPriv})
	if err := Verify(oldDoc, onlyNew); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("Verify(old doc, key set without k1) = %v, want ErrUnknownKey", err)
	}
}

func TestForeignKeyRejected(t *testing.T) {
	priv := testKey(t, testSeed)
	doc, err := Sign(testAttestation(), priv, "k1")
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	// The key is genuinely the one for k1, but it belongs to another host: a
	// verifier must not accept an attestation whose issuer is not that host.
	foreign := testKeySet("other.example.test", map[string]ed25519.PrivateKey{"k1": priv})
	if err := Verify(doc, foreign); !errors.Is(err, ErrForeignKey) {
		t.Fatalf("Verify with foreign host = %v, want ErrForeignKey", err)
	}
}

func TestIssueDisabledWithoutKey(t *testing.T) {
	t.Setenv("MULTICA_ATTESTATION_KEY", "")

	priv, err := AttestationKeyFromEnv()
	if !errors.Is(err, ErrAttestationDisabled) {
		t.Fatalf("AttestationKeyFromEnv without key = %v, want ErrAttestationDisabled", err)
	}
	if _, err := Sign(testAttestation(), priv, "k1"); !errors.Is(err, ErrAttestationDisabled) {
		t.Fatalf("Sign without key = %v, want ErrAttestationDisabled", err)
	}

	// Verification and the public key set keep working with a known key.
	known := testKey(t, testSeed)
	doc, err := Sign(testAttestation(), known, "k1")
	if err != nil {
		t.Fatalf("Sign with a known key: %v", err)
	}
	keys := testKeySet("ocf.example.test", map[string]ed25519.PrivateKey{"k1": known})
	if err := Verify(doc, keys); err != nil {
		t.Fatalf("Verify while issuing is disabled: %v", err)
	}
	if _, err := PublicKeysDocument(keys); err != nil {
		t.Fatalf("PublicKeysDocument while issuing is disabled: %v", err)
	}
}

func TestAttestationKeyFromEnv(t *testing.T) {
	t.Run("valid seed", func(t *testing.T) {
		t.Setenv("MULTICA_ATTESTATION_KEY", base64.StdEncoding.EncodeToString(testSeed))
		priv, err := AttestationKeyFromEnv()
		if err != nil {
			t.Fatalf("AttestationKeyFromEnv: %v", err)
		}
		if string(priv) != string(testKey(t, testSeed)) {
			t.Fatalf("key from env does not match the seed")
		}
	})
	t.Run("wrong length is a configuration error, not disabled", func(t *testing.T) {
		t.Setenv("MULTICA_ATTESTATION_KEY", base64.StdEncoding.EncodeToString([]byte("short")))
		_, err := AttestationKeyFromEnv()
		if err == nil || errors.Is(err, ErrAttestationDisabled) {
			t.Fatalf("AttestationKeyFromEnv with 5-byte seed = %v, want a non-disabled error", err)
		}
	})
	t.Run("not base64", func(t *testing.T) {
		t.Setenv("MULTICA_ATTESTATION_KEY", "!!!not-base64!!!")
		if _, err := AttestationKeyFromEnv(); err == nil {
			t.Fatalf("AttestationKeyFromEnv with invalid base64 returned no error")
		}
	})
}

func TestPublicKeysDocumentHasNoSeed(t *testing.T) {
	newPriv := testKey(t, []byte("delegora-attest-test-seed-000002"))
	oldPriv := testKey(t, testSeed)
	keys := testKeySet("ocf.example.test", map[string]ed25519.PrivateKey{"k1": oldPriv, "k2": newPriv})

	doc, err := PublicKeysDocument(keys)
	if err != nil {
		t.Fatalf("PublicKeysDocument: %v", err)
	}

	var got struct {
		Host string `json:"host"`
		Keys []struct {
			KeyID     string `json:"key_id"`
			Alg       string `json:"alg"`
			PublicKey string `json:"public_key"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(doc, &got); err != nil {
		t.Fatalf("document is not JSON: %v", err)
	}
	if got.Host != "ocf.example.test" {
		t.Fatalf("host = %q, want ocf.example.test", got.Host)
	}
	if len(got.Keys) != 2 {
		t.Fatalf("got %d keys, want 2 (rotation keeps the old key)", len(got.Keys))
	}
	for _, k := range got.Keys {
		if k.Alg != "Ed25519" {
			t.Fatalf("key %s alg = %q, want Ed25519", k.KeyID, k.Alg)
		}
	}

	// The seed and the 64-byte private key must not appear in any encoding.
	for _, secret := range [][]byte{testSeed, oldPriv, newPriv.Seed()} {
		for _, enc := range []string{
			base64.StdEncoding.EncodeToString(secret),
			base64.URLEncoding.EncodeToString(secret),
		} {
			if strings.Contains(string(doc), enc) {
				t.Fatalf("public key document leaks secret material")
			}
		}
	}
}
