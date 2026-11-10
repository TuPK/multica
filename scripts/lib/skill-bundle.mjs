// Helpers shared by the generators that turn a folder of SKILL.md files into a TypeScript
// skill bundle (scripts/generate-sdlc-skills.mjs and scripts/generate-kitviet-pack.mjs).

import { readdirSync } from "node:fs";
import { resolve } from "node:path";

// Every file beneath a skill folder except SKILL.md, as paths relative to the folder.
export function skillFiles(dir, prefix = "") {
  return readdirSync(resolve(dir, prefix), { withFileTypes: true }).flatMap((entry) => {
    const rel = prefix ? `${prefix}/${entry.name}` : entry.name;
    if (entry.isDirectory()) return skillFiles(dir, rel);
    return rel === "SKILL.md" ? [] : [rel];
  });
}

// The description of a SKILL.md frontmatter: a double-quoted, single-quoted (with `''`
// for an apostrophe) or plain single line, or a folded `>` block whose lines join with spaces.
export function frontmatterDescription(content, name) {
  const lines = content.split("\n");
  const start = lines.findIndex((line) => line.startsWith("description:"));
  if (start === -1) throw new Error(`${name}/SKILL.md has no description`);
  const value = lines[start].slice("description:".length).trim();
  if (value === ">" || value === "|") {
    const block = [];
    for (const line of lines.slice(start + 1)) {
      if (!/^\s/.test(line)) break;
      block.push(line.trim());
    }
    return block.join(" ");
  }
  const double = /^"(.*)"$/.exec(value);
  if (double) return double[1];
  const single = /^'(.*)'$/.exec(value);
  if (single) return single[1].replaceAll("''", "'");
  return value;
}
