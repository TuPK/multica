// @vitest-environment node
import { describe, expect, it } from "vitest";
import { parseWithFallback } from "../api/schema";
import {
  EMPTY_ISSUE_WORKFLOW_DRY_RUN,
  EMPTY_LIST_ISSUE_WORKFLOWS_RESPONSE,
  EMPTY_LIST_ISSUE_WORKFLOW_TEMPLATES_RESPONSE,
  ListIssueWorkflowTemplatesResponseSchema,
  EMPTY_LIST_ISSUE_WORKFLOW_USAGE_RESPONSE,
  IssueWorkflowDryRunResponseSchema,
  IssueWorkflowSchema,
  ListIssueWorkflowsResponseSchema,
  ListIssueWorkflowUsageResponseSchema,
  parseIssueWorkflowBlockers,
  WorkflowHandoffPreviewSchema,
  EMPTY_WORKFLOW_HANDOFF_PREVIEW,
} from "../api/schemas";

const baseWorkflow = {
  id: "wf-1",
  workspace_id: "ws-1",
  name: "Delivery",
  description: "",
  is_default: false,
  steps: [
    { status_key: "todo", handler: { type: "none" }, instructions: "" },
    {
      status_key: "code_review",
      handler: { type: "agent", id: "agent-1" },
      instructions: "Review the PR.",
      next_status_key: "done",
      back_status_key: "todo",
    },
  ],
  project_ids: ["project-1"],
  created_at: "2026-09-24T00:00:00Z",
  updated_at: "2026-09-24T00:00:00Z",
};

describe("workflow schemas (MUL-7420)", () => {
  it("parses a workflow list", () => {
    const parsed = ListIssueWorkflowsResponseSchema.parse({ workflows: [baseWorkflow], total: 1 });
    expect(parsed.workflows[0]?.steps[1]?.handler).toEqual({ type: "agent", id: "agent-1" });
    expect(parsed.workflows[0]?.project_ids).toEqual(["project-1"]);
  });

  it("falls back to an empty list on a malformed response", () => {
    const parsed = parseWithFallback(
      { workflows: "nope" },
      ListIssueWorkflowsResponseSchema,
      EMPTY_LIST_ISSUE_WORKFLOWS_RESPONSE,
      { endpoint: "GET /api/issue-workflows" },
    );
    expect(parsed).toEqual(EMPTY_LIST_ISSUE_WORKFLOWS_RESPONSE);
  });

  it("degrades an unknown handler type to a manual step", () => {
    const parsed = IssueWorkflowSchema.parse({
      ...baseWorkflow,
      steps: [{ status_key: "todo", handler: { type: "robot", id: "x" } }],
    });
    expect(parsed.steps[0]?.handler.type).toBe("none");
    expect(parsed.steps[0]?.instructions).toBe("");
  });

  it("defaults the optional fields an older server may omit", () => {
    const { description: _d, project_ids: _p, ...minimal } = baseWorkflow;
    const parsed = IssueWorkflowSchema.parse({ ...minimal, steps: [{ status_key: "todo" }] });
    expect(parsed.description).toBe("");
    expect(parsed.project_ids).toEqual([]);
    expect(parsed.steps[0]?.handler.type).toBe("none");
  });

  it("reads a workflow row's status-only steps as manual columns", () => {
    const parsed = IssueWorkflowSchema.parse({ ...baseWorkflow, is_default: true, steps: [{ status_key: "todo" }] });
    expect(parsed.is_default).toBe(true);
    expect(parsed.steps[0]).toEqual({
      status_key: "todo",
      handler: { type: "none" },
      instructions: "",
      transitions: [],
      tasks: [],
      human_only: false,
    });
  });

  it("falls back to an empty plan on a malformed dry run", () => {
    const parsed = parseWithFallback(
      { plan: { required: 5 } },
      IssueWorkflowDryRunResponseSchema,
      EMPTY_ISSUE_WORKFLOW_DRY_RUN,
      { endpoint: "POST /api/projects/{id}/workflow (dry run)" },
    );
    expect(parsed).toEqual(EMPTY_ISSUE_WORKFLOW_DRY_RUN);
  });

  it("parses a handoff preview and degrades an unknown handler type", () => {
    const parsed = WorkflowHandoffPreviewSchema.parse({
      handoff: true,
      workflow_name: "Delivery",
      from_status: "implement",
      to_status: "code_review",
      handler_type: "agent",
      handler_id: "agent-2",
      previous_assignee_type: "agent",
      previous_assignee_id: "agent-1",
      previous_runs: [{ task_id: "t1", agent_id: "agent-1", status: "running", started_at: "2026-09-24T00:00:00Z" }],
      brief: "This project's workflow — Delivery",
    });
    expect(parsed.handler_type).toBe("agent");
    expect(parsed.previous_runs).toHaveLength(1);
    expect(WorkflowHandoffPreviewSchema.parse({ handoff: true, handler_type: "robot" }).handler_type).toBeNull();
  });

  it("treats a malformed handoff preview as no handoff", () => {
    const parsed = parseWithFallback(
      { handoff: "yes", previous_runs: 3 },
      WorkflowHandoffPreviewSchema,
      EMPTY_WORKFLOW_HANDOFF_PREVIEW,
      { endpoint: "GET /api/issues/{id}/workflow-handoff" },
    );
    expect(parsed).toEqual(EMPTY_WORKFLOW_HANDOFF_PREVIEW);
  });
});

describe("workflow transitions (workflow-tab §3)", () => {
  const routedStep = {
    status_key: "in_review",
    handler: { type: "agent", id: "agent-1" },
    instructions: "Review the change.",
    transitions: [
      { id: "urgent", label: "Urgent", when: { field: "priority", op: "gte", value: "urgent" }, to_status_key: "blocked" },
      { id: "rejected", label: "Rejected", when: { field: "outcome", op: "eq", value: "rejected" }, to_status_key: "in_progress" },
      { id: "default", label: "Default", when: null, to_status_key: "done" },
    ],
  };

  it("keeps branches in order and reads the default as a null condition", () => {
    const parsed = IssueWorkflowSchema.parse({ ...baseWorkflow, steps: [routedStep] });
    const step = parsed.steps[0];
    expect(step?.transitions?.map((t) => t.id)).toEqual(["urgent", "rejected", "default"]);
    expect(step?.transitions?.[2]?.when).toBeNull();
  });

  it("gives a step without routing an empty transition list", () => {
    const parsed = IssueWorkflowSchema.parse({ ...baseWorkflow, steps: [{ status_key: "todo" }] });
    expect(parsed.steps[0]?.transitions).toEqual([]);
  });

  it("drops a malformed branch on its own and keeps the rest", () => {
    const parsed = IssueWorkflowSchema.parse({
      ...baseWorkflow,
      steps: [
        {
          ...routedStep,
          transitions: [
            { id: "no-target", label: "Broken", when: null },
            { id: "empty-group", label: "Empty", when: { all: [] }, to_status_key: "done" },
            { id: "kept", label: "Kept", when: null, to_status_key: "done" },
          ],
        },
      ],
    });
    expect(parsed.steps[0]?.transitions?.map((t) => t.id)).toEqual(["kept"]);
  });

  it("reads non-list transitions as none instead of failing the workflow", () => {
    const parsed = IssueWorkflowSchema.parse({
      ...baseWorkflow,
      steps: [{ status_key: "in_review", transitions: "yes" }],
    });
    expect(parsed.steps[0]?.transitions).toEqual([]);
  });

  it("keeps a condition field or operator this build does not know", () => {
    const parsed = IssueWorkflowSchema.parse({
      ...baseWorkflow,
      steps: [
        {
          status_key: "in_review",
          transitions: [{ id: "later", label: "Later", when: { field: "sprint", op: "contains", value: "7" }, to_status_key: "done" }],
        },
      ],
    });
    expect(parsed.steps[0]?.transitions?.[0]?.when).toEqual({ field: "sprint", op: "contains", value: "7" });
  });

  it("keeps the legacy next and back keys for installed clients", () => {
    const parsed = IssueWorkflowSchema.parse({
      ...baseWorkflow,
      steps: [{ status_key: "in_review", next_status_key: "done", back_status_key: "in_progress" }],
    });
    expect(parsed.steps[0]?.next_status_key).toBe("done");
    expect(parsed.steps[0]?.back_status_key).toBe("in_progress");
  });
});

describe("workflow usage and delete blockers (workflow-tab §4)", () => {
  it("parses where each workflow is used", () => {
    const parsed = ListIssueWorkflowUsageResponseSchema.parse({
      usage: [{ workflow_id: "wf-1", project_ids: ["p-1"], squad_ids: [], agent_ids: ["a-1"] }],
      total: 1,
    });
    expect(parsed.usage).toEqual([{ workflow_id: "wf-1", project_ids: ["p-1"], squad_ids: [], agent_ids: ["a-1"] }]);
  });

  it("defaults missing usage lists to empty", () => {
    const parsed = ListIssueWorkflowUsageResponseSchema.parse({ usage: [{ workflow_id: "wf-2" }] });
    expect(parsed.usage[0]).toEqual({ workflow_id: "wf-2", project_ids: [], squad_ids: [], agent_ids: [] });
  });

  it("drops a usage row without a workflow id and keeps the others", () => {
    const parsed = ListIssueWorkflowUsageResponseSchema.parse({
      usage: [{ project_ids: ["p-1"] }, { workflow_id: "wf-3", project_ids: [] }],
    });
    expect(parsed.usage.map((u) => u.workflow_id)).toEqual(["wf-3"]);
  });

  it("falls back to empty usage for a malformed body", () => {
    const parsed = parseWithFallback(
      { usage: "everywhere", total: "many" },
      ListIssueWorkflowUsageResponseSchema,
      EMPTY_LIST_ISSUE_WORKFLOW_USAGE_RESPONSE,
      { endpoint: "GET /api/issue-workflows/usage" },
    );
    expect(parsed.usage).toEqual([]);
    expect(parsed.total).toBe(0);
  });

  it("reads each blocking reason from a delete conflict", () => {
    const blockers = parseIssueWorkflowBlockers({
      error: "1 project(s) still use this workflow",
      code: "workflow_in_use",
      reasons: [
        { code: "workflow_in_use", count: 1, ids: ["p-1"], message: "1 project(s) still use this workflow" },
        { code: "workflow_is_squad_active", count: 2, ids: ["s-1", "s-2"], message: "2 squad(s) use this workflow" },
      ],
    });
    expect(blockers.map((b) => [b.code, b.count, b.ids])).toEqual([
      ["workflow_in_use", 1, ["p-1"]],
      ["workflow_is_squad_active", 2, ["s-1", "s-2"]],
    ]);
  });

  it("reads a conflict without reasons, or with malformed ones, as the fallback", () => {
    expect(parseIssueWorkflowBlockers({ error: "in use", code: "workflow_in_use" })).toEqual([]);
    expect(parseIssueWorkflowBlockers(null)).toEqual([]);
    expect(parseIssueWorkflowBlockers({ reasons: "projects" })).toEqual([]);
    expect(parseIssueWorkflowBlockers({ reasons: [{ count: "two" }, { code: "workflow_in_use", count: 1 }] })).toEqual([
      { code: "workflow_in_use", count: 1, ids: [], message: "" },
    ]);
  });

  it("falls back to an empty workflow list for a malformed workflows field", () => {
    const parsed = parseWithFallback(
      { workflows: "all of them" },
      ListIssueWorkflowsResponseSchema,
      EMPTY_LIST_ISSUE_WORKFLOWS_RESPONSE,
      { endpoint: "GET /api/issue-workflows" },
    );
    expect(parsed).toEqual(EMPTY_LIST_ISSUE_WORKFLOWS_RESPONSE);
  });
});

describe("workflow templates (MUL-7420)", () => {
  const endpoint = "GET /api/issue-workflows/templates";
  const delivery = {
    id: "delivery",
    steps: [
      { status_key: "todo", handler: { type: "none" }, instructions: "", transitions: [] },
      {
        status_key: "in_review",
        handler: { type: "none" },
        instructions: "",
        transitions: [
          { id: "rejected", label: "Rejected", when: { field: "outcome", op: "eq", value: "rejected" }, to_status_key: "in_progress" },
          { id: "default", label: "Default", when: null, to_status_key: "done" },
        ],
      },
    ],
  };

  it("parses a template with its rework branch", () => {
    const parsed = ListIssueWorkflowTemplatesResponseSchema.parse({ templates: [delivery] });
    expect(parsed.templates).toHaveLength(1);
    expect(parsed.templates[0]?.id).toBe("delivery");
    expect(parsed.templates[0]?.steps[1]?.transitions?.map((t) => t.to_status_key)).toEqual(["in_progress", "done"]);
  });

  it("skips a template id this build does not know and keeps the rest", () => {
    const parsed = ListIssueWorkflowTemplatesResponseSchema.parse({
      templates: [{ id: "marketing", steps: [] }, { id: "community", steps: [] }],
    });
    expect(parsed.templates.map((t) => t.id)).toEqual(["community"]);
  });

  it("drops a template with malformed steps alone", () => {
    const parsed = ListIssueWorkflowTemplatesResponseSchema.parse({
      templates: [{ id: "content", steps: "nope" }, { id: "delivery", steps: [] }],
    });
    expect(parsed.templates.map((t) => t.id)).toEqual(["delivery"]);
  });

  it("falls back to no templates for a malformed body", () => {
    expect(
      parseWithFallback({ templates: "all" }, ListIssueWorkflowTemplatesResponseSchema, EMPTY_LIST_ISSUE_WORKFLOW_TEMPLATES_RESPONSE, { endpoint }),
    ).toEqual(EMPTY_LIST_ISSUE_WORKFLOW_TEMPLATES_RESPONSE);
    expect(
      parseWithFallback(null, ListIssueWorkflowTemplatesResponseSchema, EMPTY_LIST_ISSUE_WORKFLOW_TEMPLATES_RESPONSE, { endpoint }),
    ).toEqual(EMPTY_LIST_ISSUE_WORKFLOW_TEMPLATES_RESPONSE);
  });
});

describe("workflow template columns and layout (workflow-tab round 2)", () => {
  const endpoint = "GET /api/issue-workflows/templates";
  const column = (overrides: Record<string, unknown>) => ({
    handler: { type: "none" },
    instructions: "",
    transitions: [],
    ...overrides,
  });

  it("parses every template id the server ships", () => {
    const ids = ["delivery", "content", "community", "sdlc_auto", "bmad_planning_auto", "sales_loop", "quick_fix"];
    const parsed = ListIssueWorkflowTemplatesResponseSchema.parse({
      templates: ids.map((id) => ({ id, steps: [column({ status_key: "todo" })] })),
    });
    expect(parsed.templates.map((t) => t.id)).toEqual(ids);
  });

  it("keeps a column that creates its status, with the name and category", () => {
    const parsed = ListIssueWorkflowTemplatesResponseSchema.parse({
      templates: [{
        id: "quick_fix",
        steps: [column({ new_status: { name: "QA", category: "started" }, role_hint: "qa" })],
      }],
    });
    const step = parsed.templates[0]?.steps[0];
    expect(step && "status_key" in step).toBe(false);
    expect(step && "new_status" in step ? step.new_status : null).toEqual({ name: "QA", category: "started" });
    expect(step?.role_hint).toBe("qa");
  });

  it("drops a template whose column names both an existing and a created status, and keeps the rest", () => {
    const parsed = ListIssueWorkflowTemplatesResponseSchema.parse({
      templates: [
        { id: "content", steps: [column({ status_key: "todo", new_status: { name: "Drafting" } })] },
        { id: "delivery", steps: [column({ status_key: "todo" })] },
      ],
    });
    expect(parsed.templates.map((t) => t.id)).toEqual(["delivery"]);
  });

  it("drops a template whose column names neither an existing nor a created status", () => {
    const parsed = ListIssueWorkflowTemplatesResponseSchema.parse({
      templates: [{ id: "community", steps: [column({})] }, { id: "delivery", steps: [column({ status_key: "todo" })] }],
    });
    expect(parsed.templates.map((t) => t.id)).toEqual(["delivery"]);
  });

  it("degrades an unknown handler type in a template column to none", () => {
    const parsed = ListIssueWorkflowTemplatesResponseSchema.parse({
      templates: [{ id: "delivery", steps: [column({ status_key: "todo", handler: { type: "robot" } })] }],
    });
    expect(parsed.templates[0]?.steps[0]?.handler.type).toBe("none");
  });

  it("drops a malformed template layout alone and keeps the column", () => {
    const parsed = ListIssueWorkflowTemplatesResponseSchema.parse({
      templates: [{ id: "delivery", steps: [column({ status_key: "todo", layout: { x: "10", y: 0 } })] }],
    });
    expect(parsed.templates.map((t) => t.id)).toEqual(["delivery"]);
    expect(parsed.templates[0]?.steps[0]?.layout).toBeUndefined();
  });

  it("falls back to no templates for a malformed template item among valid ones", () => {
    const parsed = ListIssueWorkflowTemplatesResponseSchema.parse({
      templates: [null, 42, "sales_loop", { id: "sales_loop", steps: "nope" }, { id: "quick_fix", steps: [column({ status_key: "todo" })] }],
    });
    expect(parsed.templates.map((t) => t.id)).toEqual(["quick_fix"]);
  });

  it("falls back to no templates for a body that is not an object", () => {
    for (const body of [null, "templates", [], 7]) {
      expect(
        parseWithFallback(body, ListIssueWorkflowTemplatesResponseSchema, EMPTY_LIST_ISSUE_WORKFLOW_TEMPLATES_RESPONSE, { endpoint }),
      ).toEqual(EMPTY_LIST_ISSUE_WORKFLOW_TEMPLATES_RESPONSE);
    }
  });

  it("reads a step's layout on a saved workflow", () => {
    const workflow = IssueWorkflowSchema.parse({
      ...baseWorkflow,
      steps: [{ status_key: "todo", layout: { x: 120, y: -40 } }, { status_key: "done" }],
    });
    expect(workflow.steps[0]?.layout).toEqual({ x: 120, y: -40 });
    expect(workflow.steps[1]?.layout).toBeUndefined();
  });

  it("treats a layout with a non-finite coordinate as no layout, and keeps the step", () => {
    for (const layout of [{ x: Number.NaN, y: 0 }, { x: 0, y: Number.POSITIVE_INFINITY }, { x: 1 }]) {
      const workflow = IssueWorkflowSchema.parse({ ...baseWorkflow, steps: [{ status_key: "todo", layout }] });
      expect(workflow.steps).toHaveLength(1);
      expect(workflow.steps[0]?.layout).toBeUndefined();
    }
  });
});
