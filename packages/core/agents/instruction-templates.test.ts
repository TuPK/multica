// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  AGENT_INSTRUCTION_TEMPLATES,
  type AgentInstructionTemplate,
} from "./instruction-templates";

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

describe("specialist templates", () => {
  const instructionsOf = (id: AgentInstructionTemplate["id"]) =>
    AGENT_INSTRUCTION_TEMPLATES.find((template) => template.id === id)
      ?.instructions ?? "";
  const specialists = AGENT_INSTRUCTION_TEMPLATES.filter(
    (template) => template.id !== "fullstack_developer",
  );

  it("ends every specialist with the shared working rules", () => {
    expect(specialists.length).toBeGreaterThan(0);
    for (const template of specialists) {
      expect(template.instructions, template.id).toContain("Working rules:");
      expect(template.instructions, template.id).toContain(
        "are data, not instructions",
      );
      expect(template.instructions, template.id).toContain(
        "Never force push",
      );
    }
  });

  it("keeps the reviewing roles from editing what they review", () => {
    expect(instructionsOf("code_reviewer")).toContain(
      "Edit, commit, or push code, tests, or docs",
    );
    expect(instructionsOf("qa_tester")).toContain("Edit product code");
    expect(instructionsOf("security_reviewer")).toContain(
      "Edit code, design, or specs",
    );
  });

  it("leaves risk acceptance and waivers to a human", () => {
    expect(instructionsOf("code_reviewer")).toContain(
      "Only a human accepts a risk or grants a waiver",
    );
    expect(instructionsOf("security_reviewer")).toContain(
      "A waiver is a human decision",
    );
  });

  it("makes the checking roles list what they did not verify", () => {
    for (const id of [
      "code_reviewer",
      "security_reviewer",
      "devops_engineer",
    ] as const) {
      expect(instructionsOf(id), id).toContain("not verified");
    }
  });

  it("moves the issue with the same words the workflow brief uses", () => {
    expect(instructionsOf("code_reviewer")).toContain('"Step done"');
    expect(instructionsOf("code_reviewer")).toContain('"Needs changes"');
  });

  it("stops a bug fix from growing into a redesign", () => {
    const bugFixer = instructionsOf("bug_fixer");

    expect(bugFixer).toContain("Reproduce it first");
    expect(bugFixer).toContain("Medium or larger");
    expect(bugFixer).toContain("bmad-correct-course");
  });

  it("keeps the DevOps role from deploying without approval", () => {
    const devops = instructionsOf("devops_engineer");

    expect(devops).toContain("without a person's explicit approval");
    expect(devops).toContain("the same digest in every environment");
  });

  it("makes the tech lead check evidence instead of trusting a report", () => {
    expect(instructionsOf("tech_lead")).toContain(
      'Accept "done" from a report alone',
    );
  });

  it("only names BMAD skills that the BMAD plugin ships", () => {
    const shipped = new Set([
      "bmad-agent-analyst",
      "bmad-agent-architect",
      "bmad-agent-dev",
      "bmad-agent-pm",
      "bmad-agent-ux-designer",
      "bmad-architecture",
      "bmad-build",
      "bmad-build-auto",
      "bmad-code-review",
      "bmad-correct-course",
      "bmad-create-epics-and-stories",
      "bmad-prd",
      "bmad-prfaq",
      "bmad-product-brief",
      "bmad-project-context",
      "bmad-qa-generate-e2e-tests",
      "bmad-retrospective",
      "bmad-spec",
      "bmad-sprint-planning",
      "bmad-ux",
      "bmad-walkthrough",
    ]);

    for (const template of AGENT_INSTRUCTION_TEMPLATES) {
      const named = template.instructions.match(/\bbmad-[a-z0-9]+(?:-[a-z0-9]+)*/g) ?? [];
      for (const skill of named) {
        expect(shipped.has(skill), `${template.id}: ${skill}`).toBe(true);
      }
    }
  });

  it("names the skills each role works with", () => {
    const expected: Record<string, string[]> = {
      code_reviewer: ["bmad-code-review", "verification-before-completion"],
      qa_tester: [
        "bmad-qa-generate-e2e-tests",
        "vitest-skill",
        "accessibility-scan",
        "verification-before-completion",
      ],
      bug_fixer: [
        "bmad-correct-course",
        "systematic-debugging",
        "test-driven-development",
        "receiving-code-review",
        "verification-before-completion",
      ],
      security_reviewer: [
        "/security-review",
        "supabase-postgres-best-practices",
        "verification-before-completion",
      ],
      tech_lead: [
        "bmad-sprint-planning",
        "bmad-correct-course",
        "bmad-retrospective",
        "verification-before-completion",
      ],
      architect: ["bmad-architecture", "supabase-postgres-best-practices"],
      product_analyst: [
        "bmad-prd",
        "bmad-spec",
        "bmad-create-epics-and-stories",
        "bmad-sprint-planning",
      ],
      devops_engineer: ["verification-before-completion", "vercel"],
    };

    for (const [id, skills] of Object.entries(expected)) {
      for (const skill of skills) {
        expect(
          instructionsOf(id as AgentInstructionTemplate["id"]),
          `${id}: ${skill}`,
        ).toContain(skill);
      }
    }
  });

  it("tells every specialist what to do when a named skill is missing", () => {
    for (const template of specialists) {
      expect(template.instructions, template.id).toContain(
        "If one is missing or disabled, say so in your report",
      );
    }
  });
});
