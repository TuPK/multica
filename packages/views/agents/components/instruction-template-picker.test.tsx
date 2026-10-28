// @vitest-environment jsdom

import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "@multica/core/agents";
import { renderWithI18n } from "../../test/i18n";
import { InstructionTemplatePicker } from "./instruction-template-picker";

const fullstack = AGENT_INSTRUCTION_TEMPLATES.find(
  (template) => template.id === "fullstack_developer",
);

async function openMenu() {
  await userEvent
    .setup()
    .click(screen.getByRole("button", { name: "Use a template" }));
  await userEvent
    .setup()
    .click(
      await screen.findByRole("menuitem", { name: /Full Stack Developer/ }),
    );
}

describe("InstructionTemplatePicker", () => {
  afterEach(cleanup);

  it("fills empty instructions straight from the template", async () => {
    const onSelect = vi.fn();
    renderWithI18n(
      <InstructionTemplatePicker currentInstructions="" onSelect={onSelect} />,
    );

    await openMenu();

    expect(onSelect).toHaveBeenCalledWith(fullstack?.instructions);
  });

  it("asks before replacing instructions the user already wrote", async () => {
    const onSelect = vi.fn();
    renderWithI18n(
      <InstructionTemplatePicker
        currentInstructions="Keep my rules"
        onSelect={onSelect}
      />,
    );

    await openMenu();

    expect(onSelect).not.toHaveBeenCalled();
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Replace" }));
    expect(onSelect).toHaveBeenCalledWith(fullstack?.instructions);
  });

  it("keeps existing instructions when the replacement is declined", async () => {
    const onSelect = vi.fn();
    renderWithI18n(
      <InstructionTemplatePicker
        currentInstructions="Keep my rules"
        onSelect={onSelect}
      />,
    );

    await openMenu();
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Keep mine" }));

    expect(onSelect).not.toHaveBeenCalled();
  });
});
