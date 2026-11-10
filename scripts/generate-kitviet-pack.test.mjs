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
  classifyAgent,
  generateOutputs,
  removeBlocks,
  removeSection,
  renameReferences,
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

describe("renames and descriptions", () => {
  it("renames kv: references and leaves other names alone", () => {
    assert.equal(
      renameReferences("kv:soan-don-tu, kv:soan-don-tu-khac, kv:soan-don-tu.", { "soan-don-tu": "soan-don-tu-phap-ly" }),
      "kv:soan-don-tu-phap-ly, kv:soan-don-tu-khac, kv:soan-don-tu-phap-ly.",
    );
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
      ["accounting.ts", "admin.ts", "b2b.ts", "common.ts", "legal.ts", "marketing.ts", "types.ts"],
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
