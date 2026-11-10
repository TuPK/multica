import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadRole, SquadTemplate } from "../templates";

/**
 * The B2B Proposal Desk squad of the Kit Viet pack (spec section 6.3). Names, skills and the agent
 * kinds come from the generated kitviet-skills-data/squads.ts; this file adds the roster lines, the
 * one-line descriptions, the charter and the lead's notes. Agent bodies are loaded by kitviet-packs.ts
 * when the quickstart creates the agents.
 */

const squad = kitvietSquad("kv_b2b");

/** The desk's roles in seat order: the lead first, then the members. */
export const KV_B2B_ROLE_KEYS = [
  "b2b_lead",
  "b2b_history",
  "b2b_company_lookup",
  "b2b_quote_drafter",
  "b2b_commit_gate",
  "b2b_data_gate",
  "b2b_tone_gate",
  "b2b_numbers_gate",
  "b2b_handover",
] as const;

export type KvB2bRoleKey = (typeof KV_B2B_ROLE_KEYS)[number];


export const KV_B2B_ROLES: Record<KvB2bRoleKey, SquadRole> = {
  b2b_lead: kvRole("b2b_lead", {
    description: "Leads the B2B Proposal Desk: plans the quote, delegates each check, counts the review rounds, and reports to the requester.",
    roster: "Squad leader",
  }),
  b2b_history: kvRole("b2b_history", {
    description: "Reads the customer's files and past deals, and reports each fact with its location. Writes nothing.",
    roster: "Customer and deal history",
  }),
  b2b_company_lookup: kvRole("b2b_company_lookup", {
    description: "Looks up the customer's public company record and news, with a source and a date for each fact.",
    roster: "Public company lookup",
  }),
  b2b_quote_drafter: kvRole("b2b_quote_drafter", {
    description: "Drafts the quote and proposal from the collectors' reports, with internal notes kept out of the customer copy.",
    roster: "Quote and proposal draft",
  }),
  b2b_commit_gate: kvRole("b2b_commit_gate", {
    description: "Checks prices, discounts and commercial terms against the approved frame. Writes no draft of its own.",
    roster: "Commercial commitments check",
  }),
  b2b_data_gate: kvRole("b2b_data_gate", {
    description: "Checks the draft for competitor, other-customer or personal data, and for internal notes that reached the customer copy.",
    roster: "Customer data and internal-notes check",
  }),
  b2b_tone_gate: kvRole("b2b_tone_gate", {
    description: "Checks the tone and channel of each customer-facing message: forms of address, blame, pressure, and calls to action.",
    roster: "Customer tone and channel check",
  }),
  b2b_numbers_gate: kvRole("b2b_numbers_gate", {
    description: "Checks every figure, date, name and claim in the draft against its source documents.",
    roster: "Figures and claims check",
  }),
  b2b_handover: kvRole("b2b_handover", {
    description: "Compares the finished quote pack with the lead's plan and the gate verdicts, and lists what a person must decide before it leaves.",
    roster: "Independent handover check",
  }),
};


export const KV_B2B_TEMPLATE: SquadTemplate = {
  id: "kv_b2b",
  name: "B2B Proposal Desk (Chào giá B2B)",
  description:
    "Turns a qualified B2B deal into a quote and proposal inside the approved pricing. Four checks run before a person sends anything.",
  roles: [...KV_B2B_ROLE_KEYS],
  leader: "b2b_lead",
  floorName: "Launch",
  routines: [],
  charter: {
    mission: "Turn a qualified deal into a quote and proposal that stay inside approved pricing.",
    responsibilities: [
      { key: "history", text: "Read the customer's files and past deals, with a location for each fact." },
      { key: "quote", text: "Quote with commercial terms inside the approved pricing, and keep internal notes apart from the customer copy." },
      { key: "gates", text: "Have independent gates check every commitment, data-use right, tone and figure before a person sends." },
    ],
    outputs: [
      { key: "quote_pack", kind: "issue", cadenceHint: "per deal" },
      { key: "internal_notes", kind: "report", cadenceHint: "per deal" },
    ],
    boundaries:
      "Nothing leaves the workspace: no sending, filing, signing, posting or publishing; the person who asked does that. Never discounts or extends terms beyond the approved range; never puts cost or margin in the customer copy.",
    escalation:
      "Request beyond the approved range: a proposal to the person with authority; tell the customer nothing. Missing price list or approved discount frame: block the request and mention the requester.",
    intake: "báo giá, proposal, chào giá, đàm phán, khách doanh nghiệp",
  },
  instructions: kvLeadInstructions("b2b_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_b2b", KV_B2B_ROLE_KEYS),
};
