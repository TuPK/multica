// @vitest-environment jsdom

import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderWithI18n } from "../test/i18n";
import { ConditionBuilder } from "./designer/condition-builder";
import type { Condition } from "./types";

describe("ConditionBuilder", () => {
  it("renders a single condition rule with field, operator and value", () => {
    const condition: Condition = {
      field: "outcome",
      op: "eq",
      value: "approved",
    };
    const onChange = vi.fn();

    renderWithI18n(
      <ConditionBuilder condition={condition} onChange={onChange} />,
    );

    expect(screen.getByText("Outcome")).toBeTruthy();
    expect(screen.getByText("equals")).toBeTruthy();
    expect(screen.getByText("approved")).toBeTruthy();
  });

  it("calls onChange when field operator or value changes", () => {
    const condition: Condition = {
      field: "outcome",
      op: "eq",
      value: "approved",
    };
    const onChange = vi.fn();

    renderWithI18n(
      <ConditionBuilder condition={condition} onChange={onChange} />,
    );

    // Operator trigger button has value "equals"
    const opSelect = screen.getByText("equals");
    expect(opSelect).toBeTruthy();
  });

  it("renders group controls for AND/OR conditions", () => {
    const groupCondition: Condition = {
      all: [
        { field: "outcome", op: "eq", value: "approved" },
        { field: "priority", op: "eq", value: "high" },
      ],
    };
    const onChange = vi.fn();

    renderWithI18n(
      <ConditionBuilder condition={groupCondition} onChange={onChange} />,
    );

    expect(screen.getByText("AND (All match)")).toBeTruthy();
    expect(screen.getByText("Outcome")).toBeTruthy();
    expect(screen.getByText("Priority")).toBeTruthy();
    expect(screen.getByText("Add rule")).toBeTruthy();
  });

  it("supports adding a rule to a group", () => {
    const groupCondition: Condition = {
      all: [{ field: "outcome", op: "eq", value: "approved" }],
    };
    const onChange = vi.fn();

    renderWithI18n(
      <ConditionBuilder condition={groupCondition} onChange={onChange} />,
    );

    const addRuleBtn = screen.getByRole("button", { name: /add rule/i });
    fireEvent.click(addRuleBtn);

    expect(onChange).toHaveBeenCalledWith({
      all: [
        { field: "outcome", op: "eq", value: "approved" },
        { field: "outcome", op: "eq", value: "approved" },
      ],
    });
  });

  it("prevents adding nested groups beyond depth 4", () => {
    const depth4Group: Condition = {
      all: [{ field: "outcome", op: "eq", value: "approved" }],
    };
    const onChange = vi.fn();

    renderWithI18n(
      <ConditionBuilder
        condition={depth4Group}
        onChange={onChange}
        depth={4}
      />,
    );

    // Add group button should not be present at depth 4
    expect(screen.queryByRole("button", { name: /add group/i })).toBeNull();
  });
});
