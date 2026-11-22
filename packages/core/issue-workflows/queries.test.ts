// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { IssueWorkflow, IssueWorkflowStep } from "../types";
import {
  issueRoomWorkflow,
  issueWorkflowKeys,
  issueWorkflowTemplatesOptions,
  projectMoveMayHandOff,
  sharedWorkflowStatusKeys,
  stepHandsOff,
  statusAcceptsHandoff,
  workflowAllowsStatus,
  workflowMiddleKeys,
  workflowNextStatus,
  workflowStep,
} from "./queries";

const manual = (status_key: string): IssueWorkflowStep => ({ status_key, handler: { type: "none" }, instructions: "" });

const columns = (...middle: string[]) =>
  ["backlog", "todo", ...middle, "blocked", "done", "cancelled"].map(manual);

const defaultWorkflow: IssueWorkflow = {
  id: "wf-default",
  workspace_id: "ws-1",
  name: "Default",
  description: "",
  is_default: true,
  owner_id: null,
  steps: columns("in_progress", "in_review"),
  created_at: "",
  updated_at: "",
};

const content: IssueWorkflow = {
  ...defaultWorkflow,
  id: "wf-content",
  name: "Content",
  is_default: false,
  owner_id: null,
  steps: columns("drafting", "editing"),
};

const lead: IssueWorkflowStep = { status_key: "in_review", handler: { type: "project_lead" }, instructions: "Review it." };

// A room is a squad with an active workflow; an agent belongs to a room through
// its home squad. The rules mirror the server's (squad-room-workflow spec, section 3).
const roster = {
  squads: [
    { id: "squad-content", active_workflow_id: "wf-content" },
    { id: "squad-bare", active_workflow_id: null },
    { id: "squad-flow", active_workflow_id: "wf-flow" },
  ],
  agents: [
    { id: "agent-content", home_squad_id: "squad-content" },
    { id: "agent-loose", home_squad_id: null },
  ],
};

const flowWorkflow: IssueWorkflow = {
  ...content,
  id: "wf-flow",
  name: "Flow",
  flow_config: { version: 1, switches: [] } as unknown as IssueWorkflow["flow_config"],
};

describe("room workflow resolution (squad-room-workflow spec, section 3)", () => {
  it("follows the squad the issue is assigned to, or the agent's home squad", () => {
    const workflows = [defaultWorkflow, content];
    const inProject = { project_id: "p-1" };
    expect(issueRoomWorkflow(workflows, roster, { ...inProject, assignee_type: "squad", assignee_id: "squad-content" })?.id).toBe("wf-content");
    expect(issueRoomWorkflow(workflows, roster, { ...inProject, assignee_type: "agent", assignee_id: "agent-content" })?.id).toBe("wf-content");
  });

  it("reads Default for an issue outside a project, with no room, or in a room without a workflow", () => {
    const workflows = [defaultWorkflow, content];
    expect(issueRoomWorkflow(workflows, roster, { project_id: null, assignee_type: "squad", assignee_id: "squad-content" })?.id).toBe("wf-default");
    expect(issueRoomWorkflow(workflows, roster, { project_id: "p-1", assignee_type: null, assignee_id: null })?.id).toBe("wf-default");
    expect(issueRoomWorkflow(workflows, roster, { project_id: "p-1", assignee_type: "agent", assignee_id: "agent-loose" })?.id).toBe("wf-default");
    expect(issueRoomWorkflow(workflows, roster, { project_id: "p-1", assignee_type: "squad", assignee_id: "squad-bare" })?.id).toBe("wf-default");
  });

  it("reads Default for a flow room, whose requests run on the flow", () => {
    const workflows = [defaultWorkflow, content, flowWorkflow];
    expect(issueRoomWorkflow(workflows, roster, { project_id: "p-1", assignee_type: "squad", assignee_id: "squad-flow" })?.id).toBe("wf-default");
  });

  it("is unknown while workflows or the roster load", () => {
    const ref = { project_id: "p-1", assignee_type: "squad", assignee_id: "squad-content" };
    expect(issueRoomWorkflow(undefined, roster, ref)).toBeNull();
    expect(issueRoomWorkflow([defaultWorkflow], { squads: undefined, agents: undefined }, ref)).toBeNull();
  });

  it("limits a batch to the statuses every issue's workflow lists, Default included", () => {
    const workflows = [defaultWorkflow, content];
    const outside = sharedWorkflowStatusKeys(workflows, roster, [{ project_id: null, assignee_type: null, assignee_id: null }]);
    expect(outside && [...outside]).toEqual(["backlog", "todo", "in_progress", "in_review", "blocked", "done", "cancelled"]);
    const mixed = sharedWorkflowStatusKeys(workflows, roster, [
      { project_id: null, assignee_type: null, assignee_id: null },
      { project_id: "p-1", assignee_type: "squad", assignee_id: "squad-content" },
    ]);
    expect(mixed && [...mixed]).toEqual(["backlog", "todo", "blocked", "done", "cancelled"]);
    expect(sharedWorkflowStatusKeys(undefined, roster, [{ project_id: null, assignee_type: null, assignee_id: null }])).toBeNull();
  });

  it("carries the room workflow's own handoffs", () => {
    const reviewed = { ...content, steps: content.steps.map((s) => (s.status_key === "editing" ? { ...lead, status_key: "editing" } : s)) };
    const resolved = issueRoomWorkflow([defaultWorkflow, reviewed], roster, { project_id: "p-1", assignee_type: "squad", assignee_id: "squad-content" });
    expect(stepHandsOff(workflowStep(resolved, "editing"))).toBe(true);
    expect(workflowStep(resolved, "editing")?.instructions).toBe("Review it.");
    expect(stepHandsOff(workflowStep(resolved, "drafting"))).toBe(false);
  });
});

describe("workflow helpers (MUL-7420)", () => {
  it("allows only the statuses a workflow lists, Default included", () => {
    expect(workflowAllowsStatus(null, "drafting")).toBe(true);
    expect(workflowAllowsStatus(defaultWorkflow, "drafting")).toBe(false);
    expect(workflowAllowsStatus(defaultWorkflow, "in_review")).toBe(true);
    expect(workflowAllowsStatus(content, "drafting")).toBe(true);
    expect(workflowAllowsStatus(content, "in_progress")).toBe(false);
  });

  it("names the middle columns and who may hand off", () => {
    expect(workflowMiddleKeys(content)).toEqual(["drafting", "editing"]);
    expect(statusAcceptsHandoff("todo")).toBe(true);
    expect(statusAcceptsHandoff("blocked")).toBe(true);
    expect(statusAcceptsHandoff("backlog")).toBe(false);
    expect(statusAcceptsHandoff("done")).toBe(false);
  });

  it("points a step at the next column unless it names its own", () => {
    expect(workflowNextStatus(content, "drafting")).toBe("editing");
    // blocked is a detour, not the next step.
    expect(workflowNextStatus(content, "editing")).toBe("done");
    const own = { ...content, steps: content.steps.map((s) => (s.status_key === "drafting" ? { ...s, next_status_key: "done" } : s)) };
    expect(workflowNextStatus(own, "drafting")).toBe("done");
    expect(workflowNextStatus(content, "cancelled")).toBe("");
    expect(workflowNextStatus(content, "missing")).toBe("");
  });
});

describe("projectMoveMayHandOff", () => {
  const handsOffDrafting: IssueWorkflow = {
    ...content,
    steps: content.steps.map((s) => (s.status_key === "drafting" ? { ...s, handler: { type: "agent", id: "a-1" } } : s)),
  };

  it("checks the column the issue keeps", () => {
    expect(projectMoveMayHandOff(handsOffDrafting, "drafting")).toBe(true);
    expect(projectMoveMayHandOff(handsOffDrafting, "todo")).toBe(false);
  });

  it("asks the server when the issue lands on another column", () => {
    expect(projectMoveMayHandOff(handsOffDrafting, "in_progress")).toBe(true);
    expect(projectMoveMayHandOff(content, "in_progress")).toBe(false);
  });

  it("never asks without a workflow", () => {
    expect(projectMoveMayHandOff(null, "todo")).toBe(false);
  });

  it("keeps the issue's status on a move into Default, listed or not", () => {
    const reviewing = { ...defaultWorkflow, steps: defaultWorkflow.steps.map((s) => (s.status_key === "in_review" ? lead : s)) };
    expect(projectMoveMayHandOff(reviewing, "drafting")).toBe(false);
    expect(projectMoveMayHandOff(reviewing, "in_review")).toBe(true);
  });
});

describe("workflow template options (workflow-tab round 2)", () => {
  it("scopes the template cache to the workspace", () => {
    const options = issueWorkflowTemplatesOptions("ws-1");
    expect(options.queryKey).toEqual(issueWorkflowKeys.templates("ws-1"));
    expect(options.queryKey).not.toEqual(issueWorkflowKeys.templates("ws-2"));
  });

  it("selects the template list from the response", () => {
    const options = issueWorkflowTemplatesOptions("ws-1");
    expect(options.select?.({ templates: [] })).toEqual([]);
  });
});
