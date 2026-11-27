// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { isValidShadowEstimateParams, shadowEstimateOptions, shadowKeys } from "./queries";

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
