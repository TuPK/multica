import type {
  IssueWorkflowStep as CoreIssueWorkflowStep,
  NewWorkflowStatus,
  WorkflowCondition,
} from "@multica/core/types";
import type workflowEn from "../locales/en/workflow.json";

// Workflow contract types come from @multica/core. The names below are the
// designer's short forms of the core types.
export type {
  IssueWorkflow,
  IssueWorkflowHandler,
  IssueWorkflowTransition,
  IssueWorkflowUsage,
  NewWorkflowStatus,
  WorkflowCondition as Condition,
  WorkflowConditionField as ConditionField,
  WorkflowConditionOperator as ConditionOperator,
} from "@multica/core/types";

/**
 * Designer-extended step: carries optional new_status (will be created on save)
 * and optional role_hint (squad role suggestion from templates).
 */
export type IssueWorkflowStep = CoreIssueWorkflowStep & {
  new_status?: NewWorkflowStatus;
  role_hint?: string;
};

/** A condition that tests one field. */
export type ConditionRule = Extract<WorkflowCondition, { field: string }>;

export type ValidationSeverity = "error" | "warning";

/** Every validation message key. Derived from en so a key missing from the copy fails typecheck. */
export type ValidationMessageKey = keyof typeof workflowEn.designer.validation.messages;

/** A validation problem as a locale key plus the values it interpolates. The UI renders it with t(). */
export interface ValidationMessage {
  key: ValidationMessageKey;
  values?: Record<string, string | number>;
}

export interface ValidationIssue {
  id: string;
  severity: ValidationSeverity;
  message: ValidationMessage;
  step_key?: string;
  transition_id?: string;
}
