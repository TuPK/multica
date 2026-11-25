import { z } from "zod";
import { parseWithFallback } from "../api/schema";

// The weekly attention budget (durable spec 7.3, 7.5): the minutes the people who decide may
// spend on questions each week, and how the live squads share them. Enabled mirrors the
// durable_principal flag; off, there is no budget to show.
export const ATTENTION_SPLITS = ["equal", "goals"] as const;
export type AttentionSplit = (typeof ATTENTION_SPLITS)[number];

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

const attentionBudgetSchema = z.object({
  enabled: z.boolean().catch(false).default(false),
  minutes_per_week: z.number().int().nonnegative().catch(60).default(60),
  split: z.enum(ATTENTION_SPLITS).catch("equal").default("equal"),
  week_start: z.string().catch("").default(""),
  spent_minutes: z.number().nonnegative().catch(0).default(0),
});

export const FALLBACK_ATTENTION_BUDGET: AttentionBudget = {
  enabled: false,
  minutes_per_week: 60,
  split: "equal",
  week_start: "",
  spent_minutes: 0,
};

export function parseAttentionBudget(raw: unknown): AttentionBudget {
  return parseWithFallback<AttentionBudget>(raw, attentionBudgetSchema, FALLBACK_ATTENTION_BUDGET, {
    endpoint: "GET /api/company/attention",
  });
}
