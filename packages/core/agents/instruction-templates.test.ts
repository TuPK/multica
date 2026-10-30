// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "./instruction-templates";

const fullstack = AGENT_INSTRUCTION_TEMPLATES.find(
  (template) => template.id === "fullstack_developer",
);

describe("AGENT_INSTRUCTION_TEMPLATES", () => {
  it("gives every template a unique id and non-empty instructions", () => {
    const ids = AGENT_INSTRUCTION_TEMPLATES.map((template) => template.id);

    expect(new Set(ids).size).toBe(ids.length);
    for (const template of AGENT_INSTRUCTION_TEMPLATES) {
      expect(template.instructions.trim().length).toBeGreaterThan(0);
    }
  });

  it("requires a runtime check before a story counts as done", () => {
    expect(fullstack?.instructions).toContain("Runtime check");
    expect(fullstack?.instructions).toContain("1440px and 390px");
  });

  it("never lets a fake stand in for a reachable user-facing backend", () => {
    expect(fullstack?.instructions).toContain(
      "Never replace a real backend with a local fake on a path a user can reach",
    );
    expect(fullstack?.instructions).not.toContain("a local fake behind the same interface");
  });

  it("keeps epics open until the retrospective accepts them", () => {
    expect(fullstack?.instructions).toContain("Do not mark an epic done yourself");
    expect(fullstack?.instructions).toContain("bmad-retrospective");
  });

  it("builds a missing backend to unblock a user path instead of stopping", () => {
    expect(fullstack?.instructions).toContain("Backend unreachable or missing");
    expect(fullstack?.instructions).toContain("added to unblock");
    expect(fullstack?.instructions).not.toContain("Set it to blocked");
  });
});
