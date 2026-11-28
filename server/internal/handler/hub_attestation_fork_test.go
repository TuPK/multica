package handler

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
)

type hubAttestationFixture struct {
	ws         string
	owner      string
	priv       ed25519.PrivateKey
	remoteHost string
}

func newHubAttestationFixture(t *testing.T) hubAttestationFixture {
	t.Helper()
	setupHubImportTest(t)
	withFeatureFlag(t, testHandler, featureflags.DurableOpen, true)

	ws := dbfx.Workspace(t, hubTestLabel(t, "att-ws"), hubTestSlug(t))
	owner := dbfx.User(t, hubTestLabel(t, "att-owner"), hubTestEmail(t))
	dbfx.Member(t, ws, owner, hubRoleOwner)
	dbfx.Cleanup(t, "DELETE FROM hub_activity WHERE workspace_id = $1", ws)
	dbfx.Cleanup(t, "DELETE FROM hub_item_version_blob WHERE workspace_id = $1", ws)
	dbfx.Cleanup(t, "DELETE FROM hub_item WHERE workspace_id = $1", ws)

	seed := []byte("delegora-hub-attestation-seed-01")
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)

	var keysDoc []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != service.AttestationKeysPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(keysDoc)
	}))
	t.Cleanup(srv.Close)

	remoteHost := strings.TrimPrefix(srv.URL, "http://")
	var err error
	keysDoc, err = service.PublicKeysDocument(service.KeySet{
		Host: remoteHost,
		Keys: map[string]ed25519.PublicKey{"k1": pub},
	})
	if err != nil {
		t.Fatalf("PublicKeysDocument: %v", err)
	}

	prevFetcher := hubAttestationKeyFetcherFor
	t.Cleanup(func() { hubAttestationKeyFetcherFor = prevFetcher })
	hubAttestationKeyFetcherFor = func(h *Handler) service.AttestationKeyResolver {
		own := h.attestationService().KeySet()
		return &service.WellKnownKeyFetcher{
			Own:    &own,
			Scheme: "http",
			Get: func(ctx context.Context, targetURL string, maxBytes int64) ([]byte, error) {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
				if err != nil {
					return nil, err
				}
				resp, err := srv.Client().Do(req)
				if err != nil {
					return nil, err
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					return nil, errors.New(resp.Status)
				}
				return io.ReadAll(io.LimitReader(resp.Body, maxBytes))
			},
		}
	}

	return hubAttestationFixture{
		ws:         ws,
		owner:      owner,
		priv:       priv,
		remoteHost: remoteHost,
	}
}

func (f hubAttestationFixture) signAttestation(t *testing.T, host string, refs ...string) []byte {
	t.Helper()
	ref := "hub:agent-reviewer@1.0.0"
	if len(refs) > 0 {
		ref = refs[0]
	}
	doc, err := service.Sign(map[string]any{
		"format":  "ocf-attestation/1",
		"subject": map[string]any{"template_ref": ref, "seat_label": "Code Reviewer"},
		"issuer":  map[string]any{"host": host},
		"period":  map[string]any{"from": "2026-08-01", "to": "2026-11-01"},
		"work_classes": []map[string]any{{
			"class": "area:flow:pr", "n": 64, "pass": 60, "partial": 2, "fail": 2, "score_lower": 0.88,
			"verification": map[string]any{"full": 20, "sampled": 44, "jury": 4},
		}},
		"models":    []string{"anthropic/claude-sonnet-5-5"},
		"issued_at": "2026-11-02T10:00:00Z",
	}, f.priv, "k1")
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	return doc
}

func (f hubAttestationFixture) insertAgentItem(t *testing.T, files map[string][]byte) (refID, digest string) {
	t.Helper()
	refID = uuid.NewString()
	entries := make([]hub.FileEntry, 0, len(files))
	var total int64
	for p, content := range files {
		entries = append(entries, hub.FileEntry{Path: p, Size: int64(len(content)), Content: content})
		total += int64(len(content))
	}
	bundle := &hub.Bundle{Files: entries, TotalSize: total}
	digest = hub.Digest(bundle)
	zipBytes, err := hub.BuildZip(bundle)
	if err != nil {
		t.Fatalf("BuildZip: %v", err)
	}
	key := hubBundleKey(f.ws, string(hub.KindAgent), refID, digest)
	if _, err := testHandler.HubStorage.Upload(context.Background(), key, zipBytes, "application/zip", ""); err != nil {
		t.Fatalf("HubStorage.Upload: %v", err)
	}
	dbfx.Insert(t, "hub_item", testutil.Cols{
		"workspace_id":   f.ws,
		"kind":           string(hub.KindAgent),
		"ref_id":         refID,
		"source_ref":     "acme/code-reviewer@1.0.0",
		"tracking":       "manual",
		"latest_version": "1.0.0",
		"bundle_key":     key,
		"bundle_digest":  digest,
		"size_bytes":     total,
		"file_count":     len(entries),
		"scan_status":    "clean",
	})
	return refID, digest
}

func (f hubAttestationFixture) getHubItemRaw(t *testing.T, kind, refID string) *testutil.Response {
	t.Helper()
	req := testutil.WithURLParams(
		hubTestRequest(http.MethodGet, "/api/hub/items/"+kind+"/"+refID, nil, f.ws, f.owner),
		"kind", kind, "id", refID,
	)
	return testutil.Call(t, testHandler.GetHubItem, req)
}

func TestHubItemAttestation_Valid(t *testing.T) {
	f := newHubAttestationFixture(t)
	signed := f.signAttestation(t, f.remoteHost)
	refID, digest := f.insertAgentItem(t, map[string][]byte{
		"pack.json":                 []byte(`{"name":"Code Reviewer","version":"1.0.0"}`),
		"attestations/2026-q4.json": signed,
	})

	var detail HubItemResponse
	f.getHubItemRaw(t, string(hub.KindAgent), refID).Want(http.StatusOK).JSON(&detail)
	if detail.Kind != string(hub.KindAgent) || detail.RefID != refID || detail.BundleDigest != digest {
		t.Fatalf("detail = %+v, want kind=agent ref_id=%s digest=%s", detail, refID, digest)
	}
	if detail.Attestation == nil {
		t.Fatalf("expected attestation on item detail, got nil")
	}
	if detail.Attestation.Verified || detail.Attestation.IssuerHost != f.remoteHost {
		t.Fatalf("detail.Attestation = %+v, want verified=false issuer_host=%q", detail.Attestation, f.remoteHost)
	}

	// ListHubItems includes the same unverified attestation.
	listed := listHubItemsForTest(t, f.ws, f.owner)
	listedItem, ok := findHubItemResponse(listed, refID)
	if !ok {
		t.Fatalf("agent item %s missing from ListHubItems", refID)
	}
	if listedItem.Attestation == nil || listedItem.Attestation.Verified || listedItem.Attestation.IssuerHost != f.remoteHost {
		t.Fatalf("listedItem.Attestation = %+v, want verified=false issuer_host=%q", listedItem.Attestation, f.remoteHost)
	}

	// Bundle route and activity filter accept kind=agent.
	bundle := getHubItemBundleForTest(t, f.ws, f.owner, string(hub.KindAgent), refID)
	if bundle.Digest != digest {
		t.Fatalf("bundle.Digest = %q, want %q", bundle.Digest, digest)
	}
	var act HubActivityResponse
	testutil.Call(t, testHandler.GetHubActivity,
		hubTestRequest(http.MethodGet, "/api/hub/activity?kind=agent", nil, f.ws, f.owner)).
		Want(http.StatusOK).JSON(&act)
}

func TestHubItemAttestation_TamperedSignature(t *testing.T) {
	f := newHubAttestationFixture(t)
	signed := f.signAttestation(t, f.remoteHost, "acme/code-reviewer@1.0.0")
	tampered := bytes.Replace(signed, []byte(`"n":64`), []byte(`"n":65`), 1)
	if bytes.Equal(signed, tampered) {
		t.Fatal("failed to tamper with signed attestation")
	}

	refID, _ := f.insertAgentItem(t, map[string][]byte{
		"pack.json":                 []byte(`{"name":"Code Reviewer","version":"1.0.0"}`),
		"attestations/2026-q4.json": tampered,
	})

	var detail HubItemResponse
	f.getHubItemRaw(t, string(hub.KindAgent), refID).Want(http.StatusOK).JSON(&detail)
	if detail.Attestation == nil {
		t.Fatalf("expected attestation field for tampered attestation, got nil")
	}
	if detail.Attestation.Verified {
		t.Fatalf("tampered attestation must have verified=false, got %+v", detail.Attestation)
	}
	if detail.Attestation.IssuerHost != f.remoteHost {
		t.Fatalf("issuer_host = %q, want %q", detail.Attestation.IssuerHost, f.remoteHost)
	}
}

func TestHubItemAttestation_MalformedStillReturns200WithFullItem(t *testing.T) {
	f := newHubAttestationFixture(t)

	cases := []struct {
		name string
		body []byte
	}{
		{"invalid json", []byte(`{not valid json`)},
		{"json array", []byte(`["not","an","object"]`)},
		{"json null", []byte(`null`)},
		{"missing fields", []byte(`{"format":"ocf-attestation/1"}`)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refID, digest := f.insertAgentItem(t, map[string][]byte{
				"pack.json":             []byte(`{"name":"Code Reviewer","version":"1.0.0"}`),
				"attestations/bad.json": tc.body,
			})

			var detail HubItemResponse
			f.getHubItemRaw(t, string(hub.KindAgent), refID).Want(http.StatusOK).JSON(&detail)
			if detail.ID == "" || detail.Kind != string(hub.KindAgent) || detail.RefID != refID ||
				detail.BundleDigest != digest || detail.LatestVersion != "1.0.0" || detail.FileCount != 2 {
				t.Fatalf("item fields damaged on malformed attestation: %+v", detail)
			}
			if detail.Attestation == nil {
				t.Fatalf("expected attestation with verified=false for %s, got nil", tc.name)
			}
			if detail.Attestation.Verified {
				t.Fatalf("malformed attestation (%s) must have verified=false, got %+v", tc.name, detail.Attestation)
			}
		})
	}
}

func TestHubItemAttestation_AbsentOmitsField(t *testing.T) {
	f := newHubAttestationFixture(t)
	refID, _ := f.insertAgentItem(t, map[string][]byte{
		"pack.json": []byte(`{"name":"Plain Agent","version":"1.0.0"}`),
	})

	rec := f.getHubItemRaw(t, string(hub.KindAgent), refID).Want(http.StatusOK)
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal raw response: %v", err)
	}
	if _, exists := raw["attestation"]; exists {
		t.Fatalf("item without attestation must omit attestation field, got %v", raw["attestation"])
	}
}

func TestHubItemAttestation_MultipleFilesPicksFirstByPath(t *testing.T) {
	f := newHubAttestationFixture(t)
	signed := f.signAttestation(t, f.remoteHost, "acme/code-reviewer@1.0.0")
	tampered := bytes.Replace(signed, []byte(`"n":64`), []byte(`"n":65`), 1)

	refID, _ := f.insertAgentItem(t, map[string][]byte{
		"pack.json":           []byte(`{"name":"Multi Attest","version":"1.0.0"}`),
		"attestations/b.json": tampered,
		"attestations/a.json": signed,
	})

	var detail HubItemResponse
	f.getHubItemRaw(t, string(hub.KindAgent), refID).Want(http.StatusOK).JSON(&detail)
	if detail.Attestation == nil || !detail.Attestation.Verified || detail.Attestation.IssuerHost != f.remoteHost {
		t.Fatalf("expected first sorted file (a.json) to verify, got %+v", detail.Attestation)
	}
}

func TestHubItemAttestation_OwnServerKeyWithoutNetworkFetch(t *testing.T) {
	f := newHubAttestationFixture(t)
	ownHost := "own.delegora.test"
	pub := f.priv.Public().(ed25519.PublicKey)

	hubAttestationKeyFetcherFor = func(h *Handler) service.AttestationKeyResolver {
		own := service.KeySet{
			Host: ownHost,
			Keys: map[string]ed25519.PublicKey{"k1": pub},
		}
		return &service.WellKnownKeyFetcher{
			Own: &own,
			Get: func(ctx context.Context, targetURL string, maxBytes int64) ([]byte, error) {
				t.Fatalf("unexpected remote key fetch for own host: %s", targetURL)
				return nil, nil
			},
		}
	}

	signed := f.signAttestation(t, ownHost, "acme/code-reviewer@1.0.0")
	refID, _ := f.insertAgentItem(t, map[string][]byte{
		"pack.json":               []byte(`{"name":"Own Key Agent","version":"1.0.0"}`),
		"attestations/local.json": signed,
	})

	var detail HubItemResponse
	f.getHubItemRaw(t, string(hub.KindAgent), refID).Want(http.StatusOK).JSON(&detail)
	if detail.Attestation == nil || !detail.Attestation.Verified || detail.Attestation.IssuerHost != ownHost {
		t.Fatalf("own-host attestation = %+v, want verified=true issuer_host=%q", detail.Attestation, ownHost)
	}
}

func TestHubItemAttestation_FlagOff(t *testing.T) {
	f := newHubAttestationFixture(t)
	signed := f.signAttestation(t, f.remoteHost, "acme/code-reviewer@1.0.0")
	ref, _ := f.insertAgentItem(t, map[string][]byte{"attestations/a.json": signed})
	withFeatureFlag(t, testHandler, featureflags.DurableOpen, false)
	f.getHubItemRaw(t, string(hub.KindAgent), ref).Want(http.StatusNotFound)
	listed := listHubItemsForTest(t, f.ws, f.owner)
	item, ok := findHubItemResponse(listed, ref)
	if ok {
		t.Fatalf("agent item listed with flag off: %+v", item)
	}
}
