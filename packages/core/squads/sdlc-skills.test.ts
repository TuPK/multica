// @vitest-environment node
import { readdirSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { SDLC_BUILTIN_SKILLS } from "./sdlc-skills-data";

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
];

// Every other skill in the bundle: the tools and plugin skills the boss attached to the
// SDLC squad. They ship beside the role skills, copied from the workspace, so every SDLC
// squad has them. A folder name stores a colon as `--`.
const TOOL_SKILLS = readdirSync(skillsDir, { withFileTypes: true })
  .filter((entry) => entry.isDirectory() && !ROLE_SKILLS.includes(entry.name))
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
    expect(SDLC_BUILTIN_SKILLS.map((s) => s.name)).toEqual([...ROLE_SKILLS, ...TOOL_SKILLS].sort());
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
    expect(lead.files.map((f) => f.path)).toEqual(["reference.md"]);
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

  it("gives every role skill a self-check before it reports", () => {
    for (const skill of SDLC_BUILTIN_SKILLS.filter((s) => ROLE_SKILLS.includes(s.name))) {
      expect(skill.content, skill.name).toContain("Self-check before you report");
    }
  });
});
