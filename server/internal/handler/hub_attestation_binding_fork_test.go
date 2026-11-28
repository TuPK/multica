package handler

import (
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/storage"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

func TestHubAttestationTemplateAndFlag(t *testing.T) {
	t.Setenv("LOCAL_UPLOAD_DIR", t.TempDir())
	store := storage.NewLocalStorageFromEnv()
	priv := ed25519.NewKeyFromSeed([]byte("delegora-hub-attestation-seed-01"))
	signed, err := service.Sign(map[string]any{"format": "ocf-attestation/1", "subject": map[string]any{"template_ref": "hub:reviewer@1"}, "issuer": map[string]any{"host": "example.test"}}, priv, "k1")
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := hub.BuildZip(&hub.Bundle{Files: []hub.FileEntry{{Path: "attestations/a.json", Size: int64(len(signed)), Content: signed}}, TotalSize: int64(len(signed))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Upload(context.Background(), "bundle", bundle, "application/zip", ""); err != nil {
		t.Fatal(err)
	}
	prev := hubAttestationKeyFetcherFor
	t.Cleanup(func() { hubAttestationKeyFetcherFor = prev })
	hubAttestationKeyFetcherFor = func(*Handler) service.AttestationKeyResolver {
		return &service.WellKnownKeyFetcher{Own: &service.KeySet{Host: "example.test", Keys: map[string]ed25519.PublicKey{"k1": priv.Public().(ed25519.PublicKey)}}}
	}
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableOpen, featureflag.Rule{Default: true})
	h := &Handler{HubStorage: store, FeatureFlags: featureflag.NewService(provider)}
	item := db.HubItem{Kind: string(hub.KindAgent), BundleKey: "bundle", SourceRef: "hub:reviewer@1"}
	got := h.hubItemAttestation(context.Background(), item)
	if got == nil || !got.Verified {
		t.Fatalf("matching: %+v", got)
	}
	item.SourceRef = "hub:other@1"
	got = h.hubItemAttestation(context.Background(), item)
	if got == nil || got.Verified {
		t.Fatalf("mismatched: %+v", got)
	}
	item.SourceRef = ""
	if got := h.hubItemAttestation(context.Background(), item); got == nil || got.Verified {
		t.Fatalf("empty item ref: %+v", got)
	}
	item.SourceRef = "hub:reviewer@1"
	provider.Set(featureflags.DurableOpen, featureflag.Rule{Default: false})
	if got := h.hubItemAttestation(context.Background(), item); got != nil {
		t.Fatalf("flag off: %+v", got)
	}
}
