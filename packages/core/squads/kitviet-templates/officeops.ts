import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadRole, SquadTemplate } from "../templates";

/**
 * The Office Ops (Vận hành văn phòng) squad of the Kit Viet pack (spec section 9.1, P1). It has no
 * upstream pipeline: its lead flow is written in KV_LEAD_NOTES.kv_officeops, and its skills are the
 * nine operations skills that Office Admin used to defer. Names, roles and roster lines come from the
 * generated kitviet-skills-data/squads.ts; this file adds the charter, the one-line descriptions and
 * the role keys. Run `pnpm generate:kitviet` after changing the source.
 */

export const KV_OFFICEOPS_ROLE_KEYS = [
  "officeops_lead",
  "officeops_files",
  "officeops_drafter",
  "officeops_numbers_gate",
  "officeops_handover",
] as const;

export type KvOfficeOpsRoleKey = (typeof KV_OFFICEOPS_ROLE_KEYS)[number];

/** One-line English descriptions of each agent, shown on its card. */
const DESCRIPTIONS: Record<KvOfficeOpsRoleKey, string> = {
  officeops_lead:
    "Leads the Office Ops squad: picks the operations job each request needs, plans it, checks each handover, counts review rounds, and reports to the requester.",
  officeops_files:
    "Reads the office files (room lists, quotes, contracts, asset lists, stock counts) and pulls the facts an operations job rests on, each with its location in the source.",
  officeops_drafter:
    "Runs the chosen operations job from the collected facts: room and travel plans, event checklists, asset and stationery registers, archive plans, visitor scripts and monthly reports.",
  officeops_numbers_gate:
    "Checks every figure, date, name, sum, VAT line and count in a plan, quote or report against its sources. Returns QUA or CHẶN.",
  officeops_handover:
    "Checks the finished product against the approved plan and every gate verdict, and lists what a person must book, pay or send before it leaves.",
};

/** The role definitions of the squad, in the shape of SQUAD_ROLES entries. */
export const KV_OFFICEOPS_ROLES: Record<KvOfficeOpsRoleKey, SquadRole> = {
  officeops_lead: kvRole("officeops_lead", { description: DESCRIPTIONS.officeops_lead, roster: "Squad leader" }),
  officeops_files: kvRole("officeops_files", { description: DESCRIPTIONS.officeops_files, roster: "Office file reading and the facts an operations job rests on" }),
  officeops_drafter: kvRole("officeops_drafter", { description: DESCRIPTIONS.officeops_drafter, roster: "Operations plans, checklists and registers from the collected facts" }),
  officeops_numbers_gate: kvRole("officeops_numbers_gate", { description: DESCRIPTIONS.officeops_numbers_gate, roster: "Figure, sum, date, name and count check against sources" }),
  officeops_handover: kvRole("officeops_handover", { description: DESCRIPTIONS.officeops_handover, roster: "Handover check against the plan and the gate verdicts" }),
};

const pack = kitvietSquad("kv_officeops");

const CHARTER_SHARED_BOUNDARY =
  "Nothing leaves the workspace: no booking, ordering, paying, signing, sending or inviting; the person who asked does that.";

export const KV_OFFICEOPS_TEMPLATE: SquadTemplate = {
  id: "kv_officeops",
  name: "Office Ops (Vận hành văn phòng)",
  description:
    "Plans room and travel logistics, events, stationery, the asset register, archiving, service contracts and the monthly admin report. Does not book, buy, pay or send.",
  roles: [...KV_OFFICEOPS_ROLE_KEYS],
  leader: "officeops_lead",
  floorName: "Executive",
  routines: [],
  charter: {
    mission: "Run the day-to-day office operations: rooms, travel, events, stock, assets, records, contracts and the monthly report.",
    responsibilities: [
      {
        key: "logistics",
        text: "Plan room bookings, travel options and event checklists from the request, with the cost and the lead time of each option.",
      },
      {
        key: "registers",
        text: "Keep the asset register, the stationery stock and the archive plan current, one line per item or file.",
      },
      {
        key: "contracts",
        text: "Track service contracts by their end and renewal dates, and report what falls due.",
      },
      {
        key: "reporting",
        text: "Compile the monthly admin report from the month's files, with each figure traced to its source.",
      },
    ],
    outputs: [
      { key: "plans", kind: "issue", cadenceHint: "per request" },
      { key: "registers", kind: "kb_document", cadenceHint: "per month" },
      { key: "monthly_report", kind: "report", cadenceHint: "monthly" },
    ],
    boundaries: `Does not book rooms, tickets or hotels, order stock, pay suppliers, sign contracts or send notices. ${CHARTER_SHARED_BOUNDARY}`,
    escalation: "Budget ceiling or booking details missing: block the issue and mention the requester.",
    intake: "đặt phòng, đặt vé, công tác, sự kiện, hội nghị, văn phòng phẩm, tài sản, lưu trữ, hợp đồng dịch vụ, báo cáo tháng",
  },
  instructions: kvLeadInstructions("officeops_lead"),
  workflow: null,
  builtinSkills: pack.builtinSkills,
  leaderSkills: pack.leaderSkills,
  roleSkills: kvRoleSkills("kv_officeops", KV_OFFICEOPS_ROLE_KEYS),
};
