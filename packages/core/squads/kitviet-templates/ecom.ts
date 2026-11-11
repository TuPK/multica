import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadRole, SquadTemplate } from "../templates";

/**
 * The E-commerce Shop Ops squad of the Kit Viet pack (spec 9.1, P1). Names, skills and the agent
 * kinds come from the generated kitviet-skills-data/squads.ts; this file adds the roster lines, the
 * one-line descriptions and the charter. The lead's notes are KV_LEAD_NOTES.kv_ecom. Agent bodies are
 * loaded by kitviet-packs.ts when the quickstart creates the agents.
 */

const squad = kitvietSquad("kv_ecom");

/** The desk's roles in seat order: the lead first, then the members. */
export const KV_ECOM_ROLE_KEYS = [
  "ecom_lead",
  "ecom_intake",
  "ecom_rival_scout",
  "ecom_listing_drafter",
  "ecom_cskh_drafter",
  "ecom_policy_gate",
  "ecom_margin_gate",
  "ecom_numbers_gate",
  "ecom_handover",
] as const;

export type KvEcomRoleKey = (typeof KV_ECOM_ROLE_KEYS)[number];

export const KV_ECOM_ROLES: Record<KvEcomRoleKey, SquadRole> = {
  ecom_lead: kvRole("ecom_lead", {
    description: "Leads the E-commerce Shop Ops squad: plans the listing or report, delegates each check, counts the review rounds, and reports to the shop owner.",
    roster: "Squad leader",
  }),
  ecom_intake: kvRole("ecom_intake", {
    description: "Reads the marketplace order exports, the shop's supplier sheets and cost file, and reports each figure with its location. Writes nothing.",
    roster: "Order and cost data intake",
  }),
  ecom_rival_scout: kvRole("ecom_rival_scout", {
    description: "Looks up public rival listings on the marketplaces: price, offers, titles and buyer complaints, with a link and a date for each fact.",
    roster: "Marketplace rival scan",
  }),
  ecom_listing_drafter: kvRole("ecom_listing_drafter", {
    description: "Drafts the listing from the intake report: titles, description, variant names and the price table, using only the cost and fee inputs the owner confirmed.",
    roster: "Listing and price draft",
  }),
  ecom_cskh_drafter: kvRole("ecom_cskh_drafter", {
    description: "Drafts after-sale and buyer messages (delivery, reviews, returns, inbox replies) in the shop's voice, with no promise the shop has not approved.",
    roster: "After-sale and buyer message draft",
  }),
  ecom_policy_gate: kvRole("ecom_policy_gate", {
    description: "Checks listing, captions and live scripts for banned words, unproven claims, missing certificates and facts that differ from the supplier sheet. Writes no draft of its own.",
    roster: "Banned words and claims check",
  }),
  ecom_margin_gate: kvRole("ecom_margin_gate", {
    description: "Checks the price table: every cost and fee is included, each has a source, and the price clears break-even and the target margin. Writes no draft of its own.",
    roster: "Price and margin check",
  }),
  ecom_numbers_gate: kvRole("ecom_numbers_gate", {
    description: "Checks every figure, date, SKU and name in the drafts against their source sheets.",
    roster: "Figures check",
  }),
  ecom_handover: kvRole("ecom_handover", {
    description: "Compares the finished listing pack with the lead's plan and the gate verdicts, and lists what the shop owner must decide before posting.",
    roster: "Independent handover check",
  }),
};

/** The floor this squad is seated on: the one default floor the Kit Viet pack creates (spec 9.1, Q4). */
export const KV_ECOM_FLOOR_NAME = "Revenue";

export const KV_ECOM_TEMPLATE: SquadTemplate = {
  id: "kv_ecom",
  name: "E-commerce Shop Ops (Bán hàng online)",
  description:
    "Takes a new product from supplier data to a marketplace listing and price, checked for banned claims and margin before the shop owner posts it.",
  roles: [...KV_ECOM_ROLE_KEYS],
  leader: "ecom_lead",
  floorName: KV_ECOM_FLOOR_NAME,
  routines: [],
  operatingReview: false,
  charter: {
    mission: "Turn a product's supplier data into a listing and a price that clear the policy and margin checks, ready for the shop owner to post.",
    responsibilities: [
      { key: "intake", text: "Read the order exports, supplier sheets and cost file, with a location for each figure." },
      { key: "listing", text: "Draft titles, descriptions and the price table from confirmed costs and fees, and draft buyer messages in the shop's voice." },
      { key: "gates", text: "Have independent gates check banned claims, the price and margin, and every figure before the shop owner posts." },
    ],
    outputs: [
      { key: "listing_pack", kind: "issue", cadenceHint: "per product" },
      { key: "sales_report", kind: "report", cadenceHint: "per request" },
    ],
    boundaries:
      "Nothing leaves the workspace: no posting, publishing, price changes or messages to a marketplace, buyer or customer; the shop owner does that. Never states a fee, tax rate or marketplace policy from memory, and never certifies that a listing complies with a marketplace's current rules.",
    escalation:
      "A missing cost price, supplier specification or confirmed fee schedule: block the request and mention the requester. A price that cannot reach the target margin under the confirmed fees: a proposal to the shop owner, with the numbers.",
    intake: "đăng sản phẩm, lên sản phẩm mới, listing, sàn, Shopee, TikTok Shop, Lazada, giá bán, đơn xuất sàn, bán hàng online",
  },
  instructions: kvLeadInstructions("ecom_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_ecom", KV_ECOM_ROLE_KEYS),
};
