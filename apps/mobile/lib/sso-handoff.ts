/**
 * PKCE-style handoff for SSO sign-in. The verifier stays in module memory
 * only (never persisted or logged): if the app restarts mid-flow the deep
 * link finds no verifier and the user starts over.
 */
import {
  CryptoDigestAlgorithm,
  CryptoEncoding,
  digestStringAsync,
  getRandomBytes,
} from "expo-crypto";

let pendingVerifier: string | null = null;

function toBase64Url(base64: string): string {
  return base64.replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/** 32 random bytes as 43 base64url characters. */
export function createVerifier(): string {
  const bytes = getRandomBytes(32);
  return toBase64Url(btoa(String.fromCharCode(...bytes)));
}

/** base64url(SHA-256(verifier)), as the server compares it. */
export async function challengeFor(verifier: string): Promise<string> {
  const digest = await digestStringAsync(CryptoDigestAlgorithm.SHA256, verifier, {
    encoding: CryptoEncoding.BASE64,
  });
  return toBase64Url(digest);
}

/** Starts a fresh handoff and returns the challenge to send to the server. */
export async function beginSsoHandoff(): Promise<string> {
  const verifier = createVerifier();
  const challenge = await challengeFor(verifier);
  pendingVerifier = verifier;
  return challenge;
}

/** Returns the verifier once and forgets it. */
export function takeSsoVerifier(): string | null {
  const verifier = pendingVerifier;
  pendingVerifier = null;
  return verifier;
}

export function clearSsoVerifier(): void {
  pendingVerifier = null;
}

export function buildSsoStartUrl(startUrl: string, challenge: string): string {
  const sep = startUrl.includes("?") ? "&" : "?";
  return `${startUrl}${sep}client=mobile&challenge=${challenge}`;
}
