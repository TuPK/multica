import type {
  IssueWorkflow,
  IssueWorkflowStepInput,
  IssueWorkflowUsage,
  NewWorkflowStatus,
} from "@multica/core/types";
import type { IssueWorkflowStep } from "./types";

/**
 * The steps as a workflow write takes them. Transitions are canonical; the
 * legacy next and back keys are not sent. Instructions only matter for a
 * handler, so they are dropped for a column that keeps its assignee.
 */
export function workflowStepInputs(steps: IssueWorkflowStep[]): IssueWorkflowStepInput[] {
  return steps.map((step) => {
    const handler = step.handler.type === "none" ? { type: "none" as const } : { ...step.handler };
    const base: { new_status: NewWorkflowStatus } | { status_key: string } =
      step.new_status
        ? { new_status: step.new_status }
        : { status_key: step.status_key };

    return {
      ...base,
      handler,
      instructions: step.handler.type === "none" ? "" : step.instructions,
      transitions: step.transitions ?? [],
      ...(step.layout && Number.isFinite(step.layout.x) && Number.isFinite(step.layout.y)
        ? {
            layout: {
              x: Math.max(-100000, Math.min(100000, Math.round(step.layout.x))),
              y: Math.max(-100000, Math.min(100000, Math.round(step.layout.y))),
            },
          }
        : {}),
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
