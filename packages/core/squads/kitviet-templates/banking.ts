import { KITVIET_P2_FLOOR_NAMES } from "../kitviet-floors";
import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadCharterTemplate, SquadRole, SquadTemplate } from "../templates";

/**
 * Banking Credit Desk (Tài chính, ngân hàng, spec 9.2): drafts the credit memo (tờ trình tín dụng) and the
 * loan analysis for one borrower from the file, with every figure traced to its source. It decides no
 * credit and gives no investment advice: a credit gate and an advice gate check the memo and any
 * customer-facing text, and a credit officer reads the result. Names, skill sets and agent kinds come from
 * the generated kitviet-skills-data/squads.ts; this file adds the English descriptions, the roster lines,
 * the charter and the lead's notes (KV_BANKING_LEAD_NOTES). Agent bodies are loaded by kitviet-packs.ts.
 */

export const KV_BANKING_ROLE_KEYS = [
  "banking_lead",
  "banking_reader",
  "banking_industry",
  "banking_analyst",
  "banking_memo_drafter",
  "banking_credit_gate",
  "banking_advice_gate",
  "banking_handover",
] as const;

export type KvBankingRoleKey = (typeof KV_BANKING_ROLE_KEYS)[number];

const role = (key: KvBankingRoleKey, description: string, roster: string): SquadRole =>
  kvRole(key, { description, roster });

/** The squad's roles. The agents keep the names the generated pack gives them. */
export const KV_BANKING_ROLES: Record<KvBankingRoleKey, SquadRole> = {
  banking_lead: role(
    "banking_lead",
    "Leads the Banking Credit Desk: plans each borrower file, stops when the file is incomplete, holds the memo for the credit officer, checks each handover, and reports.",
    "Squad leader",
  ),
  banking_reader: role(
    "banking_reader",
    "Reads the borrower's file (financial statements, plan, collateral, credit history) and lists each fact with its source, every conflict and what is missing.",
    "Borrower file facts",
  ),
  banking_industry: role(
    "banking_industry",
    "Looks up the borrower's industry and the sector's official statistics, each with its source level, link and access date.",
    "Industry research",
  ),
  banking_analyst: role(
    "banking_analyst",
    "Builds the ratio, working capital, repayment and project cash-flow tables from the reader's facts, with each formula written out and the totals checked.",
    "Ratio and cash-flow tables",
  ),
  banking_memo_drafter: role(
    "banking_memo_drafter",
    "Drafts the credit memo and any customer-facing text only from the facts and tables, with the risk and collateral sections; it fixes only the errors the gate lists.",
    "Credit memo draft",
  ),
  banking_credit_gate: role(
    "banking_credit_gate",
    "Checks the memo against the sources and tables: each figure, each formula, missing risks, conclusions beyond the officer's authority, and data exposure. Writes no memo.",
    "Credit memo check",
  ),
  banking_advice_gate: role(
    "banking_advice_gate",
    "Checks customer-facing text for unconfirmed rates and fees, promised profit, investment advice, pressure and exposed customer data. Writes no text of its own.",
    "Customer text and advice check",
  ),
  banking_handover: role(
    "banking_handover",
    "Compares the product with the lead's plan and the gate verdicts, and lists the decisions the credit officer must make before anything goes further.",
    "Final handover check",
  ),
};

export const KV_BANKING_CHARTER: SquadCharterTemplate = {
  mission:
    "Turn a borrower's file into a credit memo draft for a human credit officer, with every figure traced to its source and every risk, limit and customer line checked before the officer reads it.",
  responsibilities: [
    { key: "plan", text: "Outline each borrower file, list what is missing, and stop before drafting when the file cannot support a memo." },
    { key: "sources", text: "Record each fact with its source, level, date and any conflict between documents." },
    { key: "tables", text: "Build the ratios, working capital need, repayment schedule and cash flow in files, with each formula written out." },
    { key: "drafts", text: "Draft the credit memo and any customer text only from the collected facts and tables, with the risk and collateral sections." },
    {
      key: "gates",
      text: "Before handover, check each figure and formula against the sources, the risks against the memo, and each customer line against the advice rules.",
    },
  ],
  outputs: [
    { key: "credit_memo_draft", kind: "proposal", cadenceHint: "per borrower" },
    { key: "gate_report", kind: "report", cadenceHint: "per memo" },
  ],
  boundaries:
    "Drafts for a human credit officer. Never approves, rejects, prices or limits credit, never says that a borrower qualifies or is safe to lend to, never gives investment advice (buy or sell advice on securities, funds, bonds, gold or digital assets), and never promises a return or a profit. A figure with no source stays [cần bổ sung]; a rate, fee, limit or threshold the user did not supply stays [cần xác nhận]. Nothing leaves the workspace: no submission to the bank's credit system, no message to a customer, no signature; the credit officer does those.",
  escalation:
    "File incomplete, a conflicting figure, or no proposal: stop at human stop 1 and name the missing item. A credit memo gate still at CHẶN after two rounds: stop at human stop 3, hand over the draft with the open errors, and never change the verdict to QUA. An advice gate at CHẶN: the customer text is not handed over; the officer decides what to do with it.",
  intake:
    "Requests arrive as issues assigned to this squad. Wording people use: tờ trình tín dụng, thẩm định hồ sơ vay, phân tích BCTC khách vay, phương án vay, tài sản bảo đảm, hạn mức, tái cấp hạn mức, CIC.",
};

const squad = kitvietSquad("kv_banking");

/** The Banking Credit Desk template. Routines are none: the squad runs on requests only. */
export const KV_BANKING_TEMPLATE: SquadTemplate = {
  id: "kv_banking",
  name: squad.name,
  description:
    "Drafts the credit memo (tờ trình tín dụng) and the loan analysis for one borrower from the file, with every figure traced to its source. Credit and gate checks run before a credit officer reads it. Approves no credit, gives no investment advice, and sends nothing.",
  roles: [...KV_BANKING_ROLE_KEYS],
  leader: "banking_lead",
  floorName: KITVIET_P2_FLOOR_NAMES.banking,
  expertReview: true,
  routines: [],
  charter: KV_BANKING_CHARTER,
  instructions: kvLeadInstructions("banking_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_banking", KV_BANKING_ROLE_KEYS),
};
