import { queryOptions, useQuery } from "@tanstack/react-query";
import { api } from "../../api";
import {
  ATTESTATION_KEYS_PATH,
  parseAttestationKeySet,
  parseVerifyAttestationResponse,
  serializeAttestationBody,
} from "./schemas";

// Every key starts with "durable" and the workspace id so caches stay isolated
// across workspaces.
export const attestationKeys = {
  all: (wsId: string) => ["durable", wsId, "attestations"] as const,
  publicKeys: (wsId: string) => [...attestationKeys.all(wsId), "keys"] as const,
  verify: (wsId: string, body: string) => [...attestationKeys.all(wsId), "verify", body] as const,
};

/** Published public keys of this server (GET /.well-known/multica-attestation-keys.json). */
export function attestationKeySetOptions(wsId: string, enabled = true) {
  return queryOptions({
    queryKey: attestationKeys.publicKeys(wsId),
    queryFn: async () =>
      parseAttestationKeySet(
        await api.request(ATTESTATION_KEYS_PATH),
        `GET ${ATTESTATION_KEYS_PATH}`,
      ),
    enabled: enabled && !!wsId,
  });
}

export function useAttestationKeySet(wsId: string, enabled = true) {
  return useQuery(attestationKeySetOptions(wsId, enabled));
}

/**
 * Verifies an attestation document against its issuer host's published keys
 * (POST /api/attestations/verify). Retries are off because verification is rate-limited.
 */
export function verifyAttestationOptions(wsId: string, document: unknown, enabled = true) {
  const serialized = document === undefined || document === null ? "" : serializeAttestationBody(document);
  return queryOptions({
    queryKey: attestationKeys.verify(wsId, serialized),
    queryFn: async () =>
      parseVerifyAttestationResponse(
        await api.request("/api/attestations/verify", {
          method: "POST",
          body: serialized,
        }),
        "POST /api/attestations/verify",
      ),
    enabled: enabled && !!wsId && serialized.trim() !== "",
    retry: false,
  });
}

export function useVerifyAttestationQuery(wsId: string, document: unknown, enabled = true) {
  return useQuery(verifyAttestationOptions(wsId, document, enabled));
}
