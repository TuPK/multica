import { ApiError } from "../api";
import { IssueWorkflowMappingPlanSchema } from "../api/schemas";
import type { IssueWorkflowMappingPlan } from "../types";

/**
 * The status mapping a workflow write asks for when issues sit on statuses the
 * edit removes (409 `workflow_status_mapping_required`). Null for any other
 * error, so the caller can fall through to the message.
 */
export function issueWorkflowMappingPlanFrom(err: unknown): IssueWorkflowMappingPlan | null {
  if (!(err instanceof ApiError) || err.status !== 409) return null;
  const body = err.body;
  if (!body || typeof body !== "object") return null;
  const { code, plan } = body as { code?: unknown; plan?: unknown };
  if (code !== "workflow_status_mapping_required" || !plan) return null;
  const parsed = IssueWorkflowMappingPlanSchema.safeParse(plan);
  return parsed.success ? parsed.data : null;
}
