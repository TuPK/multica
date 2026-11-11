// @vitest-environment node
import { readdirSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { SDLC_BUILTIN_SKILLS } from "./sdlc-skills-data";
import { SDLC_AUTO_WORKFLOW, SQUAD_TEMPLATES } from "./templates";

// The bundle the quickstart imports is generated from the SKILL.md folders.
// This test fails when the generated file is stale: run `pnpm generate:sdlc-skills`.
const skillsDir = fileURLToPath(new URL("./sdlc-skills", import.meta.url));

const ROLE_SKILLS = [
  "sdlc-delivery-core",
  "sdlc-squad-lead",
  "sdlc-product-analysis",
  "sdlc-architecture",
  "sdlc-ux-design",
  "sdlc-development",
  "sdlc-bug-fix",
  "sdlc-code-review",
  "sdlc-qa-verification",
  "sdlc-security-review",
  "sdlc-devops-release",
  "sdlc-harness",
];

// The planning squad's procedure skills (planning-skills.test.ts owns their content).
const PLANNING_ROLE_SKILLS = [
  "planning-delivery-core",
  "planning-squad-lead",
  "planning-research",
  "planning-product",
  "planning-ux",
  "planning-architecture",
  "planning-readiness",
];

// Every other skill in the bundle: the tools and plugin skills the boss attached to the
// SDLC squad. They ship beside the role skills, copied from the workspace, so every SDLC
// squad has them. A folder name stores a colon as `--`.
const TOOL_SKILLS = readdirSync(skillsDir, { withFileTypes: true })
  .filter((entry) => entry.isDirectory() && !ROLE_SKILLS.includes(entry.name) && !PLANNING_ROLE_SKILLS.includes(entry.name))
  .map((entry) => entry.name.replaceAll("--", ":"))
  .sort();

// Every file under a skill folder except SKILL.md, at any depth, as the generator reads it.
function skillFiles(dir: string, prefix = ""): string[] {
  return readdirSync(resolve(dir, prefix), { withFileTypes: true }).flatMap((entry) => {
    const rel = prefix ? `${prefix}/${entry.name}` : entry.name;
    if (entry.isDirectory()) return skillFiles(dir, rel);
    return rel === "SKILL.md" ? [] : [rel];
  });
}

describe("built-in SDLC skills", () => {
  it("ships one skill per SDLC role plus the shared core, and the attached tool skills", () => {
    expect(SDLC_BUILTIN_SKILLS.map((s) => s.name)).toEqual([...ROLE_SKILLS, ...PLANNING_ROLE_SKILLS, ...TOOL_SKILLS].sort());
  });

  it("carries every skill folder, unchanged, as the generated bundle", () => {
    const folders = readdirSync(skillsDir, { withFileTypes: true })
      .filter((entry) => entry.isDirectory())
      .map((entry) => entry.name)
      .sort();
    expect(SDLC_BUILTIN_SKILLS.map((s) => s.name)).toEqual(folders.map((f) => f.replaceAll("--", ":")));

    for (const skill of SDLC_BUILTIN_SKILLS) {
      const dir = resolve(skillsDir, skill.name.replaceAll(":", "--"));
      expect(skill.content, skill.name).toBe(readFileSync(resolve(dir, "SKILL.md"), "utf8"));
      const files = skillFiles(dir)
        .sort()
        .map((file) => ({ path: file, content: readFileSync(resolve(dir, file), "utf8") }));
      expect(skill.files, skill.name).toEqual(files);
    }
  });

  it("has frontmatter whose name matches its folder and whose description says when to use it", () => {
    for (const skill of SDLC_BUILTIN_SKILLS) {
      // A plugin-qualified name (`superpowers:brainstorming`) keeps its short name in the frontmatter.
      const shortName = skill.name.split(":").pop();
      expect(skill.content, skill.name).toMatch(new RegExp(`^---\nname: ${shortName}\n`));
      expect(skill.description, skill.name).not.toBe("");
      if (ROLE_SKILLS.includes(skill.name)) expect(skill.description, skill.name).toMatch(/^Use (when|on) /);
    }
  });

  it("makes every role skill load the shared core first", () => {
    for (const skill of SDLC_BUILTIN_SKILLS.filter((s) => ROLE_SKILLS.includes(s.name))) {
      if (skill.name === "sdlc-delivery-core") continue;
      expect(skill.content, skill.name).toContain("sdlc-delivery-core");
    }
  });

  it("gives the squad lead the route playbook and the report templates", () => {
    const lead = SDLC_BUILTIN_SKILLS.find((s) => s.name === "sdlc-squad-lead")!;
    expect(lead.files.map((f) => f.path).sort()).toEqual(["reference.md", "team-playbook.md"]);
    expect(lead.content).toContain("reference.md");
    expect(lead.content).toContain("multica issue queue <issue> --after <issue>");
    expect(lead.files[0]!.content).toContain("Plan A: BMAD epics exist");
  });

  it("makes the lead turn a request into queued issues and leave the request alone", () => {
    const lead = SDLC_BUILTIN_SKILLS.find((s) => s.name === "sdlc-squad-lead")!.content;
    expect(lead).toContain("## Your whole job: turn the request into issues");
    expect(lead).toContain("Never change its status or its assignee");
    expect(lead).toContain("--start-status");
    expect(lead).toContain("multica issue queue <issue> --after <issue>");
    for (const status of ["analysis", "design", "architecture", "plan_review", "todo", "qa", "security_review", "release_prep"]) {
      expect(lead, status).toContain("`" + status + "`");
    }
    expect(lead).toContain("--until-issue-state ended");
    expect(lead).toContain("Folder mode `in_place`: one issue at a time, always");
    expect(lead).toContain("resource_ref.execution_mode");
  });

  it("keeps the unattended rules in the shared core, so no role waits for approval", () => {
    const core = SDLC_BUILTIN_SKILLS.find((s) => s.name === "sdlc-delivery-core")!;
    expect(core.content).toContain("Do not wait for an approval");
    expect(core.content).toContain("Stop only for what cannot be undone");
    expect(core.content).toContain("Report format");
  });

  it("keeps the rules that stop a role from doing another role's work", () => {
    const get = (name: string) => SDLC_BUILTIN_SKILLS.find((s) => s.name === name)!.content;
    expect(get("sdlc-squad-lead")).toContain("You never build");
    expect(get("sdlc-development")).toContain("Close an epic yourself");
    expect(get("sdlc-code-review")).toContain("you do not fix");
    expect(get("sdlc-qa-verification")).toContain("Edit product code");
    expect(get("sdlc-security-review")).toContain("You advise and gate; you do not build");
    expect(get("sdlc-devops-release")).toContain("Deploy the published image by its digest");
  });

  it("lets an owner decision replace a specialist finding, and keeps controls within the agent's permissions", () => {
    const get = (name: string) => SDLC_BUILTIN_SKILLS.find((s) => s.name === name)!.content;
    expect(get("sdlc-delivery-core")).toContain("## Owner decisions");
    expect(get("sdlc-delivery-core")).toContain("Owner waiver O-<n>");
    expect(get("sdlc-delivery-core")).toContain("## Proportion");
    expect(get("sdlc-squad-lead")).toContain("is applied, not argued");
    expect(get("sdlc-squad-lead")).toContain("must be doable with the");
    expect(get("sdlc-squad-lead")).toContain("a refused proposal is not a reason to stop");
    expect(get("sdlc-security-review")).toContain("## Context and severity");
    expect(get("sdlc-security-review")).toContain("**Required**");
  });

  it("keeps the autonomy rules: evidence ladder, release conditions, park and proceed, one request", () => {
    const get = (name: string) => SDLC_BUILTIN_SKILLS.find((s) => s.name === name)!.content;
    const core = get("sdlc-delivery-core");
    expect(core).toContain("## Evidence ladder");
    expect(core).toContain("reached L2; L1 not run because");
    expect(core).toContain("## Release conditions");
    expect(core).toContain("Never add a gate that the acceptance criteria");
    const lead = get("sdlc-squad-lead");
    expect(lead).toContain("**Capability preflight.**");
    expect(lead).toContain("**Park and proceed.**");
    expect(lead).toContain("**One request for the boss.**");
    expect(lead).toContain("**Block budget.**");
    expect(lead).toContain("Never forbid the lower level");
    expect(get("sdlc-qa-verification")).toContain("release condition");
    expect(get("sdlc-devops-release")).toContain("**Reach L2 yourself.**");
    expect(get("sdlc-squad-lead")).toContain("docs/sdlc/parked.md");
    expect(lead).toContain("An epic that stays blocked after its retrospective is accepted is your error");
    expect(lead).toContain("No issue is left at blocked whose cause is gone");
    expect(lead).toContain("**Provider limits.**");
    expect(lead).toContain("**Stalls.**");
    expect(lead).toContain("Watchdog: read the unblock issue");
    expect(get("sdlc-bug-fix")).toContain("not finished at a ruling that still needs code");
  });

  it("gives every role skill a self-check before it reports", () => {
    for (const skill of SDLC_BUILTIN_SKILLS.filter((s) => ROLE_SKILLS.includes(s.name))) {
      expect(skill.content, skill.name).toContain("Self-check before you report");
    }
  });

  // The classes a blocked issue can carry. The core defines them, the lead's playbook routes them,
  // and the workflow brief points at both: one list, so the three cannot drift apart.
  const BLOCKER_CLASSES = [
    "environment",
    "tooling",
    "requirement",
    "design",
    "ux",
    "security",
    "verification",
    "defect",
    "rework-loop",
    "review-capacity",
    "capability",
    "irreversible",
  ];

  it("defines one set of blocker classes in the core and routes each one in the lead playbook", () => {
    const core = SDLC_BUILTIN_SKILLS.find((s) => s.name === "sdlc-delivery-core")!.content;
    const line = core.match(/^Blocker-class: (.+)$/m);
    expect(line).not.toBeNull();
    expect(line![1]!.split(" | ")).toEqual(BLOCKER_CLASSES);
    const playbook = SDLC_BUILTIN_SKILLS.find((s) => s.name === "sdlc-squad-lead")!.files.find(
      (f) => f.path === "team-playbook.md",
    )!.content;
    const routed = playbook.split("## Route a blocker")[1]!.split("## Substitutes")[0]!;
    for (const cls of BLOCKER_CLASSES) {
      expect(routed, cls).toContain("| `" + cls + "` |");
    }
  });

  it("routes every class to a status the SDLC Auto workflow has, and to a role every SDLC template creates", () => {
    const playbook = SDLC_BUILTIN_SKILLS.find((s) => s.name === "sdlc-squad-lead")!.files.find(
      (f) => f.path === "team-playbook.md",
    )!.content;
    const routed = playbook.split("## Route a blocker")[1]!.split("## Substitutes")[0]!;
    const statusKeys = new Set(
      SDLC_AUTO_WORKFLOW.steps.map((s) =>
        "key" in s.status ? s.status.key : s.status.name.toLowerCase().replaceAll(" ", "_"),
      ),
    );
    const startStatuses = [...routed.matchAll(/`(analysis|design|architecture|plan_review|security_review|environment|qa|todo)`/g)].map(
      (m) => m[1]!,
    );
    expect(startStatuses.length).toBeGreaterThan(0);
    for (const key of startStatuses) expect(statusKeys.has(key), key).toBe(true);
    for (const id of ["sdlc_greenfield", "sdlc_maintenance"] as const) {
      const roles = SQUAD_TEMPLATES.find((t) => t.id === id)!.roles;
      for (const role of ["analyst", "ux", "architect", "developer", "reviewer", "qa", "security", "devops"] as const) {
        expect(roles, `${id}: ${role}`).toContain(role);
      }
    }
  });

  it("gives every specialist a Ruling mode, and DevOps an Environment mode", () => {
    for (const name of [
      "sdlc-product-analysis",
      "sdlc-architecture",
      "sdlc-ux-design",
      "sdlc-security-review",
      "sdlc-qa-verification",
      "sdlc-bug-fix",
      "sdlc-code-review",
    ]) {
      const skill = SDLC_BUILTIN_SKILLS.find((s) => s.name === name)!;
      expect(skill.content, name).toContain("## Ruling mode (unblock issues)");
    }
    const devops = SDLC_BUILTIN_SKILLS.find((s) => s.name === "sdlc-devops-release")!.content;
    expect(devops).toContain("## Mode 1: Environment");
    expect(devops).toContain("## Mode 2: Release");
  });

  it("lets the lead unblock through a specialist and keeps the unblock issue out of the in_place chain", () => {
    const lead = SDLC_BUILTIN_SKILLS.find((s) => s.name === "sdlc-squad-lead")!.content;
    expect(lead).toContain("[Unblock <identifier>]");
    expect(lead).toContain("Unblock issues are the exception");
    expect(lead).toContain("--until-issue <unblock issue> --until-issue-state ended");
    expect(lead).toContain("Readiness checks");
  });

  it("ships the harness skill with its checker, template and profiles", () => {
    const harness = SDLC_BUILTIN_SKILLS.find((s) => s.name === "sdlc-harness")!;
    expect(harness.files.map((f) => f.path).sort()).toEqual([
      "references/agents-md-template.md",
      "references/profile-baas.md",
      "references/profile-backend.md",
      "references/profile-mobile.md",
      "references/profile-web.md",
      "scripts/check-harness.mjs",
    ]);
    expect(harness.content).toContain("## Headless");
    expect(harness.content).toContain("check-harness --quick");
    for (const id of ["H1", "H2", "H3", "H4", "H6", "H7", "H8", "H10"]) {
      expect(harness.content, id).toMatch(new RegExp(`\\| ${id} \\|.*\\| blocks \\|$`, "m"));
    }
    for (const id of ["H5", "H9", "H11", "H12", "H13"]) {
      expect(harness.content, id).toMatch(new RegExp(`\\| ${id} \\|.*\\| warns \\|$`, "m"));
    }
  });

  it("gates the Developer on the harness and adds no blocker class for it", () => {
    const get = (name: string) => SDLC_BUILTIN_SKILLS.find((s) => s.name === name)!;
    const lead = get("sdlc-squad-lead");
    expect(lead.content).toContain("**Harness gate.**");
    expect(lead.content).toContain("create no `todo` issue for the Developer or the Bug Fixer");
    expect(lead.content).toMatch(/\| Architect \| harness: .* \| `harness` \|/);
    expect(lead.content).toContain("`check-harness --refresh`");
    expect(lead.files.find((f) => f.path === "reference.md")!.content).toContain("Route 0. Audit the harness");
    expect(lead.files.find((f) => f.path === "team-playbook.md")!.content).toContain("`check-harness` ids");
    expect(get("sdlc-delivery-core").content).toContain("`check-harness --quick`");
    expect(get("sdlc-development").content).toContain("Run `check-harness --quick`");
    expect(get("sdlc-development").content).toContain("block with class `tooling`");
    expect(get("sdlc-devops-release").content).toContain("Ensure command:");
    expect(get("sdlc-architecture").content).toContain("sdlc-harness");
    expect(get("planning-architecture").content).toContain("`check-harness` passes every blocking check");
    // The harness failure uses an existing class: the list stays the one the core defines.
    const line = get("sdlc-delivery-core").content.match(/^Blocker-class: (.+)$/m)!;
    expect(line[1]!.split(" | ")).not.toContain("harness");
  });

  it("starts the Harness status after Environment, handled by the Architect", () => {
    const keys = SDLC_AUTO_WORKFLOW.steps.map((s) => ("key" in s.status ? s.status.key : s.status.name));
    expect(keys.indexOf("Harness")).toBe(keys.indexOf("Environment") + 1);
    const step = SDLC_AUTO_WORKFLOW.steps[keys.indexOf("Harness")]!;
    expect(step.handler).toBe("architect");
    expect(step.instructions).toContain("sdlc-harness");
  });
});
