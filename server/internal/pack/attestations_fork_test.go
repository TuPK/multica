package pack

import (
	"errors"
	"testing"

	"github.com/multica-ai/multica/server/internal/hub"
)

func TestNormalizeReadsTemplateAttestations(t *testing.T) {
	b := &hub.Bundle{
		Files: []hub.FileEntry{
			{Path: "pack.json", Content: []byte(`{"id":"reviewer-pack","version":"1.0.0","agent":{"name":"Reviewer","instructions_file":"instructions.md"},"skills":[]}`)},
			{Path: "instructions.md", Content: []byte("# Reviewer\nReview PRs carefully.\n")},
			{Path: "attestations/2026-q4.json", Content: []byte(`{"format":"ocf-attestation/1","issuer":{"host":"ocf.example.test","key_id":"k2"}}`)},
			{Path: "attestations/2026-q3.json", Content: []byte(`{"format":"ocf-attestation/1","issuer":{"host":"ocf.example.test","key_id":"k1"}}`)},
		},
	}
	p, err := Normalize(b)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if len(p.Attestations) != 2 || p.Attestations[0].Path != "attestations/2026-q3.json" || p.Attestations[1].Path != "attestations/2026-q4.json" {
		t.Fatalf("Attestations = %+v, want sorted 2026-q3.json and 2026-q4.json", p.Attestations)
	}
}

func TestNormalizeRefusesMalformedTemplateAttestation(t *testing.T) {
	cases := []struct {
		name string
		body []byte
	}{
		{"invalid syntax", []byte(`{malformed`)},
		{"null", []byte(`null`)},
		{"array", []byte(`[1,2,3]`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := &hub.Bundle{
				Files: []hub.FileEntry{
					{Path: "pack.json", Content: []byte(`{"id":"reviewer-pack","version":"1.0.0","agent":{"name":"Reviewer","instructions_file":"instructions.md"},"skills":[]}`)},
					{Path: "instructions.md", Content: []byte("# Reviewer\n")},
					{Path: "attestations/bad.json", Content: tc.body},
				},
			}
			if _, err := Normalize(b); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Normalize with malformed attestation (%s) = %v, want ErrInvalid", tc.name, err)
			}
		})
	}
}
