// @vitest-environment node
import { describe, expect, it } from "vitest";
import { parseWithFallback } from "../api/schema";
import {
  EMPTY_SQUAD,
  EMPTY_SQUAD_WORKFLOWS_RESPONSE,
  IssueFlowStateSchema,
  IssueSchema,
  IssueWorkflowSchema,
  SquadListSchema,
  SquadSchema,
  SquadWorkflowsResponseSchema,
} from "../api/schemas";

// Responses that carry the enforced workflow engine's fields (S4 of
// docs/engineering/kitviet-workflow-plan.md). Every new field is optional on
// the wire: an older server omits it, a newer one may send a value this build
// does not know, and neither may blank a list around it.

const baseWorkflow = {
  id: "wf-1",
  workspace_id: "ws-1",
  name: "Trade",
  description: "",
  is_default: false,
  created_at: "2026-11-13T00:00:00Z",
  updated_at: "2026-11-13T00:00:00Z",
};

const baseSquad = {
  id: "squad-1",
  workspace_id: "ws-1",
  name: "Trade",
  description: "",
  instructions: "",
  avatar_url: null,
  leader_id: "agent-1",
  creator_id: "user-1",
  created_at: "2026-11-13T00:00:00Z",
  updated_at: "2026-11-13T00:00:00Z",
  archived_at: null,
  archived_by: null,
};

const baseIssue = {
  id: "11111111-1111-1111-1111-111111111111",
  workspace_id: "ws-1",
  number: 1,
  identifier: "MUL-1",
  title: "Request",
  description: null,
  status: "todo",
  priority: "medium",
  assignee_type: null,
  assignee_id: null,
  creator_type: "member",
  creator_id: "user-1",
  parent_issue_id: null,
  project_id: null,
  position: 0,
  stage: null,
  start_date: null,
  due_date: null,
  metadata: {},
  created_at: "2026-11-13T00:00:00Z",
  updated_at: "2026-11-13T00:00:00Z",
};

describe("IssueWorkflowSchema flow fields", () => {
  it("defaults every flow field on a workflow from an older server", () => {
    const parsed = IssueWorkflowSchema.parse({ ...baseWorkflow, steps: [{ status_key: "todo" }] });
    expect(parsed.flow_config).toBeNull();
    expect(parsed.template_ref).toBeNull();
    expect(parsed.flow_auto).toBe(false);
    expect(parsed.squad_id).toBeNull();
    expect(parsed.open_flow_requests).toBe(0);
    expect(parsed.steps[0]?.tasks).toEqual([]);
    expect(parsed.steps[0]?.human_only).toBe(false);
  });

  it("reads a flow workflow with its tasks and switches", () => {
    const parsed = IssueWorkflowSchema.parse({
      ...baseWorkflow,
      squad_id: "squad-1",
      template_ref: "flow_kv_trade@a1b2c3d4",
      flow_auto: true,
      open_flow_requests: 3,
      flow_config: {
        version: 1,
        task_timeout_minutes: 120,
        switches: [
          { key: "route", type: "enum", values: ["shipment_dossier"], default: "shipment_dossier" },
          { key: "export", type: "bool", default: false },
        ],
        messages: { stop1: "BLOCKED" },
      },
      steps: [
        {
          status_key: "gate",
          handler: { type: "none" },
          human_only: false,
          tasks: [
            {
              key: "docs_gate",
              title: "Documents gate",
              kind: "gate",
              handler: { type: "agent", id: "agent-2" },
              instructions: "Check.",
              requires: ["outcome"],
              when: { field: "switch.export", op: "eq", value: true },
            },
          ],
        },
      ],
    });
    expect(parsed.template_ref).toBe("flow_kv_trade@a1b2c3d4");
    expect(parsed.flow_auto).toBe(true);
    expect(parsed.open_flow_requests).toBe(3);
    expect(parsed.flow_config?.switches).toHaveLength(2);
    expect(parsed.flow_config?.switches[0]).toMatchObject({ key: "route", type: "enum", values: ["shipment_dossier"] });
    const task = parsed.steps[0]?.tasks[0];
    expect(task).toMatchObject({ key: "docs_gate", kind: "gate", requires: ["outcome"] });
    expect(task?.handler).toEqual({ type: "agent", id: "agent-2" });
    expect(task?.when).toEqual({ field: "switch.export", op: "eq", value: true });
  });

  it("turns a null tasks list into an empty one", () => {
    const parsed = IssueWorkflowSchema.parse({ ...baseWorkflow, steps: [{ status_key: "todo", tasks: null }] });
    expect(parsed.steps[0]?.tasks).toEqual([]);
  });

  it("reads an unknown task kind as work and keeps the task", () => {
    const parsed = IssueWorkflowSchema.parse({
      ...baseWorkflow,
      steps: [{ status_key: "draft", tasks: [{ key: "a", title: "A", kind: "review", handler: { type: "agent", id: "x" } }] }],
    });
    expect(parsed.steps[0]?.tasks[0]?.kind).toBe("work");
    expect(parsed.steps[0]?.tasks[0]?.requires).toEqual([]);
  });

  it("drops only a task it cannot read", () => {
    const parsed = IssueWorkflowSchema.parse({
      ...baseWorkflow,
      steps: [{ status_key: "draft", tasks: [{ title: "no key" }, { key: "ok", title: "Ok", kind: "work", handler: { type: "agent", id: "x" } }] }],
    });
    expect(parsed.steps[0]?.tasks.map((t) => t.key)).toEqual(["ok"]);
  });

  it("keeps the workflow when flow_config is malformed", () => {
    const parsed = IssueWorkflowSchema.parse({ ...baseWorkflow, flow_config: "nope", steps: [{ status_key: "todo" }] });
    expect(parsed.id).toBe("wf-1");
    expect(parsed.flow_config).toBeNull();
  });

  it("falls back for fields of the wrong type without losing the workflow", () => {
    const parsed = IssueWorkflowSchema.parse({
      ...baseWorkflow,
      flow_auto: "yes",
      open_flow_requests: "3",
      template_ref: 7,
      steps: [{ status_key: "todo", human_only: "true" }],
    });
    expect(parsed.flow_auto).toBe(false);
    expect(parsed.open_flow_requests).toBe(0);
    expect(parsed.template_ref).toBeNull();
    expect(parsed.steps[0]?.human_only).toBe(false);
  });

  it("keeps a switch condition on a task whose field this build does not know", () => {
    const parsed = IssueWorkflowSchema.parse({
      ...baseWorkflow,
      steps: [{ status_key: "collect", tasks: [{ key: "a", title: "A", kind: "work", handler: { type: "agent", id: "x" }, when: { field: "switch.later", op: "eq", value: "z" } }] }],
    });
    expect(parsed.steps[0]?.tasks[0]?.when).toEqual({ field: "switch.later", op: "eq", value: "z" });
  });
});

describe("SquadSchema workflow fields", () => {
  it("defaults the four fields when an older server omits them", () => {
    const parsed = SquadSchema.parse(baseSquad);
    expect(parsed.active_workflow_id).toBeNull();
    expect(parsed.active_workflow_name).toBeNull();
    expect(parsed.workflow_count).toBe(0);
    expect(parsed.flow_enforced).toBe(false);
    expect(SquadListSchema.parse([baseSquad])[0]?.workflow_count).toBe(0);
  });

  it("reads the active workflow and the library size", () => {
    const parsed = SquadSchema.parse({
      ...baseSquad,
      active_workflow_id: "wf-1",
      active_workflow_name: "Trade",
      workflow_count: 2,
      flow_enforced: true,
    });
    expect(parsed).toMatchObject({ active_workflow_id: "wf-1", active_workflow_name: "Trade", workflow_count: 2, flow_enforced: true });
  });

  it("keeps the squad and the list when a field has the wrong type", () => {
    const parsed = SquadListSchema.parse([
      { ...baseSquad, workflow_count: "3", flow_enforced: "yes", active_workflow_id: 9, active_workflow_name: {} },
      { ...baseSquad, id: "squad-2" },
    ]);
    expect(parsed).toHaveLength(2);
    expect(parsed[0]).toMatchObject({ workflow_count: 0, flow_enforced: false, active_workflow_id: null, active_workflow_name: null });
  });

  it("carries the new fields in the empty fallback", () => {
    expect(EMPTY_SQUAD.workflow_count).toBe(0);
    expect(EMPTY_SQUAD.flow_enforced).toBe(false);
    expect(EMPTY_SQUAD.active_workflow_id).toBeNull();
  });
});

describe("IssueSchema flow fields", () => {
  it("defaults outcome and flow when an older server omits them", () => {
    const parsed = IssueSchema.parse(baseIssue);
    expect(parsed.outcome).toBeNull();
    expect(parsed.flow).toBeNull();
  });

  it("reads the outcome and the flow reference of a request and of a task", () => {
    const request = IssueSchema.parse({
      ...baseIssue,
      flow: { role: "request", step: "gate", workflow_id: "wf-1", rework: 1, stop: 3 },
    });
    expect(request.flow).toEqual({ role: "request", step: "gate", workflow_id: "wf-1", rework: 1, stop: 3 });
    const task = IssueSchema.parse({
      ...baseIssue,
      outcome: "approved",
      flow: { role: "task", step: "gate", task: "docs_gate", round: 1 },
    });
    expect(task.outcome).toBe("approved");
    expect(task.flow).toEqual({ role: "task", step: "gate", task: "docs_gate", round: 1 });
  });

  it("keeps an outcome this build does not know", () => {
    expect(IssueSchema.parse({ ...baseIssue, outcome: "deferred" }).outcome).toBe("deferred");
  });

  it("reads an unknown stop as stalled", () => {
    const parsed = IssueSchema.parse({ ...baseIssue, flow: { role: "request", step: "gate", stop: "later" } });
    expect(parsed.flow?.stop).toBe("stalled");
  });

  it("drops only a malformed flow, keeping the issue", () => {
    for (const bad of ["x", 4, [], { step: 3 }, { role: "request" }]) {
      const parsed = IssueSchema.parse({ ...baseIssue, flow: bad, outcome: 5 });
      expect(parsed.id).toBe(baseIssue.id);
      expect(parsed.flow).toBeNull();
      expect(parsed.outcome).toBeNull();
    }
  });
});

describe("SquadWorkflowsResponseSchema", () => {
  it("reads a library with its active workflow", () => {
    const parsed = SquadWorkflowsResponseSchema.parse({
      active_workflow_id: "wf-1",
      workflows: [{ ...baseWorkflow, steps: [{ status_key: "todo" }] }],
      total: 1,
    });
    expect(parsed.active_workflow_id).toBe("wf-1");
    expect(parsed.workflows[0]?.id).toBe("wf-1");
    expect(parsed.total).toBe(1);
  });

  it("handles a squad with no active workflow", () => {
    const parsed = SquadWorkflowsResponseSchema.parse({ active_workflow_id: null, workflows: [], total: 0 });
    expect(parsed).toEqual({ active_workflow_id: null, workflows: [], total: 0 });
  });

  it("defaults a missing list and a wrong-typed total", () => {
    const parsed = SquadWorkflowsResponseSchema.parse({ total: "two" });
    expect(parsed.active_workflow_id).toBeNull();
    expect(parsed.workflows).toEqual([]);
    expect(parsed.total).toBe(0);
  });

  it("drops a workflow it cannot read and keeps the rest", () => {
    const parsed = SquadWorkflowsResponseSchema.parse({
      active_workflow_id: "wf-1",
      workflows: [{ id: 4 }, { ...baseWorkflow, steps: [] }],
      total: 2,
    });
    expect(parsed.workflows.map((w) => w.id)).toEqual(["wf-1"]);
  });

  it("falls back to the empty response on a body that is not an object", () => {
    const parsed = parseWithFallback("nope", SquadWorkflowsResponseSchema, EMPTY_SQUAD_WORKFLOWS_RESPONSE, {
      endpoint: "GET /api/squads/:id/workflows",
    });
    expect(parsed).toEqual(EMPTY_SQUAD_WORKFLOWS_RESPONSE);
  });
});

// The drafting step a stopped request can return to (rework_step). A server without the field, or
// one that sends a value this build cannot read, leaves it empty and does not blank the state.
describe("IssueFlowStateSchema rework_step", () => {
  const stoppedState = {
    managed: true,
    role: "request",
    workflow_id: "wf-1",
    workflow_name: "Trade",
    step: "blocked",
    rework: 0,
    stop: 1,
    switches: {},
    auto: false,
    entered_at: "2026-11-13T00:00:00Z",
    tasks: [],
    next: [],
  };

  it("reads the drafting step", () => {
    expect(IssueFlowStateSchema.parse({ ...stoppedState, rework_step: "draft" }).rework_step).toBe("draft");
  });

  it("defaults to empty from an older server", () => {
    const parsed = IssueFlowStateSchema.parse(stoppedState);
    expect(parsed.rework_step).toBe("");
    expect(parsed.stop).toBe(1);
  });

  it("falls back to empty for a value that is not a string, and keeps the rest of the state", () => {
    const parsed = IssueFlowStateSchema.parse({ ...stoppedState, rework_step: 3 });
    expect(parsed.rework_step).toBe("");
    expect(parsed.step).toBe("blocked");
  });
});
