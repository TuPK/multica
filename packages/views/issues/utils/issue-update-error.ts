import { errorCode } from "@multica/core/api";
import type { IssuesT } from "./priority-label";

// The flow guard codes an issue update can return. The server guard decides;
// this only names the copy the user reads for each refusal.
type FlowGuardCode =
  | "flow_outcome_required"
  | "flow_outcome_invalid"
  | "flow_attachment_required"
  | "flow_human_only"
  | "flow_managed_request"
  | "flow_children_managed"
  | "flow_gate_same_agent"
  | "flow_forbidden"
  | "flow_not_managed";

const FLOW_GUARD_COPY: Record<FlowGuardCode, (t: IssuesT) => string> = {
  flow_outcome_required: (t) => t(($) => $.flow.errors.flow_outcome_required),
  flow_outcome_invalid: (t) => t(($) => $.flow.errors.flow_outcome_invalid),
  flow_attachment_required: (t) => t(($) => $.flow.errors.flow_attachment_required),
  flow_human_only: (t) => t(($) => $.flow.errors.flow_human_only),
  flow_managed_request: (t) => t(($) => $.flow.errors.flow_managed_request),
  flow_children_managed: (t) => t(($) => $.flow.errors.flow_children_managed),
  flow_gate_same_agent: (t) => t(($) => $.flow.errors.flow_gate_same_agent),
  flow_forbidden: (t) => t(($) => $.flow.errors.flow_forbidden),
  flow_not_managed: (t) => t(($) => $.flow.errors.flow_not_managed),
};

function isFlowGuardCode(code: string): code is FlowGuardCode {
  return Object.prototype.hasOwnProperty.call(FLOW_GUARD_COPY, code);
}

/**
 * The message to show when an issue update fails. A flow refusal gets its
 * localised copy, because the server message names CLI commands a person
 * cannot run. Other failures keep the server's message, and an error without
 * one gets the caller's fallback.
 */
export function issueUpdateErrorMessage(err: unknown, t: IssuesT, fallback: string): string {
  const code = errorCode(err);
  if (code === "revision_conflict") return t(($) => $.revision.conflict);
  if (code && isFlowGuardCode(code)) return FLOW_GUARD_COPY[code](t);
  return err instanceof Error && err.message ? err.message : fallback;
}
