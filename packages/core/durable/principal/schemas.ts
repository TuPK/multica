import { z } from "zod";
import { parseWithFallback } from "../../api/schema";
import { bool, count, enumOr, listOfItems, nullableStr, str } from "../schema-helpers";

// Response shapes of the principal profile, interview, prediction, evidence candidate, and
// delegation routes (durable spec 6.2 to 6.6, server durable_principal_fork.go,
// durable_prediction_fork.go, and durable_delegation_fork.go). Unknown enum values degrade to
// safe defaults so the page renders across mixed client/server versions.

export const PRINCIPAL_STATUSES = ["active", "paused"] as const;
export const PRINCIPAL_ORIGINS = ["learned", "interview", "edited"] as const;
export const PRINCIPAL_EVIDENCE_TYPES = ["decision", "comment", "interview"] as const;
export const PRINCIPAL_PREDICTION_SUBJECT_TYPES = [
  "proposal",
  "effect_intent",
  "outcome_check",
  "inquiry",
] as const;

export type PrincipalStatus = (typeof PRINCIPAL_STATUSES)[number];
export type PrincipalOrigin = (typeof PRINCIPAL_ORIGINS)[number];
export type PrincipalEvidenceType = (typeof PRINCIPAL_EVIDENCE_TYPES)[number];
export type PrincipalPredictionSubjectType = (typeof PRINCIPAL_PREDICTION_SUBJECT_TYPES)[number];

/** Default minimum confidence when enabling delegation for a decision kind (spec 6.2). */
export const DEFAULT_DELEGATION_MIN_CONFIDENCE = 0.9;

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

const unitFloat = (fallback: number) => z.number().finite().min(0).max(1).catch(fallback).default(fallback);
const nonNegInt = z.number().int().nonnegative().catch(0).default(0);
const nullableBool = z.boolean().nullable().catch(null).default(null);

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

export const PrincipalDelegationSchema = z
  .object({
    decision_kind: z.string().min(1),
    enabled: bool,
    min_confidence: unitFloat(DEFAULT_DELEGATION_MIN_CONFIDENCE),
    enabled_at: str,
    disabled_reason: str,
  })
  .loose();

export const PrincipalKindAccuracySchema = z
  .object({
    decision_kind: z.string().min(1),
    matched: nonNegInt,
    n: nonNegInt,
    lower: unitFloat(0),
    eligible: bool,
  })
  .loose();

export const PrincipalDelegationsSchema = z
  .object({
    delegations: listOfItems(PrincipalDelegationSchema),
    kinds: listOfItems(PrincipalKindAccuracySchema),
  })
  .loose();

export const PrincipalPredictionSchema = z
  .object({
    predicted: z.string().min(1),
    confidence: unitFloat(0),
    why: str,
    actual: nullableStr,
    matched: nullableBool,
  })
  .loose();

export const PrincipalEvidenceCandidateSchema = z
  .object({
    id: z.string().min(1),
    title: str,
    decision: str,
    decided_at: str,
  })
  .loose();

export const PrincipalEvidenceCandidatesSchema = z
  .object({
    decisions: listOfItems(PrincipalEvidenceCandidateSchema),
  })
  .loose();

export type PrincipalEvidence = z.infer<typeof PrincipalEvidenceSchema>;
export type PrincipalTrait = z.infer<typeof PrincipalTraitSchema>;
export type PrincipalProfile = z.infer<typeof PrincipalProfileSchema>;
export type PrincipalInterview = z.infer<typeof PrincipalInterviewSchema>;
export type DelegationEnableResult = z.infer<typeof DelegationEnableResultSchema>;
export type PrincipalDelegation = z.infer<typeof PrincipalDelegationSchema>;
export type PrincipalKindAccuracy = z.infer<typeof PrincipalKindAccuracySchema>;
export type PrincipalDelegations = z.infer<typeof PrincipalDelegationsSchema>;
export type PrincipalPrediction = z.infer<typeof PrincipalPredictionSchema>;
export type PrincipalEvidenceCandidate = z.infer<typeof PrincipalEvidenceCandidateSchema>;
export type PrincipalEvidenceCandidates = z.infer<typeof PrincipalEvidenceCandidatesSchema>;

export const EMPTY_PRINCIPAL_DELEGATIONS: PrincipalDelegations = {
  delegations: [],
  kinds: [],
};

export const EMPTY_PRINCIPAL_EVIDENCE_CANDIDATES: PrincipalEvidenceCandidates = {
  decisions: [],
};

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

export function parsePrincipalDelegations(raw: unknown, endpoint: string): PrincipalDelegations {
  return parseWithFallback<PrincipalDelegations>(
    raw,
    PrincipalDelegationsSchema,
    EMPTY_PRINCIPAL_DELEGATIONS,
    { endpoint },
  );
}

export function parsePrincipalPrediction(raw: unknown, endpoint: string): PrincipalPrediction | null {
  return parseWithFallback<PrincipalPrediction | null>(raw, PrincipalPredictionSchema, null, { endpoint });
}

export function parsePrincipalEvidenceCandidates(
  raw: unknown,
  endpoint: string,
): PrincipalEvidenceCandidates {
  return parseWithFallback<PrincipalEvidenceCandidates>(
    raw,
    PrincipalEvidenceCandidatesSchema,
    EMPTY_PRINCIPAL_EVIDENCE_CANDIDATES,
    { endpoint },
  );
}
