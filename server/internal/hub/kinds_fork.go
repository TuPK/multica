package hub

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
)

// KindSeatOffer is the Hub item kind for a seat advertised inside a Hub org
// (durable spec 13.3, F12). Kept in a fork-owned file so upstream edits to
// types.go do not conflict with it.
const KindSeatOffer Kind = "seat_offer"

// KindAgent is the Hub item kind for an agent template carrying optional signed
// track-record attestations in attestations/*.json (durable spec 13.3, F12).
const KindAgent Kind = "agent"

// forkItemKinds lists the Hub item kinds added by the fork.
var forkItemKinds = []Kind{KindSeatOffer, KindAgent}

// AttestationDir is the directory inside an agent template bundle that holds
// signed track-record attestations (durable spec 13.3: attestations/*.json).
const AttestationDir = "attestations"

func init() {
	// Keep attestations/ from being stripped by commonTopDir when an archive
	// holds only attestations/ files.
	contentDirs[AttestationDir] = true
}

// AllItemKinds returns the upstream Hub item kinds followed by the fork's.
func AllItemKinds() []Kind {
	out := make([]Kind, 0, 2+len(forkItemKinds))
	out = append(out, KindSkill, KindPlugin)
	out = append(out, forkItemKinds...)
	return out
}

// IsKnownKind reports whether k is one of the Hub item kinds (upstream or fork).
func IsKnownKind(k Kind) bool {
	if k == KindSkill || k == KindPlugin {
		return true
	}
	for _, fk := range forkItemKinds {
		if k == fk {
			return true
		}
	}
	return false
}

// IsHubItemKind reports whether k is a Hub item kind stored in hub_item and
// served by /api/hub/items/{kind}/{id} (skill, plugin, agent).
func IsHubItemKind(k Kind) bool {
	return k == KindSkill || k == KindPlugin || k == KindAgent
}

// BundleAttestation is one JSON attestation file carried inside a template bundle.
type BundleAttestation struct {
	Path    string
	Content []byte
}

// ExtractAttestations returns the JSON files directly under attestations/ in b,
// sorted by path (durable spec 13.3). Every file matching attestations/*.json
// must be a valid JSON object; a malformed file returns an error naming its path.
func ExtractAttestations(b *Bundle) ([]BundleAttestation, error) {
	if b == nil {
		return nil, nil
	}
	var out []BundleAttestation
	for _, f := range b.Files {
		dir, file := path.Split(f.Path)
		if dir != AttestationDir+"/" || !strings.HasSuffix(strings.ToLower(file), ".json") {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(f.Content, &obj); err != nil || obj == nil {
			return nil, fmt.Errorf("%w: attestation %q is not a valid JSON object", ErrInvalidArchive, f.Path)
		}
		out = append(out, BundleAttestation{Path: f.Path, Content: f.Content})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
