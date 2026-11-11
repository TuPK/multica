import { KITVIET_P2_FLOOR_NAMES } from "../kitviet-floors";
import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadCharterTemplate, SquadRole, SquadTemplate } from "../templates";

/**
 * Construction (Xây dựng, spec 9.2): turns site records (diaries, weekly reports, photos, quantity
 * sheets) into site documents and quantity tables a person in the project checks. Two gates stand
 * before handover: the quantity gate (units, arithmetic, sources) and the site-safety and authority
 * gate (missed hazards, blame, acceptance and structural conclusions the squad may not make).
 * Acceptance, handover and completion minutes are drafts; nothing is signed or issued from the
 * workspace. Names, skill sets and agent kinds come from the generated kitviet-skills-data/squads.ts;
 * this file adds the English descriptions, the roster lines, the charter and the lead's rules.
 * Agent bodies are loaded by kitviet-packs.ts when the quickstart creates the agents.
 */

export const CONSTRUCTION_ROLE_KEYS = [
  "construction_lead",
  "construction_reader",
  "construction_standards",
  "construction_quantity",
  "construction_site_drafter",
  "construction_quantity_gate",
  "construction_safety_gate",
  "construction_handover",
] as const;

export type ConstructionRoleKey = (typeof CONSTRUCTION_ROLE_KEYS)[number];

const role = (key: ConstructionRoleKey, description: string, roster: string): SquadRole =>
  kvRole(key, { description, roster });

/** The squad's roles. The agents keep the names the generated pack gives them. */
export const CONSTRUCTION_ROLES: Record<ConstructionRoleKey, SquadRole> = {
  construction_lead: role(
    "construction_lead",
    "Leads Construction: plans each site report, quantity table or safety document, holds the approval points, checks each handover, and reports.",
    "Squad leader",
  ),
  construction_reader: role(
    "construction_reader",
    "Reads site notes, diaries, chat messages, photo notes and quantity files, and returns each fact with its file and line, plus gaps and conflicts.",
    "Site records and facts",
  ),
  construction_standards: role(
    "construction_standards",
    "Checks whether each cited TCVN, QCVN, decree or circular is still in force, with its number, status, official link and access date. Searches only; writes no drafts.",
    "Standards and code status",
  ),
  construction_quantity: role(
    "construction_quantity",
    "Builds quantity tables and rough estimates from the given dimensions and unit prices, with the formula on each line. A missing dimension or price is a gap, never a guess.",
    "Quantity tables and estimates",
  ),
  construction_site_drafter: role(
    "construction_site_drafter",
    "Drafts site diaries, weekly reports, incident reports, handover minutes, punch lists and photo reports from the facts table. Drafts only; nothing is signed.",
    "Site document drafts",
  ),
  construction_quantity_gate: role(
    "construction_quantity_gate",
    "Checks each unit, multiplication, total, duplicate line and figure against its source before a quantity table becomes a payment basis. Writes no drafts of its own.",
    "Quantity check against the sources",
  ),
  construction_safety_gate: role(
    "construction_safety_gate",
    "Checks drafts for missed site hazards, blame without evidence, acceptance or structural conclusions the squad may not make, and code clauses cited without a status check. Writes no drafts of its own.",
    "Site safety and authority check",
  ),
  construction_handover: role(
    "construction_handover",
    "Compares the finished draft with the lead's plan and both gate verdicts, and lists what a person must check and sign before anything leaves the workspace.",
    "Final handover check",
  ),
};

export const CONSTRUCTION_CHARTER: SquadCharterTemplate = {
  mission:
    "Turn site records into site reports, quantity tables and cost estimates that a person in the project checks, with a quantity gate and a site-safety gate before handover.",
  responsibilities: [
    { key: "plan", text: "Outline each report, quantity table or safety document with its sources and pass criteria, and hold it for approval before drafting." },
    { key: "sources", text: "Record each fact with its file and line, and each cited standard with its status and access date." },
    {
      key: "drafts",
      text: "Draft diaries, weekly reports, minutes, punch lists, quantity tables and estimates only from the collected facts; mark every missing figure as a gap.",
    },
    {
      key: "gates",
      text: "Before handover, check each quantity against its unit, arithmetic and source, and check each text for missed site hazards, blame, and conclusions only an engineer or signatory can make.",
    },
  ],
  outputs: [
    { key: "site_pack", kind: "issue", cadenceHint: "per report" },
    { key: "gate_report", kind: "report", cadenceHint: "per report" },
  ],
  boundaries:
    "Does not sign, stamp, accept work, approve a quantity for payment, certify a structure or material, or decide that a site is safe. Acceptance, handover and completion minutes stay drafts for the people who sign them. The quantity gate and the safety gate are checks, not a QS, a safety officer or a certified engineer. Nothing leaves the workspace: no sending to the owner, the supervisor, a contractor or an authority; the person who asked does that.",
  escalation:
    "Missing dimensions, unit prices or drawings that a quantity depends on: stop, block the request and name the missing items. A structural figure needed in the text: mark it for the engineer and do not calculate it. A gate still blocking after two rounds: hand to the requester with the open lines; a hazard in the records is never removed from a draft to pass a gate.",
  intake:
    "Requests arrive as issues assigned to this squad. Wording people use: báo cáo tuần, nhật ký thi công, biên bản nghiệm thu, biên bản giao ban, bảng khối lượng, dự toán sơ bộ, báo cáo sự cố, punch list, RFI, phát sinh, biện pháp thi công, hồ sơ hoàn công.",
};

const squad = kitvietSquad("kv_construction");

/** The Construction template. Routines are none: the squad runs on requests only. */
export const KV_CONSTRUCTION_TEMPLATE: SquadTemplate = {
  id: "kv_construction",
  name: squad.name,
  description:
    "Drafts site reports, diaries, minutes, punch lists and quantity tables (bảng khối lượng, dự toán sơ bộ) from site records, and checks each quantity and each safety line before a person uses them. Signs, accepts and issues nothing.",
  roles: [...CONSTRUCTION_ROLE_KEYS],
  leader: "construction_lead",
  floorName: KITVIET_P2_FLOOR_NAMES.construction,
  routines: [],
  charter: CONSTRUCTION_CHARTER,
  instructions: kvLeadInstructions("construction_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_construction", CONSTRUCTION_ROLE_KEYS),
};
