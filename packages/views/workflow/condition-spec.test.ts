// @vitest-environment node

import { describe, expect, it } from "vitest";
import type { FlowFactInfo } from "@multica/core/types";
import {
  FIELD_OPERATORS,
  conditionProblems,
  defaultValueFor,
  enumValuesForField,
  fieldsForFacts,
  operatorsForField,
  valueKindOf,
  listValueText,
  ruleWithField,
  ruleWithListValue,
  ruleWithOperator,
  ruleWithScalarValue,
} from "./condition-spec";
import type { Condition, ConditionRule } from "./types";

// The fact catalog of a flow: what the server says a condition may read (plan P20).
const facts: FlowFactInfo[] = [
  { key: "join.pass", type: "int", source: "engine", ops: ["eq", "gt", "gte"], values: [] },
  { key: "task.draft.verdict", type: "string", source: "engine", ops: ["eq", "in", "exists"], values: [] },
  { key: "request.labels", type: "list_string", source: "request", ops: ["contains", "exists"], values: [] },
  { key: "join.result", type: "enum", source: "engine", ops: ["eq", "in"], values: ["pass", "fail"] },
  { key: "vars.export", type: "bool", source: "var", ops: ["eq"], values: [] },
];

describe("a condition read against the fact catalog of a flow", () => {
  it("offers the facts the server lists and their operators, not the built-in fields", () => {
    expect(fieldsForFacts(facts)).toEqual(["join.pass", "task.draft.verdict", "request.labels", "join.result", "vars.export"]);
    expect(operatorsForField("request.labels", facts)).toEqual(["contains", "exists"]);
    expect(operatorsForField("join.pass", facts)).toEqual(["eq", "gt", "gte"]);
  });

  it("takes a number for an int fact, text for a string or list fact, a choice for an enum or bool fact", () => {
    expect(valueKindOf("join.pass", facts)).toBe("number");
    expect(valueKindOf("task.draft.verdict", facts)).toBe("text");
    expect(valueKindOf("request.labels", facts)).toBe("text");
    expect(valueKindOf("join.result", facts)).toBe("enum");
    expect(valueKindOf("vars.export", facts)).toBe("enum");
    expect(enumValuesForField("join.result", facts)).toEqual(["pass", "fail"]);
    expect(enumValuesForField("vars.export", facts)).toEqual(["true", "false"]);
    expect(enumValuesForField("join.pass", facts)).toBeUndefined();
  });

  it("starts each kind of fact with a value the server accepts", () => {
    expect(defaultValueFor("join.pass", facts)).toBe(0);
    expect(defaultValueFor("task.draft.verdict", facts)).toBe("");
    expect(defaultValueFor("join.result", facts)).toBe("pass");
    expect(defaultValueFor("vars.export", facts)).toBe("true");
  });

  it("keeps the built-in fields when no catalog is given", () => {
    expect(fieldsForFacts()).toContain("rework_count");
    expect(fieldsForFacts([])).toContain("outcome");
    expect(operatorsForField("outcome")).toEqual(["eq", "neq", "in", "exists"]);
    expect(valueKindOf("rework_count")).toBe("number");
  });
});

const outcomeEq: ConditionRule = { field: "outcome", op: "eq", value: "approved" };

describe("FIELD_OPERATORS", () => {
  it("offers only the operators the server accepts for each field", () => {
    expect(FIELD_OPERATORS.outcome).toEqual(["eq", "neq", "in", "exists"]);
    expect(FIELD_OPERATORS.labels).toEqual(["eq", "neq", "in", "exists"]);
    expect(FIELD_OPERATORS.priority).toEqual(["eq", "neq", "in", "gt", "gte", "lt", "lte"]);
    expect(FIELD_OPERATORS.rework_count).toEqual(["eq", "neq", "in", "gt", "gte", "lt", "lte"]);
  });
});

describe("rule edits", () => {
  it("drops the value when the operator becomes exists", () => {
    const next = ruleWithOperator(outcomeEq, "exists");
    expect(next.op).toBe("exists");
    expect(next.value).toBeUndefined();
    expect(conditionProblems(next)).toEqual([]);
  });

  it("holds a list when the operator becomes in, and a single value again when it leaves in", () => {
    const asList = ruleWithOperator(outcomeEq, "in");
    expect(asList.value).toEqual(["approved"]);
    expect(conditionProblems(asList)).toEqual([]);

    const backToEq = ruleWithOperator(ruleWithListValue(asList, "rejected, done"), "eq");
    expect(backToEq.value).toBe("rejected");
  });

  it("resets the operator and value when the field changes to one that does not take the operator", () => {
    const priorityGt: ConditionRule = { field: "priority", op: "gte", value: "high" };
    const onLabels = ruleWithField(priorityGt, "labels");
    expect(onLabels).toEqual({ field: "labels", op: "eq", value: "" });
    expect(conditionProblems(onLabels)).toEqual([
      { key: "condition_value_required", values: { field: "labels" } },
    ]);
  });

  it("keeps the operator when the new field takes it", () => {
    const next = ruleWithField(outcomeEq, "priority");
    expect(next.op).toBe("eq");
    expect(next.value).toBe("high");
  });

  it("keeps numbers as numbers for rework_count", () => {
    const next = ruleWithScalarValue({ field: "rework_count", op: "gte", value: 1 }, "2");
    expect(next.value).toBe(2);
    expect(conditionProblems(next)).toEqual([]);
  });

  it("splits a list into entries, parsing number fields", () => {
    const text = ruleWithListValue({ field: "outcome", op: "in", value: [] }, " approved, rejected ");
    expect(text.value).toEqual(["approved", "rejected"]);
    const numbers = ruleWithListValue({ field: "rework_count", op: "in", value: [] }, "1, x");
    expect(numbers.value).toEqual([1, "x"]);
    expect(listValueText(numbers.value)).toBe("1, x");
  });
});

describe("conditionProblems", () => {
  it("accepts a valid group", () => {
    const condition: Condition = {
      all: [
        outcomeEq,
        { any: [{ field: "priority", op: "gte", value: "high" }, { not: { field: "labels", op: "exists" } }] },
      ],
    };
    expect(conditionProblems(condition)).toEqual([]);
  });

  it("flags an operator the field does not take", () => {
    expect(conditionProblems({ field: "outcome", op: "gt", value: "approved" })).toEqual([
      { key: "condition_operator_not_for_field", values: { op: "gt", field: "outcome" } },
    ]);
  });

  it("flags a value outside the enum", () => {
    expect(conditionProblems({ field: "outcome", op: "eq", value: "maybe" })).toEqual([
      { key: "condition_value_not_allowed", values: { value: "maybe", field: "outcome" } },
    ]);
  });

  it("flags exists with a value and in with no list", () => {
    expect(conditionProblems({ field: "labels", op: "exists", value: "bug" })).toEqual([
      { key: "condition_exists_has_value", values: {} },
    ]);
    expect(conditionProblems({ field: "labels", op: "in", value: "bug" })).toEqual([
      { key: "condition_in_needs_list", values: {} },
    ]);
  });

  it("flags a rework_count that is not a whole number", () => {
    expect(conditionProblems({ field: "rework_count", op: "eq", value: 1.5 })).toEqual([
      { key: "condition_whole_number", values: { field: "rework_count" } },
    ]);
    expect(conditionProblems({ field: "rework_count", op: "eq", value: -1 })).toEqual([
      { key: "condition_whole_number", values: { field: "rework_count" } },
    ]);
  });

  it("flags an empty group and an unknown field from a newer server", () => {
    expect(conditionProblems({ all: [] })).toEqual([{ key: "condition_group_empty", values: {} }]);
    expect(conditionProblems({ field: "sprint", op: "eq", value: "x" })).toEqual([
      { key: "condition_unknown_field", values: { field: "sprint" } },
    ]);
  });

  it("evaluates conditions with facts catalog", () => {
    const facts = [
      { key: "join.result", type: "enum", source: "engine", ops: ["eq", "neq", "in"], values: ["pass", "fail", "block"] },
      { key: "join.pass", type: "int", source: "engine", ops: ["eq", "neq", "gt", "gte", "lt", "lte"] },
      { key: "var.approved", type: "bool", source: "var", ops: ["eq", "neq"] },
      { key: "var.notes", type: "string", source: "var", ops: ["eq", "neq", "exists"] },
    ];

    expect(conditionProblems({ field: "join.result", op: "eq", value: "pass" }, facts)).toEqual([]);
    expect(conditionProblems({ field: "join.result", op: "eq", value: "unknown_val" }, facts)).toEqual([
      { key: "condition_value_not_allowed", values: { value: "unknown_val", field: "join.result" } },
    ]);
    expect(conditionProblems({ field: "join.pass", op: "gte", value: 2 }, facts)).toEqual([]);
    expect(conditionProblems({ field: "join.pass", op: "gte", value: -1 }, facts)).toEqual([
      { key: "condition_whole_number", values: { field: "join.pass" } },
    ]);
    expect(conditionProblems({ field: "var.approved", op: "eq", value: "true" }, facts)).toEqual([]);
    expect(conditionProblems({ field: "unknown.fact", op: "eq", value: "x" }, facts)).toEqual([
      { key: "condition_unknown_field", values: { field: "unknown.fact" } },
    ]);
  });
});
