package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/service"
)

// attTestSeed is a 32-byte Ed25519 seed for these tests only. It is never a real signing key.
var attTestSeed = []byte("delegora-cli-attest-test-seed-01")

// attTestHost is the issuer the test attestation names; its keys are served by attFakeKeys.
const attTestHost = "ocf.example.test"

// attFakeKeys serves a fixed key document in place of the HTTPS fetch and records the URL asked.
type attFakeKeys struct {
	body []byte
	urls []string
}

func (f *attFakeKeys) Get(_ context.Context, rawURL string, _ int64) ([]byte, string, error) {
	f.urls = append(f.urls, rawURL)
	return f.body, "application/json", nil
}

// attKeysDocument is the key document an issuer on attTestHost publishes for seed.
func attKeysDocument(t *testing.T, seed []byte) []byte {
	t.Helper()
	pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	body, err := service.PublicKeysDocument(service.KeySet{Host: attTestHost, Keys: map[string]ed25519.PublicKey{"k1": pub}})
	if err != nil {
		t.Fatalf("publish keys: %v", err)
	}
	return body
}

// attSignedFile signs a test attestation with seed, writes it to a temp file and returns its bytes.
func attSignedFile(t *testing.T, seed []byte) (string, []byte) {
	t.Helper()
	doc := map[string]any{
		"format": "ocf-attestation/1",
		"subject": map[string]any{
			"template_ref": "hub:flow-reviewer@1.2.0",
			"seat_label":   "Reviewer",
		},
		"issuer": map[string]any{"host": attTestHost},
		"period": map[string]any{"from": "2026-08-01", "to": "2026-11-01"},
		"work_classes": []map[string]any{{
			"class": "area:flow:flow_kv_pr", "n": 64, "pass": 58, "partial": 3, "fail": 3,
			"score_lower":  0.86,
			"verification": map[string]any{"full": 20, "sampled": 44, "jury": 12},
		}},
		"models":    []string{"anthropic/claude-sonnet-5-5"},
		"issued_at": "2026-11-02T10:00:00Z",
	}
	raw, err := service.Sign(doc, ed25519.NewKeyFromSeed(seed), "k1")
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	path := filepath.Join(t.TempDir(), "record.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path, raw
}

// attUseKeys points the verify command at fake and restores the real fetcher when the test ends.
func attUseKeys(t *testing.T, fake *attFakeKeys) {
	t.Helper()
	prev := attestationKeysGetter
	attestationKeysGetter = fake
	t.Cleanup(func() { attestationKeysGetter = prev })
}

// attCaptureStdout returns what fn writes to os.Stdout, together with fn's error.
func attCaptureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prev := os.Stdout
	os.Stdout = w
	runErr := fn()
	os.Stdout = prev
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), runErr
}

func TestAttestationVerifyCLIAcceptsOriginal(t *testing.T) {
	path, _ := attSignedFile(t, attTestSeed)
	fake := &attFakeKeys{body: attKeysDocument(t, attTestSeed)}
	attUseKeys(t, fake)

	out, err := attCaptureStdout(t, func() error { return runAttestationVerify(nil, []string{path}) })
	if err != nil {
		t.Fatalf("verify original: %v", err)
	}
	if !strings.Contains(out, "valid:") {
		t.Fatalf("stdout %q does not report a valid attestation", out)
	}
	if want := "https://" + attTestHost + service.AttestationKeysPath; len(fake.urls) != 1 || fake.urls[0] != want {
		t.Fatalf("keys fetched from %v, want [%s]", fake.urls, want)
	}
}

func TestAttestationVerifyCLIRejectsOneDigitChange(t *testing.T) {
	path, raw := attSignedFile(t, attTestSeed)
	attUseKeys(t, &attFakeKeys{body: attKeysDocument(t, attTestSeed)})

	// Change one digit in the signed body (n 64 -> 65) and keep the signature as it was.
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	classes := doc["work_classes"].([]any)
	classes[0].(map[string]any)["n"] = float64(65)
	changed, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, changed, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = attCaptureStdout(t, func() error { return runAttestationVerify(nil, []string{path}) })
	if err == nil {
		t.Fatal("verify accepted an attestation with one digit changed")
	}
	if !strings.Contains(err.Error(), "does not verify") {
		t.Fatalf("failed for the wrong reason: %v", err)
	}
}

func TestAttestationVerifyCLIRejectsForeignKey(t *testing.T) {
	path, _ := attSignedFile(t, attTestSeed)
	// The host publishes a different key under the same id: the signature must not verify.
	foreignSeed := []byte("delegora-cli-attest-foreign-seed")
	attUseKeys(t, &attFakeKeys{body: attKeysDocument(t, foreignSeed)})

	_, err := attCaptureStdout(t, func() error { return runAttestationVerify(nil, []string{path}) })
	if err == nil {
		t.Fatal("verify accepted an attestation signed with a key the issuer does not publish")
	}
	if !strings.Contains(err.Error(), "does not verify") {
		t.Fatalf("failed for the wrong reason: %v", err)
	}
}
