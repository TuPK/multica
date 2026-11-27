import { z } from "zod";
import { parseWithFallback } from "../../api/schema";
import { enumOr, listOfItems, nullableStr, str } from "../schema-helpers";

// Response shapes of the disagreement jury route (durable spec 8.2 and 8.3, server
// durable_jury_panels_fork.go) and workspace jury settings. Unknown states degrade to open and
// unknown diversity degrades to none so the UI never overclaims diversity or a verdict.

export const JURY_DIVERSITIES = ["provider", "model_only", "none"] as const;
export const JURY_STATES = ["open", "unanimous", "split", "escalated", "resolved", "timed_out"] as const;
export const JURY_VOTE_RESULTS = ["pass", "partial", "fail", "unable"] as const;
export const JURY_RESOLVED_BY_TYPES = ["member", "agent"] as const;

export type JuryDiversity = (typeof JURY_DIVERSITIES)[number];
export type JuryState = (typeof JURY_STATES)[number];
export type JuryVoteResult = (typeof JURY_VOTE_RESULTS)[number];
export type JuryResolvedByType = (typeof JURY_RESOLVED_BY_TYPES)[number];

export const JuryVoteSchema = z
  .object({
    id: z.string().min(1),
    verifier_id: str,
    result: enumOr(JURY_VOTE_RESULTS, "unable"),
    note: str,
    created_at: str,
  })
  .loose();

export const JuryPanelSchema = z
  .object({
    id: z.string().min(1),
    criterion_id: str,
    juror_agent_ids: listOfItems(z.string().min(1)),
    diversity: enumOr(JURY_DIVERSITIES, "none"),
    state: enumOr(JURY_STATES, "open"),
    verdict: z.enum(JURY_VOTE_RESULTS).nullable().catch(null).default(null),
    resolved_by_type: z.enum(JURY_RESOLVED_BY_TYPES).nullable().catch(null).default(null),
    created_at: str,
    closed_at: nullableStr,
    votes: listOfItems(JuryVoteSchema),
  })
  .loose();

export const JuryPanelListSchema = z
  .object({
    panels: listOfItems(JuryPanelSchema),
  })
  .loose();

export const WorkspaceJurySettingsSchema = z
  .object({
    jury_pool: listOfItems(z.string().min(1)),
    jury_work_classes: listOfItems(z.string().min(1)),
  })
  .loose();

export type JuryVote = z.infer<typeof JuryVoteSchema>;
export type JuryPanel = z.infer<typeof JuryPanelSchema>;
export type WorkspaceJurySettings = z.infer<typeof WorkspaceJurySettingsSchema>;

const EMPTY_JURY_SETTINGS: WorkspaceJurySettings = {
  jury_pool: [],
  jury_work_classes: [],
};

/** Parses the jury panel list (`{ panels: [...] }`). Malformed items are dropped individually. */
export function parseJuryPanelList(raw: unknown, endpoint: string): JuryPanel[] {
  return parseWithFallback<{ panels: JuryPanel[] }>(
    raw,
    JuryPanelListSchema,
    { panels: [] },
    { endpoint },
  ).panels;
}

/** Parses `jury_pool` and `jury_work_classes` from `workspace.settings`. Missing or bad values read as empty. */
export function parseWorkspaceJurySettings(raw: unknown): WorkspaceJurySettings {
  return parseWithFallback<WorkspaceJurySettings>(
    raw,
    WorkspaceJurySettingsSchema,
    EMPTY_JURY_SETTINGS,
    { endpoint: "workspace.settings.jury" },
  );
}
