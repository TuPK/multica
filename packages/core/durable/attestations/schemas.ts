import { z } from "zod";
import { parseWithFallback } from "../../api/schema";
import { count, listOfItems, str } from "../schema-helpers";

// Response shapes of the track-record attestation routes (durable spec 13.2,
// server durable_attestation_fork.go and durable_attestation_issue_fork.go).

export const ATTESTATION_FORMAT = "ocf-attestation/1" as const;
export const ATTESTATION_KEYS_PATH = "/.well-known/multica-attestation-keys.json" as const;

export const VERIFY_ATTESTATION_REASONS = [
  "",
  "malformed",
  "keys_unavailable",
  "bad_signature",
  "unknown_key",
  "foreign_issuer",
  "unknown",
] as const;

export type VerifyAttestationReason = (typeof VERIFY_ATTESTATION_REASONS)[number];

const verifyReasonSchema = z
  .unknown()
  .transform((value): VerifyAttestationReason => {
    if (value === undefined || value === null || value === "") return "";
    if (typeof value !== "string") return "unknown";
    return (VERIFY_ATTESTATION_REASONS as readonly string[]).includes(value)
      ? (value as VerifyAttestationReason)
      : "unknown";
  });

const unitFloat = z.coerce.number().finite().min(0).max(1).catch(0).default(0);

export const AttestationPublicKeySchema = z
  .object({
    key_id: z.string().min(1),
    alg: str,
    public_key: z.string().min(1),
  })
  .loose();

export const AttestationKeySetSchema = z
  .object({
    host: str,
    keys: listOfItems(AttestationPublicKeySchema),
  })
  .loose();

export const AttestationVerificationCountsSchema = z
  .object({
    full: count,
    sampled: count,
    jury: count,
  })
  .loose();

export const AttestationWorkClassSchema = z
  .object({
    class: z.string().min(1),
    n: count,
    pass: count,
    partial: count,
    fail: count,
    score_lower: unitFloat,
    verification: AttestationVerificationCountsSchema.catch({
      full: 0,
      sampled: 0,
      jury: 0,
    }).default({ full: 0, sampled: 0, jury: 0 }),
  })
  .loose();

export const AttestationDocumentSchema = z
  .object({
    format: z.string().min(1),
    subject: z
      .object({
        template_ref: str,
        seat_label: str,
      })
      .loose()
      .catch({ template_ref: "", seat_label: "" })
      .default({ template_ref: "", seat_label: "" }),
    issuer: z
      .object({
        host: z.string().min(1),
        key_id: z.string().min(1),
      })
      .loose(),
    period: z
      .object({
        from: str,
        to: str,
      })
      .loose()
      .catch({ from: "", to: "" })
      .default({ from: "", to: "" }),
    work_classes: listOfItems(AttestationWorkClassSchema),
    models: listOfItems(z.string().min(1)),
    issued_at: str,
    workspace: z
      .object({
        label: str,
      })
      .loose()
      .optional(),
    signature: z.string().min(1),
  })
  .loose();

export const VerifyAttestationResponseSchema = z
  .object({
    valid: z.boolean(),
    issuer: str,
    key_id: str,
    reason: verifyReasonSchema,
  })
  .loose();

export type AttestationPublicKey = z.infer<typeof AttestationPublicKeySchema>;
export type AttestationKeySet = z.infer<typeof AttestationKeySetSchema>;
export type AttestationWorkClass = z.infer<typeof AttestationWorkClassSchema>;
export type AttestationDocument = z.infer<typeof AttestationDocumentSchema>;
export type VerifyAttestationResponse = z.infer<typeof VerifyAttestationResponseSchema>;

export const EMPTY_ATTESTATION_KEY_SET: AttestationKeySet = {
  host: "",
  keys: [],
};

// A malformed verify response never passes as verified.
export const UNVERIFIED_ATTESTATION_RESPONSE: VerifyAttestationResponse = {
  valid: false,
  issuer: "",
  key_id: "",
  reason: "malformed",
};

export function parseAttestationKeySet(
  raw: unknown,
  endpoint = `GET ${ATTESTATION_KEYS_PATH}`,
): AttestationKeySet {
  return parseWithFallback<AttestationKeySet>(
    raw,
    AttestationKeySetSchema,
    EMPTY_ATTESTATION_KEY_SET,
    { endpoint },
  );
}

export function parseAttestationDocument(
  raw: unknown,
  endpoint = "POST /api/attestations/issue",
): AttestationDocument | null {
  return parseWithFallback<AttestationDocument | null>(raw, AttestationDocumentSchema, null, {
    endpoint,
  });
}

export function parseVerifyAttestationResponse(
  raw: unknown,
  endpoint = "POST /api/attestations/verify",
): VerifyAttestationResponse {
  return parseWithFallback<VerifyAttestationResponse>(
    raw,
    VerifyAttestationResponseSchema,
    UNVERIFIED_ATTESTATION_RESPONSE,
    { endpoint },
  );
}

export function serializeAttestationBody(document: unknown): string {
  return typeof document === "string" ? document : JSON.stringify(document);
}
