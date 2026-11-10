import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadCharterTemplate, SquadRole, SquadRoutineTemplate, SquadTemplate } from "../templates";

/**
 * The Accounting & Tax squad template (Kit Viet pack, pipeline `day-chuyen-khoa-so-thang`).
 * Names, skill sets and agent kinds come from the generated kitviet-skills-data/squads.ts; this file
 * holds what the template adds: the roles' roster text, the charter and the routine. The lead's
 * squad rules are in kitviet-agent-instructions.ts. Spec: docs/engineering/kitviet-squads.md sections
 * 5.9, 6.2 and Appendix B.
 */

export type AcctRoleKey =
  | "acct_lead"
  | "acct_collector"
  | "acct_tax_research"
  | "acct_reconciler"
  | "acct_tax_drafter"
  | "acct_voucher_gate"
  | "acct_numbers_gate"
  | "acct_tax_gate"
  | "acct_handover";

/** Desk order: the lead first, then the members in the order of the spec's role table. */
const ROLE_ORDER: readonly AcctRoleKey[] = [
  "acct_lead",
  "acct_collector",
  "acct_tax_research",
  "acct_reconciler",
  "acct_tax_drafter",
  "acct_voucher_gate",
  "acct_numbers_gate",
  "acct_tax_gate",
  "acct_handover",
];

const squad = kitvietSquad("kv_accounting");

/** A role's name is generated from its key (`acct_collector` -> `Acct Collector`). */
const desk = (key: AcctRoleKey, description: string, role: string): SquadRole => kvRole(key, { description, roster: role });

export const KV_ACCOUNTING_ROLES: Record<AcctRoleKey, SquadRole> = {
  acct_lead: desk(
    "acct_lead",
    "Leads Accounting & Tax squads: plans the month-end run, delegates the stages, checks each handover, and reports.",
    "Squad leader",
  ),
  acct_collector: desk(
    "acct_collector",
    "Lists the month's vouchers and files, and flags what is missing or unusual. Reads only.",
    "Voucher and file collection",
  ),
  acct_tax_research: desk(
    "acct_tax_research",
    "Looks up current tax, accounting and social insurance rules on official sources, with the effective date and a link for each.",
    "Tax rule lookup on official sources",
  ),
  acct_reconciler: desk(
    "acct_reconciler",
    "Builds receivable, payable, bank and input-invoice reconciliations and VAT figures from the collected vouchers, computed by script.",
    "Reconciliations and VAT figures",
  ),
  acct_tax_drafter: desk(
    "acct_tax_drafter",
    "Drafts tax explanation letters, return figures and memos for the chief accountant from reconciled numbers. Drafts only.",
    "Tax letters, return figures and memos",
  ),
  acct_voucher_gate: desk(
    "acct_voucher_gate",
    "Checks reconciliations and invoice lists against the source vouchers: duplicates, wrong tax codes, wrong dates, unmatched offsets. Writes no fixes.",
    "Voucher and invoice check (gate)",
  ),
  acct_numbers_gate: desk(
    "acct_numbers_gate",
    "Recomputes every figure, date and name in a draft against its source and returns the verdict. Writes no fixes.",
    "Figures check (gate)",
  ),
  acct_tax_gate: desk(
    "acct_tax_gate",
    "Checks tax drafts for unverified legal basis, guessed ratios, overstated claims and missing signers before anything leaves. Writes no fixes.",
    "Tax compliance check (gate)",
  ),
  acct_handover: desk(
    "acct_handover",
    "Compares the finished pack with the lead's plan and each gate verdict, and lists what the chief accountant must decide and do.",
    "Handover check",
  ),
};

const SHARED_BOUNDARY =
  "Nothing leaves the workspace: no sending, filing, signing, posting or publishing; the person who asked does that.";

const CHARTER: SquadCharterTemplate = {
  mission: "Prepare month-end reconciliations and tax drafts for the chief accountant to approve.",
  responsibilities: [
    { key: "collect", text: "List the month's vouchers and files, and flag what is missing or unusual before any reconciliation starts." },
    { key: "reconcile", text: "Reconcile receivables, payables, bank statements and input invoices, computing every figure with a script." },
    { key: "tax", text: "Prepare VAT figures, PIT and CIT figures and explanation letters for the chief accountant to check." },
    { key: "gates", text: "Have independent gates check vouchers, numbers and tax compliance before the pack is handed over." },
    {
      key: "register",
      text: "Keep the Sổ quy định - Accounting register current. The reduced 8% VAT rate is recorded with its expiry date, 31/12/2026, and is confirmed on official sources before any later return.",
    },
    {
      key: "routine",
      text: "The tax-deadline routine (Lịch hạn nộp thuế) is created paused; the owner starts it once the company profile is filled in.",
    },
  ],
  outputs: [
    { key: "closing_pack", kind: "issue", cadenceHint: "per month" },
    { key: "tax_calendar", kind: "report", cadenceHint: "weekly" },
  ],
  boundaries: [
    "Does not post to accounting software, file returns or send anything. Computes with a script, never by eye, and needs Bash and Python on the runtime.",
    SHARED_BOUNDARY,
  ].join(" "),
  escalation:
    "Missing ledgers or statements, or accounting regime unknown: block the issue; gate still blocking after two rounds: hand over with the open lines.",
  intake: "khóa sổ, đối chiếu công nợ, sao kê, hóa đơn đầu vào, tờ khai, thuế, giải trình",
};

const ROUTINE: SquadRoutineTemplate = {
  title: "Lịch hạn nộp thuế",
  issueTitleTemplate: "Lịch hạn nộp thuế, tuần {{date}}",
  cron: "0 8 * * 1",
  description:
    "Đọc Hồ sơ đơn vị và Sổ quy định - Accounting. Liệt kê hạn trong 14 ngày tới theo từng đơn vị, đầu vào còn thiếu, và dòng sổ quy định có hiệu lực kết thúc trong 60 ngày. Ghi ngày chạy ở dòng đầu. Thiếu lịch nghỉ thì ghi [chưa có lịch nghỉ], không tự lùi hạn.",
  startPaused: true,
};

export const KV_ACCOUNTING_TEMPLATE: SquadTemplate = {
  id: "kv_accounting",
  name: "Accounting & Tax (Kế toán, thuế)",
  description:
    "Month-end reconciliations, VAT and income tax figures, and tax letters. Computes by script and files nothing; needs Bash and Python on the runtime.",
  roles: [...ROLE_ORDER],
  leader: "acct_lead",
  floorName: "Governance",
  routines: [ROUTINE],
  charter: CHARTER,
  instructions: kvLeadInstructions("acct_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_accounting", ROLE_ORDER),
};
