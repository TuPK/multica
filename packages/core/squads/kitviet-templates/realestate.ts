import { KITVIET_P2_FLOOR_NAMES } from "../kitviet-floors";
import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadCharterTemplate, SquadRole, SquadTemplate } from "../templates";

/**
 * Real Estate Desk (Bất động sản, spec 9.2): turns one property or borrower file into a listing draft, a
 * home-loan plan or an area note for a broker to read, with every figure traced to its source. It promises no
 * price, approves no loan and gives no legal conclusion on a title: three gates (planning source, loan plan,
 * listing claims) check each draft, and a notary or lawyer decides on the title. Names, skill sets and agent
 * kinds come from the generated kitviet-skills-data/squads.ts; this file adds the English descriptions, the
 * roster lines, the charter and the lead's notes (REALESTATE_LEAD_NOTES). Agent bodies are loaded by
 * kitviet-packs.ts when the quickstart creates the agents.
 */

export const REALESTATE_ROLE_KEYS = [
  "realestate_lead",
  "realestate_reader",
  "realestate_area_research",
  "realestate_loan_drafter",
  "realestate_listing_drafter",
  "realestate_planning_gate",
  "realestate_loan_gate",
  "realestate_listing_gate",
  "realestate_handover",
] as const;

export type RealestateRoleKey = (typeof REALESTATE_ROLE_KEYS)[number];

const role = (key: RealestateRoleKey, description: string, roster: string): SquadRole =>
  kvRole(key, { description, roster });

/** The squad's roles. The agents keep the names the generated pack gives them. */
export const REALESTATE_ROLES: Record<RealestateRoleKey, SquadRole> = {
  realestate_lead: role(
    "realestate_lead",
    "Leads the Real Estate Desk: plans each listing, loan plan or area note, stops when the file is incomplete, holds each draft for the gates, checks each handover, and reports.",
    "Squad leader",
  ),
  realestate_reader: role(
    "realestate_reader",
    "Reads a property or borrower file (owner notes, title copies, loan quotes) and lists each fact with its source, every conflict, what is missing and the personal data to hide.",
    "Property and borrower file facts",
  ),
  realestate_area_research: role(
    "realestate_area_research",
    "Looks up an area's amenities, transport and officially published planning on the web, each line with its source level, date and reliability. Writes no files.",
    "Area amenities and published planning",
  ),
  realestate_loan_drafter: role(
    "realestate_loan_drafter",
    "Builds the home-loan plan as a monthly schedule with the post-promotion rate scenarios and the debt-to-income ratio, with each formula written out and each rate marked stated or assumed.",
    "Home-loan plan and monthly schedule",
  ),
  realestate_listing_drafter: role(
    "realestate_listing_drafter",
    "Drafts listing texts for each channel only from the approved facts table, with the title area stated first. Writes draft files only and posts nothing.",
    "Listing drafts for each channel",
  ),
  realestate_planning_gate: role(
    "realestate_planning_gate",
    "Checks every planning, infrastructure and price-trend claim in a note or brochure against its source level and date. Writes nothing and returns QUA or CHẶN.",
    "Planning and infrastructure source check",
  ),
  realestate_loan_gate: role(
    "realestate_loan_gate",
    "Checks the loan plan's formulas, the floating-rate stage, the rate sources and any wording that promises approval. Writes nothing and returns QUA or CHẶN.",
    "Loan plan formula and wording check",
  ),
  realestate_listing_gate: role(
    "realestate_listing_gate",
    "Checks a listing for return or price promises (each one is its own finding), unsourced planning claims, an area that differs from the title, title wording the file does not support, and exposed owner data. Returns QUA or CHẶN.",
    "Listing claim and owner-data check",
  ),
  realestate_handover: role(
    "realestate_handover",
    "Checks the finished product against the lead's plan and every gate verdict, and lists the decisions a person must make before anything leaves the desk.",
    "Final handover check",
  ),
};

export const REALESTATE_CHARTER: SquadCharterTemplate = {
  mission:
    "Turn one property or borrower file into a listing draft, a home-loan plan or an area note for a broker to read, with every figure, planning claim and legal line traced to its source and checked by a gate before a person reads it.",
  responsibilities: [
    { key: "plan", text: "Outline each file, list what is missing, and stop before drafting when the file cannot support the product." },
    { key: "sources", text: "Record each fact with its source, its source level, its date and any conflict between documents." },
    { key: "tables", text: "Build the loan schedule and ratios in files, with each formula and each rate source written out." },
    { key: "drafts", text: "Draft the listing, loan plan or area note only from the collected facts, with the title area stated first." },
    {
      key: "gates",
      text: "Before handover, check each planning claim against its source, each listing claim against the file, and each loan figure and sentence against the formulas and the approval rule.",
    },
  ],
  outputs: [
    { key: "listing_draft", kind: "issue", cadenceHint: "per listing" },
    { key: "loan_plan", kind: "proposal", cadenceHint: "per borrower" },
    { key: "gate_report", kind: "report", cadenceHint: "per draft" },
  ],
  boundaries:
    "Drafts for a broker. Never promises a price, a rent, a return or a price rise; never says a title is clean, valid or free of dispute (a notary or lawyer decides, and the handover lists the question); never says a borrower will be approved, qualifies or is certain to get a loan; a planning claim without a level 1-2 source stays out. A rate, fee, term or area the user did not confirm stays [cần xác nhận]. Nothing leaves the workspace: no posting to a listing site, no message to an owner or a client, no bank submission, no contract signature, no payment; the broker or the owner does those.",
  escalation:
    "File missing the title area, the title type, the price or the loan terms, or a conflicting figure between documents: stop at human stop 1 and name the missing item. A gate still at CHẶN after two rounds: human stop 3, hand over the draft with the open lines, and never change the verdict to QUA. A request for a price promise, a clean-title statement or a guaranteed loan: not run; set the request to blocked and say so to the requester.",
  intake:
    "Requests arrive as issues assigned to this squad. Wording people use: tin đăng nhà đất, đăng bán căn, ký gửi căn, phương án vay mua nhà, tính vay, quy hoạch khu vực, rà hợp đồng cọc, video tour nhà, giới thiệu căn.",
};

const squad = kitvietSquad("kv_realestate");

/** The Real Estate Desk template. Routines are none: the squad runs on requests only. */
export const KV_REALESTATE_TEMPLATE: SquadTemplate = {
  id: "kv_realestate",
  name: squad.name,
  description:
    "Turns one property or borrower file into a listing draft, a home-loan plan or an area note, with each figure traced to its source. Planning, loan and listing gates check each draft before a broker reads it. Promises no price, approves no loan, gives no title opinion, and posts or sends nothing.",
  roles: [...REALESTATE_ROLE_KEYS],
  leader: "realestate_lead",
  floorName: KITVIET_P2_FLOOR_NAMES.realestate,
  routines: [],
  operatingReview: false,
  charter: REALESTATE_CHARTER,
  instructions: kvLeadInstructions("realestate_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_realestate", REALESTATE_ROLE_KEYS),
};
