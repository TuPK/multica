// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { setSchemaLogger } from "../../api/schema";
import { noopLogger, type Logger } from "../../logger";
import {
  parseShadowEstimate,
  parseShadowRun,
  parseShadowRuns,
  shadowNeedsCostConfirmation,
} from "./schemas";

describe("shadow schemas", () => {
  let warn: Mock<(msg: string, ...data: unknown[]) => void>;

  beforeEach(() => {
    warn = vi.fn<(msg: string, ...data: unknown[]) => void>();
    const logger: Logger = { debug: vi.fn(), info: vi.fn(), warn, error: vi.fn() };
    setSchemaLogger(logger);
  });

  afterEach(() => {
    setSchemaLogger(noopLogger);
  });

  it("reads a shadow estimate including remaining_micros = -1 when no budget covers the seat", () => {
    const est = parseShadowEstimate(
      { estimate_micros: 1_250_000, remaining_micros: -1, warn: false },
      "GET /api/agents/:id/shadow/estimate",
    );
    expect(est).toEqual({ estimate_micros: 1_250_000, remaining_micros: -1, warn: false });
    expect(warn).not.toHaveBeenCalled();
  });

  it("falls back safely on missing or malformed estimate fields", () => {
    expect(
      parseShadowEstimate(
        { estimate_micros: "bad", remaining_micros: "nope", warn: "yes" },
        "GET /api/agents/:id/shadow/estimate",
      ),
    ).toEqual({ estimate_micros: 0, remaining_micros: -1, warn: false });
    expect(parseShadowEstimate(null, "GET /api/agents/:id/shadow/estimate")).toEqual({
      estimate_micros: 0,
      remaining_micros: -1,
      warn: false,
    });
  });

  it("reads a shadow run response and degrades an unknown state", () => {
    const run = parseShadowRun(
      {
        id: "sr-1",
        agent_id: "a-1",
        candidate_runtime_id: "rt-2",
        candidate_model: "claude-sonnet",
        sample_rate: 0.3,
        target_pairs: 20,
        auto_promote: false,
        state: "paused_future",
      },
      "POST /api/agents/:id/shadow",
    );
    expect(run).toMatchObject({
      id: "sr-1",
      candidate_runtime_id: "rt-2",
      candidate_model: "claude-sonnet",
      sample_rate: 0.3,
      target_pairs: 20,
      auto_promote: false,
      state: "running",
    });
  });

  it("returns null for a malformed shadow run without an id", () => {
    expect(parseShadowRun({ state: "running" }, "POST /api/agents/:id/shadow")).toBeNull();
    expect(parseShadowRun(null, "POST /api/agents/:id/shadow")).toBeNull();
  });

  it("requires cost confirmation when warn is true or estimate exceeds a positive budget", () => {
    expect(shadowNeedsCostConfirmation(undefined)).toBe(false);
    expect(shadowNeedsCostConfirmation({ estimate_micros: 500_000, remaining_micros: -1, warn: false })).toBe(false);
    expect(shadowNeedsCostConfirmation({ estimate_micros: 500_000, remaining_micros: 1_000_000, warn: false })).toBe(false);
    expect(shadowNeedsCostConfirmation({ estimate_micros: 500_000, remaining_micros: 1_000_000, warn: true })).toBe(true);
    expect(shadowNeedsCostConfirmation({ estimate_micros: 1_500_000, remaining_micros: 1_000_000, warn: false })).toBe(true);
  });

  it("parses shadow run list items with reports, drops malformed items, and falls back unknown enums", () => {
    const runs = parseShadowRuns(
      {
        runs: [
          {
            id: "sr-1",
            agent_id: "a-1",
            candidate_runtime_id: "rt-2",
            candidate_model: "claude-sonnet-4",
            sample_rate: 0.3,
            target_pairs: 20,
            auto_promote: false,
            state: "completed",
            purpose: "model_trial",
            work_class_prefix: "flow:pr:",
            created_at: "2026-11-26T09:00:00Z",
            completed_at: "2026-11-27T01:00:00Z",
            report: {
              pairs: 20,
              shadow: 12,
              tie: 5,
              primary: 3,
              unable: 2,
              not_losing_lower: 0.64,
              primary_cost_avg_micros: 450_000,
              shadow_cost_avg_micros: 320_000,
            },
          },
          { state: "running" },
          "junk",
          {
            id: "sr-2",
            state: "future_state",
            purpose: "future_purpose",
            report: "bad",
          },
        ],
      },
      "GET /api/agents/:id/shadow",
    );

    expect(runs).toHaveLength(2);
    expect(runs[0]).toMatchObject({
      id: "sr-1",
      candidate_model: "claude-sonnet-4",
      state: "completed",
      purpose: "model_trial",
      work_class_prefix: "flow:pr:",
      completed_at: "2026-11-27T01:00:00Z",
      report: {
        pairs: 20,
        shadow: 12,
        tie: 5,
        primary: 3,
        unable: 2,
        not_losing_lower: 0.64,
        primary_cost_avg_micros: 450_000,
        shadow_cost_avg_micros: 320_000,
      },
    });
    expect(runs[1]).toMatchObject({
      id: "sr-2",
      state: "running",
      purpose: "model_trial",
      completed_at: null,
      report: {
        pairs: 0,
        shadow: 0,
        tie: 0,
        primary: 0,
        unable: 0,
        not_losing_lower: 0,
        primary_cost_avg_micros: 0,
        shadow_cost_avg_micros: 0,
      },
    });
    expect(warn).not.toHaveBeenCalled();
  });

  it("falls back to an empty list when the shadow runs body is malformed", () => {
    expect(parseShadowRuns(null, "GET /api/agents/:id/shadow")).toEqual([]);
    expect(parseShadowRuns("oops", "GET /api/agents/:id/shadow")).toEqual([]);
    expect(warn).toHaveBeenCalledTimes(2);
  });
});
