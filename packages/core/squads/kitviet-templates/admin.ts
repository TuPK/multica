import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadRole, SquadTemplate } from "../templates";

/**
 * The Office Admin (Hành chính) squad of the Kit Viet pack (spec section 6.5). Names, roles, skills
 * and roster lines come from the generated kitviet-skills-data/squads.ts; this file adds the
 * squad's charter, its one-line descriptions and the role keys. Agent bodies are not imported here:
 * kitviet-packs.ts loads them when the quickstart creates the agents. Run `pnpm generate:kitviet`
 * after changing the source.
 */

export const KV_ADMIN_ROLE_KEYS = [
  "admin_lead",
  "admin_files",
  "admin_format_lookup",
  "admin_drafter",
  "admin_format_gate",
  "admin_numbers_gate",
  "admin_handover",
] as const;

export type KvAdminRoleKey = (typeof KV_ADMIN_ROLE_KEYS)[number];

/** One-line English descriptions of each agent, shown on its card. */
const DESCRIPTIONS: Record<KvAdminRoleKey, string> = {
  admin_lead:
    "Leads the Office Admin squad: plans each request, checks each handover, counts review rounds, and reports to the requester.",
  admin_files:
    "Reads the office files and pulls the basis, figures, names and dates for a document, each with its location in the source.",
  admin_format_lookup:
    "Looks up the current official rules on document form, records and holidays, with the source and its access date.",
  admin_drafter:
    "Drafts official documents, decisions, proposals, notices and regulations in the right form, from the collected facts.",
  admin_format_gate:
    "Checks a draft's form, issuing authority, citations and recipients before it goes for signature. Returns QUA or CHẶN.",
  admin_numbers_gate:
    "Checks every figure, date, name and claim in a draft against its sources. Returns QUA or CHẶN.",
  admin_handover:
    "Checks the finished product against the approved plan and every gate verdict, and lists what a person must do before it leaves.",
};

/** The role definitions of the squad, in the shape of SQUAD_ROLES entries. */
export const KV_ADMIN_ROLES: Record<KvAdminRoleKey, SquadRole> = {
  admin_lead: kvRole("admin_lead", { description: DESCRIPTIONS.admin_lead, roster: "Squad leader" }),
  admin_files: kvRole("admin_files", { description: DESCRIPTIONS.admin_files, roster: "Office file reading and the facts a document rests on" }),
  admin_format_lookup: kvRole("admin_format_lookup", { description: DESCRIPTIONS.admin_format_lookup, roster: "Official rules on document form, records and holidays" }),
  admin_drafter: kvRole("admin_drafter", { description: DESCRIPTIONS.admin_drafter, roster: "Official document drafts from the collected facts" }),
  admin_format_gate: kvRole("admin_format_gate", { description: DESCRIPTIONS.admin_format_gate, roster: "Form, issuing authority and citation check before signature" }),
  admin_numbers_gate: kvRole("admin_numbers_gate", { description: DESCRIPTIONS.admin_numbers_gate, roster: "Figure, date, name and claim check against sources" }),
  admin_handover: kvRole("admin_handover", { description: DESCRIPTIONS.admin_handover, roster: "Handover check against the plan and the gate verdicts" }),
};

const pack = kitvietSquad("kv_admin");

const CHARTER_SHARED_BOUNDARY =
  "Nothing leaves the workspace: no sending, filing, signing, posting or publishing; the person who asked does that.";

export const KV_ADMIN_TEMPLATE: SquadTemplate = {
  id: "kv_admin",
  name: "Office Admin (Hành chính)",
  description:
    "Drafts official documents, decisions and proposals in the right form, and keeps the document register. Does not number, sign or issue anything.",
  roles: [...KV_ADMIN_ROLE_KEYS],
  leader: "admin_lead",
  floorName: "Executive",
  routines: [],
  charter: {
    mission: "Draft administrative documents in the right form and keep the document registers current.",
    responsibilities: [
      {
        key: "drafting",
        text: "Draft circulars, decisions, proposals, notices and regulations from the collected facts, in the form the issuing authority requires.",
      },
      {
        key: "format",
        text: "Check each draft's form, issuing authority and citations independently before it goes for signature.",
      },
      {
        key: "registers",
        text: "Keep the incoming and outgoing document register current, one line per document.",
      },
    ],
    outputs: [
      { key: "drafts", kind: "issue", cadenceHint: "per request" },
      { key: "register", kind: "issue", cadenceHint: "per signed document" },
    ],
    boundaries: `Does not assign document numbers to unsigned documents, sign, stamp or issue. ${CHARTER_SHARED_BOUNDARY}`,
    escalation: "Signer or purpose missing: block the issue and mention the requester.",
    intake: "công văn, quyết định, tờ trình, thông báo, quy chế, sổ công văn",
  },
  instructions: kvLeadInstructions("admin_lead"),
  workflow: null,
  builtinSkills: pack.builtinSkills,
  leaderSkills: pack.leaderSkills,
  roleSkills: kvRoleSkills("kv_admin", KV_ADMIN_ROLE_KEYS),
};
