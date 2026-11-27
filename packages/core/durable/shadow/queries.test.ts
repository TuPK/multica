// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  isValidShadowEstimateParams,
  shadowEstimateOptions,
  shadowKeys,
  shadowRunsOptions,
} from "./queries";

const { request } = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("../../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api")>()),
  api: { request },
}));

afterEach(() => {
  request.mockReset();
});

describe("shadow query keys", () => {
  it("include workspace id and agent id in every key", () => {
    const key = shadowEstimateOptions("ws-1", "a-1", { sample_rate: 0.3, target_pairs: 20 }).queryKey;
    expect(key.slice(0, 4)).toEqual(["durable", "ws-1", "shadow", "a-1"]);
    expect(shadowKeys.agent("ws-1", "a-1")).toEqual(["durable", "ws-1", "shadow", "a-1"]);
  });

  it("separate workspaces, agents, and parameters", () => {
    expect(shadowEstimateOptions("ws-1", "a-1", { sample_rate: 0.3, target_pairs: 20 }).queryKey).not.toEqual(
      shadowEstimateOptions("ws-2", "a-1", { sample_rate: 0.3, target_pairs: 20 }).queryKey,
    );
    expect(shadowEstimateOptions("ws-1", "a-1", { sample_rate: 0.3, target_pairs: 20 }).queryKey).not.toEqual(
      shadowEstimateOptions("ws-1", "a-1", { sample_rate: 0.5, target_pairs: 20 }).queryKey,
    );
  });
});

describe("shadow estimate query", () => {
  it("calls GET /api/agents/{id}/shadow/estimate with sample_rate and target_pairs", async () => {
    request.mockResolvedValueOnce({ estimate_micros: 2_500_000, remaining_micros: 5_000_000, warn: false });
    const est = await shadowEstimateOptions("ws-1", "a-1", { sample_rate: 0.5, target_pairs: 40 }).queryFn!({} as never);
    expect(request).toHaveBeenCalledWith("/api/agents/a-1/shadow/estimate?sample_rate=0.5&target_pairs=40");
    expect(est).toEqual({ estimate_micros: 2_500_000, remaining_micros: 5_000_000, warn: false });
  });

  it("disables requests for out-of-range parameters or missing ids", () => {
    expect(isValidShadowEstimateParams({ sample_rate: 0, target_pairs: 20 })).toBe(false);
    expect(isValidShadowEstimateParams({ sample_rate: 1.2, target_pairs: 20 })).toBe(false);
    expect(isValidShadowEstimateParams({ sample_rate: 0.3, target_pairs: 4 })).toBe(false);
    expect(isValidShadowEstimateParams({ sample_rate: 0.3, target_pairs: 201 })).toBe(false);
    expect(isValidShadowEstimateParams({ sample_rate: 0.3, target_pairs: 10.5 })).toBe(false);
    expect(shadowEstimateOptions("", "a-1").enabled).toBe(false);
    expect(shadowEstimateOptions("ws-1", "").enabled).toBe(false);
    expect(shadowEstimateOptions("ws-1", "a-1", { sample_rate: 0, target_pairs: 20 }).enabled).toBe(false);
    expect(shadowEstimateOptions("ws-1", "a-1", { sample_rate: 0.3, target_pairs: 20 }, false).enabled).toBe(false);
  });
});

describe("shadow runs query", () => {
  it("scopes the runs query key by workspace id and agent id", () => {
    expect(shadowKeys.runs("ws-1", "a-1")).toEqual(["durable", "ws-1", "shadow", "a-1", "runs"]);
    expect(shadowRunsOptions("ws-1", "a-1").queryKey).not.toEqual(shadowRunsOptions("ws-2", "a-1").queryKey);
  });

  it("calls GET /api/agents/{id}/shadow and parses the run list", async () => {
    request.mockResolvedValueOnce({
      runs: [
        {
          id: "sr-1",
          agent_id: "a-1",
          candidate_runtime_id: "rt-2",
          candidate_model: "gpt-5",
          sample_rate: 0.3,
          target_pairs: 20,
          auto_promote: false,
          state: "running",
          purpose: "model_trial",
          work_class_prefix: "",
          created_at: "2026-11-26T09:00:00Z",
          completed_at: null,
          report: {
            pairs: 4,
            shadow: 2,
            tie: 1,
            primary: 1,
            unable: 0,
            not_losing_lower: 0.3,
            primary_cost_avg_micros: 200_000,
            shadow_cost_avg_micros: 150_000,
          },
        },
      ],
    });
    const runs = await shadowRunsOptions("ws-1", "a-1").queryFn!({} as never);
    expect(request).toHaveBeenCalledWith("/api/agents/a-1/shadow");
    expect(runs).toHaveLength(1);
    expect(runs[0]).toMatchObject({ id: "sr-1", candidate_model: "gpt-5", report: { pairs: 4, shadow: 2 } });
  });

  it("disables the runs query without workspace or agent id", () => {
    expect(shadowRunsOptions("", "a-1").enabled).toBe(false);
    expect(shadowRunsOptions("ws-1", "").enabled).toBe(false);
    expect(shadowRunsOptions("ws-1", "a-1", false).enabled).toBe(false);
  });
});
