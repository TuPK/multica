import type {
  IssueWorkflow,
  IssueWorkflowStep,
  IssueWorkflowStepInput,
  IssueWorkflowUsage,
} from "@multica/core/types";

/**
 * The steps as a workflow write takes them. Transitions are canonical; the
 * legacy next and back keys are not sent. Instructions only matter for a
 * handler, so they are dropped for a column that keeps its assignee.
 */
export function workflowStepInputs(steps: IssueWorkflowStep[]): IssueWorkflowStepInput[] {
  return steps.map((step) => {
    const handler = step.handler.type === "none" ? { type: "none" as const } : { ...step.handler };
    return {
      status_key: step.status_key,
      handler,
      instructions: step.handler.type === "none" ? "" : step.instructions,
      transitions: step.transitions ?? [],
    };
  });
}

/** A reason a workflow cannot be deleted. `code` matches the server's delete conflict. */
export interface DeleteBlocker {
  code: string;
  count: number;
}

/**
 * The reasons the page can already see a workflow cannot be deleted. The
 * server checks again on delete and has the final word.
 */
export function localDeleteBlockers(
  workflow: Pick<IssueWorkflow, "is_default">,
  usage: IssueWorkflowUsage | undefined,
): DeleteBlocker[] {
  const blockers: DeleteBlocker[] = [];
  if (workflow.is_default) blockers.push({ code: "workflow_is_default", count: 0 });
  if (usage && usage.project_ids.length > 0) {
    blockers.push({ code: "workflow_in_use", count: usage.project_ids.length });
  }
  if (usage && usage.squad_ids.length > 0) {
    blockers.push({ code: "workflow_is_squad_active", count: usage.squad_ids.length });
  }
  return blockers;
}
