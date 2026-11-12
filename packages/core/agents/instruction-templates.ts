import { CATALOG_AGENT_INSTRUCTIONS, type AgentInstructionTemplateId } from "../catalog/generated/agents";

/**
 * The Kit Viet role templates. The squad quickstart gives them to its agents. The standalone
 * picker does not offer them, so they have no name or description in the views locales.
 */
export type KitvietInstructionTemplateId =
  | "kv_lead"
  | "kv_collector"
  | "kv_drafter"
  | "kv_gate"
  | "kv_handover";

/**
 * Ready-made instructions a user can start an agent from in the Configure
 * step. The instructions text lives in catalog/agents/<id>/instructions.md (shared blocks in
 * catalog/agents/_blocks); names and descriptions are translated in the views package, keyed by `id`.
 */
export interface AgentInstructionTemplate {
  id: AgentInstructionTemplateId;
  instructions: string;
}

/** A template the standalone picker offers: every id except the Kit Viet role templates. */
export type StandaloneInstructionTemplate = AgentInstructionTemplate & {
  id: Exclude<AgentInstructionTemplate["id"], KitvietInstructionTemplateId>;
};

/** Every template, in listed order, generated from the catalog by `pnpm catalog:build`. */
export const AGENT_INSTRUCTION_TEMPLATES: readonly AgentInstructionTemplate[] = CATALOG_AGENT_INSTRUCTIONS;
