// @vitest-environment node

// Every knowledge draft a Kit Viet squad files carries the headings its kind requires. The server
// refuses a filing without them (server/internal/service/knowledge_kind.go), so a draft template
// that drops one is a bug the agent would hit only at filing time.

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  KV_KB_REQUIRED_HEADINGS,
  KV_LEAD_NOTES,
  KV_PLAYBOOK_DRAFT,
  KV_REGISTER_DRAFT,
} from "./kitviet-agent-instructions";

const here = dirname(fileURLToPath(import.meta.url));
const skillText = (folder: string) => readFileSync(join(here, "kitviet-skills", folder, "SKILL.md"), "utf8");

// The "## " headings of a draft, in order, as the server reads them.
const headingsOf = (text: string) => text.split("\n").filter((line) => line.startsWith("## ")).map((line) => line.slice(3).trim());

// The text between two headings of a skill, used to test one template inside a skill.
function sectionOf(text: string, title: string, nextTitle: string): string {
  const start = text.indexOf(title);
  const end = text.indexOf(nextTitle, start + title.length);
  if (start < 0 || end < 0) throw new Error(`section ${title} .. ${nextTitle} not found`);
  return text.slice(start, end);
}

// Fails naming the missing heading, so a dropped heading shows which kind and which draft.
function expectRequiredHeadings(draft: string, kind: keyof typeof KV_KB_REQUIRED_HEADINGS, label: string) {
  const headings = headingsOf(draft);
  for (const required of KV_KB_REQUIRED_HEADINGS[kind]) {
    expect(headings, `${label} is missing "## ${required}" required by kind ${kind}`).toContain(required);
  }
}

const REGISTER_COLUMNS = [
  "id",
  "Văn bản",
  "Điều khoản dùng",
  "Hiệu lực / valid_until",
  "Nguồn và ngày truy cập",
  "Người xác minh",
  "Ngày xác minh",
];

const headerRowOf = (text: string) => text.split("\n").find((line) => line.startsWith("| id |"));

describe("the required headings mirror the server", () => {
  it("lists the headings of reference and playbook as the server does", () => {
    expect(KV_KB_REQUIRED_HEADINGS.reference).toEqual(["Summary"]);
    expect(KV_KB_REQUIRED_HEADINGS.playbook).toEqual(["Purpose", "Steps"]);
  });
});

describe("draft templates carry their kind's headings", () => {
  it("the company profile template starts its filed document with ## Summary", () => {
    const template = sectionOf(skillText("kv-company-profile"), "## Document template", "## Limits");
    expectRequiredHeadings(template, "reference", "the Hồ sơ đơn vị template");
    expect(skillText("kv-company-profile")).toContain("--kind reference");
  });

  it("the Sổ quy định draft has ## Summary and the seven register columns, in order", () => {
    expectRequiredHeadings(KV_REGISTER_DRAFT, "reference", "the Sổ quy định draft");
    expect(headerRowOf(KV_REGISTER_DRAFT)?.split("|").map((cell) => cell.trim()).filter(Boolean)).toEqual(REGISTER_COLUMNS);
  });

  it("the brand voice playbook draft has ## Purpose and ## Steps", () => {
    expectRequiredHeadings(KV_PLAYBOOK_DRAFT, "playbook", "the Cẩm nang giọng thương hiệu draft");
  });

  it("the delivery core names the headings each kind needs", () => {
    const core = skillText("kv-delivery-core");
    expect(core).toContain("## Summary");
    expect(core).toContain("## Purpose");
    expect(core).toContain("## Steps");
  });
});

describe("the lead of each register squad carries the register format", () => {
  for (const squad of ["kv_legal", "kv_accounting", "kv_marketing", "kv_admin"] as const) {
    it(`${squad} lead notes contain the Sổ quy định draft`, () => {
      const notes = KV_LEAD_NOTES[squad] ?? "";
      expect(notes).toContain(KV_REGISTER_DRAFT);
    });
  }

  it("the marketing lead notes contain the brand voice playbook draft", () => {
    expect(KV_LEAD_NOTES.kv_marketing ?? "").toContain(KV_PLAYBOOK_DRAFT);
  });

  it("the accounting register rows each have seven cells and an unverified verifier", () => {
    const rows = (KV_LEAD_NOTES.kv_accounting ?? "").split("\n").filter((line) => /^\| KT-\d\d /.test(line));
    expect(rows).toHaveLength(4);
    for (const row of rows) {
      const cells = row.split("|").slice(1, -1).map((cell) => cell.trim());
      expect(cells, row).toHaveLength(REGISTER_COLUMNS.length);
      expect(cells[5], row).toBe("[chưa xác minh]");
      expect(cells[6], row).toBe("[chưa xác minh]");
    }
  });

  it("the fixed issues are named by their exact titles", () => {
    expect(KV_LEAD_NOTES.kv_legal ?? "").toContain("\"Sổ hợp đồng\"");
    expect(KV_LEAD_NOTES.kv_admin ?? "").toContain("\"Sổ công văn <năm>\"");
  });
});
