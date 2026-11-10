// Tests for scripts/generate-kitviet-pack.mjs: the exact-match patches, the section and block
// removal, the verdict template, and the agent role classes. Run with `node --test`.

import assert from "node:assert/strict";
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import {
  applyPatches,
  applyVerdictTemplate,
  buildContext,
  classifyAgent,
  displayRole,
  generateOutputs,
  indefiniteArticle,
  removeBlocks,
  removeSection,
  resolveReferences,
  roleHeader,
  unescapeYamlQuotes,
} from "./generate-kitviet-pack.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const squadsDir = resolve(here, "../packages/core/squads");
const upstreamDir = join(squadsDir, "kitviet-upstream");
const patchesDir = join(squadsDir, "kitviet-patches");
const rewrites = JSON.parse(readFileSync(join(squadsDir, "kitviet-rewrites.json"), "utf8"));

// Every text file of the vendored upstream, so a patch can be checked against its source.
function upstreamTextFiles(dir = upstreamDir) {
  return readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) return upstreamTextFiles(path);
    return entry.endsWith(".md") ? [path] : [];
  });
}

describe("applyPatches", () => {
  it("replaces a find that occurs exactly once and counts the change", () => {
    const hits = [0];
    const out = applyPatches("a b c", [{ find: "b", replace: "B" }], "test", hits);
    assert.equal(out, "a B c");
    assert.deepEqual(hits, [1]);
  });

  it("leaves a file alone when the find does not occur in it", () => {
    const hits = [0];
    assert.equal(applyPatches("a c", [{ find: "b", replace: "B" }], "test", hits), "a c");
    assert.deepEqual(hits, [0]);
  });

  it("fails when a find occurs more than once in one file", () => {
    assert.throws(
      () => applyPatches("b b", [{ find: "b", replace: "B" }], "test", [0]),
      /matched 2 times in one file/,
    );
  });

  it("inserts the replacement literally, without $-pattern expansion", () => {
    const out = applyPatches("x", [{ find: "x", replace: "$& $1" }], "test", [0]);
    assert.equal(out, "$& $1");
  });
});

describe("every patch in kitviet-patches matches the vendored upstream", () => {
  const skillFiles = upstreamTextFiles();
  const patchFiles = readdirSync(patchesDir).filter((name) => name.endsWith(".json") && name !== "_all.json");

  it("has at least one patch file", () => {
    assert.ok(patchFiles.length > 0);
  });

  for (const file of patchFiles) {
    it(`${file}: each find occurs exactly once in some source file, and at most once in every file`, () => {
      const patches = JSON.parse(readFileSync(join(patchesDir, file), "utf8"));
      const name = file.replace(/\.json$/, "");
      const sources = skillFiles.filter((path) => path.includes(`/${name}/`) || path.endsWith(`/${name}.md`));
      assert.ok(sources.length > 0, `no vendored source for ${name}`);
      for (const { find } of patches) {
        const counts = sources.map((path) => readFileSync(path, "utf8").split(find).length - 1);
        assert.ok(
          counts.every((count) => count <= 1),
          `find occurs more than once in one file: ${find.slice(0, 80)}`,
        );
        assert.ok(
          counts.some((count) => count === 1),
          `find matches nothing in the vendored upstream: ${find.slice(0, 80)}`,
        );
      }
    });
  }
});

describe("removeBlocks and removeSection", () => {
  it("removes a Pro block with its markers and the newline after it", () => {
    const text = "a\n<!-- kv:pro:start -->\nx\n<!-- kv:pro:end -->\nb";
    assert.equal(removeBlocks(text, rewrites.removeBlocks[0]), "a\nb");
  });

  it("refuses a block with an unpaired marker", () => {
    assert.throws(() => removeBlocks("<!-- kv:pro:start -->", rewrites.removeBlocks[0]), /unpaired/);
  });

  it("removes a level-2 platform section up to the next level-2 heading", () => {
    const text = "## Mục\nkeep\n## Chạy trên từng nền tảng\n- drop\n### con\n- drop\n## Kế tiếp\nkeep";
    assert.equal(removeSection(text, "Chạy trên từng nền tảng"), "## Mục\nkeep\n## Kế tiếp\nkeep");
  });

  it("removes a platform section that ends the file", () => {
    assert.equal(removeSection("## Mục\nkeep\n## Ba môi trường\ndrop", "Ba môi trường"), "## Mục\nkeep\n");
  });
});

describe("verdict template and classes", () => {
  it("replaces only the first non-empty line under the verdict heading", () => {
    const text = "## Kết luận\n\n<QUA | CHẶN> - <lý do>\n\n## Khác\nQUA";
    assert.equal(
      applyVerdictTemplate(text, "Kết luận", "VERDICT: QUA|CHẶN - <lý do một câu>"),
      "## Kết luận\n\nVERDICT: QUA|CHẶN - <lý do một câu>\n\n## Khác\nQUA",
    );
  });

  it("leaves a body without the verdict heading unchanged, so the guard can count it", () => {
    assert.equal(applyVerdictTemplate("no heading", "Kết luận", "VERDICT: x"), "no heading");
  });

  it("classifies by tools first, then by the gate description", () => {
    assert.equal(classifyAgent("Read, Write, Edit, Glob, Grep", "Soạn bản nháp"), "drafter");
    assert.equal(classifyAgent("Read, Glob, Grep, Write", "Đối chiếu"), "handover");
    assert.equal(classifyAgent("Read, Glob, Grep", "Gác cổng - soát báo giá"), "gate");
    assert.equal(classifyAgent("Read, Glob, Grep", "Thu thập - tra cứu nguồn"), "collector");
  });
});

describe("reference resolution (F17)", () => {
  // A squad whose core has viet-email and whose role skills add soan-don-tu, with one mapped agent and
  // one agent outside the squad. Skills outside the set are known, so they read as out of squad.
  const ctx = buildContext({
    mode: "squad",
    skills: new Map([
      ["viet-email", "kv--viet-email"],
      ["soan-don-tu", "kv--soan-don-tu-phap-ly"],
    ]),
    knownSkills: new Set(["viet-email", "soan-don-tu", "bao-gia-dieu-kien-thuong-mai"]),
    roles: new Map([["soat-so-lieu", "admin_numbers_gate"]]),
    knownAgents: new Set(["soat-so-lieu", "doc-nhieu-tai-lieu"]),
    agentPhrases: {},
  });

  it("names an in-set skill by its bundle name, keeping the backticks it had", () => {
    assert.equal(resolveReferences("dùng `kv:viet-email` và kv:soan-don-tu", ctx, "t"), "dùng `kv--viet-email` và kv--soan-don-tu-phap-ly");
  });

  it("turns a skill outside the squad into a request to the lead", () => {
    assert.equal(
      resolveReferences("Luồng: `kv:bao-gia-dieu-kien-thuong-mai`.", ctx, "t"),
      "Luồng: (việc ngoài squad: nhờ lead tạo yêu cầu).",
    );
  });

  it("names a mapped agent by its role key and an unmapped agent as a lead request", () => {
    assert.equal(
      resolveReferences("giao `kv:soat-so-lieu`; bare doc-nhieu-tai-lieu", ctx, "t"),
      "giao `admin_numbers_gate`; bare (vai không có trong squad: nhờ lead)",
    );
  });

  it("does not change a name that only contains a known name", () => {
    assert.equal(resolveReferences("soat-so-lieu-thi-truong", ctx, "t"), "soat-so-lieu-thi-truong");
  });

  it("turns escaped YAML quotes into plain quotes", () => {
    assert.equal(unescapeYamlQuotes('Dùng khi nói \\"viết email\\"'), 'Dùng khi nói "viết email"');
  });
});

describe("generated output", () => {
  const outputs = generateOutputs();
  const dataDir = join(squadsDir, "kitviet-skills-data");

  it("builds the six packs and the shared types", () => {
    assert.deepEqual(
      [...outputs.keys()].sort(),
      ["accounting.ts", "admin.ts", "b2b.ts", "common.ts", "legal.ts", "marketing.ts", "squads.ts", "types.ts"],
    );
  });

  it("is deterministic: a second build yields the same text", () => {
    const again = generateOutputs();
    for (const [file, content] of outputs) assert.equal(again.get(file), content, file);
  });

  it("matches the committed files, so pnpm generate:kitviet has been run", () => {
    for (const [file, content] of outputs) {
      const path = join(dataDir, file);
      assert.ok(existsSync(path), `${file} is missing`);
      assert.equal(readFileSync(path, "utf8"), content, `${file} is stale; run pnpm generate:kitviet`);
    }
  });
});

describe("display names and the squad header", () => {
  it("writes an acronym in capitals, not as a capitalised word", () => {
    assert.equal(displayRole("b2b_lead"), "B2B Lead");
    assert.equal(displayRole("b2b_numbers_gate"), "B2B Numbers Gate");
    assert.equal(displayRole("legal_reader"), "Legal Reader");
  });

  it("takes \"an\" before a vowel-initial squad name and \"a\" otherwise", () => {
    assert.equal(indefiniteArticle("Office Admin (Hành chính)"), "an");
    assert.equal(indefiniteArticle("Accounting & Tax (Kế toán, thuế)"), "an");
    assert.equal(indefiniteArticle("B2B Proposal Desk (Chào giá B2B)"), "a");
    assert.equal(indefiniteArticle("Legal & Contracts (Pháp lý)"), "a");
    assert.equal(indefiniteArticle("Marketing Studio (Marketing)"), "a");
  });

  it("opens each role header with one \"You are\" sentence naming the role and the squad", () => {
    assert.equal(
      roleHeader("Admin Lead", "Office Admin (Hành chính)"),
      "You are the Admin Lead in an Office Admin (Hành chính) squad, unattended.",
    );
  });

  it("writes every squad name and role into squads.ts without any agent body", () => {
    const squads = generateOutputs().get("squads.ts");
    assert.ok(squads, "squads.ts is generated");
    assert.doesNotMatch(squads, /"instructions"/, "squads.ts must hold no agent body");
    assert.doesNotMatch(squads, /"content"/, "squads.ts must hold no skill text");
    assert.match(squads, /"templateId": "kv_b2b"/);
  });
});
