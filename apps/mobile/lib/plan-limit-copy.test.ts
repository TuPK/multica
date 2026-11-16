// @vitest-environment node
import { afterEach, describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api-error";
import { i18n } from "@/lib/i18n/singleton";
import { formatPlanLimitError } from "./plan-limit-copy";

describe("formatPlanLimitError", () => {
  afterEach(async () => {
    await i18n.changeLanguage("en");
  });

  it("formats plan_limit_reached with limit and used for resource gates", () => {
    const err = new ApiError("request failed", 402, {
      code: "plan_limit_reached",
      gate: "issue_count",
      limit: 50,
      used: 48,
    });
    const message = formatPlanLimitError(err);
    expect(message).toBeTypeOf("string");
    expect(message).toContain("48");
    expect(message).toContain("50");
  });

  it("formats storage_bytes plan_limit_reached without numbers", () => {
    const err = new ApiError("request failed", 402, {
      code: "plan_limit_reached",
      gate: "storage_bytes",
      limit: 214748364800,
      used: 214748364000,
    });
    const message = formatPlanLimitError(err);
    expect(message).toBeTypeOf("string");
    expect(message!.length).toBeGreaterThan(0);
    expect(message).not.toMatch(/\d/);
  });

  it("formats unknown gates on plan_limit_reached with generic copy", () => {
    const err = new ApiError("request failed", 402, {
      code: "plan_limit_reached",
      gate: "future_unknown_gate",
      limit: 10,
      used: 10,
    });
    const message = formatPlanLimitError(err);
    expect(message).toBeTypeOf("string");
    expect(message!.length).toBeGreaterThan(0);
    expect(message).not.toContain("future_unknown_gate");
  });

  it("formats plan_feature_unavailable copy (supporting feature or server gate field)", () => {
    const fromFeature = formatPlanLimitError(
      new ApiError("request failed", 402, {
        code: "plan_feature_unavailable",
        feature: "workspace_policies",
      }),
    );
    expect(fromFeature).toBeTypeOf("string");
    expect(fromFeature!.length).toBeGreaterThan(0);

    const fromGate = formatPlanLimitError(
      new ApiError("request failed", 402, {
        code: "plan_feature_unavailable",
        gate: "sso",
      }),
    );
    expect(fromGate).toBeTypeOf("string");
    expect(fromGate!.length).toBeGreaterThan(0);
  });

  it("returns null for non-plan error codes and non-ApiError values", () => {
    expect(
      formatPlanLimitError(
        new ApiError("request failed", 403, { code: "viewer_read_only" }),
      ),
    ).toBeNull();
    expect(
      formatPlanLimitError(new ApiError("request failed", 500, { code: "internal" })),
    ).toBeNull();
    expect(formatPlanLimitError(new Error("network"))).toBeNull();
    expect(formatPlanLimitError(null)).toBeNull();
  });
});
