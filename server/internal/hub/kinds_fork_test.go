package hub

import (
	"bytes"
	"errors"
	"testing"
)

func TestHubItemKindsIncludeSeatOfferAndAgent(t *testing.T) {
	if !IsKnownKind(KindSkill) || !IsKnownKind(KindPlugin) || !IsKnownKind(KindSeatOffer) || !IsKnownKind(KindAgent) {
		t.Fatalf("expected skill, plugin, seat_offer and agent to be known Hub item kinds")
	}
	if !IsHubItemKind(KindSkill) || !IsHubItemKind(KindPlugin) || !IsHubItemKind(KindAgent) {
		t.Fatalf("expected skill, plugin and agent to be valid hub_item route kinds")
	}
	if IsHubItemKind(KindSeatOffer) || IsHubItemKind(Kind("unknown")) {
		t.Fatalf("seat_offer and unknown must not be accepted as hub_item route kinds")
	}
	if IsKnownKind(Kind("unknown")) {
		t.Fatalf("unknown kind must not be accepted")
	}
	kinds := AllItemKinds()
	have := make(map[Kind]bool, len(kinds))
	for _, k := range kinds {
		have[k] = true
	}
	for _, want := range []Kind{KindSkill, KindPlugin, KindSeatOffer, KindAgent} {
		if !have[want] {
			t.Fatalf("AllItemKinds() = %v, want %q included", kinds, want)
		}
	}
}

func TestExtractAttestationsFromTemplateBundle(t *testing.T) {
	docA := []byte(`{"format":"ocf-attestation/1","issuer":{"host":"a.example.test","key_id":"k1"}}`)
	docB := []byte(`{"format":"ocf-attestation/1","issuer":{"host":"b.example.test","key_id":"k2"}}`)
	b := &Bundle{
		Files: []FileEntry{
			{Path: "pack.json", Size: 2, Content: []byte("{}")},
			{Path: "attestations/b.json", Size: int64(len(docB)), Content: docB},
			{Path: "attestations/a.json", Size: int64(len(docA)), Content: docA},
			{Path: "attestations/README.md", Size: 4, Content: []byte("# hi")},
			{Path: "attestations/nested/c.json", Size: 2, Content: []byte("{}")},
		},
	}

	got, err := ExtractAttestations(b)
	if err != nil {
		t.Fatalf("ExtractAttestations: %v", err)
	}
	if len(got) != 2 || got[0].Path != "attestations/a.json" || got[1].Path != "attestations/b.json" {
		t.Fatalf("ExtractAttestations = %+v, want sorted a.json and b.json only", got)
	}
	if !bytes.Equal(got[0].Content, docA) || !bytes.Equal(got[1].Content, docB) {
		t.Fatalf("attestation contents mismatch")
	}

	// Zip round-trip preserves the attestations/ directory even when it is the only folder.
	onlyAttest := &Bundle{
		Files: []FileEntry{{Path: "attestations/a.json", Size: int64(len(docA)), Content: docA}},
	}
	zipped, err := BuildZip(onlyAttest)
	if err != nil {
		t.Fatalf("BuildZip: %v", err)
	}
	extracted, err := ExtractZip(bytes.NewReader(zipped), int64(len(zipped)), DefaultLimits)
	if err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}
	if len(extracted.Files) != 1 || extracted.Files[0].Path != "attestations/a.json" {
		t.Fatalf("ExtractZip stripped attestations/ dir: %+v", extracted.Files)
	}
}

func TestExtractAttestationsRefusesMalformedJSON(t *testing.T) {
	cases := []struct {
		name    string
		content []byte
	}{
		{"syntax error", []byte("{not json")},
		{"null literal", []byte("null")},
		{"json array", []byte(`["not","object"]`)},
		{"json string", []byte(`"not object"`)},
		{"empty", []byte("")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := &Bundle{
				Files: []FileEntry{
					{Path: "attestations/broken.json", Size: int64(len(tc.content)), Content: tc.content},
				},
			}
			if _, err := ExtractAttestations(b); !errors.Is(err, ErrInvalidArchive) {
				t.Fatalf("malformed attestation (%s) err = %v, want ErrInvalidArchive", tc.name, err)
			}
		})
	}
}
