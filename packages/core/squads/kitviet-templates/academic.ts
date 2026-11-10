import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadRole, SquadTemplate } from "../templates";

/**
 * The Academic Evidence Desk (Học thuật, nghiên cứu) squad from the Kit Việt pack (spec section 9.1,
 * trade hoc-thuat-nghien-cuu). The skill lists, role names and agent kinds come from the generated
 * kitviet-skills-data/squads.ts; this file adds the squad's roles, charter and the one-line
 * descriptions. Agent bodies are loaded by kitviet-packs.ts when the quickstart creates the agents.
 *
 * The key gate is the citation gate (acad_citation_gate, skill kiem-tra-trich-dan-co-that): a
 * reference that no agent has opened is never reported as real.
 */

export const ACADEMIC_ROLE_KEYS = [
  "acad_lead",
  "acad_reader",
  "acad_finder",
  "acad_matrix",
  "acad_drafter",
  "acad_citation_gate",
  "acad_integrity_gate",
  "acad_handover",
] as const;

export type AcademicRoleKey = (typeof ACADEMIC_ROLE_KEYS)[number];

const squad = kitvietSquad("kv_academic");

/** Each role's name and kind come from the generated data; the description and roster line are written here. */
const roleOf = (key: AcademicRoleKey, description: string, role: string): SquadRole => kvRole(key, { description, roster: role });

export const ACADEMIC_ROLES: Record<AcademicRoleKey, SquadRole> = {
  acad_lead: roleOf(
    "acad_lead",
    "Leads the Academic Evidence Desk: plans each chapter or paper run, delegates by sub-issue, checks each handover and reports to the student or researcher.",
    "Squad leader",
  ),
  acad_reader: roleOf(
    "acad_reader",
    "Reads the PDFs in the reference folder and reports each bibliographic fact, method, result and quotation with its page.",
    "PDF reading with page locations",
  ),
  acad_finder: roleOf(
    "acad_finder",
    "Searches the web for academic sources and opens each DOI or journal page to confirm it. Reports only what it has opened, with the link and the date.",
    "Source search and verification on the web",
  ),
  acad_matrix: roleOf(
    "acad_matrix",
    "Builds the source matrix (CSV) from the reader's and finder's reports: one row per source, with its source level and verification status.",
    "Source matrix",
  ),
  acad_drafter: roleOf(
    "acad_drafter",
    "Writes a study draft of the outline and literature section from the reader's facts only, marking every gap and every passage the author must rewrite.",
    "Study draft (not a submission)",
  ),
  acad_citation_gate: roleOf(
    "acad_citation_gate",
    "Checks every citation in the draft against the collected facts and the PDFs: no invented source, correct year, title and wording. Writes no draft of its own.",
    "Citation check",
  ),
  acad_integrity_gate: roleOf(
    "acad_integrity_gate",
    "Checks the draft for copied passages, claims without a source, unsupported results and a missing AI-use declaration. Writes no draft of its own.",
    "Academic integrity check",
  ),
  acad_handover: roleOf(
    "acad_handover",
    "Compares the study draft with the lead's plan and the gate verdicts, and lists what the author must rewrite, check or decide before the draft is used.",
    "Handover check",
  ),
};

export const KV_ACADEMIC_TEMPLATE: SquadTemplate = {
  id: "kv_academic",
  name: squad.name,
  description:
    "Turns a folder of PDFs and a reference list into a source matrix and a study draft that the author rewrites. Every citation is opened before it is reported as real; gaps stay marked.",
  roles: [...ACADEMIC_ROLE_KEYS],
  leader: "acad_lead",
  floorName: "Research",
  routines: [],
  charter: {
    mission:
      "Check every source behind a thesis chapter, literature review or paper, and prepare study drafts that the author rewrites in their own words, with every citation either opened or marked unverified.",
    responsibilities: [
      { key: "reading", text: "Read the PDFs in the reference folder and report each fact, method, result and quotation with its page." },
      { key: "finding", text: "Search for sources on the web only when the runtime allows it, and confirm each one by opening its DOI or journal page." },
      { key: "matrix", text: "Keep the source matrix: one row per source, with its source level and verification status." },
      { key: "drafting", text: "Write study drafts of the outline and literature section from the collected facts, with gaps marked." },
      { key: "gates", text: "Have independent gates check every citation and the integrity of each draft before anything is handed over." },
    ],
    outputs: [
      { key: "chapter_draft", kind: "issue", cadenceHint: "per chapter" },
      { key: "source_matrix", kind: "report", cadenceHint: "per project, updated per run" },
      { key: "citation_report", kind: "report", cadenceHint: "per chapter" },
    ],
    boundaries:
      "Writes no graded work for a student to submit as their own; every draft is labelled for the author to rewrite. Never reports a reference as real unless an agent opened it; unopened references are marked [chưa xác minh]. Gives no verdict on the author's integrity and does not replace the school's plagiarism check.",
    escalation:
      "Missing reference folder, outline or the school's rule on AI use: stop type 1, block the issue and mention the requester. No web tools on a source-finding role: stop type 1 and ask for the exact text of each source. A citation or integrity gate still blocking after two rounds, or any fabricated-source finding: mark [CẦN NGƯỜI HƯỚNG DẪN] and hand the issue to the requester, named on the request.",
    intake:
      "Requests arrive as issues assigned to this squad. Keywords: luận văn, khóa luận, bài báo, tổng quan tài liệu, trích dẫn, tài liệu tham khảo, abstract, đề cương nghiên cứu, kiểm tra DOI.",
  },
  instructions: kvLeadInstructions("acad_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_academic", ACADEMIC_ROLE_KEYS),
};
