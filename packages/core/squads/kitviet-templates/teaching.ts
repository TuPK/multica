import { KITVIET_P2_FLOOR_NAMES } from "../kitviet-floors";
import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadCharterTemplate, SquadRole, SquadTemplate } from "../templates";

/**
 * Teaching Materials (Tư liệu giảng dạy, P2 trade `giao-duc`, the second Education squad): finds the
 * sources of lesson material with their levels, turns the class records into coded summaries, and drafts
 * the class documents (comments, parent messages, meeting notes, class plans) for a teacher. Three gates
 * check every source, every figure and every line a student or parent will see before a teacher uses it.
 * It maps the four trade agents that Education Desk does not use, plus the lead and the handover, the common
 * figures gate (soat-so-lieu), and the student-safety gate that Education Desk also uses. Names, skill sets and agent kinds come from
 * the generated kitviet-skills-data/squads.ts; this file adds the English descriptions, the roster lines and
 * the charter. The lead's rules are KV_LEAD_NOTES.kv_teaching (kitviet-agent-instructions.ts). The squad
 * shares the Education floor (KITVIET_P2_FLOOR_NAMES.edu).
 * Agent bodies are loaded by kitviet-packs.ts when the quickstart creates the agents.
 */

export const TEACHING_ROLE_KEYS = [
  "tm_lead",
  "tm_sources",
  "tm_records",
  "tm_drafter",
  "tm_source_gate",
  "tm_student_gate",
  "tm_numbers_gate",
  "tm_handover",
] as const;

export type TeachingRoleKey = (typeof TEACHING_ROLE_KEYS)[number];

const role = (key: TeachingRoleKey, description: string, roster: string): SquadRole =>
  kvRole(key, { description, roster });

/** The squad's roles. The agents keep the names the generated pack gives them. */
export const TEACHING_ROLES: Record<TeachingRoleKey, SquadRole> = {
  tm_lead: role(
    "tm_lead",
    "Leads Teaching Materials: plans each request, holds each stop for the teacher, checks each handover, and reports.",
    "Squad leader",
  ),
  tm_sources: role(
    "tm_sources",
    "Collects each teaching idea, figure and example from the textbooks, the curriculum and official sources, each with its source level and access date. Reads no student records.",
    "Teaching sources, with levels",
  ),
  tm_records: role(
    "tm_records",
    "Reads the class files the teacher gives (scores, attendance, class notes), codes each student, and returns coded totals and notes with no names and no sensitive notes passed on.",
    "Class records, coded summary",
  ),
  tm_drafter: role(
    "tm_drafter",
    "Drafts class documents (comments, parent messages, meeting notes, class plans, class reports) only from the coded summary and the teacher's parameters, and fixes only the lines a gate reports.",
    "Class documents draft",
  ),
  tm_source_gate: role(
    "tm_source_gate",
    "Checks each teaching draft against its sources: each claim has a source the squad opened, the citations match, and copied text carries attribution. Writes no drafts.",
    "Source check (gate)",
  ),
  tm_student_gate: role(
    "tm_student_gate",
    "Checks every text a student or parent will see for personal data, wrong channels, unsuitable content and labels, and any sign of harm to a child. Writes no drafts.",
    "Student safety and data check (gate)",
  ),
  tm_numbers_gate: role(
    "tm_numbers_gate",
    "Checks each figure in a draft (counts, averages, absence totals) against the coded class summary and its source. Writes no drafts.",
    "Figures check (gate)",
  ),
  tm_handover: role(
    "tm_handover",
    "Compares the finished draft with the lead's plan and the gate verdicts, and lists what the teacher must decide before any text is used or sent.",
    "Final handover check",
  ),
};

export const TEACHING_CHARTER: SquadCharterTemplate = {
  mission:
    "Find teaching sources with their levels, turn class records into coded summaries, and draft class documents for a teacher, with gates on every source, figure and student-facing line before a teacher uses them.",
  responsibilities: [
    {
      key: "plan",
      text: "Outline each request with its purpose, audience, class and the sources it needs, before any search or draft is written.",
    },
    {
      key: "sources",
      text: "Collect each teaching idea, figure and example with its source, level and access date; a source the squad did not open is marked, not cited.",
    },
    {
      key: "records",
      text: "Use class records only when the request states they are anonymised or approved; return coded totals and notes with no names.",
    },
    {
      key: "drafts",
      text: "Draft class documents only from the coded summary, the collected sources and the teacher's parameters.",
    },
    {
      key: "gates",
      text: "Before handover, check each source, each figure and each student-facing line against its source; a Chặn finding or a missing document blocks the text.",
    },
  ],
  outputs: [
    { key: "teaching_pack", kind: "issue", cadenceHint: "per request" },
    { key: "gate_report", kind: "report", cadenceHint: "per draft" },
  ],
  boundaries:
    "Does not send, post, upload or publish any text, and does not contact students or parents. Student records are not processed unless the request states they are anonymised or approved. No name appears in any output; the name-to-code table stays with the teacher. Nothing leaves the workspace: the teacher reads, decides and sends.",
  escalation:
    "Parameters, sources or the approval for student records missing: block the request at stop type 1 and name what is missing; a gate still blocking after two rounds: hand to the teacher with the open lines (stop type 3); a sign of harm to a child in a document or record: stop at once and name it for the school leadership.",
  intake:
    "Requests arrive as issues assigned to this squad. Wording teachers use: tư liệu bài giảng, nguồn, số liệu cho bài, ví dụ có nguồn, nhận xét học bạ, tin nhắn phụ huynh, họp phụ huynh, hồ sơ lớp, sổ chủ nhiệm, báo cáo lớp, kiểm tra nguồn.",
};

const squad = kitvietSquad("kv_teaching");

/** The Teaching Materials template. Routines are none: the squad runs on requests only. */
export const KV_TEACHING_TEMPLATE: SquadTemplate = {
  id: "kv_teaching",
  name: squad.name,
  description:
    "Finds teaching sources with their levels, turns class records into coded summaries and drafts class documents for a teacher. Gates check every source, every figure and every line a student or parent will see; the teacher reads, sends and posts. The squad sends nothing.",
  roles: [...TEACHING_ROLE_KEYS],
  leader: "tm_lead",
  floorName: KITVIET_P2_FLOOR_NAMES.edu,
  routines: [],
  operatingReview: false,
  charter: TEACHING_CHARTER,
  instructions: kvLeadInstructions("tm_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_teaching", TEACHING_ROLE_KEYS),
};
