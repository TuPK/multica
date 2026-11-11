// @vitest-environment node
// Guards the text a Kit Viet lead reads about its human-stop comments (drill defect D5): each stop is its own
// comment, and its first line is the stop header, so no plan or summary sits above it. The texts are the
// generated lead skill and the composed lead instruction. This checks the wording only; no lead is run here.

import { describe, expect, it } from "vitest";
import { KV_LEAD_NOTES, composeKvAgentInstructions } from "./kitviet-agent-instructions";
import { KITVIET_BUILTIN_SKILLS } from "./kitviet-skills-data/common";

function skillContent(name: string): string {
  const skill = KITVIET_BUILTIN_SKILLS.find((s) => s.name === name);
  if (!skill) throw new Error(`missing skill ${name}`);
  return skill.content;
}

const LEAD = skillContent("kv--squad-lead");
const LEAD_KIND = composeKvAgentInstructions({ kind: "kv_lead" });

// The three stop headers, in the order the lead skill's Templates section lists them.
const STOP_HEADERS = ["BLOCKED · stop 1 · ", "IN REVIEW · stop 2 · ", "BLOCKED · stop 3 · "];

// The fenced template blocks under "## Templates", up to the next section.
function stopTemplates(text: string): string[] {
  const start = text.indexOf("## Templates");
  const end = text.indexOf("## Rules for the summary");
  if (start < 0 || end < start) throw new Error("the lead skill has no Templates section");
  return [...text.slice(start, end).matchAll(/```\n([\s\S]*?)```/g)].map((m) => m[1] ?? "");
}

describe("each stop comment starts with its stop header (drill D5)", () => {
  const templates = stopTemplates(LEAD);

  it("the lead skill has the three stop templates", () => {
    expect(templates).toHaveLength(STOP_HEADERS.length);
  });

  it.each(STOP_HEADERS.map((header, i) => [header, i] as const))(
    "stop template %s begins with its header on the first line",
    (header, i) => {
      expect(templates[i]?.split("\n")[0]?.startsWith(header)).toBe(true);
    },
  );

  it("the lead skill states that the stop header is the first line, with nothing above it", () => {
    expect(LEAD).toContain("its first line is the header below");
    expect(LEAD).toContain("with nothing above it");
    expect(LEAD).toContain("never put a `## Plan` heading or a summary in front of a stop header");
  });

  it("the lead skill's step 7 posts the stop as its own comment", () => {
    expect(LEAD).toContain("as its own comment, whose first line is the stop header");
  });

  it("the composed lead instruction states the same rule", () => {
    expect(LEAD_KIND).toContain("A human-stop comment is its own comment on the request.");
    expect(LEAD_KIND).toContain("Its first line is the stop header from kv--squad-lead");
    expect(LEAD_KIND).toContain("Nothing goes above that line");
  });

  it("the squad notes that post a stop after a plan name the stop 1 header", () => {
    for (const key of ["kv_marketing", "kv_pr"] as const) {
      expect(KV_LEAD_NOTES[key], key).toContain("its first line is the stop 1 header, not a Plan");
    }
  });
});
