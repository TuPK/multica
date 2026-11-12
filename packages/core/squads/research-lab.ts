import { catalogRoleInstructions } from "../catalog/role-instructions";
import { skillsetMembers } from "../catalog/skillsets";

/**
 * The Research Lab squad (docs/engineering/research-lab/litscout-research-spec.md).
 *
 * A literature squad: it takes a research question, searches scholarly sources over a few bounded
 * rounds, screens the papers against written criteria, summarises only what the abstracts say, and
 * hands back a table and a digest in which every claim points at a paper. The deterministic work
 * (search, dedup, candidate ranking, checking the agents' files, the quality gate, the provenance
 * gate, export) is done by `multica research`, which uses no model. The agents plan, screen,
 * summarise and audit.
 *
 * Like Content Studio, each role's agent starts from an existing instruction template and the
 * role's own instructions replace it (see instructionsFor in quickstart.ts). The procedure
 * lives in the research-lab-* skills the squad carries.
 *
 * This squad is separate from Market Research. It shares none of that squad's agents, and none of
 * the shared research instruction templates (research_lead, market_scout, critic) is used here.
 */

export type RlRoleKey = "lab_lead" | "lab_scout" | "lab_screener" | "lab_synthesizer" | "lab_auditor";

/** The roles in roster order. */
export const RL_ROLE_KEYS: readonly RlRoleKey[] = [
  "lab_lead",
  "lab_scout",
  "lab_screener",
  "lab_synthesizer",
  "lab_auditor",
];

/** The built-in skills of the Research Lab: the catalog skills whose `sets` include `research_lab`. */
export const RESEARCH_LAB_SKILL_NAMES: readonly string[] = skillsetMembers("research_lab");

/** The instructions each Research Lab role's agent follows (catalog/squads/research_lab/roles). */
export const RESEARCH_LAB_ROLE_AGENT_INSTRUCTIONS: Record<RlRoleKey, string> = catalogRoleInstructions(
  ["research_lab"],
  RL_ROLE_KEYS,
);
