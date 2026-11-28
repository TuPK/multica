package service

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"testing"
)

func TestSeatAttestationTemplateBinding(t *testing.T) {
	priv := testKey(t, testSeed)
	signed, err := Sign(testAttestation(), priv, "k1")
	if err != nil {
		t.Fatal(err)
	}
	keys := testKeySet("ocf.example.test", map[string]ed25519.PrivateKey{"k1": priv})
	resolver := &WellKnownKeyFetcher{Own: &keys}
	for _, tc := range []struct {
		name, ref string
		doc       []byte
		want      bool
	}{
		{"matching", "hub:flow-reviewer@1.2.0", signed, true},
		{"mismatched", "hub:other@1.2.0", signed, false},
		{"tampered", "hub:flow-reviewer@1.2.0", bytes.Replace(signed, []byte(`"n":64`), []byte(`"n":65`), 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(hirePayload{Template: tc.ref, Attestation: tc.doc})
			if err != nil {
				t.Fatal(err)
			}
			got, err := VerifySeatAttestation(context.Background(), payload, resolver)
			if err != nil || got != tc.want {
				t.Fatalf("verified = %v, err = %v; want %v", got, err, tc.want)
			}
		})
	}
}
