import { KITVIET_P2_FLOOR_NAMES } from "../kitviet-floors";
import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadCharterTemplate, SquadRole, SquadTemplate } from "../templates";

/**
 * Education Desk (Giáo dục, spec 9.2): drafts periodic tests (đề kiểm tra định kỳ) with their matrix,
 * specification, answer key and grading guide from the curriculum outcomes, and checks every answer
 * and every line a student or parent will see before a teacher prints or shares anything. Names,
 * skill sets and agent kinds come from the generated kitviet-skills-data/squads.ts; this file adds
 * the English descriptions, the roster lines, the charter. Four trade agents of giao-duc are mapped
 * here (plus the lead and the handover); the other four are in the deferred list of the proposal.
 * Agent bodies are loaded by kitviet-packs.ts when the quickstart creates the agents.
 */

export const EDU_ROLE_KEYS = [
  "edu_lead",
  "edu_curriculum",
  "edu_test_drafter",
  "edu_answer_gate",
  "edu_student_gate",
  "edu_handover",
] as const;

export type EduRoleKey = (typeof EDU_ROLE_KEYS)[number];

const role = (key: EduRoleKey, description: string, roster: string): SquadRole =>
  kvRole(key, { description, roster });

/** The squad's roles. The agents keep the names the generated pack gives them. */
export const EDU_ROLES: Record<EduRoleKey, SquadRole> = {
  edu_lead: role(
    "edu_lead",
    "Leads Education Desk: plans each test set, holds the scope and the criteria for the teacher's approval, checks each handover, and reports.",
    "Squad leader",
  ),
  edu_curriculum: role(
    "edu_curriculum",
    "Collects the learning outcomes and lesson content from the curriculum, the textbooks and the group's files, each with its file, page or link. Reads no student records.",
    "Learning outcomes and curriculum sources",
  ),
  edu_test_drafter: role(
    "edu_test_drafter",
    "Writes the test matrix, specification, test paper, answer key and grading guide from the approved parameters and outcomes, and fixes only the lines a gate reports.",
    "Test, answer key and grading draft",
  ),
  edu_answer_gate: role(
    "edu_answer_gate",
    "Solves each question on its own before it reads the key, and flags wrong keys, two correct answers, out-of-scope items and matrix mismatches. Writes no drafts of its own.",
    "Answer key check",
  ),
  edu_student_gate: role(
    "edu_student_gate",
    "Checks every text a student or parent will see for personal data, wrong channels, unsuitable content and labels, and any sign of harm to a child. Writes no drafts of its own.",
    "Student safety and data check",
  ),
  edu_handover: role(
    "edu_handover",
    "Compares the finished draft with the lead's plan and the gate verdicts, and lists what the teacher must decide before the test is printed or shared.",
    "Final handover check",
  ),
};

export const EDU_CHARTER: SquadCharterTemplate = {
  mission:
    "Draft periodic tests and their answer keys from approved parameters and curriculum outcomes, and check every answer and every student-facing line before a teacher prints or shares them.",
  responsibilities: [
    {
      key: "plan",
      text: "Outline each test set with its subject, grade, time, matrix and criteria, and hold it for the teacher's approval before any draft is written.",
    },
    {
      key: "sources",
      text: "Collect each learning outcome from the curriculum or the files the teacher gave, with its file, page or link.",
    },
    {
      key: "drafts",
      text: "Draft the matrix, specification, test paper, answer key and grading guide only from the approved parameters and the collected outcomes.",
    },
    {
      key: "gates",
      text: "Before handover, solve each question against the key without reading it first, check the matrix, and check every student-facing line for personal data and unsuitable content.",
    },
  ],
  outputs: [
    { key: "test_pack", kind: "issue", cadenceHint: "per test" },
    { key: "answer_check", kind: "report", cadenceHint: "per test" },
  ],
  boundaries:
    "Does not print, share, upload or post a test, an answer key or a grading guide, and does not contact students or parents. Student records are not processed here: a request that attaches them is blocked. The answer key stays out of the student copy. Nothing leaves the workspace: no sending, filing, signing, posting or publishing; the teacher does that.",
  escalation:
    "Parameters missing (subject, grade, time, score scale, matrix) or an outcome with no source: block the request and name what is missing; a gate still blocking after two rounds: hand to the teacher with the open lines; a sign of harm to a child in a document: stop at once and name it for the school leadership.",
  intake:
    "Requests arrive as issues assigned to this squad. Wording teachers use: ra đề, đề kiểm tra, kiểm tra định kỳ, ma trận, đặc tả, đáp án, hướng dẫn chấm, ngân hàng câu hỏi, trắc nghiệm, yêu cầu cần đạt.",
};

const squad = kitvietSquad("kv_education");

/** The Education Desk template. Routines are none: the squad runs on requests only. */
export const KV_EDU_TEMPLATE: SquadTemplate = {
  id: "kv_education",
  name: squad.name,
  description:
    "Drafts periodic tests (đề kiểm tra định kỳ) with their matrix, specification, answer key and grading guide from the curriculum outcomes. Gates check every answer and every line a student or parent will see; a teacher prints and shares, and the squad sends nothing.",
  roles: [...EDU_ROLE_KEYS],
  leader: "edu_lead",
  floorName: KITVIET_P2_FLOOR_NAMES.edu,
  routines: [],
  charter: EDU_CHARTER,
  instructions: kvLeadInstructions("edu_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_education", EDU_ROLE_KEYS),
};
