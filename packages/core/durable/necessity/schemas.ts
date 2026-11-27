import { z } from "zod";
import { parseWithFallback } from "../../api/schema";
import { enumOr, listOfItems, nullableStr, str } from "../schema-helpers";

// Response shapes of the scaffolding sunset necessity test routes (durable spec 10.2 and 10.3,
// server durable_necessity_routes_fork.go and durable_necessity_fork.go). Unknown states degrade
// to running and unknown subject types degrade to skill so one unexpected row never breaks the
// seat view.

export const NECESSITY_SUBJECT_TYPES = ["skill", "instruction_block", "flow_stage"] as const;
export const NECESSITY_STATES = ["running", "not_needed", "needed", "inconclusive", "stopped"] as const;

export type NecessitySubjectType = (typeof NECESSITY_SUBJECT_TYPES)[number];
export type NecessityState = (typeof NECESSITY_STATES)[number];

const finiteNumber = z.coerce.number().finite().catch(0).default(0);

export const NecessityResultSchema = z
  .object({
    pairs: finiteNumber,
    primary: finiteNumber,
    shadow: finiteNumber,
    tie: finiteNumber,
    d_lower: finiteNumber,
    d_upper: finiteNumber,
    margin: finiteNumber,
  })
  .loose();

export type NecessityResult = z.infer<typeof NecessityResultSchema>;

const EMPTY_NECESSITY_RESULT: NecessityResult = {
  pairs: 0,
  primary: 0,
  shadow: 0,
  tie: 0,
  d_lower: 0,
  d_upper: 0,
  margin: 0,
};

export const NecessityTestSchema = z
  .object({
    id: z.string().min(1),
    subject_type: enumOr(NECESSITY_SUBJECT_TYPES, "skill"),
    subject_ref: str,
    agent_id: str,
    shadow_run_id: str,
    state: enumOr(NECESSITY_STATES, "running"),
    result: NecessityResultSchema.catch(EMPTY_NECESSITY_RESULT).default(EMPTY_NECESSITY_RESULT),
    proposal_id: str,
    created_at: str,
    completed_at: nullableStr,
  })
  .loose();

export const NecessityTestListSchema = z
  .object({
    tests: listOfItems(NecessityTestSchema),
  })
  .loose();

export const SeatSkillSchema = z
  .object({
    id: z.string().min(1),
    name: str,
  })
  .loose();

export type NecessityTest = z.infer<typeof NecessityTestSchema>;
export type SeatSkill = z.infer<typeof SeatSkillSchema>;

/** Parses a seat's necessity test list (`{ tests: [...] }`). Malformed items are dropped individually. */
export function parseNecessityTests(raw: unknown, endpoint: string): NecessityTest[] {
  return parseWithFallback<{ tests: NecessityTest[] }>(
    raw,
    NecessityTestListSchema,
    { tests: [] },
    { endpoint },
  ).tests;
}

/** Parses one necessity test (e.g. the create response). A body without an id reads as null. */
export function parseNecessityTest(raw: unknown, endpoint: string): NecessityTest | null {
  return parseWithFallback<NecessityTest | null>(raw, NecessityTestSchema, null, { endpoint });
}

/** Parses a seat's skills list (`GET /api/agents/{id}/skills`). Malformed items are dropped individually. */
export function parseSeatSkills(raw: unknown, endpoint: string): SeatSkill[] {
  return parseWithFallback<SeatSkill[]>(raw, listOfItems(SeatSkillSchema), [], { endpoint });
}
