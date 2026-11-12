import { catalogRoleInstructions } from "../catalog/role-instructions";

/**
 * The Content Studio squad (docs/engineering/content-studio-squad.md, phases 1 to 3).
 *
 * A content squad drafts items for two markets, runs them through three gate
 * agents that are separate from the drafter, and files one `publish` proposal per
 * item. The boss approves it in the decision queue; the server then publishes each
 * approved channel version through Ayrshare. No role here posts to a network.
 *
 * Each role's agent starts from the closest existing instruction template, and
 * the role's own instructions replace it (see instructionsFor in quickstart.ts).
 * The role skills in the spec are not built as skill bundles: the instructions
 * carry the procedure for this repository.
 */

export type CsRoleKey =
  | "cs_lead"
  | "cs_scout"
  | "cs_researcher"
  | "cs_drafter"
  | "cs_localizer"
  | "cs_claim_gate"
  | "cs_market_gate"
  | "cs_voice_gate"
  | "cs_handover"
  | "cs_publisher";

/** The roles in roster order. There is no producer: video is a later phase. */
export const CS_ROLE_KEYS: readonly CsRoleKey[] = [
  "cs_lead",
  "cs_scout",
  "cs_researcher",
  "cs_drafter",
  "cs_localizer",
  "cs_claim_gate",
  "cs_market_gate",
  "cs_voice_gate",
  "cs_handover",
  "cs_publisher",
];

/** The instructions each Content Studio role's agent follows (catalog/squads/content_studio/roles). */
export const CONTENT_STUDIO_ROLE_AGENT_INSTRUCTIONS: Record<CsRoleKey, string> = catalogRoleInstructions(
  ["content_studio"],
  CS_ROLE_KEYS,
);
