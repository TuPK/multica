// @vitest-environment node
import { describe, expect, it } from "vitest";
import { parseWithFallback } from "./schema";
import {
  EMPTY_FLOW_INSIGHTS,
  EMPTY_PACK_COMMIT_RESPONSE,
  EMPTY_PACK_PREVIEW,
  FlowCapabilitiesResponseSchema,
  FlowInsightsSchema,
  PackCommitResponseSchema,
  PackPreviewSchema,
} from "./schemas";

// The R3 responses (spec 16.1 to 16.4, 16.6): a missing field, a field of the wrong type, a list that is null and a
// value the build does not know never take the screen down, and an answer that is not an object reads as empty.

const bodies: unknown[] = [null, undefined, "nope", 7, true, []];
const parse = <T>(schema: Parameters<typeof parseWithFallback<T>>[1], raw: unknown, fallback: T) =>
  parseWithFallback(raw, schema, fallback, { endpoint: "test" });

describe("PackPreview", () => {
  const good = {
    digest: "sha256:" + "a".repeat(64),
    format: "native_bundle",
    id: "kv-writing",
    name: "Writing pack",
    version: "1.2.0",
    agents: [{ key: "writer", name: "Writer", description: "d", instructions_bytes: 120, skills: ["style"], skills_declared: true, existing_agent_id: null }],
    skills: [{ name: "style", files: 2, existing_skill_id: null, same_content: false }],
    leader: "writer",
    flow: { present: false },
    references: [{ path: "docs/procedure.md", bytes: 900, kind: "procedure" }],
    ignored: [{ agent: "writer", key: "model" }],
    scan: [{ path: "skills/style/SKILL.md", severity: "warn", rule: "shell_pipe" }],
    issues: [],
  };

  it("falls back to the empty preview, which has no digest, when the answer is not an object", () => {
    for (const raw of bodies) expect(parse(PackPreviewSchema, raw, EMPTY_PACK_PREVIEW).digest).toBe("");
  });

  it("reads a good answer as it is", () => {
    const preview = PackPreviewSchema.parse(good);
    expect(preview).toMatchObject({ name: "Writing pack", leader: "writer", flow: { present: false } });
    expect(preview.agents[0]).toMatchObject({ key: "writer", skills: ["style"], skills_declared: true, existing_agent_id: null });
    expect(preview.skills[0]).toMatchObject({ name: "style", files: 2, same_content: false });
    expect(preview.ignored).toEqual([{ agent: "writer", key: "model" }]);
    expect(preview.scan).toEqual([{ path: "skills/style/SKILL.md", severity: "warn", rule: "shell_pipe" }]);
  });

  it("fills what is missing with lists that are empty and no leader", () => {
    const preview = PackPreviewSchema.parse({ digest: "sha256:x" });
    expect(preview).toMatchObject({ agents: [], skills: [], leader: null, references: [], ignored: [], scan: [], issues: [], flow: { present: false } });
  });

  it("reads a list that is null or of the wrong type as an empty list and drops the items it cannot read", () => {
    const preview = PackPreviewSchema.parse({ ...good, agents: null, skills: "x", ignored: {}, scan: [1, null, { path: "a", severity: "block", rule: "r" }], references: [7] });
    expect(preview.agents).toEqual([]);
    expect(preview.skills).toEqual([]);
    expect(preview.ignored).toEqual([]);
    expect(preview.scan).toEqual([{ path: "a", severity: "block", rule: "r" }]);
    expect(preview.references).toEqual([]);
  });

  it("reads a field of the wrong type as its default", () => {
    const preview = PackPreviewSchema.parse({
      ...good,
      name: 4,
      leader: 12,
      agents: [{ key: "a", name: "A", instructions_bytes: "many", skills: "x", skills_declared: "yes", existing_agent_id: 3 }],
      skills: [{ name: "s", files: "two", existing_skill_id: {}, same_content: "yes" }],
    });
    expect(preview.name).toBe("");
    expect(preview.leader).toBeNull();
    expect(preview.agents[0]).toMatchObject({ instructions_bytes: 0, skills: [], skills_declared: false, existing_agent_id: null });
    expect(preview.skills[0]).toMatchObject({ files: 0, existing_skill_id: null, same_content: false });
  });

  it("reads a severity it does not know as a warning, so that the person has to look", () => {
    const preview = PackPreviewSchema.parse({
      ...good,
      scan: [{ path: "a", severity: "catastrophic", rule: "r" }],
      issues: [{ code: "pack_x", severity: "fatal", message: "m" }],
    });
    expect(preview.scan[0]!.severity).toBe("warn");
    expect(preview.issues[0]!.severity).toBe("warning");
  });

  it("keeps the check of the flow of the pack when it can be read and drops it when it cannot", () => {
    const kept = PackPreviewSchema.parse({ ...good, flow: { present: true, validation: { ok: false, revision: 0, issues: [{ code: "role_unused", severity: "error", message: "x" }] }, issues: [{ code: "pack_flow_role_unmatched", severity: "warning", message: "m" }] } });
    expect(kept.flow.present).toBe(true);
    expect(kept.flow.validation?.issues[0]).toMatchObject({ code: "role_unused" });
    expect(kept.flow.issues).toHaveLength(1);
    const dropped = PackPreviewSchema.parse({ ...good, flow: { present: "yes", validation: "broken", issues: 4 } });
    expect(dropped.flow).toMatchObject({ present: false, validation: null, issues: [] });
  });
});

describe("PackCommitResponse", () => {
  it("falls back to the empty result, which has no install id, when the answer is not an object", () => {
    for (const raw of bodies) expect(parse(PackCommitResponseSchema, raw, EMPTY_PACK_COMMIT_RESPONSE).install_id).toBe("");
  });

  it("reads a good answer and fills what is missing", () => {
    const done = PackCommitResponseSchema.parse({
      install_id: "i-1",
      already_installed: true,
      squad_id: "sq-1",
      agents: [{ key: "writer", agent_id: "a-1", created: true }],
      skills: [{ name: "style", skill_id: "s-1", created: false }],
      draft_id: "d-1",
      flow_issues: [{ code: "role_unused", severity: "error", message: "x" }],
    });
    expect(done).toMatchObject({ install_id: "i-1", already_installed: true, squad_id: "sq-1", draft_id: "d-1" });
    expect(done.agents[0]).toEqual({ key: "writer", agent_id: "a-1", created: true });
    expect(done.flow_issues).toHaveLength(1);
    expect(PackCommitResponseSchema.parse({ install_id: "i-2" })).toMatchObject({ already_installed: false, squad_id: null, agents: [], skills: [], draft_id: null, flow_issues: [] });
  });

  it("reads wrong types as defaults and drops the rows it cannot read", () => {
    const done = PackCommitResponseSchema.parse({ install_id: "i-1", already_installed: "yes", squad_id: 4, agents: [3, { key: "a", agent_id: "x", created: "y" }], skills: null, draft_id: {}, flow_issues: "many" });
    expect(done).toMatchObject({ already_installed: false, squad_id: null, skills: [], draft_id: null, flow_issues: [] });
    expect(done.agents).toEqual([{ key: "a", agent_id: "x", created: false }]);
  });
});

describe("flow insights", () => {
  const good = {
    requests: { total: 42, open: 5, by_end: { "terminal:done": 30, "stop:loop_exhausted": 4, stalled: 1 } },
    steps: [{ key: "gate", entered: 51, median_minutes: 38, p90_minutes: 190 }],
    edges: [{ step: "gate", edge: "gate_fix", taken: 14 }],
    loops: [{ key: "fix", max: 2, histogram: [28, 10, 4] }],
    stalls: [{ reason: "flow_agent_unavailable", count: 1 }],
    manual_closes: 3,
    truncated: false,
  };

  it("falls back to the report with no requests when the answer is not an object", () => {
    for (const raw of bodies) expect(parse(FlowInsightsSchema, raw, EMPTY_FLOW_INSIGHTS)).toEqual(EMPTY_FLOW_INSIGHTS);
  });

  it("reads a good answer as it is", () => {
    expect(FlowInsightsSchema.parse(good)).toEqual(good);
  });

  it("keeps a step without a duration as null and reads wrong types as defaults", () => {
    const report = FlowInsightsSchema.parse({
      ...good,
      requests: { total: "many", open: null, by_end: { a: "x", b: 2 } },
      steps: [{ key: "plan", entered: 6, median_minutes: null, p90_minutes: "slow" }, 4],
      loops: [{ key: "fix", max: "x", histogram: [1, "two", 3] }],
      manual_closes: "3",
      truncated: "yes",
    });
    expect(report.requests).toEqual({ total: 0, open: 0, by_end: { b: 2 } });
    expect(report.steps).toEqual([{ key: "plan", entered: 6, median_minutes: null, p90_minutes: null }]);
    expect(report.loops).toEqual([{ key: "fix", max: 0, histogram: [1, 0, 3] }]);
    expect(report.manual_closes).toBe(0);
    expect(report.truncated).toBe(false);
  });

  it("reads lists that are null as empty lists", () => {
    const report = FlowInsightsSchema.parse({ requests: null, steps: null, edges: "x", loops: {}, stalls: null });
    expect(report).toEqual(EMPTY_FLOW_INSIGHTS);
  });
});

describe("capabilities features", () => {
  it("reads insights, and takes it as off when the server does not say", () => {
    expect(FlowCapabilitiesResponseSchema.parse({ features: { insights: true } }).features.insights).toBe(true);
    expect(FlowCapabilitiesResponseSchema.parse({ features: { authoring: true } }).features.insights).toBe(false);
    expect(FlowCapabilitiesResponseSchema.parse({ features: { insights: "yes" } }).features.insights).toBe(false);
  });
});
