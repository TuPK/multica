import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadCharterTemplate, SquadRole, SquadTemplate } from "../templates";

/**
 * Marketing Studio (spec 6.4): produces campaign content and ad copy and checks every claim
 * before a person publishes it. Names, skill sets and agent kinds come from the generated
 * kitviet-skills-data/squads.ts; this file adds the English descriptions, the roster lines, the
 * charter, and the lead's rules (kitviet-agent-instructions.ts). Agent bodies are loaded by
 * kitviet-packs.ts when the quickstart creates the agents.
 */

export const MARKETING_ROLE_KEYS = [
  "mkt_lead",
  "mkt_scout",
  "mkt_insight",
  "mkt_content",
  "mkt_ads",
  "mkt_claim_gate",
  "mkt_voice_gate",
  "mkt_numbers_gate",
  "mkt_handover",
] as const;

export type MarketingRoleKey = (typeof MARKETING_ROLE_KEYS)[number];

const role = (key: MarketingRoleKey, description: string, roster: string): SquadRole =>
  kvRole(key, { description, roster });

/** The squad's roles. The agents keep the names the generated pack gives them. */
export const MARKETING_ROLES: Record<MarketingRoleKey, SquadRole> = {
  mkt_lead: role(
    "mkt_lead",
    "Leads Marketing Studio: plans each campaign, holds its main message for approval, checks each handover, and reports.",
    "Squad leader",
  ),
  mkt_scout: role(
    "mkt_scout",
    "Reads competitors' public content and market sources, with the link and the date for each finding.",
    "Competitor and market scan on the web",
  ),
  mkt_insight: role(
    "mkt_insight",
    "Reads the comments, reviews and surveys in the files it is given, counts the themes, and quotes customers without their names.",
    "Customer voice from comments and reviews",
  ),
  mkt_content: role(
    "mkt_content",
    "Drafts the campaign calendar, posts, short video scripts and emails from the approved message, keeping internal notes apart from the sendable text.",
    "Multi-channel content drafts",
  ),
  mkt_ads: role(
    "mkt_ads",
    "Drafts ad copy variants for Meta, TikTok and Google, counted against each channel's limits, with the landing page copy.",
    "Ad copy and landing page drafts",
  ),
  mkt_claim_gate: role(
    "mkt_claim_gate",
    "Checks every draft for claims that need a source or a legal basis, against the Marketing regulation register. Writes no drafts of its own.",
    "Claim check against the advertising rules",
  ),
  mkt_voice_gate: role(
    "mkt_voice_gate",
    "Checks every draft against the brand voice playbook: forms of address, banned words, emoji, length and call to action. Writes no drafts of its own.",
    "Brand voice check",
  ),
  mkt_numbers_gate: role(
    "mkt_numbers_gate",
    "Recomputes every market figure in a draft with a script and checks its source, year and scope. Writes no drafts of its own.",
    "Market figure check",
  ),
  mkt_handover: role(
    "mkt_handover",
    "Compares the finished campaign with the lead's plan and the gate verdicts, and lists what a person must decide before anything goes out.",
    "Final handover check",
  ),
};

export const MARKETING_CHARTER: SquadCharterTemplate = {
  mission: "Produce campaign content and ad copy and check every claim before it is published.",
  responsibilities: [
    { key: "plan", text: "Outline each campaign and hold its main message for approval before any content is drafted." },
    { key: "content", text: "Draft multi-channel content and the campaign calendar from the approved message and the research." },
    { key: "ads", text: "Draft ad copy variants and landing page copy within each channel's character limits." },
    {
      key: "gates",
      text: "Before handover, check each draft's claims against the Marketing regulation register, its tone against the brand voice playbook, and its market figures against their sources.",
    },
  ],
  outputs: [
    { key: "content_pack", kind: "issue", cadenceHint: "per campaign" },
    { key: "claim_report", kind: "report", cadenceHint: "per campaign" },
  ],
  boundaries:
    "Does not post, run ads or brief creators; sensitive sectors always pass the claim gate. Nothing leaves the workspace: no sending, filing, signing, posting or publishing; the person who asked does that.",
  escalation:
    "Message not approved: stop at the message gate; claim blocked after two rounds: hand to the requester with the blocked lines.",
  intake:
    "Requests arrive as issues assigned to this squad. Wording people use: chiến dịch, bài fanpage, ad copy, landing, kịch bản, soát claim quảng cáo, minigame.",
};

const squad = kitvietSquad("kv_marketing");

/** The Marketing Studio template. Routines are none (spec Q3): the squad runs on requests only. */
export const KV_MARKETING_TEMPLATE: SquadTemplate = {
  id: "kv_marketing",
  name: squad.name,
  description:
    "Drafts campaign content and ad copy from Growth & GTM briefs, and checks every claim against the Marketing regulation register before a person publishes. Posts and runs nothing.",
  roles: [...MARKETING_ROLE_KEYS],
  leader: "mkt_lead",
  floorName: "Launch",
  routines: [],
  operatingReview: false,
  charter: MARKETING_CHARTER,
  instructions: kvLeadInstructions("mkt_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_marketing", MARKETING_ROLE_KEYS),
};
