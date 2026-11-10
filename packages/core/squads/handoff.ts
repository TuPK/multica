import { z } from "zod";
import { parseWithFallback } from "../api/schema";

/**
 * Handoff between squads: where a squad hands the requests it has finished, and
 * how. off leaves the handoff to the boss, ask waits for the boss's approval on each
 * one, and auto runs it as soon as the squad's leader asks.
 */
export const HANDOFF_MODES = ["off", "ask", "auto"] as const;
export type HandoffMode = (typeof HANDOFF_MODES)[number];

/** Where a request stands: none (no rule applies), off, pending, handed_off or declined. */
export const HANDOFF_STATUSES = ["none", "off", "pending", "handed_off", "declined"] as const;
export type HandoffStatus = (typeof HANDOFF_STATUSES)[number];

export interface SquadHandoffRule {
  squad_id: string;
  target_squad_id: string | null;
  target_squad_name: string | null;
  mode: HandoffMode;
}

export interface SquadHandoff {
  id: string;
  source_issue_id: string;
  target_issue_id: string | null;
  target_squad_id: string;
  target_squad_name: string | null;
  mode: HandoffMode;
  /** pending, handed_off or declined. A status this client does not know shows as pending. */
  status: "pending" | "handed_off" | "declined";
  created_at: string;
  decided_at: string | null;
}

export interface IssueHandoff {
  rule: SquadHandoffRule | null;
  handoff: SquadHandoff | null;
  status: HandoffStatus;
  reason: string;
}

export interface SetSquadHandoffRuleRequest {
  /** The squad to hand to; null removes the rule. */
  target_squad_id: string | null;
  mode: HandoffMode;
}

/** A mode from a newer server shows as off, so no handoff is promised that this client cannot name. */
const modeSchema = z
  .string()
  .nullish()
  .transform((mode): HandoffMode => ((HANDOFF_MODES as readonly string[]).includes(mode ?? "") ? (mode as HandoffMode) : "off"));

const nullableString = z.string().nullish().transform((s) => s ?? null);

export const SquadHandoffRuleSchema = z
  .object({
    squad_id: z.string().default(""),
    target_squad_id: nullableString,
    target_squad_name: nullableString,
    mode: modeSchema,
  })
  .loose()
  .transform((r): SquadHandoffRule => ({ ...r }));

const SquadHandoffSchema = z
  .object({
    id: z.string().default(""),
    source_issue_id: z.string().default(""),
    target_issue_id: nullableString,
    target_squad_id: z.string().default(""),
    target_squad_name: nullableString,
    mode: modeSchema,
    status: z
      .string()
      .nullish()
      .transform((s): SquadHandoff["status"] => (s === "handed_off" || s === "declined" ? s : "pending")),
    created_at: z.string().default(""),
    decided_at: nullableString,
  })
  .loose()
  .transform((h): SquadHandoff => ({ ...h }));

export const IssueHandoffSchema = z
  .object({
    rule: SquadHandoffRuleSchema.nullish().transform((r) => r ?? null),
    handoff: SquadHandoffSchema.nullish().transform((h) => h ?? null),
    status: z
      .string()
      .nullish()
      .transform((s): HandoffStatus => ((HANDOFF_STATUSES as readonly string[]).includes(s ?? "") ? (s as HandoffStatus) : "none")),
    reason: z.string().nullish().transform((r) => r ?? ""),
  })
  .loose()
  .transform((h): IssueHandoff => ({ ...h }));

export const FALLBACK_SQUAD_HANDOFF_RULE: SquadHandoffRule = {
  squad_id: "",
  target_squad_id: null,
  target_squad_name: null,
  mode: "off",
};

export const FALLBACK_ISSUE_HANDOFF: IssueHandoff = { rule: null, handoff: null, status: "none", reason: "" };

/** Reads a squad's handoff rule; a malformed answer shows no rule. */
export function parseSquadHandoffRule(raw: unknown, endpoint: string): SquadHandoffRule {
  return parseWithFallback<SquadHandoffRule>(raw, SquadHandoffRuleSchema, FALLBACK_SQUAD_HANDOFF_RULE, { endpoint });
}

/** Reads a request's handoff state; a malformed answer shows no handoff. */
export function parseIssueHandoff(raw: unknown, endpoint: string): IssueHandoff {
  return parseWithFallback<IssueHandoff>(raw, IssueHandoffSchema, FALLBACK_ISSUE_HANDOFF, { endpoint });
}
