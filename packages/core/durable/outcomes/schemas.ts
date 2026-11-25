import { z } from "zod";
import { parseWithFallback } from "../../api/schema";
import { bool, count, enumOr, listOfItems, nullableStr, str } from "../schema-helpers";

// Response shapes of the outcome and criteria routes (durable spec 2.7, server
// durable_outcome_fork.go). A value this build does not know degrades to the value listed
// as the fallback: an unknown verdict reads as unverified, so it never counts as a pass.

export const OUTCOME_VERDICTS = ["pending", "pass", "partial", "fail", "unverified"] as const;
export const SEVERITIES = ["none", "low", "medium", "high"] as const;
export const FAIL_CAUSES = ["", "criteria", "reopened", "reverted", "metric", "jury", "member"] as const;
export const VERIFICATION_MODES = ["full", "sampled", "spot"] as const;
export const CHECK_KINDS = ["manual", "agent_judgment", "command", "artifact", "metric"] as const;
export const CRITERION_SOURCES = ["member", "agent", "flow", "template"] as const;
export const VERIFIER_TYPES = ["system", "agent", "member", "metric", "jury"] as const;
export const VERIFICATION_RESULTS = ["pass", "partial", "fail", "unable"] as const;

export type OutcomeVerdict = (typeof OUTCOME_VERDICTS)[number];
export type Severity = (typeof SEVERITIES)[number];
export type FailCause = (typeof FAIL_CAUSES)[number];
export type VerificationMode = (typeof VERIFICATION_MODES)[number];
export type CheckKind = (typeof CHECK_KINDS)[number];
export type CriterionSource = (typeof CRITERION_SOURCES)[number];
export type VerifierType = (typeof VERIFIER_TYPES)[number];
export type VerificationResult = (typeof VERIFICATION_RESULTS)[number];

// The check results are what an agent or a person records; "unable" is the safe reading of
// anything else, because the reputation counts only pass, partial and fail.
const jsonArray = z.array(z.unknown()).catch([]).default([]);
const jsonRecord = z.record(z.string(), z.unknown()).catch({}).default({});

export const VerificationSchema = z
  .object({
    id: str,
    outcome_id: str,
    criterion_id: nullableStr,
    verifier_type: enumOr(VERIFIER_TYPES, "system"),
    verifier_id: nullableStr,
    task_id: nullableStr,
    result: enumOr(VERIFICATION_RESULTS, "unable"),
    severity: enumOr(SEVERITIES, "none"),
    note: str,
    evidence: jsonArray,
    created_at: str,
  })
  .loose();

export const OutcomeSchema = z
  .object({
    id: str,
    issue_id: str,
    agent_id: nullableStr,
    member_id: nullableStr,
    squad_id: nullableStr,
    work_class: str,
    verification_mode: enumOr(VERIFICATION_MODES, "full"),
    sampled: bool,
    verdict: enumOr(OUTCOME_VERDICTS, "unverified"),
    severity: enumOr(SEVERITIES, "none"),
    fail_cause: enumOr(FAIL_CAUSES, ""),
    settle_at: str,
    settled_at: nullableStr,
    superseded_by: nullableStr,
    created_at: str,
    // Set on the detail view only; a list item has none.
    verifications: listOfItems(VerificationSchema),
  })
  .loose();

export const CriterionSchema = z
  .object({
    id: str,
    issue_id: str,
    position: count,
    text: str,
    check_kind: enumOr(CHECK_KINDS, "manual"),
    check_spec: jsonRecord,
    source: enumOr(CRITERION_SOURCES, "member"),
    created_by_type: str,
    created_by_id: nullableStr,
    archived_at: nullableStr,
    created_at: str,
  })
  .loose();

export const OutcomeListSchema = z.object({ outcomes: listOfItems(OutcomeSchema), next_cursor: str }).loose();

export const CriteriaListSchema = z.object({ criteria: listOfItems(CriterionSchema) }).loose();

export const OutcomeSummaryGroupSchema = z
  .object({
    key: str,
    total: count,
    pass: count,
    partial: count,
    fail: count,
    unverified: count,
    pending: count,
  })
  .loose();

export const OutcomeSummarySchema = z
  .object({ group_by: str.default("work_class"), groups: listOfItems(OutcomeSummaryGroupSchema) })
  .loose();

export type Verification = z.infer<typeof VerificationSchema>;
export type Outcome = z.infer<typeof OutcomeSchema>;
export type Criterion = z.infer<typeof CriterionSchema>;
export type OutcomeList = { outcomes: Outcome[]; next_cursor: string };
export type OutcomeSummaryGroup = z.infer<typeof OutcomeSummaryGroupSchema>;
export type OutcomeSummary = { group_by: string; groups: OutcomeSummaryGroup[] };

// A single outcome that does not parse is null: showing an empty outcome would read as a
// real one. A list that does not parse is empty, and the page says so.
export function parseOutcome(raw: unknown, endpoint: string): Outcome | null {
  return parseWithFallback<Outcome | null>(raw, OutcomeSchema, null, { endpoint });
}

export function parseVerification(raw: unknown, endpoint: string): Verification | null {
  return parseWithFallback<Verification | null>(raw, VerificationSchema, null, { endpoint });
}

export function parseCriterion(raw: unknown, endpoint: string): Criterion | null {
  return parseWithFallback<Criterion | null>(raw, CriterionSchema, null, { endpoint });
}

export function parseCriteriaList(raw: unknown, endpoint: string): Criterion[] {
  return parseWithFallback<{ criteria: Criterion[] }>(raw, CriteriaListSchema, { criteria: [] }, { endpoint }).criteria;
}

export function parseOutcomeList(raw: unknown, endpoint: string): OutcomeList {
  return parseWithFallback<OutcomeList>(raw, OutcomeListSchema, { outcomes: [], next_cursor: "" }, { endpoint });
}

export function parseOutcomeSummary(raw: unknown, endpoint: string): OutcomeSummary {
  return parseWithFallback<OutcomeSummary>(raw, OutcomeSummarySchema, { group_by: "work_class", groups: [] }, { endpoint });
}
