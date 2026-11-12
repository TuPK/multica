import { catalogRoleInstructions } from "../catalog/role-instructions";
import type { SquadRoleKey } from "./templates";

/**
 * The instructions of the agents a BMAD planning squad creates. Like the SDLC ones
 * (sdlc-agent-instructions.ts) they are short on purpose: the procedure lives in the
 * built-in planning skills the squad carries (catalog/skills/builtin folder, planning-*).
 * The texts live in catalog/squads/bmad_planning/roles.
 */

/** The planning role keys a planning squad creates agents for. */
export type PlanningRoleKey = Extract<
  SquadRoleKey,
  "plan_lead" | "plan_researcher" | "plan_analyst" | "plan_pm" | "plan_ux" | "plan_architect" | "plan_reviewer"
>;

export const PLANNING_ROLE_AGENT_INSTRUCTIONS: Record<PlanningRoleKey, string> = catalogRoleInstructions(
  ["bmad_planning"],
  ["plan_lead", "plan_researcher", "plan_analyst", "plan_pm", "plan_ux", "plan_architect", "plan_reviewer"],
);
