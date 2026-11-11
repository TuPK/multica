import { KITVIET_P2_FLOOR_NAMES } from "../kitviet-floors";
import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadCharterTemplate, SquadRole, SquadTemplate } from "../templates";

/**
 * Import & Export Desk (Xuất nhập khẩu, P2 trade `xuat-nhap-khau`): assembles a shipment file from its
 * invoice, packing list, transport document and certificates, cross-checks the figures, and checks the
 * customs declaration draft (HS code, value, duty rate, source of each rate) with two independent gates
 * before a person declares anything. Names, skill sets and agent kinds come from the generated
 * kitviet-skills-data/squads.ts; this file adds the English descriptions, the roster lines, the charter
 * and the floor. The squad is seated on its own floor (KITVIET_P2_FLOOR_NAMES).
 * Agent bodies are loaded by kitviet-packs.ts when the quickstart creates the agents.
 */

export const TRADE_ROLE_KEYS = [
  "trade_lead",
  "trade_docs_reader",
  "trade_customs_research",
  "trade_crosscheck_drafter",
  "trade_docs_gate",
  "trade_customs_gate",
  "trade_handover",
] as const;

export type TradeRoleKey = (typeof TRADE_ROLE_KEYS)[number];

const role = (key: TradeRoleKey, description: string, roster: string): SquadRole => kvRole(key, { description, roster });

/** The squad's roles. The agents keep the names the generated pack gives them. */
export const TRADE_ROLES: Record<TradeRoleKey, SquadRole> = {
  trade_lead: role(
    "trade_lead",
    "Leads Import & Export Desk: plans each shipment file, holds each stop for the declarant, checks each handover, and reports.",
    "Squad leader",
  ),
  trade_docs_reader: role(
    "trade_docs_reader",
    "Inventories the shipment's documents (invoice, packing list, transport document, contract, L/C, certificates) and returns each field with its file and location.",
    "Shipment documents and field extraction",
  ),
  trade_customs_research: role(
    "trade_customs_research",
    "Researches HS code candidates, duty and permit rules, and FTA origin rules for a product on official sources, with each source's level and access date.",
    "HS code and customs policy research, with sources",
  ),
  trade_crosscheck_drafter: role(
    "trade_crosscheck_drafter",
    "Drafts the cross-document check table, the header and line items of the declaration, the landed cost and the customer update from the collected facts only.",
    "Cross-check table, declaration figures and landed cost drafts",
  ),
  trade_docs_gate: role(
    "trade_docs_gate",
    "Checks the draft against the original documents field by field, recomputes the totals, and checks the C/O and the L/C terms. Writes no drafts and changes nothing.",
    "Document cross-check (gate)",
  ),
  trade_customs_gate: role(
    "trade_customs_gate",
    "Checks the declaration draft for HS codes without a basis, value and tax formulas, unconfirmed rates, goods that may need a permit, and overstated claims. Writes no drafts.",
    "Customs declaration and duty check (gate)",
  ),
  trade_handover: role(
    "trade_handover",
    "Compares the finished draft with the approved plan and both gate verdicts, and lists what the declarant must decide before anything is declared or sent.",
    "Final handover check",
  ),
};

export const TRADE_CHARTER: SquadCharterTemplate = {
  mission:
    "Assemble each import or export shipment file from its own documents, cross-check the figures, and check the customs declaration draft against its sources before a person declares anything.",
  responsibilities: [
    { key: "plan", text: "Outline each shipment file and list the documents it needs, before any cross-check or draft is written." },
    { key: "sources", text: "Collect each field of each document with its file and location; a missing document or field stays [cần bổ sung]." },
    {
      key: "research",
      text: "Look up HS code candidates, duty and permit rules, and FTA origin rules on official sources, with each source's level and access date; a rate is never stated as fact.",
    },
    {
      key: "drafts",
      text: "Draft the cross-check table, the declaration figures and the landed cost from the collected facts only, with every total computed by script.",
    },
    {
      key: "gates",
      text: "Before handover, the docs gate checks each field against the original documents and the customs gate checks each HS code, value, formula and rate basis; a Chặn finding blocks the file.",
    },
  ],
  outputs: [
    { key: "shipment_file", kind: "issue", cadenceHint: "per shipment" },
    { key: "gate_report", kind: "report", cadenceHint: "per shipment draft" },
  ],
  boundaries:
    "Does not choose the final HS code, does not state a duty rate, exchange rate or FTA eligibility as fact, and does not advise how to describe goods to lower a duty. Does not declare, transmit, sign, or submit anything to customs, a bank, a forwarder or a buyer. Nothing leaves the workspace: the declarant or the company's representative does that after the handover.",
  escalation:
    "Document or field missing: stop 1 and name it; a HS code or rate with no source after the research: stop with [cần tra biểu thuế] on that line; a file with a Chặn finding after two rounds: stop 3 with the open findings.",
  intake:
    "Wording people use: lô hàng, tờ khai, nhập khẩu, xuất khẩu, invoice, packing list, B/L, AWB, vận đơn, C/O, L/C, mã HS, thuế nhập khẩu, landed cost, Incoterms, hải quan, claim hàng hư hỏng, booking.",
};

const squad = kitvietSquad("kv_trade");

/** The Import & Export Desk template. Routines are none: the squad runs on requests only. */
export const KV_TRADE_TEMPLATE: SquadTemplate = {
  id: "kv_trade",
  name: squad.name,
  description:
    "Assembles an import or export shipment file from its documents and checks the cross-document figures and the customs declaration draft with two independent gates. Declares, submits and sends nothing.",
  roles: [...TRADE_ROLE_KEYS],
  leader: "trade_lead",
  floorName: KITVIET_P2_FLOOR_NAMES.trade,
  routines: [],
  operatingReview: false,
  charter: TRADE_CHARTER,
  instructions: kvLeadInstructions("trade_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_trade", TRADE_ROLE_KEYS),
};
