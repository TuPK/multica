// Tests for scripts/kitviet-update.mjs: the version and ref choice, the dirty-tree guard, the vendoring
// of the upstream trades, the registry append, and the all-or-nothing replacement. Run with `node --test`.

import assert from "node:assert/strict";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { describe, it } from "node:test";
import {
  appendRegistryEntries,
  compareVersions,
  detectLicense,
  dirtyPaths,
  GUARDED_PATHS,
  latestReleaseTag,
  parseRemoteRefs,
  pickTarget,
  registryAdditions,
  replaceTrees,
  summarizeByKey,
  upstreamSkillFolders,
  vendorUpstream,
} from "./kitviet-update.mjs";

function tempDir() {
  return mkdtempSync(join(tmpdir(), "kitviet-update-test-"));
}

function writeTree(root, files) {
  for (const [rel, content] of Object.entries(files)) {
    const path = join(root, rel);
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, content);
  }
}

describe("release tags", () => {
  it("orders semantic versions by number, not by text", () => {
    assert.ok(compareVersions("v1.10.0", "v1.9.0") > 0);
    assert.ok(compareVersions("v1.4.0", "v1.5.0") < 0);
    assert.equal(compareVersions("v1.5.0", "v1.5.0"), 0);
  });

  it("picks the highest plain release tag and ignores pre-releases and other names", () => {
    assert.equal(latestReleaseTag(["v1.4.0", "v1.5.0", "v1.10.0", "v1.11.0-rc1", "main", "v2"]), "v1.10.0");
    assert.equal(latestReleaseTag(["main"]), null);
  });
});

describe("remote refs", () => {
  it("reads the commit a peeled annotated tag points to", () => {
    const output = [
      "1111111111111111111111111111111111111111\trefs/tags/v1.5.0",
      "2222222222222222222222222222222222222222\trefs/tags/v1.5.0^{}",
      "3333333333333333333333333333333333333333\tHEAD",
    ].join("\n");
    const refs = parseRemoteRefs(output);
    assert.equal(refs.get("refs/tags/v1.5.0"), "2222222222222222222222222222222222222222");
    assert.equal(refs.get("HEAD"), "3333333333333333333333333333333333333333");
  });

  const SHA_OLD = "a".repeat(40);
  const SHA_NEW = "b".repeat(40);
  const SHA_MAIN = "c".repeat(40);
  const refs = new Map([
    ["refs/tags/v1.4.0", SHA_OLD],
    ["refs/tags/v1.5.0", SHA_NEW],
    ["refs/heads/main", SHA_MAIN],
    ["HEAD", SHA_MAIN],
  ]);

  it("uses the latest release when no ref is given", () => {
    assert.deepEqual(pickTarget({ requested: undefined, refs }), { sha: SHA_NEW, version: "v1.5.0" });
  });

  it("uses the main HEAD when the remote has no release tag", () => {
    const noTags = new Map([["HEAD", SHA_MAIN], ["refs/heads/main", SHA_MAIN]]);
    assert.deepEqual(pickTarget({ requested: undefined, refs: noTags }), { sha: SHA_MAIN, version: null });
  });

  it("resolves a requested tag, branch, or full commit", () => {
    assert.deepEqual(pickTarget({ requested: "v1.4.0", refs }), { sha: SHA_OLD, version: "v1.4.0" });
    assert.deepEqual(pickTarget({ requested: "main", refs }), { sha: SHA_MAIN, version: null });
    const full = "d".repeat(40);
    assert.deepEqual(pickTarget({ requested: full, refs }), { sha: full, version: null });
  });

  it("fails on a ref the remote does not have", () => {
    assert.throws(() => pickTarget({ requested: "v9.9.9", refs }), /unknown ref v9\.9\.9/);
  });
});

describe("dirty guard", () => {
  it("guards every input that the registry capture reads, not only the written paths", () => {
    for (const path of [
      "catalog/_sources/kitviet/rewrites.json",
      "catalog/_sources/kitviet/squads",
      "packages/core/squads/kitviet-agent-instructions.ts",
      "packages/core/squads/kitviet-squad-roles.ts",
      "catalog/squads",
      "packages/core/squads/templates.ts",
      "packages/core/agents/instruction-blocks.ts",
      "scripts/generate-kitviet-pack.mjs",
    ]) {
      assert.ok(GUARDED_PATHS.includes(path), path);
    }
  });

  it("lists only the changed paths inside the paths the update writes", () => {
    const porcelain = [
      " M packages/core/squads/kitviet-previous-texts.ts",
      "?? catalog/_sources/kitviet/upstream/new.md",
      " M docs/engineering/kitviet-squads.md",
      " M catalog/_sources/kitviet/squads/construction.json",
    ].join("\n");
    const writes = [
      "catalog/_sources/kitviet/upstream",
      "packages/core/squads/kitviet-skills-data",
      "packages/core/squads/kitviet-previous-texts.ts",
    ];
    assert.deepEqual(dirtyPaths(porcelain, writes), [
      " M packages/core/squads/kitviet-previous-texts.ts",
      "?? catalog/_sources/kitviet/upstream/new.md",
    ]);
  });

  it("does not match a path that only shares a prefix with a write path", () => {
    assert.deepEqual(dirtyPaths(" M catalog/_sources/kitviet/upstream-old.md", ["catalog/_sources/kitviet/upstream"]), []);
  });
});

describe("license", () => {
  it("recognises the MIT license text", () => {
    assert.equal(detectLicense("MIT License\n\nCopyright (c) 2026 X"), "MIT");
  });

  it("refuses a license it does not know", () => {
    assert.throws(() => detectLicense("Apache License\nVersion 2.0"), /unknown license/);
  });
});

describe("vendoring the upstream trades", () => {
  // Two trades; a file byte-identical in both goes to common/, a file that differs stays in its trade.
  // Only claude-code/kv is copied. CRLF text is stored with LF, as the vendored copy has always been.
  function fixture() {
    const up = tempDir();
    writeTree(up, {
      "LICENSE": "MIT License\n",
      "alpha/claude-code/kv/skills/shared/SKILL.md": "same\n",
      "alpha/claude-code/kv/skills/own/SKILL.md": "alpha\n",
      "alpha/claude-code/kv/agents/bot.md": "agent a\r\nline\r\n",
      "alpha/codex/.agents/skills/x/SKILL.md": "not copied\n",
      "beta/claude-code/kv/skills/shared/SKILL.md": "same\n",
      "beta/claude-code/kv/skills/own/SKILL.md": "beta\n",
      "beta/claude-code/kv/agents/bot.md": "agent b\n",
    });
    return up;
  }

  it("moves identical files to common/ and keeps the others in their trade", () => {
    const up = fixture();
    const out = join(tempDir(), "kitviet-upstream");
    const result = vendorUpstream({ treeDir: up, trades: ["alpha", "beta"], outDir: out });
    assert.equal(readFileSync(join(out, "common/skills/shared/SKILL.md"), "utf8"), "same\n");
    assert.equal(readFileSync(join(out, "alpha/skills/own/SKILL.md"), "utf8"), "alpha\n");
    assert.equal(readFileSync(join(out, "beta/skills/own/SKILL.md"), "utf8"), "beta\n");
    assert.equal(existsSync(join(out, "alpha/skills/shared")), false);
    assert.equal(existsSync(join(out, "alpha/codex")), false);
    assert.deepEqual(result.mismatches, []);
    assert.equal(result.common, 1);
  });

  it("stores CRLF text with LF endings", () => {
    const up = fixture();
    const out = join(tempDir(), "kitviet-upstream");
    vendorUpstream({ treeDir: up, trades: ["alpha", "beta"], outDir: out });
    assert.equal(readFileSync(join(out, "alpha/agents/bot.md"), "utf8"), "agent a\nline\n");
  });

  it("fails when a trade has no claude-code/kv folder", () => {
    const up = fixture();
    assert.throws(() => vendorUpstream({ treeDir: up, trades: ["alpha", "gamma"], outDir: join(tempDir(), "out") }), /gamma/);
  });
});

describe("registry additions", () => {
  const empty = { agents: {}, squads: [] };

  it("captures the replaced text of a role whose text changed", () => {
    const additions = registryAdditions(empty, { agents: { r1: "old1", r2: "same" }, squads: {} }, { agents: { r1: "new1", r2: "same" }, squads: {} });
    assert.deepEqual(additions, { agents: { r1: ["old1"] }, squads: [] });
  });

  it("captures nothing when no text changed", () => {
    const same = { agents: { r1: "x" }, squads: { t1: "y" } };
    assert.deepEqual(registryAdditions(empty, same, same), { agents: {}, squads: [] });
  });

  it("does not duplicate a text the registry already lists", () => {
    const previous = { agents: { r1: ["old1"] }, squads: ["sqA"] };
    const additions = registryAdditions(
      previous,
      { agents: { r1: "old1" }, squads: { t1: "sqA" } },
      { agents: { r1: "new1" }, squads: { t1: "sqB" } },
    );
    assert.deepEqual(additions, { agents: {}, squads: [] });
  });

  it("captures the text of a role that the new release removed", () => {
    assert.deepEqual(registryAdditions(empty, { agents: { gone: "g" }, squads: {} }, { agents: {}, squads: {} }), {
      agents: { gone: ["g"] },
      squads: [],
    });
  });

  it("captures nothing for a role that is new in the release", () => {
    assert.deepEqual(registryAdditions(empty, { agents: {}, squads: {} }, { agents: { fresh: "f" }, squads: {} }), {
      agents: {},
      squads: [],
    });
  });

  it("captures a replaced squad text by its template", () => {
    assert.deepEqual(registryAdditions(empty, { agents: {}, squads: { t1: "sqA" } }, { agents: {}, squads: { t1: "sqB" } }), {
      agents: {},
      squads: ["sqA"],
    });
  });
});

describe("appending to the registry source", () => {
  const source = [
    "// header",
    "export const KITVIET_PREVIOUS_AGENT_INSTRUCTIONS: Record<string, readonly string[]> = {",
    '  "r1": [',
    '    "old1",',
    "  ],",
    "};",
    "",
    "export const KITVIET_PREVIOUS_SQUAD_INSTRUCTIONS: readonly string[] = [",
    '  "sq1",',
    "];",
    "",
  ].join("\n");

  it("returns the source unchanged when there is nothing to add", () => {
    assert.equal(appendRegistryEntries(source, { agents: {}, squads: [] }), source);
  });

  it("adds texts under an existing role, adds a new role, and adds squad texts, keeping the old lines", () => {
    const out = appendRegistryEntries(source, { agents: { r1: ["old1b"], r9: ["n9"] }, squads: ["sq2"] });
    assert.equal(
      out,
      [
        "// header",
        "export const KITVIET_PREVIOUS_AGENT_INSTRUCTIONS: Record<string, readonly string[]> = {",
        '  "r1": [',
        '    "old1",',
        '    "old1b",',
        "  ],",
        '  "r9": [',
        '    "n9",',
        "  ],",
        "};",
        "",
        "export const KITVIET_PREVIOUS_SQUAD_INSTRUCTIONS: readonly string[] = [",
        '  "sq1",',
        '  "sq2",',
        "];",
        "",
      ].join("\n"),
    );
  });

  it("escapes a text the way a string literal needs", () => {
    const out = appendRegistryEntries(source, { agents: {}, squads: ['say "hi"\nnext'] });
    assert.ok(out.includes('  "say \\"hi\\"\\nnext",\n];'));
  });
});

describe("summaries", () => {
  it("lists skill folders per trade and in common, and ignores agents", () => {
    const root = tempDir();
    writeTree(root, {
      "common/skills/shared/SKILL.md": "x",
      "alpha/skills/own/SKILL.md": "x",
      "beta/skills/own/SKILL.md": "x",
      "alpha/agents/bot.md": "x",
    });
    assert.deepEqual(upstreamSkillFolders(root, ["alpha", "beta"]), ["alpha/own", "beta/own", "common/shared"]);
  });

  it("lists added, removed and changed keys", () => {
    const before = new Map([
      ["a", "1"],
      ["b", "2"],
      ["c", "3"],
    ]);
    const after = new Map([
      ["b", "2"],
      ["c", "33"],
      ["d", "4"],
    ]);
    assert.deepEqual(summarizeByKey(before, after), { added: ["d"], removed: ["a"], changed: ["c"] });
  });
});

describe("all-or-nothing replacement", () => {
  it("leaves every target unchanged when a later source is missing", () => {
    const root = tempDir();
    writeTree(root, { "repo/a/x.txt": "old", "repo/b.txt": "old-b", "stage/a/x.txt": "new" });
    const plan = [
      { from: join(root, "stage/a"), to: join(root, "repo/a"), kind: "dir" },
      { from: join(root, "stage/missing.txt"), to: join(root, "repo/b.txt"), kind: "file" },
    ];
    assert.throws(() => replaceTrees(plan, join(root, "backup")), /missing source/);
    assert.equal(readFileSync(join(root, "repo/a/x.txt"), "utf8"), "old");
    assert.equal(readFileSync(join(root, "repo/b.txt"), "utf8"), "old-b");
  });

  it("replaces a directory whole, removing stale files, and a file", () => {
    const root = tempDir();
    writeTree(root, {
      "repo/a/x.txt": "old",
      "repo/a/stale.txt": "stale",
      "repo/b.txt": "old-b",
      "stage/a/x.txt": "new",
      "stage/b.txt": "new-b",
    });
    replaceTrees(
      [
        { from: join(root, "stage/a"), to: join(root, "repo/a"), kind: "dir" },
        { from: join(root, "stage/b.txt"), to: join(root, "repo/b.txt"), kind: "file" },
      ],
      join(root, "backup"),
    );
    assert.equal(readFileSync(join(root, "repo/a/x.txt"), "utf8"), "new");
    assert.equal(existsSync(join(root, "repo/a/stale.txt")), false);
    assert.equal(readFileSync(join(root, "repo/b.txt"), "utf8"), "new-b");
  });
});
