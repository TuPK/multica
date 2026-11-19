// @vitest-environment node
import { createHash, randomBytes } from "node:crypto";
import { beforeEach, describe, expect, it, vi } from "vitest";

// expo-crypto needs the native runtime; back it with node:crypto so the test
// exercises our encoding and state handling, not the native module.
vi.mock("expo-crypto", () => ({
  CryptoDigestAlgorithm: { SHA256: "SHA-256" },
  CryptoEncoding: { BASE64: "base64" },
  getRandomBytes: (n: number) => new Uint8Array(randomBytes(n)),
  digestStringAsync: async (_alg: string, data: string) =>
    createHash("sha256").update(data).digest("base64"),
}));

import {
  beginSsoHandoff,
  buildSsoStartUrl,
  challengeFor,
  clearSsoVerifier,
  createVerifier,
  takeSsoVerifier,
} from "./sso-handoff";

describe("sso handoff", () => {
  beforeEach(() => clearSsoVerifier());

  it("derives the RFC 7636 Appendix B challenge", async () => {
    await expect(challengeFor("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")).resolves.toBe(
      "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
    );
  });

  it("generates a 43-character base64url verifier without padding", () => {
    for (let i = 0; i < 50; i++) {
      expect(createVerifier()).toMatch(/^[A-Za-z0-9_-]{43}$/);
    }
    expect(createVerifier()).not.toBe(createVerifier());
  });

  it("keeps the verifier in memory until it is taken once", async () => {
    const challenge = await beginSsoHandoff();
    expect(challenge).toMatch(/^[A-Za-z0-9_-]{43}$/);
    const verifier = takeSsoVerifier();
    expect(verifier).not.toBeNull();
    await expect(challengeFor(verifier!)).resolves.toBe(challenge);
    expect(takeSsoVerifier()).toBeNull();
  });

  it("clears the verifier on request", async () => {
    await beginSsoHandoff();
    clearSsoVerifier();
    expect(takeSsoVerifier()).toBeNull();
  });

  it("appends client and challenge to the start URL", () => {
    expect(buildSsoStartUrl("https://api.example.test/auth/sso/start?connection=c1", "abc")).toBe(
      "https://api.example.test/auth/sso/start?connection=c1&client=mobile&challenge=abc",
    );
    expect(buildSsoStartUrl("https://api.example.test/auth/sso/start", "abc")).toBe(
      "https://api.example.test/auth/sso/start?client=mobile&challenge=abc",
    );
  });
});
