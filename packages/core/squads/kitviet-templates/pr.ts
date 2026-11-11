import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadCharterTemplate, SquadRole, SquadTemplate } from "../templates";

/**
 * PR & Comms (Truyền thông báo chí, spec 9.1): drafts press releases, Q&A and media content from
 * verified facts, and checks every fact, quote and legal or ethical line before a person sends
 * anything to the press. Names, skill sets and agent kinds come from the generated
 * kitviet-skills-data/squads.ts; this file adds the English descriptions, the roster lines, the
 * charter. The lead's rules are KV_LEAD_NOTES.kv_pr (kitviet-agent-instructions.ts).
 * Agent bodies are loaded by kitviet-packs.ts when the quickstart creates the agents.
 */

export const PR_ROLE_KEYS = [
  "pr_lead",
  "pr_source",
  "pr_transcript",
  "pr_writer",
  "pr_release",
  "pr_fact_gate",
  "pr_ethics_gate",
  "pr_numbers_gate",
  "pr_handover",
] as const;

export type PrRoleKey = (typeof PR_ROLE_KEYS)[number];

const role = (key: PrRoleKey, description: string, roster: string): SquadRole =>
  kvRole(key, { description, roster });

/** The squad's roles. The agents keep the names the generated pack gives them. */
export const PR_ROLES: Record<PrRoleKey, SquadRole> = {
  pr_lead: role(
    "pr_lead",
    "Leads PR & Comms: plans each release or crisis response, holds the main message for approval, checks each handover, and reports.",
    "Squad leader",
  ),
  pr_source: role(
    "pr_source",
    "Finds the sources for a story on the web and in the files: official documents, past coverage and figures, each with its level, link and access date.",
    "Source research and media monitoring",
  ),
  pr_transcript: role(
    "pr_transcript",
    "Reads interview transcripts, notes and documents from people, and pulls out direct quotes with their timestamps and speakers.",
    "Quotes and facts from interviews",
  ),
  pr_writer: role(
    "pr_writer",
    "Drafts news stories and interview articles from the collected facts, in inverted pyramid order, with the source table and gaps marked as [cần bổ sung].",
    "News article drafts",
  ),
  pr_release: role(
    "pr_release",
    "Drafts press releases, Q&A sets and crisis holding statements from the approved message, with each leader quote marked for approval.",
    "Press release, Q&A and statement drafts",
  ),
  pr_fact_gate: role(
    "pr_fact_gate",
    "Checks every fact, name, date, claim and quote in a draft against the collected sources and transcripts. Writes no drafts of its own.",
    "Fact check against the sources",
  ),
  pr_ethics_gate: role(
    "pr_ethics_gate",
    "Checks every draft for defamation, prejudging, leaked sources, private life, children, unverified persons and plagiarism. Writes no drafts of its own.",
    "Legal and ethics check",
  ),
  pr_numbers_gate: role(
    "pr_numbers_gate",
    "Recomputes every figure in a draft with a script and checks its source, year and scope. Writes no drafts of its own.",
    "Figure check",
  ),
  pr_handover: role(
    "pr_handover",
    "Compares the finished draft with the lead's plan and the gate verdicts, and lists what a person must decide before anything goes to the press.",
    "Final handover check",
  ),
};

export const PR_CHARTER: SquadCharterTemplate = {
  mission: "Produce press releases and media content from verified facts, and check every fact, quote and legal line before it reaches the press.",
  responsibilities: [
    { key: "plan", text: "Outline each release or response and hold its main message for approval before any draft is written." },
    { key: "sources", text: "Collect the source for each claim, figure and quote, with its level, link and date." },
    { key: "drafts", text: "Draft the press release, news story, Q&A or crisis statement only from the collected facts and the approved message." },
    {
      key: "gates",
      text: "Before handover, check each fact against its source, each quote against its transcript, each figure by script, and each line against the legal and ethics rules.",
    },
  ],
  outputs: [
    { key: "release_pack", kind: "issue", cadenceHint: "per release" },
    { key: "fact_report", kind: "report", cadenceHint: "per release" },
  ],
  boundaries:
    "Does not send anything to journalists, post on pages, or publish; a claim without a source and a quote without a transcript stay out of the draft. Nothing leaves the workspace: no sending, filing, signing, posting or publishing; the person who asked does that.",
  escalation:
    "Message not approved: stop at the message gate; facts missing for a crisis statement: stop and name the facts; a fact or claim blocked after two rounds: hand to the requester with the blocked lines.",
  intake:
    "Requests arrive as issues assigned to this squad. Wording people use: thông cáo, họp báo, Q&A, bài báo, phỏng vấn, tuyên bố, khủng hoảng truyền thông, kiểm chứng tin.",
};

const squad = kitvietSquad("kv_pr");

/** The PR & Comms template. Routines are none: the squad runs on requests only. */
export const KV_PR_TEMPLATE: SquadTemplate = {
  id: "kv_pr",
  name: squad.name,
  description:
    "Drafts press releases (thông cáo báo chí), Q&A and crisis holding statements (khủng hoảng truyền thông) from verified facts, and checks every fact, quote and legal line before a person sends anything to the press. Sends and publishes nothing.",
  roles: [...PR_ROLE_KEYS],
  leader: "pr_lead",
  floorName: "Launch",
  routines: [],
  operatingReview: false,
  charter: PR_CHARTER,
  instructions: kvLeadInstructions("pr_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_pr", PR_ROLE_KEYS),
};
