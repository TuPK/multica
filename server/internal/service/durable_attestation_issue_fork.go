package service

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Issuing track-record attestations (spec 13.2, F12). The server signs an agent's scored track
// record for a period with its Ed25519 key. Keys are read from the environment (D28); none is
// generated or stored in the database.

// AttestationKeysPath is where a host publishes the public keys its attestations verify with.
const AttestationKeysPath = "/.well-known/multica-attestation-keys.json"

// attestationMinResults is the number of scored outcomes an agent needs in the period (spec 13.2).
const attestationMinResults = 30

// AttestationRetiredKeysEnv lists the public keys of rotated-out signing keys, comma separated,
// base64 encoded. They stay in the published set so older documents still verify (spec 13.2).
const AttestationRetiredKeysEnv = "MULTICA_ATTESTATION_RETIRED_KEYS"

var (
	// ErrAttestationSampleSmall: the agent has fewer than 30 scored outcomes in the period.
	ErrAttestationSampleSmall = errors.New("attestation_sample_too_small")
	// ErrAttestationNoHost: the server has no public host to name as the issuer.
	ErrAttestationNoHost = errors.New("attestation: no public host to name as issuer")
	// ErrAttestationPeriod: the period starts after it ends.
	ErrAttestationPeriod = errors.New("attestation: period from must be on or before to")
	// ErrAttestationAgentNotFound: the agent is not an agent of this workspace.
	ErrAttestationAgentNotFound = errors.New("attestation: agent not found")
	// ErrAttestationTemplate: the template reference is not hub:<item>@<version>.
	ErrAttestationTemplate = errors.New("attestation: template_ref must be hub:<item>@<version>")
)

// AttestationService issues attestations and publishes the keys that check them.
type AttestationService struct {
	Queries *db.Queries
	// Key signs attestations. Nil turns issuing off (D28); verification and the key document keep working.
	Key ed25519.PrivateKey
	// Host is this server's public host, which every attestation names as its issuer.
	Host string
	// Retired are the public keys of rotated-out signing keys.
	Retired []ed25519.PublicKey
	// Now is the clock for issued_at. Nil means time.Now.
	Now func() time.Time
}

// KeyID names a public key: "ed25519-" and the first 8 bytes of its SHA-256, in hex.
func KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return "ed25519-" + hex.EncodeToString(sum[:8])
}

// RetiredKeysFromEnv parses MULTICA_ATTESTATION_RETIRED_KEYS. An unset variable has no retired keys.
func RetiredKeysFromEnv(raw string) ([]ed25519.PublicKey, error) {
	var keys []ed25519.PublicKey
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pub, err := base64.StdEncoding.DecodeString(part)
		if err != nil || len(pub) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("attestation: %s has an entry that is not a base64 Ed25519 public key", AttestationRetiredKeysEnv)
		}
		keys = append(keys, ed25519.PublicKey(pub))
	}
	return keys, nil
}

// KeySet is this server's public keys: the signing key and the retired ones.
func (s *AttestationService) KeySet() KeySet {
	keys := map[string]ed25519.PublicKey{}
	if s.Key != nil {
		pub := s.Key.Public().(ed25519.PublicKey)
		keys[KeyID(pub)] = pub
	}
	for _, pub := range s.Retired {
		keys[KeyID(pub)] = pub
	}
	return KeySet{Host: s.Host, Keys: keys}
}

// ParsePublicKeys reads a host's published key document and checks that it names host. The
// keys it returns are the ones Verify checks signatures against.
func ParsePublicKeys(host string, raw []byte) (KeySet, error) {
	var doc struct {
		Host string `json:"host"`
		Keys []struct {
			KeyID     string `json:"key_id"`
			Alg       string `json:"alg"`
			PublicKey string `json:"public_key"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return KeySet{}, fmt.Errorf("attestation: key document: %w", err)
	}
	if doc.Host != host {
		return KeySet{}, ErrForeignKey
	}
	keys := make(map[string]ed25519.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Alg != "Ed25519" {
			continue
		}
		pub, err := base64.StdEncoding.DecodeString(k.PublicKey)
		if err != nil || len(pub) != ed25519.PublicKeySize {
			return KeySet{}, fmt.Errorf("attestation: key %q is not an Ed25519 public key", k.KeyID)
		}
		keys[k.KeyID] = ed25519.PublicKey(pub)
	}
	return KeySet{Host: host, Keys: keys}, nil
}

// AttestationRequest is what an owner or an admin asks for.
type AttestationRequest struct {
	Agent pgtype.UUID
	From  time.Time
	To    time.Time
	// TemplateRef names the Hub item the agent was hired from, as hub:<item>@<version>. Empty
	// when the agent did not come from the Hub.
	TemplateRef string
	// ShowWorkspace names the workspace in the attestation. It is hidden unless the issuer chooses.
	ShowWorkspace bool
}

// attestationClass is one work class of the attestation, coarsened to its area (spec 13.2).
type attestationClass struct {
	Class               string
	Pass, Partial, Fail int64
	Full, Sampled, Jury int64
}

// coarsenedClasses merges the agent's work classes into areas, the level an attestation names
// (spec 13.2), so an internal id inside a class never leaves the workspace.
func coarsenedClasses(rows []db.AttestationOutcomeCountsRow) []attestationClass {
	byArea := map[string]*attestationClass{}
	for _, r := range rows {
		area := "area:" + workClassPrefix(r.WorkClass)
		c, ok := byArea[area]
		if !ok {
			c = &attestationClass{Class: area}
			byArea[area] = c
		}
		c.Pass += r.NPass
		c.Partial += r.NPartial
		c.Fail += r.NFail
		c.Full += r.NFull
		c.Sampled += r.NSampled
		c.Jury += r.NJury
	}
	out := make([]attestationClass, 0, len(byArea))
	for _, c := range byArea {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Class < out[j].Class })
	return out
}

// Issue signs the agent's track record for the period. It needs the signing key, a public host and
// at least 30 scored outcomes in the period. The document holds the coarsened work classes, the
// models, and the workspace name only when the request asks for it.
func (s *AttestationService) Issue(ctx context.Context, ws pgtype.UUID, req AttestationRequest) ([]byte, error) {
	if s.Key == nil {
		return nil, ErrAttestationDisabled
	}
	if s.Host == "" {
		return nil, ErrAttestationNoHost
	}
	if req.From.After(req.To) {
		return nil, ErrAttestationPeriod
	}
	if req.TemplateRef != "" && !validTemplateRef(req.TemplateRef) {
		return nil, ErrAttestationTemplate
	}
	agent, err := s.Queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: req.Agent, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAttestationAgentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load agent: %w", err)
	}
	window := struct{ start, end pgtype.Timestamptz }{
		start: pgtype.Timestamptz{Time: req.From.UTC(), Valid: true},
		end:   pgtype.Timestamptz{Time: req.To.AddDate(0, 0, 1).UTC(), Valid: true},
	}
	rows, err := s.Queries.AttestationOutcomeCounts(ctx, db.AttestationOutcomeCountsParams{
		WorkspaceID: ws, AgentID: agent.ID, WindowStart: window.start, WindowEnd: window.end,
	})
	if err != nil {
		return nil, fmt.Errorf("count outcomes: %w", err)
	}
	classes := coarsenedClasses(rows)
	var n int64
	for _, c := range classes {
		n += c.Pass + c.Partial + c.Fail
	}
	if n < attestationMinResults {
		return nil, ErrAttestationSampleSmall
	}
	models, err := s.Queries.AttestationOutcomeModels(ctx, db.AttestationOutcomeModelsParams{
		WorkspaceID: ws, AgentID: agent.ID, WindowStart: window.start, WindowEnd: window.end,
	})
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	if models == nil {
		models = []string{}
	}

	workClasses := make([]any, 0, len(classes))
	for _, c := range classes {
		n := c.Pass + c.Partial + c.Fail
		workClasses = append(workClasses, map[string]any{
			"class": c.Class, "n": n, "pass": c.Pass, "partial": c.Partial, "fail": c.Fail,
			"score_lower":  math.Round(WilsonLower(int(c.Pass), int(c.Partial), int(c.Fail))*10000) / 10000,
			"verification": map[string]any{"full": c.Full, "sampled": c.Sampled, "jury": c.Jury},
		})
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	doc := map[string]any{
		"format":       "ocf-attestation/1",
		"subject":      map[string]any{"template_ref": req.TemplateRef, "seat_label": agent.Name},
		"issuer":       map[string]any{"host": s.Host, "key_id": KeyID(s.Key.Public().(ed25519.PublicKey))},
		"period":       map[string]any{"from": req.From.Format(time.DateOnly), "to": req.To.Format(time.DateOnly)},
		"work_classes": workClasses,
		"models":       models,
		"issued_at":    now.UTC().Format(time.RFC3339),
	}
	if req.ShowWorkspace {
		w, err := s.Queries.GetWorkspace(ctx, ws)
		if err != nil {
			return nil, fmt.Errorf("load workspace: %w", err)
		}
		doc["workspace"] = map[string]any{"label": w.Name}
	}
	return Sign(doc, s.Key, KeyID(s.Key.Public().(ed25519.PublicKey)))
}

// validTemplateRef accepts hub:<item>@<version> with both parts present.
func validTemplateRef(ref string) bool {
	rest, ok := strings.CutPrefix(ref, "hub:")
	if !ok {
		return false
	}
	item, version, ok := strings.Cut(rest, "@")
	return ok && item != "" && version != ""
}
