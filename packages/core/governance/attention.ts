import { z } from "zod";
import { parseWithFallback } from "../api/schema";

// The weekly attention budget (durable spec 7.3, 7.5): the minutes the people who decide may
// spend on questions each week, and how the live squads share them. Enabled mirrors the
// durable_principal flag; off, there is no budget to show.
export const ATTENTION_SPLITS = ["equal", "goals"] as const;
export type AttentionSplit = (typeof ATTENTION_SPLITS)[number];

export const ATTENTION_SUBJECT_TYPES = [
  "proposal",
  "effect_intent",
  "outcome_check",
  "app_action_request",
  "inquiry",
  "unknown",
] as const;
export type AttentionSubjectType = (typeof ATTENTION_SUBJECT_TYPES)[number];

export const ATTENTION_REASONS = [
  "judgment",
  "irreversible",
  "missing_access",
  "missing_info",
  "policy",
  "verification",
  "unknown",
] as const;
export type AttentionReason = (typeof ATTENTION_REASONS)[number];

export interface AttentionBudget {
  enabled: boolean;
  minutes_per_week: number;
  split: AttentionSplit;
  /** The Monday (YYYY-MM-DD, UTC) the spend is counted from. */
  week_start: string;
  spent_minutes: number;
}

export interface AttentionBudgetInput {
  minutes_per_week: number;
  split: AttentionSplit;
}

export interface QueuedAttentionAsk {
  subject_type: AttentionSubjectType;
  subject_id: string;
  reason: AttentionReason;
  estimated_minutes: number;
  created_at: string;
}

export interface QueuedAttentionList {
  asks: QueuedAttentionAsk[];
}

const attentionBudgetSchema = z.object({
  enabled: z.boolean().catch(false).default(false),
  minutes_per_week: z.number().int().nonnegative().catch(60).default(60),
  split: z.enum(ATTENTION_SPLITS).catch("equal").default("equal"),
  week_start: z.string().catch("").default(""),
  spent_minutes: z.number().nonnegative().catch(0).default(0),
});

export const QueuedAttentionAskSchema = z
  .object({
    subject_type: z.enum(ATTENTION_SUBJECT_TYPES).catch("unknown").default("unknown"),
    subject_id: z.string().min(1),
    reason: z.enum(ATTENTION_REASONS).catch("unknown").default("unknown"),
    estimated_minutes: z.number().finite().nonnegative().catch(0).default(0),
    created_at: z.string().catch("").default(""),
  })
  .loose();

export const QueuedAttentionListSchema = z
  .object({
    asks: z.unknown().transform((value): QueuedAttentionAsk[] =>
      Array.isArray(value)
        ? value.flatMap((item) => {
            const parsed = QueuedAttentionAskSchema.safeParse(item);
            return parsed.success ? [parsed.data] : [];
          })
        : [],
    ),
  })
  .loose();

export const FALLBACK_ATTENTION_BUDGET: AttentionBudget = {
  enabled: false,
  minutes_per_week: 60,
  split: "equal",
  week_start: "",
  spent_minutes: 0,
};

export const EMPTY_QUEUED_ATTENTION: QueuedAttentionList = {
  asks: [],
};

export function parseAttentionBudget(raw: unknown): AttentionBudget {
  return parseWithFallback<AttentionBudget>(raw, attentionBudgetSchema, FALLBACK_ATTENTION_BUDGET, {
    endpoint: "GET /api/company/attention",
  });
}

export function parseQueuedAttention(raw: unknown): QueuedAttentionList {
  return parseWithFallback<QueuedAttentionList>(raw, QueuedAttentionListSchema, EMPTY_QUEUED_ATTENTION, {
    endpoint: "GET /api/company/attention/queued",
  });
}
