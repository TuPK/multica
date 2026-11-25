package service

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Track-record attestations (spec 13.2, D23, D28). An attestation is signed
// with the server's Ed25519 key over its RFC 8785 canonical form, with the
// signature field removed. Verification recomputes that form, so the signed
// bytes and the checked bytes come from one serializer.

// attestationKeyEnv names the environment variable that holds the base64 seed.
// D28: no generated fallback and no storage of the key in the database.
const attestationKeyEnv = "MULTICA_ATTESTATION_KEY"

var (
	// ErrAttestationDisabled means no signing key is configured. Issuing is off;
	// verification and the public key document keep working (D28).
	ErrAttestationDisabled = errors.New("attestation_disabled")
	// ErrBadSignature means the signature does not match the document, or is absent or malformed.
	ErrBadSignature = errors.New("attestation: bad signature")
	// ErrForeignKey means the issuer host is not the host of the key set used to verify.
	ErrForeignKey = errors.New("attestation: issuer host does not match key set host")
	// ErrUnknownKey means the issuer's key_id is not in the key set.
	ErrUnknownKey = errors.New("attestation: unknown key_id")
)

// KeySet holds the public keys of one host, keyed by key_id. Keeping retired keys
// in the set lets documents signed before a rotation still verify.
type KeySet struct {
	Host string
	Keys map[string]ed25519.PublicKey
}

// AttestationKeyFromEnv reads the signing seed from MULTICA_ATTESTATION_KEY. An
// unset or empty variable returns ErrAttestationDisabled. A set but malformed
// value is an error of its own, so a misconfigured key is not silently disabled.
func AttestationKeyFromEnv() (ed25519.PrivateKey, error) {
	raw := strings.TrimSpace(os.Getenv(attestationKeyEnv))
	if raw == "" {
		return nil, ErrAttestationDisabled
	}
	seed, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("attestation: %s is not base64: %w", attestationKeyEnv, err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("attestation: %s decodes to %d bytes, want %d", attestationKeyEnv, len(seed), ed25519.SeedSize)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// Sign returns the attestation as canonical JSON with issuer.key_id set to keyID
// and a base64 signature added. The input map is not modified. A nil key
// returns ErrAttestationDisabled.
func Sign(attestation map[string]any, priv ed25519.PrivateKey, keyID string) ([]byte, error) {
	if priv == nil {
		return nil, ErrAttestationDisabled
	}
	if len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("attestation: private key has %d bytes, want %d", len(priv), ed25519.PrivateKeySize)
	}
	if keyID == "" {
		return nil, errors.New("attestation: key_id is empty")
	}

	doc, err := decodeAttestationMap(attestation)
	if err != nil {
		return nil, err
	}
	issuer, ok := doc["issuer"].(map[string]any)
	if !ok {
		return nil, errors.New("attestation: issuer is missing")
	}
	issuer["key_id"] = keyID
	delete(doc, "signature")

	payload, err := Canonicalize(doc)
	if err != nil {
		return nil, err
	}
	doc["signature"] = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, payload))
	return Canonicalize(doc)
}

// Verify checks the signature of a signed attestation document. The issuer host
// must equal keys.Host, and issuer.key_id must name a key in keys.Keys. Any
// change to the signed content makes it fail with ErrBadSignature.
func Verify(doc []byte, keys KeySet) error {
	m, err := decodeAttestationBytes(doc)
	if err != nil {
		return err
	}
	sigText, _ := m["signature"].(string)
	sig, err := base64.StdEncoding.DecodeString(sigText)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return ErrBadSignature
	}
	issuer, ok := m["issuer"].(map[string]any)
	if !ok {
		return errors.New("attestation: issuer is missing")
	}
	host, _ := issuer["host"].(string)
	keyID, _ := issuer["key_id"].(string)
	if keys.Host == "" || host != keys.Host {
		return ErrForeignKey
	}
	pub, ok := keys.Keys[keyID]
	if !ok {
		return ErrUnknownKey
	}
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("attestation: public key for %q has %d bytes, want %d", keyID, len(pub), ed25519.PublicKeySize)
	}

	delete(m, "signature")
	payload, err := Canonicalize(m)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, payload, sig) {
		return ErrBadSignature
	}
	return nil
}

// PublicKeysDocument renders the body of /.well-known/multica-attestation-keys.json.
// KeySet holds only public keys, so no seed can appear in the output.
func PublicKeysDocument(keys KeySet) ([]byte, error) {
	ids := make([]string, 0, len(keys.Keys))
	for id := range keys.Keys {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	list := make([]any, 0, len(ids))
	for _, id := range ids {
		pub := keys.Keys[id]
		if len(pub) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("attestation: public key for %q has %d bytes, want %d", id, len(pub), ed25519.PublicKeySize)
		}
		list = append(list, map[string]any{
			"key_id":     id,
			"alg":        "Ed25519",
			"public_key": base64.StdEncoding.EncodeToString(pub),
		})
	}
	return Canonicalize(map[string]any{"host": keys.Host, "keys": list})
}

// decodeAttestationMap copies an attestation through JSON so that Canonicalize
// receives only the supported types (json.Number for numbers). Struct fields,
// int and float64 values all serialize to the same canonical form.
func decodeAttestationMap(attestation map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(attestation)
	if err != nil {
		return nil, fmt.Errorf("attestation: encode: %w", err)
	}
	return decodeAttestationBytes(raw)
}

func decodeAttestationBytes(raw []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("attestation: decode: %w", err)
	}
	if m == nil {
		return nil, errors.New("attestation: document is null")
	}
	return m, nil
}
