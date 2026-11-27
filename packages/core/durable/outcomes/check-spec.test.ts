// @vitest-environment node
import { describe, expect, it } from "vitest";
import { checkSpecFor, emptyCheckSpecFields, type CheckSpecFields } from "./check-spec";

function fields(overrides: Partial<CheckSpecFields> = {}): CheckSpecFields {
  return { ...emptyCheckSpecFields(), ...overrides };
}

describe("checkSpecFor", () => {
  it("sends no spec for a manual check", () => {
    expect(checkSpecFor("manual", fields())).toEqual({ ok: true, spec: undefined });
  });

  it("needs a rubric of at most 2000 characters for an agent judgment", () => {
    expect(checkSpecFor("agent_judgment", fields({ rubric: "   " }))).toEqual({ ok: false });
    expect(checkSpecFor("agent_judgment", fields({ rubric: "Clear" }))).toEqual({ ok: true, spec: { rubric: "Clear" } });
    expect(checkSpecFor("agent_judgment", fields({ rubric: "x".repeat(2001) }))).toEqual({ ok: false });
    expect(checkSpecFor("agent_judgment", fields({ rubric: "x".repeat(2000) })).ok).toBe(true);
  });

  it("needs a command, and an expected exit code only when one is typed", () => {
    expect(checkSpecFor("command", fields({ run: " " }))).toEqual({ ok: false });
    expect(checkSpecFor("command", fields({ run: "npm test" }))).toEqual({ ok: true, spec: { run: "npm test" } });
    expect(checkSpecFor("command", fields({ run: "npm test", expectExit: "0" }))).toEqual({
      ok: true,
      spec: { run: "npm test", expect_exit: 0 },
    });
    expect(checkSpecFor("command", fields({ run: "npm test", expectExit: "-1" }))).toEqual({
      ok: true,
      spec: { run: "npm test", expect_exit: -1 },
    });
    expect(checkSpecFor("command", fields({ run: "npm test", expectExit: "1.5" }))).toEqual({ ok: false });
    expect(checkSpecFor("command", fields({ run: "npm test", expectExit: "one" }))).toEqual({ ok: false });
  });

  it("needs a document kind for an artifact", () => {
    expect(checkSpecFor("artifact", fields({ artifactKind: "" }))).toEqual({ ok: false });
    expect(checkSpecFor("artifact", fields({ artifactKind: " spec " }))).toEqual({ ok: true, spec: { kind: "spec" } });
  });

  it("sends the company metric the server expects, with a whole window of 1 to 365 days", () => {
    const valid = fields({ metricKey: "revenue", metricValue: "1000", withinDays: "30" });
    expect(checkSpecFor("metric", valid)).toEqual({
      ok: true,
      spec: { metric_key: "revenue", scope: "company", op: ">=", value: 1000, within_days: 30 },
    });
    expect(checkSpecFor("metric", { ...valid, metricKey: "" })).toEqual({ ok: false });
    expect(checkSpecFor("metric", { ...valid, metricValue: "" })).toEqual({ ok: false });
    expect(checkSpecFor("metric", { ...valid, metricValue: "lots" })).toEqual({ ok: false });
    expect(checkSpecFor("metric", { ...valid, withinDays: "0" })).toEqual({ ok: false });
    expect(checkSpecFor("metric", { ...valid, withinDays: "366" })).toEqual({ ok: false });
    expect(checkSpecFor("metric", { ...valid, withinDays: "2.5" })).toEqual({ ok: false });
  });
});
