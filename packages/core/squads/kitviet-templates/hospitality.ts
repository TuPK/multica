import { KITVIET_P2_FLOOR_NAMES } from "../kitviet-floors";
import { kitvietSquad, kvLeadInstructions, kvRole, kvRoleSkills } from "../kitviet-squad-roles";
import type { SquadCharterTemplate, SquadRole, SquadTemplate } from "../templates";

/**
 * Hospitality Ops (Du lịch, khách sạn, spec 9.2): drafts group tour files (itinerary, cost breakdown,
 * quote) and hotel guest text (confirmations, OTA listings, review replies, front-desk scripts) from
 * the facts the hotel or the tour operator supplies. It books nothing, prices nothing from memory and
 * promises nothing to a guest: a quote gate checks the itinerary and the prices, a promise gate checks
 * every guest-facing line, and a manager approves each rate, refund, deposit and compensation before
 * anything goes out. Risky: the promise gate is read by a manager before the squad is switched on
 * (kitviet-p2-notes.md section 6, item 6). Names, skill sets and agent kinds come from the generated
 * kitviet-skills-data/squads.ts; this file adds the English descriptions, the roster lines, the
 * charter and the lead's rules. Agent bodies are loaded by kitviet-packs.ts when the quickstart
 * creates the agents.
 */

export const HOSPITALITY_ROLE_KEYS = [
  "hospitality_lead",
  "hospitality_intake",
  "hospitality_destination",
  "hospitality_tour_drafter",
  "hospitality_hotel_drafter",
  "hospitality_quote_gate",
  "hospitality_promise_gate",
  "hospitality_handover",
] as const;

export type HospitalityRoleKey = (typeof HOSPITALITY_ROLE_KEYS)[number];

const role = (key: HospitalityRoleKey, description: string, roster: string): SquadRole =>
  kvRole(key, { description, roster });

/** The squad's roles. The agents keep the names the generated pack gives them. */
export const HOSPITALITY_ROLES: Record<HospitalityRoleKey, SquadRole> = {
  hospitality_lead: role(
    "hospitality_lead",
    "Leads Hospitality Ops: plans each group tour, quote or guest message, stops when a rate, a policy or a promise has no approved basis, checks each handover, and reports.",
    "Squad leader",
  ),
  hospitality_intake: role(
    "hospitality_intake",
    "Reads the group's request, emails, call notes, supplier quotes and rate sheets, and lists each requirement, price and gap with its file and location.",
    "Group request and supplier documents",
  ),
  hospitality_destination: role(
    "hospitality_destination",
    "Searches official tourism sources for a destination and keeps the source level, link and access date for each fact. Writes no product itself.",
    "Destination sources, level 1-3",
  ),
  hospitality_tour_drafter: role(
    "hospitality_tour_drafter",
    "Drafts the day-by-day itinerary, the cost breakdown and the customer quote from the collected facts, with each unit price traced to its source.",
    "Itinerary, cost breakdown and quote drafts",
  ),
  hospitality_hotel_drafter: role(
    "hospitality_hotel_drafter",
    "Drafts guest-facing text from the approved room facts and policies: confirmation emails, OTA room listings, review replies and front-desk scripts.",
    "Confirmations, listings, review replies and scripts",
  ),
  hospitality_quote_gate: role(
    "hospitality_quote_gate",
    "Checks each itinerary and quote for timing, nights, meals, cost arithmetic, price sources and leaked supplier margins. Writes no drafts. Returns QUA or CHẶN.",
    "Itinerary and price check",
  ),
  hospitality_promise_gate: role(
    "hospitality_promise_gate",
    "Checks each guest-facing text for refund, deposit or compensation promises without approval, wrong policy, guest data and exaggerated claims. Writes no drafts. Returns QUA or CHẶN.",
    "Guest promise and guest data check",
  ),
  hospitality_handover: role(
    "hospitality_handover",
    "Checks the finished draft against the lead's plan and every gate verdict, and lists what a manager must confirm or do before anything goes to a guest.",
    "Final handover check and the manager's list",
  ),
};

export const HOSPITALITY_CHARTER: SquadCharterTemplate = {
  mission:
    "Draft group tour files and guest-facing hotel text from the supplier and hotel facts, and check every price, promise and guest detail before a manager sends or posts anything.",
  responsibilities: [
    { key: "plan", text: "Outline each request and its sources. Stop when a rate, a policy or a guest promise has no approved basis." },
    {
      key: "sources",
      text: "Collect each price, night, meal, room fact and policy from the supplier quote, the rate sheet or an official source, with its location and date.",
    },
    {
      key: "drafts",
      text: "Draft itineraries, cost breakdowns, quotes, confirmations, listings, review replies and scripts only. A draft names no price, refund or deposit that the manager has not approved.",
    },
    {
      key: "gates",
      text: "Before handover, check each itinerary and price against its source and the arithmetic, and each guest-facing line against the approved policy, the promise rules and the guest-data rules.",
    },
  ],
  outputs: [
    { key: "tour_pack", kind: "issue", cadenceHint: "per request" },
    { key: "gate_report", kind: "report", cadenceHint: "per request" },
  ],
  boundaries:
    "Nothing is booked, charged or sent to guests from the workspace. No reservation, room block, payment, refund, deposit, invoice, email to a guest, agency or supplier, listing change or public review reply is made by the squad. Guest names, room numbers, contact details, ID and card data stay out of every draft that leaves the workspace. The manager approves rates, refunds, deposits, cancellation terms and any compensation before anything goes out.",
  escalation:
    "Missing rate sheet, supplier quote, cancellation or deposit policy, or the group's dates and size: stop at type 1 and name the missing item. A refund, discount, free upgrade or compensation with no approved basis is never written as a promise: the line waits for the manager and the decision goes to a proposal. A gate still at CHẶN after two rounds: hand the request to the requester with the open lines.",
  intake:
    "Requests arrive as issues assigned to this squad. Wording people use: tour đoàn, lịch trình, báo giá, khách đoàn, MICE, đặt phòng, xác nhận đặt phòng, review, listing, buồng phòng, check-in, check-out, overbooking, khiếu nại, điểm đến, thuyết minh.",
};

const squad = kitvietSquad("kv_hospitality");

/** The Hospitality Ops template. Routines are none: the squad runs on requests only. */
export const KV_HOSPITALITY_TEMPLATE: SquadTemplate = {
  id: "kv_hospitality",
  name: squad.name,
  description:
    "Drafts group tour itineraries and quotes, and hotel guest text (confirmations, OTA listings, review replies, front-desk scripts) from the supplier and hotel facts. Two gates check every price, promise and guest detail before a manager sends anything. Books, charges and sends nothing.",
  roles: [...HOSPITALITY_ROLE_KEYS],
  leader: "hospitality_lead",
  floorName: KITVIET_P2_FLOOR_NAMES.hospitality,
  routines: [],
  operatingReview: false,
  charter: HOSPITALITY_CHARTER,
  instructions: kvLeadInstructions("hospitality_lead"),
  workflow: null,
  builtinSkills: squad.builtinSkills,
  leaderSkills: squad.leaderSkills,
  roleSkills: kvRoleSkills("kv_hospitality", HOSPITALITY_ROLE_KEYS),
};
