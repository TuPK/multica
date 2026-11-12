// Tests for scripts/generate-kitviet-pack.mjs: the exact-match patches, the section and block
// removal, the verdict template, and the agent role classes. Run with `node --test`.

import assert from "node:assert/strict";
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
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
  loadConfig,
  removeBlocks,
  removeSection,
  resolveReferences,
  roleHeader,
  unescapeYamlQuotes,
} from "./generate-kitviet-pack.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const squadsDir = resolve(here, "../catalog/_sources/kitviet");
const upstreamDir = join(squadsDir, "upstream");
const patchesDir = join(squadsDir, "patches");
const rewrites = JSON.parse(readFileSync(join(squadsDir, "rewrites.json"), "utf8"));

// Every text file of the vendored upstream (prose and the export scripts), so a patch can be checked
// against its source.
function upstreamTextFiles(dir = upstreamDir) {
  return readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) return upstreamTextFiles(path);
    return entry.endsWith(".md") || entry.endsWith(".py") ? [path] : [];
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

describe("every patch in patches matches the vendored upstream", () => {
  const skillFiles = upstreamTextFiles();
  const patchFiles = readdirSync(patchesDir).filter((name) => name.endsWith(".json"));

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

  it("builds one pack per config file, and the shared files", () => {
    const ids = (dir) =>
      readdirSync(join(squadsDir, dir))
        .filter((file) => file.endsWith(".json") && file !== "base.json")
        .map((file) => file.replace(/\.json$/, ""));
    const packs = ["common", ...ids("squads"), ...ids("extensions")].map((id) => `${id}.ts`);
    assert.deepEqual([...outputs.keys()].sort(), [...packs, "extensions.ts", "packs.ts", "squads.ts", "types.ts"].sort());
  });

  it("names every squad and extension in packs.ts, so the loader reaches each pack", () => {
    const loaders = outputs.get("packs.ts");
    for (const id of loadConfig(squadsDir).squads.map((squad) => squad.id)) assert.match(loaders, new RegExp(`"${id}": \\(\\) => import`));
    for (const extension of loadConfig(squadsDir).extensions) assert.match(loaders, new RegExp(`"${extension.id}": \\(\\) => import`));
  });

  it("is deterministic: a second build yields the same text", () => {
    const again = generateOutputs();
    for (const [file, content] of outputs) assert.equal(again.get(file), content, file);
  });

  // The generator guard throws on these too; this test names them so a failure reads as F9 or F22.
  it("carries no install hint, versioned file name or install instruction (F9, F22)", () => {
    const leftovers = [/pip install|apt install/, /hoặc cài LibreOffice/, /Cài bằng lệnh/, /-v\d\.md/];
    for (const [file, content] of outputs) {
      for (const re of leftovers) assert.doesNotMatch(content, re, `${file} still matches ${re}`);
    }
  });
});

describe("display names and the squad header", () => {
  it("writes an acronym in capitals, not as a capitalised word", () => {
    assert.equal(displayRole("b2b_lead"), "B2B Lead");
    assert.equal(displayRole("b2b_numbers_gate"), "B2B Numbers Gate");
    assert.equal(displayRole("legal_reader"), "Legal Reader");
    assert.equal(displayRole("fnb_lead"), "F&B Lead");
    assert.equal(displayRole("fnb_allergen_gate"), "F&B Allergen Gate");
    assert.equal(displayRole("edu_lead"), "Edu Lead");
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

// Each guard of the config split fires on a bad config (spec 5.5, 5.10, 7). The fixture is a copy of the
// real base config plus the configs under test, so the other packs stay as they are.
describe("config guards", () => {
  const readConfig = (path) => JSON.parse(readFileSync(join(squadsDir, path), "utf8"));
  // A copy of the real configs (so the global patch and rule checks see every P0 entry), with the files
  // under test written over it. A test then fails only on the guard it names.
  function fixture(files) {
    const root = mkdtempSync(join(tmpdir(), "kv-config-"));
    for (const folder of ["squads", "extensions"]) {
      mkdirSync(join(root, folder), { recursive: true });
      for (const file of readdirSync(join(squadsDir, folder))) copyFileSync(join(squadsDir, folder, file), join(root, folder, file));
    }
    for (const [path, value] of Object.entries(files)) writeFileSync(join(root, path), JSON.stringify(value, null, 2));
    return root;
  }
  const admin = readConfig("squads/admin.json");
  const lead = { "lap-ke-hoach-dieu-phoi": { role: "office_lead", kind: "kv_lead" } };
  const bareSquad = (id, extra) => ({
    templateId: `kv_${id}`,
    name: "Test Squad",
    trade: "hanh-chinh-van-phong",
    agents: lead,
    builtinSkills: [],
    leaderSkills: ["kv-squad-lead", "kv-company-profile"],
    roleSkills: {},
    promoted: [],
    ...extra,
  });

  it("rejects a skill listed by a squad and by an extension", () => {
    const root = fixture({
      "squads/admin.json": admin,
      "extensions/dup.json": { trade: "hanh-chinh-van-phong", targets: ["some_target"], skills: ["soan-don-tu"] },
    });
    assert.throws(() => generateOutputs({ configRoot: root }), /soan-don-tu \(hanh-chinh-van-phong\) is listed by squad admin and by extension dup/);
  });

  it("rejects a pipeline squad with no upstream role besides its lead", () => {
    const root = fixture({ "squads/lonely.json": bareSquad("lonely", {}) });
    assert.throws(() => generateOutputs({ configRoot: root }), /a pipeline squad needs an upstream role besides the lead/);
  });

  it("rejects an authored squad that attaches an upstream pipeline skill", () => {
    const root = fixture({
      "squads/office.json": bareSquad("office", { flow: "authored", builtinSkills: ["day-chuyen-van-ban-hanh-chinh"] }),
    });
    assert.throws(() => generateOutputs({ configRoot: root }), /an authored flow cannot attach the upstream pipeline day-chuyen-van-ban-hanh-chinh/);
  });

  it("accepts an authored squad with only its lead, and gives it an empty pack", () => {
    const root = fixture({ "squads/office.json": bareSquad("office", { flow: "authored" }) });
    const packs = generateOutputs({ configRoot: root });
    assert.match(packs.get("packs.ts"), /"office": \(\) => import/);
    assert.match(packs.get("squads.ts"), /"templateId": "kv_office"/);
    assert.match(packs.get("office.ts"), /export const KITVIET_BUILTIN_SKILLS: readonly KitvietSkill\[\] = \[\];/);
  });
});
