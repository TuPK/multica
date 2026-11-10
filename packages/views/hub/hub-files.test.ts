// @vitest-environment node

import { describe, expect, it } from "vitest";
import {
  applyPluginChanges,
  deletePluginChange,
  flattenHubTree,
  formatHubSize,
  folderMembers,
  isSemanticVersion,
  previewKind,
  prettyJson,
  renamePairs,
  renamePluginChange,
  validateHubPath,
} from "./hub-files";

describe("validateHubPath", () => {
  const existing = ["SKILL.md", "docs/guide.md", "notes.md"];

  it.each<[string, string | null]>([
    ["", "empty"],
    ["   ", "empty"],
    ["/etc/passwd", "absolute"],
    ["a/../b.md", "double_dot"],
    ["SKILL.md", "reserved"],
    ["notes.md", "exists"],
    ["docs", "is_directory"],
    ["notes.md/inner.md", "under_file"],
    ["fresh/new.md", null],
    ["  fresh.md  ", null],
  ])("%j -> %s", (path, expected) => {
    expect(validateHubPath(path, existing)).toBe(expected);
  });

  it("lets a rename keep its own name out of the existing list", () => {
    const withoutSelf = existing.filter((p) => p !== "notes.md");
    expect(validateHubPath("notes.md", withoutSelf)).toBeNull();
  });
});

describe("renamePairs and folderMembers", () => {
  const paths = ["SKILL.md", "docs/guide.md", "docs/sub/a.md", "docsx/other.md"];

  it("returns one pair for a file", () => {
    expect(renamePairs(paths, "docs/guide.md", "docs/manual.md", false)).toEqual([
      ["docs/guide.md", "docs/manual.md"],
    ]);
  });

  it("moves every file under a folder, not a sibling with a shared prefix", () => {
    expect(folderMembers(paths, "docs")).toEqual(["docs/guide.md", "docs/sub/a.md"]);
    expect(renamePairs(paths, "docs", "guides", true)).toEqual([
      ["docs/guide.md", "guides/guide.md"],
      ["docs/sub/a.md", "guides/sub/a.md"],
    ]);
  });
});

describe("flattenHubTree", () => {
  it("puts SKILL.md first, then folders, then files, and expands everything by default", () => {
    const rows = flattenHubTree(
      ["zeta.md", "SKILL.md", "docs/b.md", "docs/a.md", "alpha.md"],
      new Set(),
    );
    expect(rows.map((row) => `${row.depth}:${row.path}`)).toEqual([
      "0:SKILL.md",
      "0:docs",
      "1:docs/a.md",
      "1:docs/b.md",
      "0:alpha.md",
      "0:zeta.md",
    ]);
  });

  it("hides the rows of a collapsed folder without changing the paths", () => {
    const rows = flattenHubTree(["docs/a.md", "docs/b.md"], new Set(["docs"]));
    expect(rows).toEqual([
      { path: "docs", name: "docs", depth: 0, isFolder: true, expanded: false },
    ]);
  });
});

describe("plugin draft overlay", () => {
  const base = ["multica.plugin.json", "index.js", "README.md"];

  it("adds, edits and removes paths over the base version", () => {
    const changes = { "index.js": "new", "extra.js": "x", "README.md": null };
    expect(applyPluginChanges(base, changes)).toEqual([
      "extra.js",
      "index.js",
      "multica.plugin.json",
    ]);
  });

  it("renames a base file by removing the old path and writing the new one", () => {
    expect(renamePluginChange({}, "index.js", "main.js", "body", true)).toEqual({
      "index.js": null,
      "main.js": "body",
    });
  });

  it("renames a draft-only file without leaving a removal behind", () => {
    expect(renamePluginChange({ "scratch.js": "s" }, "scratch.js", "kept.js", "s", false)).toEqual({
      "kept.js": "s",
    });
  });

  it("deletes base files as removals and drops drafts outright", () => {
    expect(deletePluginChange({}, "index.js", true)).toEqual({ "index.js": null });
    expect(deletePluginChange({ "new.js": "n" }, "new.js", false)).toEqual({});
  });
});

describe("previewKind", () => {
  it.each<[string, boolean, string]>([
    ["docs/guide.md", false, "markdown"],
    ["page.MDX", false, "markdown"],
    ["config.json", false, "json"],
    ["icon.svg", false, "svg"],
    ["main.ts", false, "text"],
    ["photo.png", true, "binary"],
    ["photo.png", false, "binary"],
    ["data.bin", true, "binary"],
  ])("%s (binary %s) -> %s", (path, isBinary, expected) => {
    expect(previewKind(path, isBinary)).toBe(expected);
  });
});

describe("prettyJson", () => {
  it("indents valid JSON and returns null for text that does not parse", () => {
    expect(prettyJson('{"a":1,"b":[2]}')).toBe('{\n  "a": 1,\n  "b": [\n    2\n  ]\n}');
    expect(prettyJson("{not json")).toBeNull();
  });
});

describe("isSemanticVersion", () => {
  it.each([
    ["1.2.0", true],
    ["1.2.0-rc.1", true],
    ["1.2.0+build.7", true],
    ["v1.2.0", false],
    ["1.2", false],
    ["01.2.0", false],
    ["", false],
  ])("%s -> %s", (version, expected) => {
    expect(isSemanticVersion(version)).toBe(expected);
  });
});

describe("formatHubSize", () => {
  it("uses the largest unit that fits, with one decimal below 10 of it", () => {
    expect(formatHubSize(0)).toMatch(/0/);
    expect(formatHubSize(512)).toMatch(/512/);
    expect(formatHubSize(1500)).toMatch(/1\.5/);
    expect(formatHubSize(2_500_000)).toMatch(/2\.5/);
  });
});
