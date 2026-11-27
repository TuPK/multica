import { z } from "zod";
import { parseWithFallback } from "../../api/schema";
import { bool, count, enumOr, listOfItems, str } from "../schema-helpers";

// Response shapes of the principal profile, interview and delegation routes (durable spec 6.2 to
// 6.6, server durable_principal_fork.go and durable_delegation_fork.go). Unknown enum values
// degrade to safe defaults so the page renders across mixed client/server versions.

export const PRINCIPAL_STATUSES = ["active", "paused"] as const;
export const PRINCIPAL_ORIGINS = ["learned", "interview", "edited"] as const;
export const PRINCIPAL_EVIDENCE_TYPES = ["decision", "comment", "interview"] as const;

export type PrincipalStatus = (typeof PRINCIPAL_STATUSES)[number];
export type PrincipalOrigin = (typeof PRINCIPAL_ORIGINS)[number];
export type PrincipalEvidenceType = (typeof PRINCIPAL_EVIDENCE_TYPES)[number];

/** Default minimum confidence when enabling delegation for a decision kind (spec 6.2). */
export const DEFAULT_DELEGATION_MIN_CONFIDENCE = 0.9;

/** Title the server gives the Secretary's start-up interview chat (service/durable_principal_fork.go). */
export const PRINCIPAL_INTERVIEW_SESSION_TITLE = "Phỏng vấn hồ sơ";

/**
 * Terms of a decision kind that can never be delegated (spec 6.5, server
 * service/durable_delegation_fork.go:forbiddenDelegationTerms). Irreversible intents and urgent
 * questions are also forbidden regardless of kind. The server enforces the same rule; this list
 * keeps the UI locked for clarity.
 */
export const FORBIDDEN_DELEGATION_TERMS = [
  "budget",
  "autonomy",
  "envelope",
  "policy",
  "profile",
  "launch",
  "stage_gate",
  "org_change",
  "effect:spend",
  "effect:share",
] as const;

/** Reports whether a decision kind is statically forbidden from prediction delegation (spec 6.5). */
export function isForbiddenDelegationKind(kind: string): boolean {
  const lower = kind.trim().toLowerCase();
  return FORBIDDEN_DELEGATION_TERMS.some((term) => lower.includes(term));
}

export const PrincipalEvidenceSchema = z
  .object({
    type: enumOr(PRINCIPAL_EVIDENCE_TYPES, "decision"),
    ref: str,
  })
  .loose();

export const PrincipalTraitSchema = z
  .object({
    id: z.string().min(1),
    decision_kind: str,
    statement: str,
    origin: enumOr(PRINCIPAL_ORIGINS, "learned"),
    evidence: listOfItems(PrincipalEvidenceSchema),
    created_at: str,
  })
  .loose();

export const PrincipalProfileSchema = z
  .object({
    member_id: z.string().min(1),
    status: enumOr(PRINCIPAL_STATUSES, "active"),
    consent_at: str,
    share_with_owners: bool,
    version: count,
    traits: listOfItems(PrincipalTraitSchema),
  })
  .loose();

export const PrincipalInterviewSchema = z
  .object({
    questions: listOfItems(z.string().min(1)),
  })
  .loose();

export const DelegationEnableResultSchema = z
  .object({
    decision_kind: z.string().min(1),
    enabled: z.literal(true),
  })
  .loose();

export type PrincipalEvidence = z.infer<typeof PrincipalEvidenceSchema>;
export type PrincipalTrait = z.infer<typeof PrincipalTraitSchema>;
export type PrincipalProfile = z.infer<typeof PrincipalProfileSchema>;
export type PrincipalInterview = z.infer<typeof PrincipalInterviewSchema>;
export type DelegationEnableResult = z.infer<typeof DelegationEnableResultSchema>;

export function parsePrincipalProfile(raw: unknown, endpoint: string): PrincipalProfile | null {
  return parseWithFallback<PrincipalProfile | null>(raw, PrincipalProfileSchema, null, { endpoint });
}

export function parsePrincipalTrait(raw: unknown, endpoint: string): PrincipalTrait | null {
  return parseWithFallback<PrincipalTrait | null>(raw, PrincipalTraitSchema, null, { endpoint });
}

export function parsePrincipalInterview(raw: unknown, endpoint: string): PrincipalInterview {
  return parseWithFallback<PrincipalInterview>(raw, PrincipalInterviewSchema, { questions: [] }, { endpoint });
}

export function parseDelegationEnableResult(raw: unknown, endpoint: string): DelegationEnableResult | null {
  return parseWithFallback<DelegationEnableResult | null>(raw, DelegationEnableResultSchema, null, { endpoint });
}
