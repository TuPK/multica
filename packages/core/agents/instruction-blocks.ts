import { CATALOG_AGENT_BLOCKS } from "../catalog/generated/agents";

/**
 * Rules the agent instruction templates share: the working rules every specialist ends with, and
 * the lessons, money and language rules every template carries (spec 3.4). The text lives in
 * catalog/agents/_blocks and is generated into packages/core/catalog/generated/agents.ts.
 */

export const WORKING_RULES: string = CATALOG_AGENT_BLOCKS.working_rules;

/**
 * Included in every instruction template, so the company's roles and the
 * engineering roles follow the same evidence, knowledge, money and language
 * rules (spec 3.4).
 */
export const SHARED_BLOCKS: string = CATALOG_AGENT_BLOCKS.shared_blocks;
