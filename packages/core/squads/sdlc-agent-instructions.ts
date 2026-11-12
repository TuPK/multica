import { catalogRoleInstructions } from "../catalog/role-instructions";
import type { SquadRoleKey } from "./templates";

/**
 * The instructions of the agents an SDLC squad creates. They are short on
 * purpose: the procedure lives in the built-in SDLC skills the squad carries
 * (catalog/skills/builtin folder), which every member loads. The texts live in
 * catalog/squads/sdlc_greenfield/roles and catalog/squads/sdlc_maintenance/roles.
 * The shared agent templates in agents/instruction-templates.ts stay as they are,
 * since other squads use them.
 */

/** The SDLC role keys an SDLC squad creates agents for. */
export type SdlcRoleKey = Extract<
  SquadRoleKey,
  "lead" | "analyst" | "ux" | "architect" | "developer" | "fixer" | "reviewer" | "qa" | "security" | "devops"
>;

export const SDLC_ROLE_AGENT_INSTRUCTIONS: Record<SdlcRoleKey, string> = catalogRoleInstructions(
  ["sdlc_greenfield", "sdlc_maintenance"],
  ["lead", "analyst", "ux", "architect", "developer", "fixer", "reviewer", "qa", "security", "devops"],
);
