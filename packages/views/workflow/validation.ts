import type {
  Condition,
  IssueWorkflowStep,
  ValidationIssue,
  ValidationMessage,
  ValidationMessageKey,
} from "./types";
import { conditionProblems } from "./condition-spec";

// Limits the server enforces (server/internal/issueworkflow/transitions.go).
const MAX_STEPS = 50;
const MAX_TRANSITIONS_PER_STEP = 10;
const MAX_CONDITION_DEPTH = 4;
const MAX_LABEL_RUNES = 64;

function message(key: ValidationMessageKey, values?: ValidationMessage["values"]): ValidationMessage {
  return values ? { key, values } : { key };
}

export function getConditionDepth(cond: Condition | null | undefined): number {
  if (!cond) return 0;
  if ("all" in cond) {
    if (!cond.all.length) return 1;
    return 1 + Math.max(...cond.all.map(getConditionDepth));
  }
  if ("any" in cond) {
    if (!cond.any.length) return 1;
    return 1 + Math.max(...cond.any.map(getConditionDepth));
  }
  if ("not" in cond) {
    return 1 + getConditionDepth(cond.not);
  }
  return 1;
}

export function validateWorkflow(steps: IssueWorkflowStep[]): ValidationIssue[] {
  const issues: ValidationIssue[] = [];
  const statusKeys = steps.map((s) => s.status_key);
  const statusKeySet = new Set(statusKeys);

  // 1. Max 50 steps
  if (steps.length > MAX_STEPS) {
    issues.push({
      id: "max-steps",
      severity: "error",
      message: message("max_steps", { limit: MAX_STEPS }),
    });
  }

  // 2. Required statuses: todo, done, cancelled
  for (const required of ["todo", "done", "cancelled"] as const) {
    if (!statusKeySet.has(required)) {
      issues.push({
        id: `missing-${required}`,
        severity: "error",
        message: message("missing_required_status", { status: required }),
      });
    }
  }

  // 3. Fixed head: backlog (if present) must be 1st; todo must follow head
  const hasBacklog = statusKeySet.has("backlog");
  if (hasBacklog && steps[0]?.status_key !== "backlog") {
    issues.push({
      id: "fixed-head-backlog",
      severity: "error",
      message: message("backlog_not_first"),
      step_key: "backlog",
    });
  }
  const expectedTodoIndex = hasBacklog ? 1 : 0;
  if (statusKeySet.has("todo") && steps[expectedTodoIndex]?.status_key !== "todo") {
    issues.push({
      id: "fixed-head-todo",
      severity: "error",
      message: message(hasBacklog ? "todo_after_backlog" : "todo_first"),
      step_key: "todo",
    });
  }

  // 4. Fixed tail: cancelled must be last, done before cancelled, blocked before done (if present)
  const lastIndex = steps.length - 1;
  if (statusKeySet.has("cancelled") && steps[lastIndex]?.status_key !== "cancelled") {
    issues.push({
      id: "fixed-tail-cancelled",
      severity: "error",
      message: message("cancelled_not_last"),
      step_key: "cancelled",
    });
  }
  if (statusKeySet.has("done") && steps[lastIndex - 1]?.status_key !== "done") {
    issues.push({
      id: "fixed-tail-done",
      severity: "error",
      message: message("done_before_cancelled"),
      step_key: "done",
    });
  }
  const hasBlocked = statusKeySet.has("blocked");
  if (hasBlocked) {
    const expectedBlockedIndex = lastIndex - 2;
    if (steps[expectedBlockedIndex]?.status_key !== "blocked") {
      issues.push({
        id: "fixed-tail-blocked",
        severity: "error",
        message: message("blocked_before_done"),
        step_key: "blocked",
      });
    }
  }

  // 5. Per-step validations: transitions, targets, defaults, depth, label length
  for (const step of steps) {
    const transitions = step.transitions ?? [];

    // The server hands off only to an agent, a member or the project lead, and
    // an agent or member needs an id.
    const handlerType = step.handler.type;
    if (handlerType === "squad" || handlerType === "creator") {
      issues.push({
        id: `handler-type-${step.status_key}`,
        severity: "error",
        message: message("handler_not_allowed", { step: step.status_key, handler: handlerType }),
        step_key: step.status_key,
      });
    }
    if ((handlerType === "agent" || handlerType === "member") && !step.handler.id) {
      issues.push({
        id: `handler-id-${step.status_key}`,
        severity: "error",
        message: message("handler_id_required", { step: step.status_key, handler: handlerType }),
        step_key: step.status_key,
      });
    }

    const transitionIds = new Set<string>();
    for (const transition of transitions) {
      if (transitionIds.has(transition.id)) {
        issues.push({
          id: `duplicate-transition-${transition.id}`,
          severity: "error",
          message: message("duplicate_transition_id", { step: step.status_key }),
          step_key: step.status_key,
          transition_id: transition.id,
        });
      }
      transitionIds.add(transition.id);
    }

    if (transitions.length > MAX_TRANSITIONS_PER_STEP) {
      issues.push({
        id: `max-transitions-${step.status_key}`,
        severity: "error",
        message: message("max_transitions", {
          step: step.status_key,
          limit: MAX_TRANSITIONS_PER_STEP,
        }),
        step_key: step.status_key,
      });
    }

    const defaultTransitions = transitions.filter((t) => t.when === null);

    // If more than 1 transition, a default branch is required
    if (transitions.length > 1) {
      if (defaultTransitions.length === 0) {
        issues.push({
          id: `missing-default-${step.status_key}`,
          severity: "error",
          message: message("missing_default", { step: step.status_key }),
          step_key: step.status_key,
        });
      } else if (defaultTransitions.length > 1) {
        issues.push({
          id: `multiple-defaults-${step.status_key}`,
          severity: "error",
          message: message("multiple_defaults", {
            step: step.status_key,
            count: defaultTransitions.length,
          }),
          step_key: step.status_key,
        });
      } else {
        const lastTransition = transitions[transitions.length - 1];
        if (lastTransition && lastTransition.when !== null) {
          issues.push({
            id: `default-not-last-${step.status_key}`,
            severity: "error",
            message: message("default_not_last", { step: step.status_key }),
            step_key: step.status_key,
          });
        }
      }
    }

    for (const transition of transitions) {
      const branch = transition.label || transition.id;

      // The server refuses an empty label; a branch cannot route to its own step.
      if (transition.label.trim() === "") {
        issues.push({
          id: `label-required-${transition.id}`,
          severity: "error",
          message: message("label_required", { step: step.status_key }),
          step_key: step.status_key,
          transition_id: transition.id,
        });
      }
      if (transition.to_status_key === step.status_key) {
        issues.push({
          id: `self-target-${transition.id}`,
          severity: "error",
          message: message("self_target", { label: branch, step: step.status_key }),
          step_key: step.status_key,
          transition_id: transition.id,
        });
      }
      if (transition.when) {
        for (const problem of conditionProblems(transition.when)) {
          issues.push({
            id: `condition-invalid-${transition.id}-${issues.length}`,
            severity: "error",
            message: message(problem.key, { label: branch, ...problem.values }),
            step_key: step.status_key,
            transition_id: transition.id,
          });
        }
      }

      // Target exists in workflow
      if (!statusKeySet.has(transition.to_status_key)) {
        issues.push({
          id: `invalid-target-${transition.id}`,
          severity: "error",
          message: message("unknown_target", {
            label: branch,
            status: transition.to_status_key,
          }),
          step_key: step.status_key,
          transition_id: transition.id,
        });
      }

      // Label length <= 64 runes
      if (transition.label && Array.from(transition.label).length > MAX_LABEL_RUNES) {
        issues.push({
          id: `label-too-long-${transition.id}`,
          severity: "error",
          message: message("label_too_long", { limit: MAX_LABEL_RUNES }),
          step_key: step.status_key,
          transition_id: transition.id,
        });
      }

      // Condition depth <= 4
      if (transition.when) {
        const depth = getConditionDepth(transition.when);
        if (depth > MAX_CONDITION_DEPTH) {
          issues.push({
            id: `condition-depth-${transition.id}`,
            severity: "error",
            message: message("condition_depth", {
              label: branch,
              limit: MAX_CONDITION_DEPTH,
              depth,
            }),
            step_key: step.status_key,
            transition_id: transition.id,
          });
        }
      }
    }
  }

  // 6. Terminal reachability analysis (cycle / dead-end check)
  // Terminal statuses are "done" and "cancelled".
  const terminalStatuses = new Set(["done", "cancelled"]);
  const graph = new Map<string, string[]>();

  for (let i = 0; i < steps.length; i++) {
    const step = steps[i]!;
    const targets: string[] = [];
    if (step.transitions && step.transitions.length > 0) {
      for (const t of step.transitions) {
        if (statusKeySet.has(t.to_status_key)) {
          targets.push(t.to_status_key);
        }
      }
    } else {
      // Default legacy behaviour: moves to next status in order if exists
      if (i + 1 < steps.length) {
        targets.push(steps[i + 1]!.status_key);
      }
    }
    graph.set(step.status_key, targets);
  }

  // Backwards BFS from all terminal nodes to find all reachable nodes
  const canReachTerminal = new Set<string>();
  for (const term of terminalStatuses) {
    if (statusKeySet.has(term)) {
      canReachTerminal.add(term);
    }
  }

  let changed = true;
  while (changed) {
    changed = false;
    for (const [node, targets] of graph.entries()) {
      if (canReachTerminal.has(node)) continue;
      // If any target can reach terminal, then this node can too
      if (targets.some((tgt) => canReachTerminal.has(tgt))) {
        canReachTerminal.add(node);
        changed = true;
      }
    }
  }

  // Any non-terminal step that cannot reach a terminal emits a warning
  for (const step of steps) {
    if (terminalStatuses.has(step.status_key)) continue;
    if (!canReachTerminal.has(step.status_key)) {
      issues.push({
        id: `unreachable-terminal-${step.status_key}`,
        severity: "warning",
        message: message("unreachable_terminal", { step: step.status_key }),
        step_key: step.status_key,
      });
    }
  }

  return issues;
}
