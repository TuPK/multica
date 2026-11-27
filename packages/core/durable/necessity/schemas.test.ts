// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { setSchemaLogger } from "../../api/schema";
import { noopLogger, type Logger } from "../../logger";
import { parseNecessityTest, parseNecessityTests, parseSeatSkills } from "./schemas";

const finishedRow = {
  id: "nt-1",
  subject_type: "skill",
  subject_ref: "code-review",
  agent_id: "a-1",
  shadow_run_id: "sr-1",
  state: "not_needed",
  result: {
    pairs: 40,
    primary: 2,
    shadow: 6,
    tie: 32,
    d_lower: -0.18,
    d_upper: 0.02,
    margin: 0.05,
  },
  proposal_id: "p-1",
  created_at: "2026-11-26T09:00:00Z",
  completed_at: "2026-11-27T01:00:00Z",
};

const runningRow = {
  id: "nt-2",
  subject_type: "skill",
  subject_ref: "sk-2",
  agent_id: "a-1",
  state: "running",
  result: {},
  created_at: "2026-11-27T02:00:00Z",
};

describe("necessity schemas", () => {
  let warn: Mock<(msg: string, ...data: unknown[]) => void>;

  beforeEach(() => {
    warn = vi.fn<(msg: string, ...data: unknown[]) => void>();
    const logger: Logger = { debug: vi.fn(), info: vi.fn(), warn, error: vi.fn() };
    setSchemaLogger(logger);
  });

  afterEach(() => {
    setSchemaLogger(noopLogger);
  });

  it("parses finished and running necessity tests with result counts and optional fields", () => {
    const tests = parseNecessityTests(
      { tests: [finishedRow, runningRow] },
      "GET /api/agents/:id/necessity",
    );
    expect(tests).toHaveLength(2);
    expect(tests[0]).toMatchObject({
      id: "nt-1",
      subject_type: "skill",
      subject_ref: "code-review",
      agent_id: "a-1",
      shadow_run_id: "sr-1",
      state: "not_needed",
      proposal_id: "p-1",
      completed_at: "2026-11-27T01:00:00Z",
      result: { pairs: 40, primary: 2, shadow: 6, tie: 32 },
    });
    expect(tests[1]).toMatchObject({
      id: "nt-2",
      state: "running",
      shadow_run_id: "",
      proposal_id: "",
      completed_at: null,
      result: { pairs: 0, primary: 0, shadow: 0, tie: 0 },
    });
    expect(warn).not.toHaveBeenCalled();
  });

  it("falls back unknown state to running and unknown subject_type to skill", () => {
    const tests = parseNecessityTests(
      {
        tests: [
          {
            ...finishedRow,
            state: "paused_by_future_server",
            subject_type: "future_scaffolding",
            result: "not-an-object",
          },
        ],
      },
      "GET /api/agents/:id/necessity",
    );
    expect(tests).toHaveLength(1);
    expect(tests[0]).toMatchObject({
      state: "running",
      subject_type: "skill",
      result: { pairs: 0, primary: 0, shadow: 0, tie: 0 },
    });
  });

  it("drops malformed items individually while keeping valid tests", () => {
    const tests = parseNecessityTests(
      {
        tests: [
          { state: "running" },
          "bad",
          null,
          { id: "", state: "needed" },
          finishedRow,
        ],
      },
      "GET /api/agents/:id/necessity",
    );
    expect(tests.map((t) => t.id)).toEqual(["nt-1"]);
  });

  it("falls back to an empty list for a body of the wrong shape and logs the endpoint", () => {
    expect(parseNecessityTests(null, "GET /api/agents/:id/necessity")).toEqual([]);
    expect(parseNecessityTests("oops", "GET /api/agents/:id/necessity")).toEqual([]);
    expect(warn).toHaveBeenCalledTimes(2);
    expect(String(warn.mock.calls[0]?.[0])).toContain("GET /api/agents/:id/necessity");
  });

  it("parses a single created necessity test and returns null for malformed replies", () => {
    expect(parseNecessityTest(runningRow, "POST /api/agents/:id/necessity")).toMatchObject({
      id: "nt-2",
      state: "running",
    });
    expect(parseNecessityTest({ state: "running" }, "POST /api/agents/:id/necessity")).toBeNull();
    expect(parseNecessityTest("oops", "POST /api/agents/:id/necessity")).toBeNull();
  });

  it("parses seat skills and drops malformed entries", () => {
    const skills = parseSeatSkills(
      [{ id: "sk-1", name: "Deploy" }, { name: "missing-id" }, null, { id: "sk-2", name: "SQL" }],
      "GET /api/agents/:id/skills",
    );
    expect(skills).toEqual([
      { id: "sk-1", name: "Deploy" },
      { id: "sk-2", name: "SQL" },
    ]);
    expect(parseSeatSkills("oops", "GET /api/agents/:id/skills")).toEqual([]);
  });
});
