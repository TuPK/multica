import { KITVIET_P2_FLOOR_NAMES } from "../kitviet-floors";
import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadCharterTemplate, SquadRole, SquadTemplate } from "../templates";

/**
 * Insurance Advisory (Bảo hiểm, spec 9.2): drafts advice materials for a licensed insurance agent to
 * review: needs analysis, benefit comparison, exclusion explanations, advice scripts, claim checklists
 * and guidance letters. It binds no coverage, promises no claim outcome or return, and sends nothing.
 * The advice-commitment gate checks every draft before handover. Names, skill sets and agent kinds come
 * from the generated kitviet-skills-data/squads.ts; this file adds the English descriptions, the roster
 * lines, the charter. Agent bodies are loaded by kitviet-packs.ts when the quickstart creates the agents.
 */

export const INSURANCE_ROLE_KEYS = [
  "insurance_lead",
  "insurance_gather",
  "insurance_claim_reader",
  "insurance_proposal",
  "insurance_claim_drafter",
  "insurance_advice_gate",
  "insurance_handover",
] as const;

export type InsuranceRoleKey = (typeof INSURANCE_ROLE_KEYS)[number];

const role = (key: InsuranceRoleKey, description: string, roster: string): SquadRole =>
  kvRole(key, { description, roster });

/** The squad's roles. The agents keep the names the generated pack gives them. */
export const INSURANCE_ROLES: Record<InsuranceRoleKey, SquadRole> = {
  insurance_lead: role(
    "insurance_lead",
    "Leads Insurance Advisory: plans each client file, holds the scope for the licensed agent's approval, checks each handover, and reports.",
    "Squad leader",
  ),
  insurance_gather: role(
    "insurance_gather",
    "Reads the client file and the product documents, and pulls out client facts, benefits, exclusions, waiting periods and fees, each with its document and page.",
    "Client facts and product terms",
  ),
  insurance_claim_reader: role(
    "insurance_claim_reader",
    "Checks a claim folder against the claim procedure in the policy terms: documents held, missing or needing correction, and the filing deadline.",
    "Claim file inventory",
  ),
  insurance_proposal: role(
    "insurance_proposal",
    "Drafts the needs analysis, benefit comparison, exclusion explanations and advice script from the gathered facts, with gaps marked. Chooses no product for the client.",
    "Advice drafts for the licensed agent",
  ),
  insurance_claim_drafter: role(
    "insurance_claim_drafter",
    "Drafts the claim checklist and the guidance letter to the client from the inventory. Promises no outcome and sends nothing.",
    "Claim checklist and guidance letter drafts",
  ),
  insurance_advice_gate: role(
    "insurance_advice_gate",
    "Checks every draft for profit or return promises, benefit figures not in the documents, unfair comparisons, hidden exclusions, pressure and false claims. Writes no drafts of its own.",
    "Advice commitment check",
  ),
  insurance_handover: role(
    "insurance_handover",
    "Compares the finished draft with the lead's plan and the gate verdict, and lists what the licensed agent must decide before anything reaches the client.",
    "Final handover check",
  ),
};

export const INSURANCE_CHARTER: SquadCharterTemplate = {
  mission:
    "Draft insurance advice materials (needs analysis, benefit comparison, exclusion explanations, claim checklists and guidance letters) for a licensed insurance agent to review, and check every promise and figure against the product documents before the agent uses them.",
  responsibilities: [
    { key: "plan", text: "Outline each client file and its scope before drafting; the licensed agent approves the scope." },
    { key: "facts", text: "Collect the client facts and each product benefit, exclusion, waiting period and fee, with its document and page." },
    {
      key: "drafts",
      text: "Draft the needs analysis, comparison, exclusion explanation, advice script, claim checklist and guidance letter only from the collected facts and the product documents.",
    },
    {
      key: "gates",
      text: "Before handover, check each benefit figure against its document and each sentence against the advice commitment rules; a CHẶN verdict blocks handover.",
    },
  ],
  outputs: [
    { key: "advice_pack", kind: "proposal", cadenceHint: "per client" },
    { key: "claim_checklist", kind: "issue", cadenceHint: "per claim" },
    { key: "gate_report", kind: "report", cadenceHint: "per client" },
  ],
  boundaries:
    "Drafts only, for a licensed insurance agent to review, adapt and present to the client. The squad never binds coverage, never issues a policy, never accepts an application or a claim, and never promises a claim outcome, a payout amount, a return, a guaranteed rate or a benefit the documents do not state. Illustrations are labelled as illustrations, and a benefit with no document page stays out of the draft. Nothing leaves the workspace: no sending to clients, no filing with an insurer, no signing, no payment, no posting; the licensed agent does that.",
  escalation:
    "Client facts or product documents missing or contradicting each other: stop at type 1 and name them; a choice of product or benefit that belongs to the client: stop and name the decision for the agent; a gate still at CHẶN after two rounds: hand to the agent with the open findings; a question about current insurance sales-conduct rules or an insurer's terms: flag it for the agent's check, never decide it.",
  intake:
    "Requests arrive as issues assigned to this squad. Wording people use: đề xuất bảo hiểm, tư vấn bảo hiểm, so sánh quyền lợi, loại trừ, thời gian chờ, hồ sơ bồi thường, thư hướng dẫn claim, kịch bản tư vấn.",
};

const squad = kitvietSquad("kv_insurance");

/** The Insurance Advisory template. Routines are none: the squad runs on requests only. */
export const KV_INSURANCE_TEMPLATE: SquadTemplate = {
  id: "kv_insurance",
  name: squad.name,
  description:
    "Drafts insurance advice materials (đề xuất bảo hiểm): needs analysis, benefit comparison, exclusion explanations, claim checklists (hồ sơ bồi thường) and guidance letters, for a licensed agent to review. Binds no coverage, promises no claim outcome or return, and sends nothing.",
  roles: [...INSURANCE_ROLE_KEYS],
  leader: "insurance_lead",
  floorName: KITVIET_P2_FLOOR_NAMES.insurance,
  routines: [],
  charter: INSURANCE_CHARTER,
  instructions: kvLeadInstructions("insurance_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_insurance", INSURANCE_ROLE_KEYS),
};
