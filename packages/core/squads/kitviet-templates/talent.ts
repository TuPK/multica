import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadRole, SquadTemplate } from "../templates";

/**
 * The Talent Acquisition (Tuyển dụng) squad of the Kit Viet pack (spec section 9.1). Names, roles,
 * skills and roster lines come from the generated kitviet-skills-data/squads.ts; this file adds the
 * squad's charter, its one-line descriptions and the role keys. The lead's squad rules are KV_LEAD_NOTES.kv_talent. Agent bodies
 * are not imported here: kitviet-packs.ts loads them when the quickstart creates the agents. Run
 * `pnpm generate:kitviet` after changing the source.
 */

export const KV_TALENT_ROLE_KEYS = [
  "talent_lead",
  "talent_reader",
  "talent_market",
  "talent_screener",
  "talent_drafter",
  "talent_bias_gate",
  "talent_data_gate",
  "talent_handover",
] as const;

export type KvTalentRoleKey = (typeof KV_TALENT_ROLE_KEYS)[number];

/** One-line English descriptions of each agent, shown on its card. */
const DESCRIPTIONS: Record<KvTalentRoleKey, string> = {
  talent_lead:
    "Leads the Talent Acquisition squad: plans each hiring round, checks each handover, counts review rounds, and reports to the requester.",
  talent_reader:
    "Reads the job description and the CV folder, and pulls each candidate's experience and skills with the location of each quote. Reads only; never scores.",
  talent_market:
    "Looks up public salary ranges and similar job postings for the role, with the source and its date. Receives the job title only, never candidate data.",
  talent_screener:
    "Scores each CV against the approved weighted criteria, with evidence from the CV for each point, and writes the screening table. Proposes; the hiring manager decides.",
  talent_drafter:
    "Drafts the job description, posting, interview questions with a scoring sheet, and invitation or rejection letters, from the approved sources only.",
  talent_bias_gate:
    "Checks the job description, criteria, questions, scores and letters for age, gender, marital, hometown or appearance criteria, and for scores without evidence. Returns QUA or CHẶN.",
  talent_data_gate:
    "Checks the files for leaked personal data (ID numbers, phone, address, health, salary of others) and for promises beyond the approved offer. Returns QUA or CHẶN.",
  talent_handover:
    "Checks the finished pack against the approved plan and both gate verdicts, and lists what a person must do before it leaves: send, schedule, or decide.",
};

/** The role definitions of the squad, in the shape of SQUAD_ROLES entries. */
export const KV_TALENT_ROLES: Record<KvTalentRoleKey, SquadRole> = {
  talent_lead: kvRole("talent_lead", { description: DESCRIPTIONS.talent_lead, roster: "Squad leader" }),
  talent_reader: kvRole("talent_reader", { description: DESCRIPTIONS.talent_reader, roster: "Job description and CV reading, with source locations" }),
  talent_market: kvRole("talent_market", { description: DESCRIPTIONS.talent_market, roster: "Public salary and posting research for the role title" }),
  talent_screener: kvRole("talent_screener", { description: DESCRIPTIONS.talent_screener, roster: "Weighted CV screening with evidence per point" }),
  talent_drafter: kvRole("talent_drafter", { description: DESCRIPTIONS.talent_drafter, roster: "Job posts, questions, scoring sheets and letters" }),
  talent_bias_gate: kvRole("talent_bias_gate", { description: DESCRIPTIONS.talent_bias_gate, roster: "Bias check on criteria, questions, scores and letters" }),
  talent_data_gate: kvRole("talent_data_gate", { description: DESCRIPTIONS.talent_data_gate, roster: "Personal data and offer-promise check before anything leaves" }),
  talent_handover: kvRole("talent_handover", { description: DESCRIPTIONS.talent_handover, roster: "Handover check against the plan and both gate verdicts" }),
};

const pack = kitvietSquad("kv_talent");

const CHARTER_SHARED_BOUNDARY =
  "Nothing leaves the workspace: no sending, filing, signing, posting or publishing; the person who asked does that.";

export const KV_TALENT_TEMPLATE: SquadTemplate = {
  id: "kv_talent",
  name: "Talent Acquisition (Tuyển dụng)",
  description:
    "Screens CVs against approved criteria, drafts job posts, interview questions and letters, and checks each pack for bias and personal data. Does not contact, schedule or decide on candidates.",
  roles: [...KV_TALENT_ROLE_KEYS],
  leader: "talent_lead",
  floorName: "Governance",
  routines: [],
  operatingReview: false,
  charter: {
    mission: "Screen candidates against approved criteria and prepare hiring documents, with bias and personal-data checks before anything leaves the workspace.",
    responsibilities: [
      {
        key: "reading",
        text: "Read the job description and the CV folder, and report each candidate's facts with the location of each quote.",
      },
      {
        key: "screening",
        text: "Score CVs against the approved weighted criteria, with evidence from the CV for each point; propose an interview list for the hiring manager to decide.",
      },
      {
        key: "drafting",
        text: "Draft job posts, interview questions with scoring sheets, and invitation, offer and rejection letters from the approved sources only.",
      },
      {
        key: "gates",
        text: "Check every pack for age, gender, marital, hometown or appearance criteria, for scores without evidence, and for leaked personal data or promises beyond the approved offer.",
      },
    ],
    outputs: [
      { key: "screening_pack", kind: "issue", cadenceHint: "per hiring round" },
      { key: "handover_note", kind: "report", cadenceHint: "per hiring round" },
    ],
    boundaries: `Does not contact candidates, schedule interviews, decide who is hired, or make an offer. Personal data stays in the pack the person already holds. ${CHARTER_SHARED_BOUNDARY}`,
    escalation:
      "Job description, approved criteria or candidate folder missing: block the request and mention the requester. A gate still blocking after two rounds: hand over with the open findings.",
    intake: "tuyển dụng, JD, mô tả công việc, CV, ứng viên, sàng lọc, phỏng vấn, thư mời, offer, thư từ chối",
  },
  instructions: kvLeadInstructions("talent_lead"),
  workflow: null,
  builtinSkills: pack.builtinSkills,
  leaderSkills: pack.leaderSkills,
  roleSkills: kvRoleSkills("kv_talent", KV_TALENT_ROLE_KEYS),
};
