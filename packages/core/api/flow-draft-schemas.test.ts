// @vitest-environment node
import { describe, expect, it } from "vitest";
import { parseWithFallback } from "./schema";
import {
  CloseIssueFlowTaskResponseSchema,
  EMPTY_CLOSE_ISSUE_FLOW_TASK_RESPONSE,
  EMPTY_FLOW_CAPABILITIES_RESPONSE,
  EMPTY_FLOW_DRAFT,
  EMPTY_FLOW_MANIFEST_RESPONSE,
  EMPTY_FLOW_TRACE_RESPONSE,
  EMPTY_FLOW_VALIDATION_RESPONSE,
  EMPTY_ISSUE_FLOW_STATE,
  EMPTY_LIST_FLOW_DRAFTS_RESPONSE,
  FlowCapabilitiesResponseSchema,
  FlowDraftSchema,
  FlowManifestResponseSchema,
  FlowTraceResponseSchema,
  FlowValidationResponseSchema,
  IssueFlowStateSchema,
  ListFlowDraftsResponseSchema,
} from "./schemas";

// Criteria C13 and K12 of the flow spec: every response of the flow routes survives a missing field, a field
// of the wrong type, a list that is null and a value the build does not know, and never takes the screen down.
// Each block is one response; `bodies` are answers that are not an object at all.

const bodies: unknown[] = [null, undefined, "nope", 7, true, []];

const parse = <T>(schema: Parameters<typeof parseWithFallback<T>>[1], raw: unknown, fallback: T) =>
  parseWithFallback(raw, schema, fallback, { endpoint: "test" });

describe("flow draft (detail)", () => {
  const good = { id: "d-1", squad_id: "sq-1", name: "Flow", revision: 3, doc: { version: 2, start: "plan", steps: [], terminals: [] } };

  it("falls back to the empty draft, which has no id, when the answer is not an object", () => {
    for (const raw of bodies) expect(parse(FlowDraftSchema, raw, EMPTY_FLOW_DRAFT).id).toBe("");
  });

  it("fills what is missing", () => {
    const draft = FlowDraftSchema.parse({ id: "d-1" });
    expect(draft).toMatchObject({ id: "d-1", revision: 1, lineage_id: null, base_workflow_id: null, origin: "manual", name: "" });
    expect(draft.doc.version).toBe(2);
  });

  it("reads a field of the wrong type as its default", () => {
    const draft = FlowDraftSchema.parse({ ...good, revision: "three", lineage_id: 12, base_workflow_id: {}, name: 5, origin: null, created_at: 0 });
    expect(draft).toMatchObject({ id: "d-1", revision: 1, lineage_id: null, base_workflow_id: null, name: "", origin: "manual", created_at: "" });
  });

  it("reads a document that is not an object as the empty document, and keeps the lists it can read", () => {
    for (const doc of [null, "plan", 4, []]) expect(FlowDraftSchema.parse({ ...good, doc }).doc).toEqual(EMPTY_FLOW_DRAFT.doc);
    const doc = FlowDraftSchema.parse({ ...good, doc: { version: 2, start: "a", steps: null, terminals: "x", roles: 3, vars: {}, loops: [1, "b"] } }).doc;
    expect(doc.steps).toEqual([]);
    expect(doc.terminals).toEqual([]);
    expect(doc.roles).toEqual([]);
    expect(doc.loops).toEqual([]);
  });

  it("drops a step it cannot read and keeps the others, and keeps a kind it does not know as text", () => {
    const doc = FlowDraftSchema.parse({
      ...good,
      doc: {
        version: 2,
        start: "a",
        terminals: [],
        steps: [null, "x", { key: "a", kind: "subflow", edges: null, tasks: [{ key: "t", kind: "robot", role: "r" }, { key: "u", role: 7 }, 3] }],
      },
    }).doc;
    expect(doc.steps).toHaveLength(1);
    expect(doc.steps[0]).toMatchObject({ key: "a", kind: "subflow", edges: [] });
    // A task of a kind it does not know is kept as text; one it cannot read at all is left out.
    expect(doc.steps[0]?.tasks).toHaveLength(1);
    expect(doc.steps[0]?.tasks?.[0]).toMatchObject({ key: "t", kind: "robot" });
  });

  it("keeps a field it does not know on the draft, so a newer server's answer is not cut", () => {
    expect(FlowDraftSchema.parse({ ...good, future_field: { a: 1 } })).toMatchObject({ future_field: { a: 1 } });
  });
});

describe("flow draft list", () => {
  it("falls back to an empty list when the answer is not an object", () => {
    for (const raw of bodies) expect(parse(ListFlowDraftsResponseSchema, raw, EMPTY_LIST_FLOW_DRAFTS_RESPONSE)).toEqual(EMPTY_LIST_FLOW_DRAFTS_RESPONSE);
  });

  it("reads drafts that is null, or not a list, as none", () => {
    expect(ListFlowDraftsResponseSchema.parse({ drafts: null, total: 0 }).drafts).toEqual([]);
    expect(ListFlowDraftsResponseSchema.parse({ drafts: "many", total: "x" })).toMatchObject({ drafts: [], total: 0 });
  });

  it("drops an entry it cannot read, and fills the counts of one without a summary", () => {
    const list = ListFlowDraftsResponseSchema.parse({ drafts: [null, 3, { id: "d-1", name: "A", revision: "x" }, { id: "d-2", summary: { steps: "n", tasks: 4 } }] });
    expect(list.drafts.map((d) => d.id)).toEqual(["d-1", "d-2"]);
    expect(list.drafts[0]).toMatchObject({ revision: 1, summary: { steps: 0, tasks: 0, gates: 0 } });
    expect(list.drafts[1]?.summary).toEqual({ steps: 0, tasks: 4, gates: 0 });
  });
});

describe("flow validation (check)", () => {
  it("falls back to an answer that says it is not ok when the answer is not an object", () => {
    for (const raw of bodies) {
      const out = parse(FlowValidationResponseSchema, raw, EMPTY_FLOW_VALIDATION_RESPONSE);
      expect(out).toMatchObject({ ok: false, issues: [], analysis: null, fact_catalog: {} });
    }
  });

  it("reads each field of the wrong type as its default", () => {
    const out = FlowValidationResponseSchema.parse({ ok: "yes", revision: "4", issues: "many", analysis: "deep", fact_catalog: [] });
    expect(out).toMatchObject({ ok: false, revision: 0, issues: [], analysis: null, fact_catalog: {} });
  });

  it("drops an issue it cannot read and keeps the rest, with a severity it does not know as a warning", () => {
    const out = FlowValidationResponseSchema.parse({
      ok: false,
      issues: [null, "x", { code: "a", severity: "catastrophe", message: "m" }, { code: 5, severity: "error" }, { code: "b", severity: "error", params: "p", ack: 3 }],
    });
    expect(out.issues.map((i) => [i.code, i.severity])).toEqual([["a", "warning"], ["b", "error"]]);
    expect(out.issues[1]?.params).toBeUndefined();
    expect(out.issues[1]?.ack).toBeUndefined();
  });

  it("reads an analysis with lists that are null, and a catalog of facts that is not a list", () => {
    const out = FlowValidationResponseSchema.parse({
      ok: true,
      analysis: { worst_case_task_issues: "many", steps: null, loops: [{ key: "fix", max: "x" }], stops: [3], release_paths_without_human: null },
      fact_catalog: { "edge:gate": [{ key: "join.result", type: "enum", values: ["pass", 3] }, null], "task_when:c": "none" },
    });
    expect(out.analysis).toMatchObject({ worst_case_task_issues: 0, steps: [], stops: [], release_paths_without_human: [] });
    expect(out.analysis?.loops[0]).toMatchObject({ key: "fix", max: 0 });
    expect(out.fact_catalog["edge:gate"]).toHaveLength(1);
    expect(out.fact_catalog["task_when:c"]).toEqual([]);
  });
});

describe("flow trace (simulate)", () => {
  it("falls back to a trace that stalled when the answer is not an object", () => {
    for (const raw of bodies) expect(parse(FlowTraceResponseSchema, raw, EMPTY_FLOW_TRACE_RESPONSE)).toEqual(EMPTY_FLOW_TRACE_RESPONSE);
  });

  it("reads hops that are null as none, an end that is missing as stalled, and a truncated flag of the wrong type as false", () => {
    expect(FlowTraceResponseSchema.parse({ hops: null, end: null, truncated: "yes" })).toMatchObject({ hops: [], end: { kind: "stalled" }, truncated: false });
  });

  it("keeps an end it does not know as text and drops a hop it cannot read", () => {
    const out = FlowTraceResponseSchema.parse({
      hops: [{ step: "a", activation: "x", closed: "no", loops: [1] }, 4, null],
      end: { kind: "waiting", code: 7 },
    });
    expect(out.end).toMatchObject({ kind: "waiting" });
    expect(out.end.code).toBeUndefined();
    expect(out.hops).toHaveLength(1);
    expect(out.hops[0]).toMatchObject({ step: "a", activation: 1 });
  });
});

describe("flow manifest (export)", () => {
  it("falls back to an empty manifest when the answer is not an object", () => {
    for (const raw of bodies) expect(parse(FlowManifestResponseSchema, raw, EMPTY_FLOW_MANIFEST_RESPONSE).format).toBe("multica.flow-manifest/v1");
  });

  it("reads a missing or malformed field as its default", () => {
    expect(FlowManifestResponseSchema.parse({ format: 3, name: null, doc: "x" })).toMatchObject({
      format: "multica.flow-manifest/v1",
      name: "",
      doc: EMPTY_FLOW_DRAFT.doc,
    });
  });
});

describe("flow capabilities", () => {
  it("falls back to the defaults, with authoring off, when the answer is not an object", () => {
    for (const raw of bodies) expect(parse(FlowCapabilitiesResponseSchema, raw, EMPTY_FLOW_CAPABILITIES_RESPONSE).features.authoring).toBe(false);
  });

  it("reads each list and object of the wrong type as its default", () => {
    const out = FlowCapabilitiesResponseSchema.parse({
      schemas: "2",
      limits: "big",
      step_kinds: null,
      task_kinds: [1, 2],
      features: "all",
      ops_by_type: { bool: "eq", int: ["eq", "lt"] },
      builtin_facts: [null, { key: "join.result", type: "enum", ops: "eq" }],
    });
    expect(out.schemas).toEqual([1, 2]);
    expect(out.limits.steps).toBe(40);
    expect(out.step_kinds).toEqual(["tasks", "lead", "manual"]);
    expect(out.task_kinds).toEqual(["work", "gate"]);
    expect(out.features.authoring).toBe(false);
    expect(out.ops_by_type).toEqual({});
    expect(out.builtin_facts).toHaveLength(1);
    expect(out.builtin_facts[0]?.ops).toEqual([]);
  });

  it("keeps a kind or a feature it does not know, so the screen can say it is unsupported", () => {
    const out = FlowCapabilitiesResponseSchema.parse({ step_kinds: ["tasks", "subflow"], features: { authoring: true, teleport: true }, limits: { steps: 12, hyper: 1 } });
    expect(out.step_kinds).toContain("subflow");
    expect(out.features).toMatchObject({ authoring: true, teleport: true });
    expect(out.limits).toMatchObject({ steps: 12, hyper: 1 });
  });
});

describe("request state of a flow (extended)", () => {
  const base = { managed: true, role: "request", workflow_id: "wf", step: "gate", tasks: [], next: [] };

  it("falls back to the empty state when the answer is not an object", () => {
    for (const raw of bodies) expect(parse(IssueFlowStateSchema, raw, EMPTY_ISSUE_FLOW_STATE).managed).toBe(false);
  });

  it("reads the stop as an object, null, or something this build cannot read as stalled", () => {
    expect(IssueFlowStateSchema.parse({ ...base, stop: null }).stop).toBeNull();
    expect(IssueFlowStateSchema.parse(base).stop).toBeNull();
    expect(IssueFlowStateSchema.parse({ ...base, stop: { code: "loop_exhausted", legacy_no: 3, from: "gate", edge: "e", at: "t" } }).stop).toEqual({
      code: "loop_exhausted",
      legacy_no: 3,
      from: "gate",
      edge: "e",
      at: "t",
    });
    // An older server's number, or a text: the request is parked, so it reads as stalled.
    for (const stop of [1, "stalled", true, ["x"]]) {
      expect(IssueFlowStateSchema.parse({ ...base, stop }).stop).toMatchObject({ code: "stalled", legacy_no: 0 });
    }
    expect(IssueFlowStateSchema.parse({ ...base, stop: { code: 4, legacy_no: "x" } }).stop).toMatchObject({ code: "stalled", legacy_no: 0 });
  });

  it("reads the new fields of the wrong type as their defaults", () => {
    const out = IssueFlowStateSchema.parse({ ...base, schema: "2", loops: [], activations: "x", vars: null, facts: 3, checkpoints: {}, unreviewed: "no", trail: "path", trial: 1 });
    expect(out).toMatchObject({ schema: 1, loops: {}, activations: {}, vars: {}, facts: {}, checkpoints: [], unreviewed: false, trail: [], trial: false });
  });

  it("drops a checkpoint or a trail entry it cannot read, and keeps the others", () => {
    const out = IssueFlowStateSchema.parse({
      ...base,
      checkpoints: [null, { step: "a", activation: "x", by: 9 }, "z"],
      trail: [{ step: "a", activation: 2, edge: "e1" }, 7, { step: 3 }],
    });
    expect(out.checkpoints).toHaveLength(1);
    expect(out.checkpoints[0]).toMatchObject({ step: "a", activation: 0 });
    expect(out.checkpoints[0]?.by).toBeUndefined();
    expect(out.trail?.map((e) => e.step)).toEqual(["a"]);
  });

  it("reads the viewer and who closed a task, each falling back to nothing to do", () => {
    expect(IssueFlowStateSchema.parse({ ...base, viewer: { can_manage: "yes", closable_task_ids: [1, "t1"] } }).viewer).toEqual({ can_manage: false, closable_task_ids: [] });
    const out = IssueFlowStateSchema.parse({
      ...base,
      tasks: [{ task: "docs", closed_by: { type: 1, id: null, on_behalf: "yes" } }, { task: "gate", closed_by: "someone" }],
    });
    // An unreadable closer never hides the task: it reads as nobody known.
    expect(out.tasks).toHaveLength(2);
    expect(out.tasks[0]?.closed_by).toBeNull();
    expect(out.tasks[1]?.closed_by).toBeNull();
  });

  it("reads the new fields of a task of the wrong type as their defaults, and drops a task without a key", () => {
    const out = IssueFlowStateSchema.parse({
      ...base,
      tasks: [
        { task: "docs", instance: "2", class: 5, slot_status: [null, { slot: "report", min: "1", count: 2 }], inputs: "none", feedback: [3, { issue_id: "i", task: "t" }], sets: "score" },
        { instance: 1 },
      ],
    });
    expect(out.tasks).toHaveLength(1);
    expect(out.tasks[0]).toMatchObject({ instance: 0, class: "", inputs: [], sets: [] });
    expect(out.tasks[0]?.slot_status).toEqual([{ slot: "report", min: 0, max: 0, count: 2 }]);
    expect(out.tasks[0]?.feedback).toHaveLength(1);
  });

  it("reads the answer of an on-behalf close, with either half unreadable", () => {
    for (const raw of bodies) expect(parse(CloseIssueFlowTaskResponseSchema, raw, EMPTY_CLOSE_ISSUE_FLOW_TASK_RESPONSE).state.managed).toBe(false);
    const out = CloseIssueFlowTaskResponseSchema.parse({ task: { task: "docs_gate", closed_by: { type: "member", id: "u", on_behalf: true } }, state: { managed: true, stop: null } });
    expect(out.task.closed_by).toEqual({ type: "member", id: "u", on_behalf: true });
    expect(out.state.managed).toBe(true);
  });
});
