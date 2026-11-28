// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  attestationKeys,
  attestationKeySetOptions,
  verifyAttestationOptions,
} from "./queries";

const { request } = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("../../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api")>()),
  api: { request },
}));

afterEach(() => {
  request.mockReset();
});

describe("attestation query options", () => {
  it("scopes query keys by workspace id", () => {
    expect(attestationKeys.all("ws-1")).toEqual(["durable", "ws-1", "attestations"]);
    expect(attestationKeys.publicKeys("ws-1")).toEqual([
      "durable",
      "ws-1",
      "attestations",
      "keys",
    ]);
    expect(attestationKeys.verify("ws-1", "{}")).toEqual([
      "durable",
      "ws-1",
      "attestations",
      "verify",
      "{}",
    ]);
  });

  it("loads published keys from the well-known path", async () => {
    request.mockResolvedValueOnce({
      host: "delegora.example.com",
      keys: [{ key_id: "k-1", alg: "Ed25519", public_key: "pub" }],
    });
    const opts = attestationKeySetOptions("ws-1");
    const result = await opts.queryFn!({} as never);

    expect(request).toHaveBeenCalledWith("/.well-known/multica-attestation-keys.json");
    expect(result.host).toBe("delegora.example.com");
    expect(result.keys).toHaveLength(1);
  });

  it("disables verification when document is null, undefined, or blank", () => {
    expect(verifyAttestationOptions("ws-1", null).enabled).toBe(false);
    expect(verifyAttestationOptions("ws-1", undefined).enabled).toBe(false);
    expect(verifyAttestationOptions("ws-1", "").enabled).toBe(false);
    expect(verifyAttestationOptions("ws-1", "   ").enabled).toBe(false);
    expect(verifyAttestationOptions("ws-1", { format: "ocf-attestation/1" }).enabled).toBe(true);
  });

  it("posts the serialized document to /api/attestations/verify without retries", async () => {
    request.mockResolvedValueOnce({ valid: true, issuer: "delegora.example.com", key_id: "k-1" });
    const opts = verifyAttestationOptions("ws-1", { format: "ocf-attestation/1" });
    expect(opts.retry).toBe(false);

    const res = await opts.queryFn!({} as never);
    expect(request).toHaveBeenCalledWith("/api/attestations/verify", {
      method: "POST",
      body: '{"format":"ocf-attestation/1"}',
    });
    expect(res).toEqual({
      valid: true,
      issuer: "delegora.example.com",
      key_id: "k-1",
      reason: "",
    });
  });
});
