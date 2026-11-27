package service

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/util"
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

// A document signed with the server's key verifies against the key set the server publishes, and
// the published set is what a verifier on another host fetches and parses (spec 13.2).
func TestPublishedKeysVerifyDocument(t *testing.T) {
	priv := testKey(t, testSeed)
	svc := &AttestationService{Key: priv, Host: "ocf.example.test"}
	doc, err := Sign(testAttestation(), priv, KeyID(priv.Public().(ed25519.PublicKey)))
	if err != nil {
		t.Fatal(err)
	}
	published, err := PublicKeysDocument(svc.KeySet())
	if err != nil {
		t.Fatal(err)
	}
	keys, err := ParsePublicKeys("ocf.example.test", published)
	if err != nil {
		t.Fatalf("parse published keys: %v", err)
	}
	if err := Verify(doc, keys); err != nil {
		t.Fatalf("verify with published keys: %v", err)
	}
	if _, err := ParsePublicKeys("other.example.test", published); !errors.Is(err, ErrForeignKey) {
		t.Fatalf("keys of another host: %v, want ErrForeignKey", err)
	}
}

// A rotated-out key stays published, so a document signed before the rotation still verifies (D28).
func TestRetiredKeyStaysPublished(t *testing.T) {
	old := testKey(t, []byte("delegora-attest-old-seed-0000002"))
	current := testKey(t, testSeed)
	retired, err := RetiredKeysFromEnv(base64.StdEncoding.EncodeToString(old.Public().(ed25519.PublicKey)))
	if err != nil {
		t.Fatal(err)
	}
	svc := &AttestationService{Key: current, Host: "ocf.example.test", Retired: retired}
	published, err := PublicKeysDocument(svc.KeySet())
	if err != nil {
		t.Fatal(err)
	}
	keys, err := ParsePublicKeys("ocf.example.test", published)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys.Keys) != 2 {
		t.Fatalf("published keys = %d, want the current and the retired one", len(keys.Keys))
	}
	doc, err := Sign(testAttestation(), old, KeyID(old.Public().(ed25519.PublicKey)))
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(doc, keys); err != nil {
		t.Fatalf("document signed by the retired key: %v", err)
	}
	if _, err := RetiredKeysFromEnv("not-base64!"); err == nil {
		t.Fatal("a malformed retired key was accepted")
	}
}

// attestationEnv is a workspace with one agent and a signing service on a fixed clock.
type attestationEnv struct {
	*policyEnv
	svc *AttestationService
}

func newAttestationEnv(t *testing.T) attestationEnv {
	t.Helper()
	e := newPolicyEnv(t)
	return attestationEnv{
		policyEnv: e,
		svc: &AttestationService{
			Queries: e.q, Key: testKey(t, testSeed), Host: "ocf.example.test",
			Now: func() time.Time { return time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC) },
		},
	}
}

// outcomes settles n scored outcomes for the agent in the period, in one work class.
func (e attestationEnv) outcomes(t *testing.T, workClass string, n int, verdict string) {
	t.Helper()
	for i := 0; i < n; i++ {
		e.fx.Exec(t, `INSERT INTO work_outcome
			(workspace_id, issue_id, done_transition_id, agent_id, work_class, verification_mode, sampled, verdict, model, settle_at, settled_at)
			VALUES ($1, $2, $3, $4, $5, 'full', false, $6, 'anthropic/claude-sonnet-5-5', $7, $7)`,
			util.UUIDToString(e.ws), uuid.NewString(), uuid.NewString(), util.UUIDToString(e.agent), workClass, verdict,
			time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC))
	}
}

func (e attestationEnv) request(showWorkspace bool) AttestationRequest {
	return AttestationRequest{
		Agent:       e.agent,
		From:        time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		To:          time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
		TemplateRef: "hub:flow-reviewer@1.2.0", ShowWorkspace: showWorkspace,
	}
}

// Issuing needs at least 30 scored outcomes in the period (spec 13.5, 2).
func TestIssueRequires30(t *testing.T) {
	e := newAttestationEnv(t)
	ctx := context.Background()
	e.outcomes(t, "flow:flow_kv_pr:review", 29, "pass")
	if _, err := e.svc.Issue(ctx, e.ws, e.request(false)); !errors.Is(err, ErrAttestationSampleSmall) {
		t.Fatalf("29 outcomes: %v, want ErrAttestationSampleSmall", err)
	}
	e.outcomes(t, "flow:flow_kv_pr:review", 1, "fail")
	doc, err := e.svc.Issue(ctx, e.ws, e.request(false))
	if err != nil {
		t.Fatalf("30 outcomes: %v", err)
	}
	if err := Verify(doc, e.svc.KeySet()); err != nil {
		t.Fatalf("issued document does not verify: %v", err)
	}
}

// Work classes are named by their area only, and merge within it (spec 13.2).
func TestWorkClassCoarsenedToArea(t *testing.T) {
	e := newAttestationEnv(t)
	ctx := context.Background()
	e.outcomes(t, "flow:flow_kv_pr:review", 20, "pass")
	e.outcomes(t, "flow:flow_kv_pr:merge", 10, "partial")
	doc, err := e.svc.Issue(ctx, e.ws, e.request(false))
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	var got struct {
		WorkClasses []struct {
			Class   string `json:"class"`
			N       int    `json:"n"`
			Pass    int    `json:"pass"`
			Partial int    `json:"partial"`
		} `json:"work_classes"`
	}
	if err := json.Unmarshal(doc, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.WorkClasses) != 1 {
		t.Fatalf("work classes = %+v, want one area", got.WorkClasses)
	}
	c := got.WorkClasses[0]
	if c.Class != "area:flow:flow_kv_pr" || c.N != 30 || c.Pass != 20 || c.Partial != 10 {
		t.Fatalf("area = %+v, want area:flow:flow_kv_pr with n 30, pass 20, partial 10", c)
	}
	for _, internal := range []string{"flow:flow_kv_pr:review", "flow:flow_kv_pr:merge"} {
		if strings.Contains(string(doc), internal) {
			t.Fatalf("the document names the internal work class %q", internal)
		}
	}
}

// The workspace is named only when the issuer asks for it (spec 13.2).
func TestWorkspaceHiddenByDefault(t *testing.T) {
	e := newAttestationEnv(t)
	ctx := context.Background()
	e.outcomes(t, "flow:flow_kv_pr:review", 30, "pass")
	hidden, err := e.svc.Issue(ctx, e.ws, e.request(false))
	if err != nil {
		t.Fatalf("issue hidden: %v", err)
	}
	if strings.Contains(string(hidden), `"workspace"`) {
		t.Fatal("the workspace is named although the issuer did not ask for it")
	}
	shown, err := e.svc.Issue(ctx, e.ws, e.request(true))
	if err != nil {
		t.Fatalf("issue shown: %v", err)
	}
	var got struct {
		Workspace struct {
			Label string `json:"label"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal(shown, &got); err != nil {
		t.Fatal(err)
	}
	if got.Workspace.Label == "" {
		t.Fatal("the workspace is not named although the issuer asked for it")
	}
}

// A template reference must name a Hub item and its version.
func TestIssueRefusesBadTemplateRef(t *testing.T) {
	e := newAttestationEnv(t)
	req := e.request(false)
	req.TemplateRef = "flow-reviewer"
	if _, err := e.svc.Issue(context.Background(), e.ws, req); !errors.Is(err, ErrAttestationTemplate) {
		t.Fatalf("bare template ref: %v, want ErrAttestationTemplate", err)
	}
}
