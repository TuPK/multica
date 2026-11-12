// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "../agents/instruction-templates";
import { composeKvAgentInstructions } from "../squads/kitviet-agent-instructions";

// The five Kit Viet role templates are written out in catalog/agents/kv_* so the Go service, which
// embeds the catalog, can read them. The Kit Viet composer is code, so this guards the two copies.
describe("catalog agent templates", () => {
  it.each(["kv_lead", "kv_collector", "kv_drafter", "kv_gate", "kv_handover"] as const)(
    "%s equals what the Kit Viet composer produces",
    (kind) => {
      const template = AGENT_INSTRUCTION_TEMPLATES.find((t) => t.id === kind);
      expect(template?.instructions).toBe(composeKvAgentInstructions({ kind }));
    },
  );

  it("expands every shared block", () => {
    for (const t of AGENT_INSTRUCTION_TEMPLATES) expect(t.instructions, t.id).not.toMatch(/\{\{block:/);
  });
});
