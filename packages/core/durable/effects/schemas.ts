import { z } from "zod";
import { parseWithFallback } from "../../api/schema";
import { bool, enumOr, listOfItems, nullableStr, str } from "../schema-helpers";

// Response shapes of the effect log and its policy routes (durable spec 4.4 to 4.7, server
// durable_effect_fork.go and durable_effect_policy_fork.go). An unknown class, source or
// decision reads as a safe value, so the row still shows and nothing reads as allowed. A
// policy cell is different: a cell whose class this build does not know is dropped, because
// showing it under another class would let a person change the wrong rule.

export const EFFECT_CLASSES = ["read", "write_internal", "write", "send", "share", "delete", "spend"] as const;
export const REVERSIBILITIES = ["reversible", "compensable", "irreversible"] as const;
export const DECISIONS = ["allow", "confirm", "rehearse", "block"] as const;
export const EFFECT_SOURCES = [
  "connected_app",
  "plugin_action",
  "cli",
  "channel",
  "webhook",
  "publish_channel",
  "declared",
] as const;
export const EFFECT_STATUSES = ["executed", "blocked", "awaiting", "failed", "undone", "declared"] as const;

export type EffectClass = (typeof EFFECT_CLASSES)[number];
export type Reversibility = (typeof REVERSIBILITIES)[number];
export type Decision = (typeof DECISIONS)[number];
export type EffectSource = (typeof EFFECT_SOURCES)[number];
export type EffectStatus = (typeof EFFECT_STATUSES)[number];

export const EffectSchema = z
  .object({
    id: z.string().min(1),
    created_at: str,
    source: enumOr(EFFECT_SOURCES, "connected_app"),
    target: str,
    effect_class: enumOr(EFFECT_CLASSES, "write"),
    reversibility: enumOr(REVERSIBILITIES, "irreversible"),
    decision: enumOr(DECISIONS, "block"),
    status: enumOr(EFFECT_STATUSES, "awaiting"),
    summary: str,
    task_id: nullableStr,
    agent_id: nullableStr,
    member_id: nullableStr,
    undo_available: bool,
    undo_until: nullableStr,
  })
  .loose();

export const EffectListSchema = z.object({ effects: listOfItems(EffectSchema) }).loose();

// An undo answer names the effect it took back. Anything else is not an undo, so it is null.
export const UndoResultSchema = z
  .object({ effect_id: z.string().min(1), status: z.literal("undone"), undo_event_id: str })
  .loose();

const PolicyOptionSchema = z
  .object({
    decision: z.enum(DECISIONS),
    allowed: z.boolean().catch(false),
    reason: str,
  })
  .loose();

// Strict on purpose: a cell is dropped unless its class, reversibility and default decision
// are ones this build knows.
export const PolicyCellSchema = z
  .object({
    autonomy_level: z.number().int().min(0).max(3),
    effect_class: z.enum(EFFECT_CLASSES),
    reversibility: z.enum(REVERSIBILITIES),
    default: z.enum(DECISIONS),
    override: z.enum(DECISIONS).nullable().catch(null).default(null),
    options: listOfItems(PolicyOptionSchema),
  })
  .loose();

export const PolicyGridSchema = z.object({ cells: listOfItems(PolicyCellSchema) }).loose();

export type Effect = z.infer<typeof EffectSchema>;
export type UndoResult = z.infer<typeof UndoResultSchema>;
export type PolicyOption = z.infer<typeof PolicyOptionSchema>;
export type PolicyCell = z.infer<typeof PolicyCellSchema>;

export function parseEffectList(raw: unknown, endpoint: string): Effect[] {
  return parseWithFallback<{ effects: Effect[] }>(raw, EffectListSchema, { effects: [] }, { endpoint }).effects;
}

/** An undo answer that does not parse is null: the caller must not report the effect as undone. */
export function parseUndoResult(raw: unknown, endpoint: string): UndoResult | null {
  return parseWithFallback<UndoResult | null>(raw, UndoResultSchema, null, { endpoint });
}

export function parsePolicyCell(raw: unknown, endpoint: string): PolicyCell | null {
  return parseWithFallback<PolicyCell | null>(raw, PolicyCellSchema, null, { endpoint });
}

export function parsePolicyGrid(raw: unknown, endpoint: string): PolicyCell[] {
  return parseWithFallback<{ cells: PolicyCell[] }>(raw, PolicyGridSchema, { cells: [] }, { endpoint }).cells;
}
