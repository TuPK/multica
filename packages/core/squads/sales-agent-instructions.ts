import { catalogRoleInstructions } from "../catalog/role-instructions";
import type { SquadRoleKey } from "./templates";

/**
 * The instructions of the agents a Sales Loop squad creates. Like the planning ones
 * (planning-agent-instructions.ts) they are short on purpose: the procedure lives in the sales
 * skills the squad carries (catalog/skills/builtin folder, sales-*). The texts live in
 * catalog/squads/sales_loop/roles.
 */

/** The sales role keys a Sales Loop squad creates agents for. */
export type SalesRoleKey = Extract<
  SquadRoleKey,
  "sales_lead" | "sales_scout" | "sales_writer" | "sales_analyst" | "sales_checker"
>;

export const SALES_ROLE_AGENT_INSTRUCTIONS: Record<SalesRoleKey, string> = catalogRoleInstructions(
  ["sales_loop"],
  ["sales_lead", "sales_scout", "sales_writer", "sales_analyst", "sales_checker"],
);
