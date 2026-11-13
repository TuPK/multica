import { flowTemplateRef } from "../catalog/flows";
import type { CatalogFlow } from "../catalog/types";
import type {
  IssueWorkflow,
  IssueWorkflowStepInput,
  IssueWorkflowTask,
  IssueWorkflowTransition,
  IssueWorkflowWriteRequest,
  ListIssueWorkflowsResponse,
  SetSquadActiveWorkflowRequest,
} from "../types";
import type { SquadRoleKey } from "./templates";

/** The API calls that put a catalog flow on a squad; the web client satisfies it as is. */
export interface FlowWorkflowClient {
  listIssueWorkflows(): Promise<ListIssueWorkflowsResponse>;
  createIssueWorkflow(data: IssueWorkflowWriteRequest): Promise<IssueWorkflow>;
  setSquadActiveWorkflow(squadId: string, data: SetSquadActiveWorkflowRequest): Promise<unknown>;
}

/** The `code` the server put on an API error body, if any. */
export function errorCode(err: unknown): string | undefined {
  const body = (err as { body?: unknown } | null)?.body;
  if (body && typeof body === "object" && "code" in body) {
    const code = (body as { code?: unknown }).code;
    return typeof code === "string" ? code : undefined;
  }
  return undefined;
}

const MAX_WORKFLOW_NAME_RUNES = 64;

/** `<squad name> · <rev>`, cut to the 64 characters a workflow name may have. */
export function flowWorkflowName(squadName: string, rev: string): string {
  const suffix = ` · ${rev}`;
  const room = MAX_WORKFLOW_NAME_RUNES - [...suffix].length;
  return `${[...squadName].slice(0, room).join("").trimEnd()}${suffix}`;
}

/** The statuses the flow creates in the workspace, as a list for an error message: "Collect, Draft, Gate". */
function newStatusNames(flow: CatalogFlow): string {
  return flow.steps.flatMap((step) => ("name" in step.status ? [step.status.name] : [])).join(", ");
}

/**
 * The create request of a catalog flow: each task's role becomes the agent that has the role, the
 * flow's steps become workflow steps and the config rides along as `flow_config`. Null when a role
 * has no agent, because a task nobody can take would stall every request.
 */
export function flowWorkflowRequest(
  flow: CatalogFlow,
  squadName: string,
  agentIds: Partial<Record<SquadRoleKey, string>>,
  squadId: string,
): IssueWorkflowWriteRequest | null {
  const steps: IssueWorkflowStepInput[] = [];
  for (const step of flow.steps) {
    const tasks: IssueWorkflowTask[] = [];
    for (const task of step.tasks ?? []) {
      const id = agentIds[task.role as SquadRoleKey];
      if (!id) return null;
      tasks.push({
        key: task.key,
        title: task.title,
        kind: task.kind,
        handler: { type: "agent", id },
        instructions: task.instructions,
        ...(task.when ? { when: task.when as IssueWorkflowTask["when"] } : {}),
        requires: task.requires ?? [],
      });
    }
    const transitions: IssueWorkflowTransition[] = (step.transitions ?? []).map((t) => ({
      id: t.id,
      label: t.label,
      when: t.when as IssueWorkflowTransition["when"],
      to_status_key: t.to,
      ...(t.rework ? { rework: true } : {}),
    }));
    const base = {
      handler: { type: "none" as const },
      instructions: "",
      transitions,
      tasks,
      ...(step.humanOnly ? { human_only: true } : {}),
    };
    steps.push(
      "key" in step.status
        ? { status_key: step.status.key, ...base }
        : { new_status: { name: step.status.name, category: step.status.category ?? "started" }, ...base },
    );
  }
  return {
    name: flowWorkflowName(squadName, flow.rev),
    description: "",
    flow_config: { ...flow.config, switches: flow.config.switches ?? [] },
    template_ref: flowTemplateRef(flow),
    // The server checks the flow against the squad's roster: every task goes to an agent of the
    // squad, none to its lead, and every agent of the squad has a task.
    squad_id: squadId,
    steps,
  };
}

/** The flow's workflow row in the workspace: the one made from this revision of the flow, if any. */
export function findFlowWorkflow(
  workflows: readonly IssueWorkflow[],
  flow: Pick<CatalogFlow, "id" | "rev">,
): IssueWorkflow | undefined {
  const ref = flowTemplateRef(flow);
  return workflows.find((w) => w.template_ref === ref);
}

/**
 * Puts a catalog flow on a squad: one workflow row per revision (found by `template_ref`, never by
 * name), then that row as the squad's active workflow when the squad has none. A squad that already
 * runs another workflow keeps it; moving it to a new revision is the upgrade's job. A rerun creates
 * and sets nothing twice.
 */
export async function ensureSquadFlow(
  client: FlowWorkflowClient,
  args: {
    flow: CatalogFlow;
    squadId: string;
    squadName: string;
    /** The workflow the squad runs now. */
    activeWorkflowId: string | null | undefined;
    agentIds: Partial<Record<SquadRoleKey, string>>;
  },
): Promise<{ status: "created" | "existing"; workflowId: string }> {
  const { flow, squadId, squadName, activeWorkflowId, agentIds } = args;
  const { workflows } = await client.listIssueWorkflows();
  let row = findFlowWorkflow(workflows, flow);
  let status: "created" | "existing" = "existing";
  if (!row) {
    const request = flowWorkflowRequest(flow, squadName, agentIds, squadId);
    if (!request) throw new Error("a flow task has no agent to hand to");
    try {
      row = await client.createIssueWorkflow(request);
      status = "created";
    } catch (err) {
      const code = errorCode(err);
      if (code === "status_create_forbidden" || code === "flow_workflow_forbidden") {
        throw new Error(`the flow needs a workspace admin to create its statuses: ${newStatusNames(flow)}`);
      }
      throw err;
    }
  }
  if (!activeWorkflowId) await client.setSquadActiveWorkflow(squadId, { workflow_id: row.id });
  return { status, workflowId: row.id };
}
