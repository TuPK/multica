// @vitest-environment jsdom

import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderWithI18n } from "../../test/i18n";
import { StartPointBar } from "./start-point-bar";

describe("StartPointBar", () => {
  it("renders all 3 starting points and handles clicks", () => {
    const onSelectBlank = vi.fn();
    const onOpenClone = vi.fn();
    const onOpenTemplate = vi.fn();

    renderWithI18n(
      <StartPointBar
        activeChoice="blank"
        onSelectBlank={onSelectBlank}
        onOpenClone={onOpenClone}
        onOpenTemplate={onOpenTemplate}
      />,
    );

    expect(screen.getByRole("toolbar", { name: "Starting point" })).toBeTruthy();
    const blankBtn = screen.getByRole("button", { name: "Blank" });
    const cloneBtn = screen.getByRole("button", { name: "Clone an existing workflow" });
    const templateBtn = screen.getByRole("button", { name: "Start from a template" });

    expect(blankBtn).toBeTruthy();
    expect(cloneBtn).toBeTruthy();
    expect(templateBtn).toBeTruthy();

    fireEvent.click(blankBtn);
    expect(onSelectBlank).toHaveBeenCalledTimes(1);

    fireEvent.click(cloneBtn);
    expect(onOpenClone).toHaveBeenCalledTimes(1);

    fireEvent.click(templateBtn);
    expect(onOpenTemplate).toHaveBeenCalledTimes(1);
  });
});
