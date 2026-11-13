import type { FlowFactInfo } from "@multica/core/types";
import type {
  Condition,
  ConditionField,
  ConditionOperator,
  ConditionRule,
  ValidationMessageKey,
} from "./types";

export type FactSpec = FlowFactInfo;

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

export function fieldsForFacts(facts?: FlowFactInfo[]): readonly string[] {
  if (facts && facts.length > 0) {
    return facts.map((f) => f.key);
  }
  return CONDITION_FIELDS;
}

export function operatorsForField(field: string, facts?: FlowFactInfo[]): readonly ConditionOperator[] {
  if (facts) {
    const f = facts.find((fact) => fact.key === field);
    if (f) return f.ops as ConditionOperator[];
  }
  return (FIELD_OPERATORS[field] ?? ["eq", "neq"]) as readonly ConditionOperator[];
}

export function enumValuesForField(field: string, facts?: FlowFactInfo[]): readonly string[] | undefined {
  if (facts) {
    const f = facts.find((fact) => fact.key === field);
    if (f) {
      if (f.values && f.values.length > 0) return f.values;
      if (f.type === "bool") return ["true", "false"];
      return undefined;
    }
  }
  return FIELD_ENUM_VALUES[field as ConditionField];
}

export function valueKindOf(field: ConditionField | string, facts?: FlowFactInfo[]): ValueKind {
  if (facts) {
    const f = facts.find((fact) => fact.key === field);
    if (f) {
      if (f.type === "int") return "number";
      if (f.type === "enum" || f.type === "bool") return "enum";
      if (f.values && f.values.length > 0) return "enum";
      return "text";
    }
  }
  if (field === "rework_count") return "number";
  return FIELD_ENUM_VALUES[field as ConditionField] ? "enum" : "text";
}

/** The value a field starts with when chosen. */
export function defaultValueFor(field: ConditionField | string, facts?: FlowFactInfo[]): string | number {
  if (facts) {
    const f = facts.find((fact) => fact.key === field);
    if (f) {
      if (f.type === "int") return 0;
      if (f.type === "bool") return "true";
      if (f.values && f.values.length > 0) return f.values[0] ?? "";
      return "";
    }
  }
  if (field === "rework_count") return 3;
  if (field === "priority") return "high";
  return FIELD_ENUM_VALUES[field as ConditionField]?.[0] ?? "";
}

// A rule read from the server can name a field or operator this build does not
// know. The lookups below take the raw string and answer "unknown" for it.
function fieldOf(rule: ConditionRule): ConditionField | string {
  return rule.field;
}

function operatorOf(rule: ConditionRule): ConditionOperator {
  return rule.op as ConditionOperator;
}

/**
 * The rule after its field changes: the operator stays when the new field takes
 * it, and the value resets to the new field's default.
 */
export function ruleWithField(rule: ConditionRule, field: ConditionField | string, facts?: FlowFactInfo[]): ConditionRule {
  const ops = operatorsForField(field, facts);
  const current = operatorOf(rule);
  const op = ops.includes(current) ? current : (ops[0] ?? "eq");
  return { field, op, value: valueForOperator(op, defaultValueFor(field, facts)) };
}

/**
 * The rule after its operator changes. `exists` drops the value, `in` holds a
 * list, and any other operator holds one value; a value carries over from the
 * old shape when it can.
 */
export function ruleWithOperator(rule: ConditionRule, op: ConditionOperator, facts?: FlowFactInfo[]): ConditionRule {
  const current = Array.isArray(rule.value) ? rule.value[0] : rule.value;
  const scalar = current === undefined ? defaultValueFor(fieldOf(rule), facts) : (current as string | number);
  return { field: rule.field, op, value: valueForOperator(op, scalar) };
}

function valueForOperator(op: ConditionOperator, scalar: string | number): unknown {
  if (op === "exists") return undefined;
  if (op === "in") return [scalar];
  return scalar;
}

/** The rule after its scalar value is typed. Numbers stay numbers. */
export function ruleWithScalarValue(rule: ConditionRule, raw: string | number, facts?: FlowFactInfo[]): ConditionRule {
  return { ...rule, value: valueKindOf(fieldOf(rule), facts) === "number" ? Number(raw) : raw };
}

/**
 * The rule after its list value is typed (`in`). Text is split on commas. Number
 * fields parse each entry; an entry that is not a number stays text, so the
 * validation list flags it.
 */
export function ruleWithListValue(rule: ConditionRule, raw: string, facts?: FlowFactInfo[]): ConditionRule {
  const parts = raw.split(",").map((part) => part.trim());
  const numeric = valueKindOf(fieldOf(rule), facts) === "number";
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
export function conditionProblems(condition: Condition, facts?: FlowFactInfo[]): ConditionProblem[] {
  const problems: ConditionProblem[] = [];
  collectProblems(condition, problems, facts);
  return problems;
}

function problem(
  key: ConditionProblemKey,
  values: Record<string, string | number> = {},
): ConditionProblem {
  return { key, values };
}

function collectProblems(condition: Condition, problems: ConditionProblem[], facts?: FlowFactInfo[]) {
  if ("all" in condition || "any" in condition) {
    const children = "all" in condition ? condition.all : condition.any;
    if (children.length === 0) problems.push(problem("condition_group_empty"));
    for (const child of children) collectProblems(child, problems, facts);
    return;
  }
  if ("not" in condition) {
    collectProblems(condition.not, problems, facts);
    return;
  }
  problems.push(...ruleProblems(condition, facts));
}

function ruleProblems(rule: ConditionRule, facts?: FlowFactInfo[]): ConditionProblem[] {
  const field = fieldOf(rule);
  const op = operatorOf(rule);

  if (facts) {
    const fact = facts.find((f) => f.key === field);
    if (!fact) return [problem("condition_unknown_field", { field: rule.field })];
    if (!fact.ops.includes(op)) {
      return [problem("condition_operator_not_for_field", { op: rule.op, field })];
    }
    if (op === "exists") {
      return rule.value === undefined ? [] : [problem("condition_exists_has_value")];
    }
    if (op === "in") {
      if (!Array.isArray(rule.value) || rule.value.length === 0) {
        return [problem("condition_in_needs_list")];
      }
      return rule.value.flatMap((item) => scalarProblems(field, item, facts));
    }
    return scalarProblems(field, rule.value, facts);
  }

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

function scalarProblems(field: ConditionField | string, value: unknown, facts?: FlowFactInfo[]): ConditionProblem[] {
  if (facts) {
    const fact = facts.find((f) => f.key === field);
    if (fact?.type === "int") {
      return typeof value === "number" && Number.isInteger(value) && value >= 0
        ? []
        : [problem("condition_whole_number", { field })];
    }
    if (fact?.type === "bool") {
      return value === "true" || value === "false" || typeof value === "boolean"
        ? []
        : [problem("condition_value_not_allowed", { value: String(value), field })];
    }
    if (fact?.values && fact.values.length > 0) {
      return typeof value === "string" && fact.values.includes(value)
        ? []
        : [problem("condition_value_not_allowed", { value: String(value), field })];
    }
    return typeof value === "string" && value.trim() !== ""
      ? []
      : [problem("condition_value_required", { field })];
  }

  switch (valueKindOf(field)) {
    case "number":
      return typeof value === "number" && Number.isInteger(value) && value >= 0
        ? []
        : [problem("condition_whole_number", { field })];
    case "enum": {
      const allowed = FIELD_ENUM_VALUES[field as ConditionField] ?? [];
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
