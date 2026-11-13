import type {
  Condition,
  ConditionField,
  ConditionOperator,
  ConditionRule,
  ValidationMessageKey,
} from "./types";

export type ConditionProblemKey = Extract<ValidationMessageKey, `condition_${string}`>;

/** One reason the server would refuse a condition: a locale key and its values. */
export interface ConditionProblem {
  key: ConditionProblemKey;
  values: Record<string, string | number>;
}

// Mirrors the server's condition table (server/internal/issueworkflow/transitions.go,
// fieldSpecs). A rule the server would refuse must not be offered or saved.

export const CONDITION_FIELDS: readonly ConditionField[] = [
  "outcome",
  "priority",
  "labels",
  "project_id",
  "rework_count",
  "creator_type",
  "assignee_type",
];

export const FIELD_OPERATORS: Partial<Record<ConditionField, readonly ConditionOperator[]>> &
  Record<string, readonly ConditionOperator[]> = {
  outcome: ["eq", "neq", "in", "exists"],
  tasks_outcome: ["eq", "neq", "in", "exists"],
  priority: ["eq", "neq", "in", "gt", "gte", "lt", "lte"],
  labels: ["eq", "neq", "in", "exists"],
  project_id: ["eq", "neq", "in", "exists"],
  rework_count: ["eq", "neq", "in", "gt", "gte", "lt", "lte"],
  creator_type: ["eq", "neq", "in", "exists"],
  assignee_type: ["eq", "neq", "in", "exists"],
};

/** The values an enum field takes. Fields not listed here take free text or whole numbers. */
export const FIELD_ENUM_VALUES: Partial<Record<ConditionField, readonly string[]>> = {
  outcome: ["approved", "rejected", "blocked", "done"],
  priority: ["none", "low", "medium", "high", "urgent"],
  creator_type: ["member", "agent"],
  assignee_type: ["member", "agent", "squad"],
};

export type ValueKind = "enum" | "text" | "number";

export function valueKindOf(field: ConditionField): ValueKind {
  if (field === "rework_count") return "number";
  return FIELD_ENUM_VALUES[field] ? "enum" : "text";
}

/** The value a field starts with when chosen. */
export function defaultValueFor(field: ConditionField): string | number {
  if (field === "rework_count") return 3;
  if (field === "priority") return "high";
  return FIELD_ENUM_VALUES[field]?.[0] ?? "";
}

// A rule read from the server can name a field or operator this build does not
// know. The lookups below take the raw string and answer "unknown" for it.
function fieldOf(rule: ConditionRule): ConditionField {
  return rule.field as ConditionField;
}

function operatorOf(rule: ConditionRule): ConditionOperator {
  return rule.op as ConditionOperator;
}

/**
 * The rule after its field changes: the operator stays when the new field takes
 * it, and the value resets to the new field's default.
 */
export function ruleWithField(rule: ConditionRule, field: ConditionField): ConditionRule {
  const ops = FIELD_OPERATORS[field] ?? ["eq", "neq"];
  const current = operatorOf(rule);
  const op = ops.includes(current) ? current : (ops[0] ?? "eq");
  return { field, op, value: valueForOperator(op, defaultValueFor(field)) };
}

/**
 * The rule after its operator changes. `exists` drops the value, `in` holds a
 * list, and any other operator holds one value; a value carries over from the
 * old shape when it can.
 */
export function ruleWithOperator(rule: ConditionRule, op: ConditionOperator): ConditionRule {
  const current = Array.isArray(rule.value) ? rule.value[0] : rule.value;
  const scalar = current === undefined ? defaultValueFor(fieldOf(rule)) : (current as string | number);
  return { field: rule.field, op, value: valueForOperator(op, scalar) };
}

function valueForOperator(op: ConditionOperator, scalar: string | number): unknown {
  if (op === "exists") return undefined;
  if (op === "in") return [scalar];
  return scalar;
}

/** The rule after its scalar value is typed. Numbers stay numbers. */
export function ruleWithScalarValue(rule: ConditionRule, raw: string | number): ConditionRule {
  return { ...rule, value: valueKindOf(fieldOf(rule)) === "number" ? Number(raw) : raw };
}

/**
 * The rule after its list value is typed (`in`). Text is split on commas. Number
 * fields parse each entry; an entry that is not a number stays text, so the
 * validation list flags it.
 */
export function ruleWithListValue(rule: ConditionRule, raw: string): ConditionRule {
  const parts = raw.split(",").map((part) => part.trim());
  const numeric = valueKindOf(fieldOf(rule)) === "number";
  return {
    ...rule,
    value: parts.map((part) => {
      if (!numeric || part === "") return part;
      const n = Number(part);
      return Number.isFinite(n) ? n : part;
    }),
  };
}

/** The text an `in` list shows in its input. */
export function listValueText(value: unknown): string {
  return Array.isArray(value) ? value.map((item) => String(item)).join(", ") : String(value ?? "");
}

/**
 * What the server would refuse in a condition, one message per problem. Depth
 * is checked separately (getConditionDepth). Empty means the condition is valid.
 */
export function conditionProblems(condition: Condition): ConditionProblem[] {
  const problems: ConditionProblem[] = [];
  collectProblems(condition, problems);
  return problems;
}

function problem(
  key: ConditionProblemKey,
  values: Record<string, string | number> = {},
): ConditionProblem {
  return { key, values };
}

function collectProblems(condition: Condition, problems: ConditionProblem[]) {
  if ("all" in condition || "any" in condition) {
    const children = "all" in condition ? condition.all : condition.any;
    if (children.length === 0) problems.push(problem("condition_group_empty"));
    for (const child of children) collectProblems(child, problems);
    return;
  }
  if ("not" in condition) {
    collectProblems(condition.not, problems);
    return;
  }
  problems.push(...ruleProblems(condition));
}

function ruleProblems(rule: ConditionRule): ConditionProblem[] {
  const field = fieldOf(rule);
  const op = operatorOf(rule);
  const ops = FIELD_OPERATORS[field] as readonly ConditionOperator[] | undefined;
  if (!ops) return [problem("condition_unknown_field", { field: rule.field })];
  if (!ops.includes(op)) {
    return [problem("condition_operator_not_for_field", { op: rule.op, field })];
  }
  if (op === "exists") {
    return rule.value === undefined ? [] : [problem("condition_exists_has_value")];
  }
  if (op === "in") {
    if (!Array.isArray(rule.value) || rule.value.length === 0) {
      return [problem("condition_in_needs_list")];
    }
    return rule.value.flatMap((item) => scalarProblems(field, item));
  }
  return scalarProblems(field, rule.value);
}

function scalarProblems(field: ConditionField, value: unknown): ConditionProblem[] {
  switch (valueKindOf(field)) {
    case "number":
      return typeof value === "number" && Number.isInteger(value) && value >= 0
        ? []
        : [problem("condition_whole_number", { field })];
    case "enum": {
      const allowed = FIELD_ENUM_VALUES[field] ?? [];
      return typeof value === "string" && allowed.includes(value)
        ? []
        : [problem("condition_value_not_allowed", { value: String(value), field })];
    }
    case "text":
      return typeof value === "string" && value.trim() !== ""
        ? []
        : [problem("condition_value_required", { field })];
  }
}
