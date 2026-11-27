import { z } from "zod";
import { parseWithFallback } from "../../api/schema";
import { bool, enumOr, str } from "../schema-helpers";

// Response shapes of the shadow run routes (durable spec 9.4 and 9.5, server
// durable_shadow_routes_fork.go). RemainingMicros of -1 means no active budget covers the seat.

export const SHADOW_STATES = ["running", "completed", "stopped"] as const;

export const SHADOW_DEFAULT_SAMPLE_RATE = 0.3;
export const SHADOW_DEFAULT_TARGET_PAIRS = 20;
export const SHADOW_MIN_TARGET_PAIRS = 5;
export const SHADOW_MAX_TARGET_PAIRS = 200;

export type ShadowState = (typeof SHADOW_STATES)[number];

export const ShadowEstimateSchema = z
  .object({
    estimate_micros: z.coerce.number().finite().catch(0).default(0),
    remaining_micros: z.coerce.number().finite().catch(-1).default(-1),
    warn: bool,
  })
  .loose();

export const ShadowRunSchema = z
  .object({
    id: z.string().min(1),
    agent_id: str,
    candidate_runtime_id: str,
    candidate_model: str,
    sample_rate: z.coerce
      .number()
      .finite()
      .catch(SHADOW_DEFAULT_SAMPLE_RATE)
      .default(SHADOW_DEFAULT_SAMPLE_RATE),
    target_pairs: z.coerce
      .number()
      .int()
      .catch(SHADOW_DEFAULT_TARGET_PAIRS)
      .default(SHADOW_DEFAULT_TARGET_PAIRS),
    auto_promote: bool,
    state: enumOr(SHADOW_STATES, "running"),
  })
  .loose();

export type ShadowEstimate = z.infer<typeof ShadowEstimateSchema>;
export type ShadowRun = z.infer<typeof ShadowRunSchema>;

const EMPTY_SHADOW_ESTIMATE: ShadowEstimate = {
  estimate_micros: 0,
  remaining_micros: -1,
  warn: false,
};

/** Parses the shadow cost estimate. Missing or bad fields fall back to zero / no budget. */
export function parseShadowEstimate(raw: unknown, endpoint: string): ShadowEstimate {
  return parseWithFallback<ShadowEstimate>(raw, ShadowEstimateSchema, EMPTY_SHADOW_ESTIMATE, {
    endpoint,
  });
}

/** Parses the created shadow run. A body without an id reads as null. */
export function parseShadowRun(raw: unknown, endpoint: string): ShadowRun | null {
  return parseWithFallback<ShadowRun | null>(raw, ShadowRunSchema, null, { endpoint });
}

/**
 * Whether starting a shadow run needs explicit budget confirmation (spec 9.5): true when the
 * server flags `warn` or when the estimate exceeds a covering budget's remaining micros.
 */
export function shadowNeedsCostConfirmation(estimate: ShadowEstimate | undefined): boolean {
  if (!estimate) return false;
  if (estimate.warn) return true;
  return estimate.remaining_micros >= 0 && estimate.estimate_micros > estimate.remaining_micros;
}
