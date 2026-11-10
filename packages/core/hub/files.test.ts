// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import { MAX_SKILL_FILE_BYTES } from "../skills/pack-archive";
import type { PluginPackage, Skill } from "../types";
import {
  assertSkillFileUnchanged,
  HubFileConflictError,
  hubFileContentOptions,
  hubFileListOptions,
  hubKeys,
  isSkillContentPath,
  skillFileContent,
  skillFileList,
  validateHubFilePath,
} from "./files";

function makeSkill(overrides: Partial<Skill> = {}): Skill {
  return {
    id: "skill-1",
    workspace_id: "ws-1",
    name: "Writer",
    description: "",
    config: {},
    created_by: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-05T00:00:00Z",
    content: "# Writer",
    files: [
      { id: "f-b", skill_id: "skill-1", path: "scripts/run.sh", content: "echo hi", created_at: "", updated_at: "2026-01-03T00:00:00Z" },
      { id: "f-a", skill_id: "skill-1", path: "README.md", content: "readme", created_at: "", updated_at: "2026-01-02T00:00:00Z" },
    ],
    ...overrides,
  };
}

function makePackage(): PluginPackage {
  return {
    id: "pkg-1",
    plugin_key: "acme.hello",
    name: "Hello",
    created_at: "2026-01-01T00:00:00Z",
    versions: [
      { id: "v-new", version: "1.1.0", digest: "", size_bytes: 0, published_at: "", installed: false },
      { id: "v-old", version: "1.0.0", digest: "", size_bytes: 0, published_at: "", installed: false },
    ],
  };
}

afterEach(() => {
  setApiInstance(null as unknown as ApiClient);
});

describe("hub file keys and paths", () => {
  it("scopes every hub file key by workspace", () => {
    expect(hubKeys.fileList("ws-1", "skill", "s1")).toContain("ws-1");
    expect(hubKeys.fileContent("ws-2", "plugin", "p1", "a.js", "v1")).toContain("ws-2");
    expect(hubKeys.fileList("ws-1", "skill", "s1")).not.toEqual(hubKeys.fileList("ws-2", "skill", "s1"));
  });

  it("matches SKILL.md case-insensitively", () => {
    expect(isSkillContentPath("SKILL.md")).toBe(true);
    expect(isSkillContentPath("skill.md")).toBe(true);
    expect(isSkillContentPath("docs/SKILL.md")).toBe(false);
  });

  it.each(["scripts/run.sh", "README.md", "a/b/c.txt"])("accepts %s", (path) => {
    expect(validateHubFilePath(path)).toBe(path);
  });

  it.each(["", "/abs.md", "../escape.md", "a/../b", "a//b", "./a", "a/", "a\\b", "C:x", "a\0b"])(
    "rejects %j",
    (path) => {
      expect(() => validateHubFilePath(path)).toThrow(/Invalid file path/);
    },
  );
});

describe("skill file mapping", () => {
  it("lists SKILL.md first, then other files sorted by path, with byte sizes", () => {
    const list = skillFileList(makeSkill({ content: "é" }));
    expect(list).toEqual([
      { path: "SKILL.md", size_bytes: 2 },
      { path: "README.md", size_bytes: 6 },
      { path: "scripts/run.sh", size_bytes: 7 },
    ]);
  });

  it("uses the skill updated_at as SKILL.md's etag and the file updated_at for other files", () => {
    const skill = makeSkill();
    expect(skillFileContent(skill, "SKILL.md")).toEqual({
      path: "SKILL.md",
      content: "# Writer",
      is_binary: false,
      size_bytes: 8,
      etag: "2026-01-05T00:00:00Z",
    });
    expect(skillFileContent(skill, "README.md")).toMatchObject({
      path: "README.md",
      content: "readme",
      etag: "2026-01-02T00:00:00Z",
    });
  });

  it("returns null content for a file over 1 MiB", () => {
    const big = "x".repeat(MAX_SKILL_FILE_BYTES + 1);
    const file = skillFileContent(makeSkill({ files: [
      { id: "f", skill_id: "skill-1", path: "big.txt", content: big, created_at: "", updated_at: "t" },
    ] }), "big.txt");
    expect(file.content).toBeNull();
    expect(file.size_bytes).toBe(MAX_SKILL_FILE_BYTES + 1);
  });

  it("throws for a path the skill does not have", () => {
    expect(() => skillFileContent(makeSkill(), "missing.md")).toThrow("File not found: missing.md");
  });
});

describe("conflict detection", () => {
  const skill = makeSkill();

  it("passes when the etag still matches", () => {
    expect(() => assertSkillFileUnchanged(skill, "README.md", "2026-01-02T00:00:00Z")).not.toThrow();
  });

  it("throws HubFileConflictError when the file changed since it was loaded", () => {
    expect(() => assertSkillFileUnchanged(skill, "README.md", "2025-12-31T00:00:00Z")).toThrow(HubFileConflictError);
  });

  it("throws when the file was deleted since it was loaded", () => {
    expect(() => assertSkillFileUnchanged(skill, "gone.md", "2026-01-02T00:00:00Z")).toThrow(HubFileConflictError);
  });

  it("allows an empty etag only for a file that does not exist yet", () => {
    expect(() => assertSkillFileUnchanged(skill, "new.md", "")).not.toThrow();
    expect(() => assertSkillFileUnchanged(skill, "README.md", "")).toThrow(HubFileConflictError);
  });

  it("is a conflict error that is also an Error", () => {
    const error = new HubFileConflictError();
    expect(error).toBeInstanceOf(Error);
    expect(error.name).toBe("HubFileConflictError");
  });
});

describe("query options", () => {
  it("loads skill files from getSkill", async () => {
    const getSkill = vi.fn(async () => makeSkill());
    setApiInstance({ getSkill } as unknown as ApiClient);
    const list = await hubFileListOptions("ws-1", "skill", "skill-1").queryFn?.({} as never);
    expect(getSkill).toHaveBeenCalledWith("skill-1");
    expect(list).toEqual(expect.arrayContaining([{ path: "SKILL.md", size_bytes: 8 }]));
  });

  it("resolves the newest plugin version when none is named", async () => {
    const listPluginPackages = vi.fn(async () => ({ packages: [makePackage()] }));
    const listPluginVersionFiles = vi.fn(async () => ({
      files: [{ path: "z.js", size_bytes: 3, sha256: "h2" }, { path: "a.js", size_bytes: 2, sha256: "h1" }],
    }));
    setApiInstance({ listPluginPackages, listPluginVersionFiles } as unknown as ApiClient);
    const list = await hubFileListOptions("ws-1", "plugin", "pkg-1").queryFn?.({} as never);
    expect(listPluginVersionFiles).toHaveBeenCalledWith("ws-1", "pkg-1", "v-new");
    expect(list).toEqual([{ path: "a.js", size_bytes: 2 }, { path: "z.js", size_bytes: 3 }]);
  });

  it("does not look up packages when a version is named", async () => {
    const listPluginPackages = vi.fn();
    const getPluginVersionFileContent = vi.fn(async () => ({
      path: "a.js", content: "x", is_binary: false, size_bytes: 1, sha256: "h1",
    }));
    setApiInstance({ listPluginPackages, getPluginVersionFileContent } as unknown as ApiClient);
    const content = await hubFileContentOptions("ws-1", "plugin", "pkg-1", "a.js", "v-old").queryFn?.({} as never);
    expect(listPluginPackages).not.toHaveBeenCalled();
    expect(getPluginVersionFileContent).toHaveBeenCalledWith("ws-1", "pkg-1", "v-old", "a.js");
    expect(content).toEqual({ path: "a.js", content: "x", is_binary: false, size_bytes: 1, etag: "h1" });
  });

  it("fails loudly when a plugin has no published version", async () => {
    setApiInstance({ listPluginPackages: vi.fn(async () => ({ packages: [{ ...makePackage(), versions: [] }] })) } as unknown as ApiClient);
    await expect(hubFileListOptions("ws-1", "plugin", "pkg-1").queryFn?.({} as never)).rejects.toThrow("no published version");
  });

  it("is disabled without a workspace or a path", () => {
    expect(hubFileListOptions("", "skill", "s").enabled).toBe(false);
    expect(hubFileContentOptions("ws-1", "skill", "s", "").enabled).toBe(false);
  });
});
