package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"

	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// HubItemAttestationResponse summarizes the signature check of a Hub item's
// bundled attestation (durable spec 13.2, 13.3). Omitted when the item's bundle
// carries no attestations/*.json file.
type HubItemAttestationResponse struct {
	Verified   bool   `json:"verified"`
	IssuerHost string `json:"issuer_host"`
}

// hubAttestationKeyFetcherFor builds the key resolver used to verify a Hub
// item's bundled attestation. It checks this server's own KeySet first and
// fetches remote hosts via WellKnownKeyFetcher. Tests may override it.
var hubAttestationKeyFetcherFor = func(h *Handler) service.AttestationKeyResolver {
	own := h.attestationService().KeySet()
	return &service.WellKnownKeyFetcher{Own: &own}
}

// hubItemAttestation reads attestations/*.json from the item's stored bundle
// (without modifying or rebuilding the bundle) and verifies the first entry
// sorted by path using service.Verify. Returns nil when the item has no stored
// bundle or no attestations/*.json files. Malformed or unverified attestations
// return verified=false and never fail the item response.
func (h *Handler) hubItemAttestation(ctx context.Context, item db.HubItem) *HubItemAttestationResponse {
	// Only agent templates carry attestations (spec 13.3). Skipping other kinds
	// keeps a list of skills and plugins from reading every bundle.
	if item.Kind != string(hub.KindAgent) {
		return nil
	}
	if item.BundleKey == "" || h.HubStorage == nil {
		return nil
	}
	rc, err := h.HubStorage.GetReader(ctx, item.BundleKey)
	if err != nil {
		slog.Debug("hub item attestation: bundle read failed", "item_id", uuidToString(item.ID), "error", err)
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(rc, hubBundleReadLimit+1))
	rc.Close()
	if err != nil || int64(len(data)) > hubBundleReadLimit {
		slog.Debug("hub item attestation: bundle body unreadable", "item_id", uuidToString(item.ID), "error", err)
		return nil
	}
	bundle, err := hub.ExtractZip(bytes.NewReader(data), int64(len(data)), hubBundleVerifyLimits)
	if err != nil {
		slog.Debug("hub item attestation: zip extract failed", "item_id", uuidToString(item.ID), "error", err)
		return nil
	}
	atts, err := hub.ExtractAttestations(bundle)
	if err != nil {
		// At least one attestations/*.json file is present but not a valid JSON object.
		return &HubItemAttestationResponse{Verified: false, IssuerHost: ""}
	}
	if len(atts) == 0 {
		return nil
	}
	return h.verifyHubBundleAttestation(ctx, atts[0].Content)
}

func (h *Handler) verifyHubBundleAttestation(ctx context.Context, raw []byte) *HubItemAttestationResponse {
	var probe struct {
		Issuer struct {
			Host string `json:"host"`
		} `json:"issuer"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return &HubItemAttestationResponse{Verified: false, IssuerHost: ""}
	}
	issuerHost := strings.TrimSpace(probe.Issuer.Host)
	if issuerHost == "" {
		return &HubItemAttestationResponse{Verified: false, IssuerHost: ""}
	}
	resolver := hubAttestationKeyFetcherFor(h)
	if resolver == nil {
		return &HubItemAttestationResponse{Verified: false, IssuerHost: issuerHost}
	}
	keys, err := resolver.KeysForHost(ctx, issuerHost)
	if err != nil {
		return &HubItemAttestationResponse{Verified: false, IssuerHost: issuerHost}
	}
	verified := service.Verify(raw, keys) == nil
	return &HubItemAttestationResponse{Verified: verified, IssuerHost: issuerHost}
}
