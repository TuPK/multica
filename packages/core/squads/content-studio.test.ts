// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "../agents/instruction-templates";
import { CONTENT_STUDIO_ROLE_AGENT_INSTRUCTIONS, CONTENT_STUDIO_TEMPLATE, CS_ROLE_KEYS, CS_ROLES } from "./content-studio";
import { SQUAD_ROLES, SQUAD_TEMPLATES } from "./templates";

describe("content studio template", () => {
  it("is registered with the squad templates", () => {
    expect(SQUAD_TEMPLATES.map((t) => t.id)).toContain("content_studio");
  });

  it("has the ten roles of the spec and no producer", () => {
    expect(CONTENT_STUDIO_TEMPLATE.roles).toEqual([
      "cs_lead",
      "cs_scout",
      "cs_researcher",
      "cs_drafter",
      "cs_localizer",
      "cs_claim_gate",
      "cs_market_gate",
      "cs_voice_gate",
      "cs_handover",
      "cs_publisher",
    ]);
    expect(CONTENT_STUDIO_TEMPLATE.roles).not.toContain("cs_producer" as never);
    expect(CS_ROLE_KEYS).toEqual(CONTENT_STUDIO_TEMPLATE.roles);
  });

  it("leads with the content lead and runs no routine", () => {
    expect(CONTENT_STUDIO_TEMPLATE.leader).toBe("cs_lead");
    expect(CONTENT_STUDIO_TEMPLATE.roles[0]).toBe("cs_lead");
    expect(CONTENT_STUDIO_TEMPLATE.routines).toEqual([]);
    expect(CONTENT_STUDIO_TEMPLATE.operatingReview).toBe(false);
    expect(CONTENT_STUDIO_TEMPLATE.workflow).toBeNull();
  });

  it("gives each role an agent template and an instruction", () => {
    const templateIds = new Set(AGENT_INSTRUCTION_TEMPLATES.map((t) => t.id));
    for (const key of CS_ROLE_KEYS) {
      const role = CS_ROLES[key];
      expect(templateIds.has(role.agentTemplate), key).toBe(true);
      expect(CONTENT_STUDIO_ROLE_AGENT_INSTRUCTIONS[key].trim().length, key).toBeGreaterThan(100);
      expect([...role.description].length, key).toBeLessThanOrEqual(255);
      expect(SQUAD_ROLES[key], key).toBe(role);
    }
  });

  it("keeps the three gates separate from the drafter and the publisher out of the decision", () => {
    const drafter = CS_ROLES.cs_drafter.name;
    for (const gate of ["cs_claim_gate", "cs_market_gate", "cs_voice_gate"] as const) {
      expect(CS_ROLES[gate].name).not.toBe(drafter);
      expect(CONTENT_STUDIO_ROLE_AGENT_INSTRUCTIONS[gate]).toContain("VERDICT: PASS");
    }
    expect(CONTENT_STUDIO_ROLE_AGENT_INSTRUCTIONS.cs_publisher).toContain("You do not post");
  });

  it("files the publish proposal through the CLI with kind publish", () => {
    const handover = CONTENT_STUDIO_ROLE_AGENT_INSTRUCTIONS.cs_handover;
    expect(handover).toContain("--kind publish");
    expect(handover).toContain("--payload-file");
    expect(handover).toContain('"item_id"');
    expect(handover).toContain('"channels"');
  });

  it("names no role that the template does not have", () => {
    for (const key of Object.keys(CONTENT_STUDIO_ROLE_AGENT_INSTRUCTIONS)) {
      expect(CONTENT_STUDIO_TEMPLATE.roles).toContain(key as never);
    }
  });
});
