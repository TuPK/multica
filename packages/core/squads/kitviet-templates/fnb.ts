import { KITVIET_P2_FLOOR_NAMES } from "../kitviet-floors";
import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadCharterTemplate, SquadRole, SquadTemplate } from "../templates";

/**
 * F&B Ops (Nhà hàng, quán ăn, P2 trade `nha-hang-fnb`): turns a new dish or a menu change into a
 * costed, allergen-checked draft for the owner, and checks every allergen line and food-cost figure
 * with two independent gates before anything is printed or posted. Names, skill sets and agent kinds
 * come from the generated kitviet-skills-data/squads.ts; this file adds the English descriptions, the
 * roster lines, the charter and the floor. The squad is seated on its own floor (KITVIET_P2_FLOOR_NAMES).
 * Agent bodies are loaded by kitviet-packs.ts when the quickstart creates the agents.
 */

export const FNB_ROLE_KEYS = [
  "fnb_lead",
  "fnb_reader",
  "fnb_regulation",
  "fnb_drafter",
  "fnb_allergen_gate",
  "fnb_margin_gate",
  "fnb_handover",
] as const;

export type FnbRoleKey = (typeof FNB_ROLE_KEYS)[number];

const role = (key: FnbRoleKey, description: string, roster: string): SquadRole => kvRole(key, { description, roster });

/** The squad's roles. The agents keep the names the generated pack gives them. */
export const FNB_ROLES: Record<FnbRoleKey, SquadRole> = {
  fnb_lead: role(
    "fnb_lead",
    "Leads F&B Ops: plans each dish launch or menu change, holds each stop for the owner, checks each handover, and reports.",
    "Squad leader",
  ),
  fnb_reader: role(
    "fnb_reader",
    "Reads the shop's recipes, purchase invoices, product labels and POS or stock files, and returns each figure with its file and location.",
    "Recipes, purchase prices and label text from the shop's files",
  ),
  fnb_regulation: role(
    "fnb_regulation",
    "Looks up the current food safety, business licence and fire rules for a restaurant or cafe, on official sources, with each source's level and access date.",
    "Official rules for licences and food safety, with sources",
  ),
  fnb_drafter: role(
    "fnb_drafter",
    "Drafts food cost tables, 2-3 price options, menu descriptions with allergen lines, SOPs, checklists, rosters and reports from the collected facts only.",
    "Food cost tables, menu descriptions and operating drafts",
  ),
  fnb_allergen_gate: role(
    "fnb_allergen_gate",
    "Checks every ingredient and label line of a dish against its allergen lines, vegan labels and health claims. Writes no drafts and changes nothing.",
    "Allergen, food safety and claim check (gate)",
  ),
  fnb_margin_gate: role(
    "fnb_margin_gate",
    "Recomputes every food cost line, unit conversion, yield, VAT and packaging figure, and flags any fee or rate the owner has not confirmed. Writes no drafts.",
    "Food cost, margin and figure check (gate)",
  ),
  fnb_handover: role(
    "fnb_handover",
    "Compares the finished draft with the approved plan and both gate verdicts, and lists what the owner and the chef must confirm before anything is printed or posted.",
    "Final handover check",
  ),
};

export const FNB_CHARTER: SquadCharterTemplate = {
  mission:
    "Cost each new dish and check its allergen lines and food cost against the shop's own recipes and purchase invoices, before the owner prints a menu or posts a dish.",
  responsibilities: [
    { key: "plan", text: "Outline each dish launch or menu change, and list the recipes, invoices and labels it needs, before any costing is done." },
    { key: "sources", text: "Collect each recipe quantity, purchase price, date and label line with its file and location; a missing item stays [cần bổ sung]." },
    {
      key: "drafts",
      text: "Draft the food cost table, 2-3 price options, menu descriptions and operating documents from the collected facts only, with every figure computed by script.",
    },
    {
      key: "gates",
      text: "Before handover, an independent allergen and food safety gate checks each dish line and claim, and an independent margin gate recomputes each cost figure; a Chặn or Cần giấy tờ finding blocks the dish.",
    },
    { key: "regulation", text: "Look up the current licence, food safety and fire rules on official sources, with each source's level and access date, and mark any rule that depends on the district or the shop type." },
  ],
  outputs: [
    { key: "dish_pack", kind: "issue", cadenceHint: "per dish launch or menu change" },
    { key: "gate_report", kind: "report", cadenceHint: "per draft" },
  ],
  boundaries:
    "Does not certify food safety, does not give a legal opinion, and does not state that a dish is free of an allergen, vegan, organic or healthy without a source the owner confirms. Does not set the selling price, change a supplier price, or assign a VAT rate or a delivery-app fee. Nothing leaves the workspace: no printing, posting, ordering, paying, sending to a supplier, a customer or an authority; the owner or the chef does that.",
  escalation:
    "Recipe, purchase price or label ingredient missing: stop 1 and name the missing item; a dish with a Chặn finding after two rounds: stop 3 with the open findings; an allergen line the chef cannot confirm: the dish stays out of the menu until it is confirmed.",
  intake:
    "Wording people use: ra món mới, menu, thực đơn, food cost, giá vốn, công thức, dị ứng, nhiễm chéo, SOP vệ sinh ATTP, lịch ca, báo cáo doanh thu, tồn kho bếp, giấy phép quán ăn.",
};

const squad = kitvietSquad("kv_fnb");

/** The F&B Ops template. Routines are none: the squad runs on requests only. */
export const KV_FNB_TEMPLATE: SquadTemplate = {
  id: "kv_fnb",
  name: squad.name,
  description:
    "Costs new dishes and menu changes from the shop's recipes and purchase invoices, and checks every allergen line and food cost figure with two independent gates before anything is printed or posted. Prints, posts and orders nothing.",
  roles: [...FNB_ROLE_KEYS],
  leader: "fnb_lead",
  floorName: KITVIET_P2_FLOOR_NAMES.fnb,
  routines: [],
  charter: FNB_CHARTER,
  instructions: kvLeadInstructions("fnb_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_fnb", FNB_ROLE_KEYS),
};
