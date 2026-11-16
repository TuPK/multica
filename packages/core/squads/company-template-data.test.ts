// @vitest-environment node
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { AGENT_INSTRUCTION_TEMPLATES } from "../agents/instruction-templates";
import { COMPANY_SQUAD_CARDS_DATA } from "../catalog/generated/company";
import {
  COMPANY_FLOOR_KEYS,
  COMPANY_FLOOR_NAMES,
  COMPANY_STAGE_OWNERS,
  COMPANY_START_OPTIONS,
} from "./company-template";

function skillFiles(dir: string, prefix = ""): string[] {
  return readdirSync(resolve(dir, prefix), { withFileTypes: true }).flatMap((entry) => {
    const rel = prefix ? `${prefix}/${entry.name}` : entry.name;
    if (entry.isDirectory()) return skillFiles(dir, rel);
    return rel === "SKILL.md" ? [] : [rel];
  });
}

function frontmatterDescription(content: string, name: string): string {
  const lines = content.split("\n");
  const start = lines.findIndex((line) => line.startsWith("description:"));
  if (start === -1) throw new Error(`${name}/SKILL.md has no description`);
  const value = lines[start]!.slice("description:".length).trim();
  if (value === ">" || value === "|") {
    const block: string[] = [];
    for (const line of lines.slice(start + 1)) {
      if (!/^\s/.test(line)) break;
      block.push(line.trim());
    }
    return block.join(" ");
  }
  const double = /^"(.*)"$/.exec(value);
  if (double) return double[1]!;
  const single = /^'(.*)'$/.exec(value);
  if (single) return single[1]!.replaceAll("''", "'");
  return value;
}

// The Go apply service embeds this file. `pnpm catalog:build` writes it from the same catalog data the
// constants above come from; this test fails when it is missing or stale (build not rerun).
const generated = JSON.parse(
  readFileSync(new URL("../../../server/internal/catalog/generated/company.json", import.meta.url), "utf8"),
) as Record<string, unknown>;

describe("company template data for the server", () => {
  it("carries every squad card, unchanged", () => {
    expect(generated.squads).toEqual(COMPANY_SQUAD_CARDS_DATA);
  });

  it("carries the floors, the start options and the stage owners", () => {
    expect(generated.floor_keys).toEqual([...COMPANY_FLOOR_KEYS]);
    expect(generated.floor_names).toEqual(COMPANY_FLOOR_NAMES);
    expect(generated.start_options).toEqual([...COMPANY_START_OPTIONS]);
    expect(generated.stage_owners).toEqual(COMPANY_STAGE_OWNERS);
  });

  it("carries the starter skills, unchanged from their folders", () => {
    const catalogDir = fileURLToPath(new URL("../../../catalog", import.meta.url));
    const skillsetsDir = resolve(catalogDir, "skillsets");
    const starterSets = new Set(
      readdirSync(skillsetsDir).filter((set) => {
        const manifest = resolve(skillsetsDir, set, "template.json");
        return existsSync(manifest) && (JSON.parse(readFileSync(manifest, "utf8")) as { target?: string }).target === "starter";
      }),
    );

    const skillsDir = resolve(catalogDir, "skills");
    const skillEntries: { folder: string; dir: string; id: string }[] = [];
    for (const group of readdirSync(skillsDir)) {
      const groupDir = resolve(skillsDir, group);
      for (const name of readdirSync(groupDir)) {
        const dir = resolve(groupDir, name);
        const manifest = resolve(dir, "template.json");
        if (!existsSync(manifest)) continue;
        const tmpl = JSON.parse(readFileSync(manifest, "utf8")) as { id: string; sets?: string[] };
        if ((tmpl.sets ?? []).some((set) => starterSets.has(set))) {
          skillEntries.push({ folder: name, dir, id: tmpl.id });
        }
      }
    }
    skillEntries.sort((a, b) => a.folder.localeCompare(b.folder));

    const expected = skillEntries.map((entry) => {
      const content = readFileSync(resolve(entry.dir, "SKILL.md"), "utf8");
      const files = skillFiles(entry.dir)
        .filter((file) => file !== "template.json" && !file.startsWith("i18n/") && file.endsWith(".md") && !file.includes("/"))
        .sort()
        .map((file) => ({ path: file, content: readFileSync(resolve(entry.dir, file), "utf8") }));
      return {
        name: entry.id,
        description: frontmatterDescription(content, entry.folder),
        content,
        files,
      };
    });

    expect(generated.starter_skills).toEqual(expected);
  });

  it("carries every agent instruction template, id and text", () => {
    expect(generated.instruction_templates).toEqual(
      AGENT_INSTRUCTION_TEMPLATES.map(({ id, instructions }) => ({ id, instructions })),
    );
  });
});
