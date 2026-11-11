import { KITVIET_P2_FLOOR_NAMES } from "../kitviet-floors";
import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadCharterTemplate, SquadRole, SquadTemplate } from "../templates";

/**
 * Healthcare (Y tế, spec 9.2): drafts administrative and educational documents for a clinic or a
 * health unit (SOPs, patient leaflets, staff training, duty rosters, medicine-expiry reports, quality
 * self-assessments) from the unit's own sources. It never diagnoses, prescribes or advises on
 * treatment, and two gates check every draft before a clinician reads it: the scope gate (clinical
 * claims beyond the source) and the privacy gate (patient identifiers). Risky: the scope gate is
 * read by a clinician before the squad is switched on (kitviet-p2-notes.md section 6, item 6).
 * Names, skill sets and agent kinds come from the generated kitviet-skills-data/squads.ts; this file
 * adds the English descriptions, the roster lines, the charter and the lead's rules.
 * Agent bodies are loaded by kitviet-packs.ts when the quickstart creates the agents.
 */

export const HEALTH_ROLE_KEYS = [
  "health_lead",
  "health_reader",
  "health_research",
  "health_regulation",
  "health_drafter",
  "health_table",
  "health_scope_gate",
  "health_privacy_gate",
  "health_handover",
] as const;

export type HealthRoleKey = (typeof HEALTH_ROLE_KEYS)[number];

const role = (key: HealthRoleKey, description: string, roster: string): SquadRole =>
  kvRole(key, { description, roster });

/** The squad's roles. The agents keep the names the generated pack gives them. */
export const HEALTH_ROLES: Record<HealthRoleKey, SquadRole> = {
  health_lead: role(
    "health_lead",
    "Leads Healthcare: plans each request, stops it when the scope is clinical advice or no clinical reviewer is named, checks each handover, and reports.",
    "Squad leader",
  ),
  health_reader: role(
    "health_reader",
    "Reads the unit's protocols, SOPs, circulars and clinical documents, and lists each point with its file and location. Copies doses and thresholds word for word.",
    "Reading the unit's own clinical and administrative documents",
  ),
  health_research: role(
    "health_research",
    "Searches medical literature on the web for a review or a quality project, keeps only level 1-2 sources, and lists each with its link, date and access date.",
    "Medical literature, level 1-2 sources",
  ),
  health_regulation: role(
    "health_regulation",
    "Finds the current health regulations on official sources, with the number, date, status and link. Points to the text; does not rule what is allowed.",
    "Current health regulations and their validity",
  ),
  health_drafter: role(
    "health_drafter",
    "Drafts SOPs, patient leaflets, training material, incident reports and appointment reminders from the collected facts, in the form each document needs.",
    "SOPs, patient leaflets, training and reminders",
  ),
  health_table: role(
    "health_table",
    "Builds duty rosters, medicine-expiry reports, checklists, self-assessment tables and survey tables from the unit's data, with the rule and the count shown.",
    "Rosters, expiry reports, checklists and survey tables",
  ),
  health_scope_gate: role(
    "health_scope_gate",
    "Checks each draft for diagnosis, prescription, dose or treatment advice, cure promises and claims beyond the source. Writes no drafts. Returns QUA or CHẶN.",
    "Clinical scope check: no diagnosis, prescription or treatment advice",
  ),
  health_privacy_gate: role(
    "health_privacy_gate",
    "Checks each draft and its sources for patient identifiers and health details tied to a person, before anything leaves the workspace. Writes no drafts. Returns QUA or CHẶN.",
    "Patient data check before anything leaves the workspace",
  ),
  health_handover: role(
    "health_handover",
    "Checks the finished draft against the approved plan and every gate verdict, and lists what a clinician or the unit must approve before it is used.",
    "Final handover check and the approval list",
  ),
};

export const HEALTH_CHARTER: SquadCharterTemplate = {
  mission:
    "Draft administrative and educational documents for a clinic or health unit from the unit's own sources, and check every clinical claim and every patient identifier before a clinician reads them.",
  responsibilities: [
    { key: "plan", text: "Outline each request and its sources. Stop when the request asks for clinical advice or when no clinical reviewer is named." },
    {
      key: "sources",
      text: "Collect each fact, dose and threshold from the unit's documents or an approved source, copied word for word with its location.",
    },
    {
      key: "drafts",
      text: "Draft SOPs, patient leaflets, training material, duty rosters, expiry reports, checklists and reminders only. A draft tells no one what to take, change or do about their own condition.",
    },
    {
      key: "gates",
      text: "Before handover, check each draft against the scope rules (no diagnosis, prescription, dose change, cure promise or claim beyond the source) and the privacy rules (no patient identifier, no photo, no case that a reader could recognise).",
    },
  ],
  outputs: [
    { key: "draft_pack", kind: "issue", cadenceHint: "per request" },
    { key: "scope_report", kind: "report", cadenceHint: "per request" },
  ],
  boundaries:
    "Writes administrative and educational drafts only. Never gives a diagnosis, prescribes or changes a medicine or dose, advises on treatment for a person, or promises a result. Patient data (names, bed, record number, phone, ID or insurance card, photos) stays in the workspace: it never enters a draft, a web search, an outside AI tool, a public post or a chat group. A clinician named by the requester approves any clinical content before a patient or staff member sees it. Nothing leaves the workspace: no sending, posting, printing for distribution, signing, submitting or prescribing; the clinician and the person who asked do that.",
  escalation:
    "Request for a diagnosis, a dose or treatment for a named person, or no named clinical reviewer: stop at type 1 and name the question. A scope or privacy gate still at CHẶN after two rounds: hand the request to the requester with the open lines. Patient identifiers found in a source: stop, name the file, and do not copy the data.",
  intake:
    "SOP, quy trình, tờ hướng dẫn, tài liệu tập huấn, phác đồ, tóm tắt hướng dẫn, lịch trực, hạn dùng thuốc, bảng kiểm, tự đánh giá, khảo sát hài lòng, báo cáo sự cố, nhắc tái khám.",
};

const squad = kitvietSquad("kv_health");

/** The Healthcare template. Routines are none: the squad runs on requests only. */
export const KV_HEALTH_TEMPLATE: SquadTemplate = {
  id: "kv_health",
  name: squad.name,
  description:
    "Drafts SOPs, patient leaflets, staff training, duty rosters and medicine-expiry reports from the unit's own sources. Writes administrative and educational drafts only: no diagnosis, prescription or treatment advice. Two gates check each draft for out-of-scope clinical claims and patient data, and a clinician approves before use. Sends and publishes nothing.",
  roles: [...HEALTH_ROLE_KEYS],
  leader: "health_lead",
  floorName: KITVIET_P2_FLOOR_NAMES.health,
  expertReview: true,
  routines: [],
  operatingReview: false,
  charter: HEALTH_CHARTER,
  instructions: kvLeadInstructions("health_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_health", HEALTH_ROLE_KEYS),
};
