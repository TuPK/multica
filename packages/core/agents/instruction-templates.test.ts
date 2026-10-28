// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "./instruction-templates";

describe("AGENT_INSTRUCTION_TEMPLATES", () => {
  it("gives every template a unique id and non-empty instructions", () => {
    const ids = AGENT_INSTRUCTION_TEMPLATES.map((template) => template.id);

    expect(new Set(ids).size).toBe(ids.length);
    for (const template of AGENT_INSTRUCTION_TEMPLATES) {
      expect(template.instructions.trim().length).toBeGreaterThan(0);
    }
  });
});
