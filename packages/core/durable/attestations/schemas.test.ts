// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  EMPTY_ATTESTATION_KEY_SET,
  parseAttestationDocument,
  parseAttestationKeySet,
  parseVerifyAttestationResponse,
  serializeAttestationBody,
  UNVERIFIED_ATTESTATION_RESPONSE,
} from "./schemas";

describe("parseVerifyAttestationResponse", () => {
  it("parses valid verification responses", () => {
    expect(
      parseVerifyAttestationResponse({
        valid: true,
        issuer: "delegora.example.com",
        key_id: "ed25519-0123456789abcdef",
      }),
    ).toEqual({
      valid: true,
      issuer: "delegora.example.com",
      key_id: "ed25519-0123456789abcdef",
      reason: "",
    });
  });

  it("maps unknown reasons to unknown", () => {
    const res = parseVerifyAttestationResponse({
      valid: false,
      issuer: "other.example.com",
      reason: "expired_in_v2",
    });
    expect(res).toEqual({
      valid: false,
      issuer: "other.example.com",
      key_id: "",
      reason: "unknown",
    });
  });

  it("falls back to UNVERIFIED_ATTESTATION_RESPONSE on malformed payloads", () => {
    expect(parseVerifyAttestationResponse(null)).toEqual(UNVERIFIED_ATTESTATION_RESPONSE);
    expect(parseVerifyAttestationResponse("bad")).toEqual(UNVERIFIED_ATTESTATION_RESPONSE);
    expect(parseVerifyAttestationResponse({ issuer: "x" })).toEqual(
      UNVERIFIED_ATTESTATION_RESPONSE,
    );
  });
});

describe("parseAttestationKeySet", () => {
  it("parses valid keys and drops malformed entries", () => {
    const set = parseAttestationKeySet({
      host: "delegora.example.com",
      keys: [
        { key_id: "k-1", alg: "Ed25519", public_key: "abc123" },
        { key_id: "", alg: "Ed25519", public_key: "missing-id" },
      ],
    });
    expect(set).toEqual({
      host: "delegora.example.com",
      keys: [{ key_id: "k-1", alg: "Ed25519", public_key: "abc123" }],
    });
  });

  it("returns an empty key set on malformed payloads", () => {
    expect(parseAttestationKeySet(null)).toEqual(EMPTY_ATTESTATION_KEY_SET);
    expect(parseAttestationKeySet("bad")).toEqual(EMPTY_ATTESTATION_KEY_SET);
  });
});

describe("parseAttestationDocument", () => {
  it("returns null on malformed documents", () => {
    expect(parseAttestationDocument(null)).toBeNull();
    expect(parseAttestationDocument({ format: "ocf-attestation/1" })).toBeNull();
  });

  it("parses a complete attestation document", () => {
    const doc = parseAttestationDocument({
      format: "ocf-attestation/1",
      subject: { template_ref: "hub:coder@1.0.0", seat_label: "Backend" },
      issuer: { host: "delegora.example.com", key_id: "ed25519-1234" },
      period: { from: "2026-08-01", to: "2026-11-01" },
      work_classes: [
        {
          class: "area:flow:pr",
          n: 30,
          pass: 28,
          partial: 1,
          fail: 1,
          score_lower: 0.85,
          verification: { full: 10, sampled: 20, jury: 2 },
        },
      ],
      models: ["anthropic/claude-sonnet"],
      issued_at: "2026-11-02T00:00:00Z",
      signature: "c2lnbmF0dXJl",
    });

    expect(doc?.issuer.host).toBe("delegora.example.com");
    expect(doc?.work_classes).toHaveLength(1);
  });
});

describe("serializeAttestationBody", () => {
  it("returns a string as-is and stringifies objects", () => {
    expect(serializeAttestationBody('{"format":"ocf-attestation/1"}')).toBe(
      '{"format":"ocf-attestation/1"}',
    );
    expect(serializeAttestationBody({ format: "ocf-attestation/1" })).toBe(
      '{"format":"ocf-attestation/1"}',
    );
  });
});
