import { KITVIET_P2_FLOOR_NAMES } from "../kitviet-floors";
import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadCharterTemplate, SquadRole, SquadTemplate } from "../templates";

/**
 * Factory Ops (Sản xuất nhà máy, spec 9.2, P2): drafts the monthly production and quality KPI report,
 * shift and maintenance reports, and NCR, CAPA and incident drafts from the plant's own records, and
 * checks every figure and every safety line before a person issues anything. It runs no machine, changes
 * no process setting and issues no official record. Names, skill sets and agent kinds come from the
 * generated kitviet-skills-data/squads.ts; this file adds the English descriptions, the roster lines, the
 * charter. The lead's rules are FACTORY_LEAD_NOTES (move them to KV_LEAD_NOTES.kv_factory in
 * kitviet-agent-instructions.ts). Agent bodies are loaded by kitviet-packs.ts when the quickstart creates the agents.
 */

export const FACTORY_ROLE_KEYS = [
  "factory_lead",
  "factory_reader",
  "factory_standards",
  "factory_kpi_drafter",
  "factory_doc_drafter",
  "factory_numbers_gate",
  "factory_safety_gate",
  "factory_handover",
] as const;

export type FactoryRoleKey = (typeof FACTORY_ROLE_KEYS)[number];

/** One-line English descriptions of each agent, shown on its card. */
const DESCRIPTIONS: Record<FactoryRoleKey, string> = {
  factory_lead:
    "Leads Factory Ops: plans each report or record, holds the scope and safety stops, checks each handover, and reports to the requester.",
  factory_reader:
    "Reads the shift reports, output and downtime files, NCR and incident logs and IQC sheets, and lists each fact with its source, missing shifts and conflicts.",
  factory_standards:
    "Looks up the standards, regulations and customer specifications a report relies on, on official sources first, with the source and its access date.",
  factory_kpi_drafter:
    "Builds the KPI, OEE and Pareto tables from the collected facts, with the formula and source of each figure.",
  factory_doc_drafter:
    "Drafts the monthly commentary, NCR, CAPA and incident records from the collected facts, with gaps marked. Drafts only: nothing is signed or numbered.",
  factory_numbers_gate:
    "Recomputes each KPI and total, checks units, definitions and sources, and returns QUA or CHẶN. Writes no drafts of its own.",
  factory_safety_gate:
    "Checks that every safety event in the sources appears in the draft, unsoftened and without blame, and that no safety value or authority claim is invented. Returns QUA or CHẶN.",
  factory_handover:
    "Compares the finished draft with the plan and the gate verdicts, and lists what a person must decide before any record is issued.",
};

const role = (key: FactoryRoleKey, roster: string): SquadRole => kvRole(key, { description: DESCRIPTIONS[key], roster });

/** The squad's roles. The agents keep the names the generated pack gives them. */
export const FACTORY_ROLES: Record<FactoryRoleKey, SquadRole> = {
  factory_lead: role("factory_lead", "Squad leader"),
  factory_reader: role("factory_reader", "Plant records and shift files: facts and their sources"),
  factory_standards: role("factory_standards", "Standards, regulations and specifications on official sources"),
  factory_kpi_drafter: role("factory_kpi_drafter", "KPI, OEE and Pareto tables with formulas"),
  factory_doc_drafter: role("factory_doc_drafter", "Commentary, NCR, CAPA and incident drafts"),
  factory_numbers_gate: role("factory_numbers_gate", "Figure, formula and source check"),
  factory_safety_gate: role("factory_safety_gate", "Safety event and authority check"),
  factory_handover: role("factory_handover", "Handover check against the plan and the gate verdicts"),
};

export const FACTORY_CHARTER: SquadCharterTemplate = {
  mission:
    "Produce production and quality reports and record drafts from the plant's own records, and check every figure and every safety line before a person issues them. The squad runs no machine and issues no official record.",
  responsibilities: [
    { key: "plan", text: "Outline each report or record, its KPI definitions and its targets before any draft is written." },
    { key: "sources", text: "Collect each figure with its file and row, list missing shifts and conflicting counts, and record every safety event in the sources." },
    {
      key: "drafts",
      text: "Draft the KPI tables, commentary, NCR, CAPA and incident records only from the collected facts. A gap stays marked as [cần bổ sung].",
    },
    {
      key: "gates",
      text: "Before handover, recompute each KPI and total, check each units and definitions line, and check that every safety event in the sources appears unsoftened in the draft.",
    },
  ],
  outputs: [
    { key: "kpi_draft", kind: "issue", cadenceHint: "per month" },
    { key: "record_draft", kind: "issue", cadenceHint: "per NCR, CAPA or incident" },
    { key: "fact_report", kind: "report", cadenceHint: "per report" },
  ],
  boundaries:
    "Does not operate, start, stop or adjust any machine, and does not set or suggest a process value (temperature, pressure, speed, time, tolerance, inspection or maintenance interval) without an approved source. Does not disposition a lot, close an NCR or CAPA, approve a work instruction, or report to any authority. Signatures, document numbers and approval dates stay [chưa ký] or [cần bổ sung] until a person adds them. Nothing leaves the workspace: no sending, filing, signing, posting or publishing; the person who asked does that.",
  escalation:
    "Definition, target or shift data missing: stop at type 1 and name the missing item. A request to run or change a machine, to soften or leave out a safety event, or to record a closure or approval: stop, name the line, and hand it to the requester. A gate still at CHẶN after two rounds: hand to the requester with the open lines.",
  intake:
    "báo cáo KPI tháng, OEE, NCR, CAPA, báo cáo ca, báo cáo sự cố, tai nạn, suýt tai nạn, WI, SOP, ISO 9001, 5S, IQC, bảo trì phòng ngừa",
};

const squad = kitvietSquad("kv_factory");

/** The Factory Ops template. Routines are none: the squad runs on requests only. */
export const KV_FACTORY_TEMPLATE: SquadTemplate = {
  id: "kv_factory",
  name: squad.name,
  description:
    "Drafts the monthly production and quality KPI report, shift and maintenance reports, and NCR, CAPA and incident drafts from the plant's records, and checks every figure and safety line before a person issues anything. Runs no machine and issues no official record.",
  roles: [...FACTORY_ROLE_KEYS],
  leader: "factory_lead",
  floorName: KITVIET_P2_FLOOR_NAMES.factory,
  expertReview: true,
  routines: [],
  operatingReview: false,
  charter: FACTORY_CHARTER,
  instructions: kvLeadInstructions("factory_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_factory", FACTORY_ROLE_KEYS),
};
