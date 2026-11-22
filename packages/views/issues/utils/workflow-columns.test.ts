// @vitest-environment node
import { describe, expect, it, vi } from "vitest";
import type { IssueWorkflow, IssueWorkflowStep } from "@multica/core/types";
import type { RoomIssueRef, RoomRoster } from "@multica/core/issue-workflows/queries";
import {
  mergeColumnWorkflows,
  projectBoardWorkflows,
  useColumnWorkflow,
  workflowColumns,
} from "./workflow-columns";

function makeWorkflow(
  id: string,
  name: string,
  steps: Array<string | IssueWorkflowStep>,
  opts: { is_default?: boolean } = {},
): IssueWorkflow {
  return {
    id,
    workspace_id: "ws-1",
    name,
    description: "",
    is_default: opts.is_default ?? false,
    owner_id: null,
    steps: steps.map((s) =>
      typeof s === "string"
        ? { status_key: s, handler: { type: "none" as const }, instructions: "" }
        : s,
    ),
    created_at: "",
    updated_at: "",
  };
}

const defaultWorkflow = makeWorkflow(
  "default",
  "Default",
  ["todo", "in_progress", "in_review", "done", "cancelled"],
  { is_default: true },
);

const deliveryWorkflow = makeWorkflow("wf-delivery", "Delivery", [
  { status_key: "todo", handler: { type: "agent", id: "agent-dev" }, instructions: "" },
  "in_progress",
  { status_key: "qa", handler: { type: "agent", id: "agent-qa" }, instructions: "" },
  "done",
]);

const designWorkflow = makeWorkflow("wf-design", "Design", [
  { status_key: "todo", handler: { type: "agent", id: "agent-designer" }, instructions: "" },
  { status_key: "design_review", handler: { type: "member", id: "user-lead" }, instructions: "" },
  "done",
]);

const unusedWorkflow = makeWorkflow("wf-marketing", "Marketing", [
  "todo",
  "copywriting",
  "done",
]);

const allWorkflows = [defaultWorkflow, deliveryWorkflow, designWorkflow, unusedWorkflow];

const roster: RoomRoster = {
  squads: [
    { id: "squad-delivery", active_workflow_id: "wf-delivery" },
    { id: "squad-design", active_workflow_id: "wf-design" },
    { id: "squad-marketing", active_workflow_id: "wf-marketing" },
    { id: "squad-empty", active_workflow_id: null },
  ],
  agents: [
    { id: "agent-dev", home_squad_id: "squad-delivery" },
    { id: "agent-designer", home_squad_id: "squad-design" },
    { id: "agent-no-wf", home_squad_id: "squad-empty" },
  ],
};

const mockState = vi.hoisted(() => ({
  workflows: [] as IssueWorkflow[],
  roster: { squads: undefined, agents: undefined } as RoomRoster,
}));

vi.mock("@multica/core/issue-workflows", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/issue-workflows")>();
  return {
    ...actual,
    useIssueWorkflows: () => ({ workflows: mockState.workflows, isLoaded: true }),
    useRoomRoster: () => mockState.roster,
  };
});

// React's useMemo works outside a component only if mocked or invoked via a test renderer;
// stub useMemo in this node-environment unit suite so useColumnWorkflow can be called directly.
vi.mock("react", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react")>();
  return {
    ...actual,
    useMemo: <T,>(fn: () => T) => fn(),
  };
});

describe("workflowColumns (MUL-7420)", () => {
  it("passes catalog columns through when workflow is null", () => {
    expect(workflowColumns(["backlog", "todo", "done"], null)).toEqual(["backlog", "todo", "done"]);
  });

  it("uses workflow order and drops statuses the workflow does not list", () => {
    expect(workflowColumns(["backlog", "todo", "in_progress", "done", "qa"], deliveryWorkflow)).toEqual([
      "todo",
      "in_progress",
      "qa",
      "done",
    ]);
  });

  it("keeps hidden columns hidden", () => {
    expect(workflowColumns(["todo", "done"], deliveryWorkflow)).toEqual(["todo", "done"]);
    expect(workflowColumns(null, deliveryWorkflow)).toEqual(["todo", "in_progress", "qa", "done"]);
  });
});

describe("projectBoardWorkflows & mergeColumnWorkflows (P3)", () => {
  it("returns one room's workflow columns when every project issue belongs to that room", () => {
    const issues: RoomIssueRef[] = [
      { project_id: "proj-1", assignee_type: "squad", assignee_id: "squad-delivery" },
      { project_id: "proj-1", assignee_type: "agent", assignee_id: "agent-dev" },
    ];
    const resolved = projectBoardWorkflows(allWorkflows, roster, issues);
    const merged = mergeColumnWorkflows(resolved);
    expect(workflowColumns(null, merged.workflow)).toEqual(["todo", "in_progress", "qa", "done"]);
    expect(merged.workflowByStatus.get("qa")?.id).toBe("wf-delivery");
  });

  it("unions columns across two rooms with different workflows in first-appearance order", () => {
    const issues: RoomIssueRef[] = [
      { project_id: "proj-1", assignee_type: "squad", assignee_id: "squad-delivery" },
      { project_id: "proj-1", assignee_type: "agent", assignee_id: "agent-designer" },
    ];
    const resolved = projectBoardWorkflows(allWorkflows, roster, issues);
    const merged = mergeColumnWorkflows(resolved);

    expect(workflowColumns(null, merged.workflow)).toEqual([
      "todo",
      "in_progress",
      "qa",
      "done",
      "design_review",
    ]);
    // A workflow with no issues in the project adds no column.
    expect(workflowColumns(null, merged.workflow)).not.toContain("copywriting");
    // Shared status uses the first workflow's handoff; unique statuses keep their owner.
    expect(merged.workflowByStatus.get("todo")?.id).toBe("wf-delivery");
    expect(merged.workflowByStatus.get("qa")?.id).toBe("wf-delivery");
    expect(merged.workflowByStatus.get("design_review")?.id).toBe("wf-design");
  });

  it("adds no custom column for a room without a workflow (reads Default as server does)", () => {
    const issues: RoomIssueRef[] = [
      { project_id: "proj-1", assignee_type: "squad", assignee_id: "squad-empty" },
    ];
    const resolved = projectBoardWorkflows(allWorkflows, roster, issues);
    const merged = mergeColumnWorkflows(resolved);
    expect(workflowColumns(null, merged.workflow)).toEqual([
      "todo",
      "in_progress",
      "in_review",
      "done",
      "cancelled",
    ]);
    expect(workflowColumns(null, merged.workflow)).not.toContain("qa");
    expect(workflowColumns(null, merged.workflow)).not.toContain("copywriting");
  });

  it("does not add columns from a room without a workflow that has no issues in the project", () => {
    const issues: RoomIssueRef[] = [
      { project_id: "proj-1", assignee_type: "squad", assignee_id: "squad-delivery" },
    ];
    const resolved = projectBoardWorkflows(allWorkflows, roster, issues);
    const merged = mergeColumnWorkflows(resolved);
    // Neither Default-only columns (in_review/cancelled) nor unused room columns appear.
    expect(workflowColumns(null, merged.workflow)).toEqual(["todo", "in_progress", "qa", "done"]);
  });
});

describe("useColumnWorkflow (P3)", () => {
  it("follows Default on a workspace-wide board (no project)", () => {
    mockState.workflows = allWorkflows;
    mockState.roster = roster;
    const { workflow } = useColumnWorkflow("ws-1", null, false);
    expect(workflow?.id).toBe("default");
  });

  it("returns the union of room workflows on a project board", () => {
    mockState.workflows = allWorkflows;
    mockState.roster = roster;
    const issues: RoomIssueRef[] = [
      { project_id: "proj-1", assignee_type: "squad", assignee_id: "squad-delivery" },
      { project_id: "proj-1", assignee_type: "squad", assignee_id: "squad-design" },
    ];
    const { workflow, workflowByStatus } = useColumnWorkflow("ws-1", "proj-1", false, issues);
    expect(workflowColumns(null, workflow)).toEqual([
      "todo",
      "in_progress",
      "qa",
      "done",
      "design_review",
    ]);
    expect(workflowByStatus.get("design_review")?.id).toBe("wf-design");
  });

  it("lets an explicit status filter reach a status outside the room workflows", () => {
    mockState.workflows = allWorkflows;
    mockState.roster = roster;
    const issues: RoomIssueRef[] = [
      { project_id: "proj-1", assignee_type: "squad", assignee_id: "squad-delivery" },
    ];
    const { workflow } = useColumnWorkflow("ws-1", "proj-1", true, issues);
    expect(workflowColumns(["blocked"], workflow)).toEqual(["blocked"]);
  });
});
