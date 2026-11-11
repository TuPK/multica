// @vitest-environment jsdom

import { describe, expect, it } from "vitest";
import type { HubSource } from "@multica/core/hub";
import {
  importNeedsWebAddress,
  parseWebAddress,
  resolveRegistryItemUrl,
  suggestSourceName,
} from "./hub-import-request";
import { filesFromDrop, filesFromEntries, prepareImportFiles } from "./hub-upload-drop";
import { isKnownSourceType, safeSourceLink, syncStatusKind } from "./hub-source-meta";

function fileEntry(name: string, body = "x"): FileSystemFileEntry {
  return {
    isFile: true,
    isDirectory: false,
    name,
    file: (ok: (file: File) => void) => ok(new File([body], name)),
  } as unknown as FileSystemFileEntry;
}

function dirEntry(name: string, children: FileSystemEntry[]): FileSystemDirectoryEntry {
  return {
    isFile: false,
    isDirectory: true,
    name,
    createReader: () => {
      let served = false;
      return {
        readEntries: (ok: (entries: FileSystemEntry[]) => void) => {
          // Browsers return a directory in batches and end with an empty batch.
          if (served) return ok([]);
          served = true;
          ok(children);
        },
      };
    },
  } as unknown as FileSystemDirectoryEntry;
}

describe("parseWebAddress", () => {
  it("accepts http and https addresses only", () => {
    expect(parseWebAddress(" https://github.com/acme/skills ")).toBe("https://github.com/acme/skills");
    expect(parseWebAddress("http://registry.internal/index.json")).toBe("http://registry.internal/index.json");
    expect(parseWebAddress("javascript:alert(1)")).toBeNull();
    expect(parseWebAddress("ftp://example.com/file.zip")).toBeNull();
    expect(parseWebAddress("not a url")).toBeNull();
  });

  it("lets ClawHub take a slug instead of an address", () => {
    expect(importNeedsWebAddress("clawhub")).toBe(false);
    expect(importNeedsWebAddress("github")).toBe(true);
  });
});

describe("resolveRegistryItemUrl", () => {
  it("resolves relative item links against the registry file", () => {
    expect(resolveRegistryItemUrl("https://example.com/hub/hub-index.json", "items/pulse.zip")).toBe(
      "https://example.com/hub/items/pulse.zip",
    );
  });

  it("keeps absolute item links", () => {
    expect(resolveRegistryItemUrl("https://example.com/hub-index.json", "https://cdn.example.com/a.zip")).toBe(
      "https://cdn.example.com/a.zip",
    );
  });
});

describe("source helpers", () => {
  it("never turns a non-web URL into a link", () => {
    expect(safeSourceLink("https://github.com/acme/skills")).toBe("https://github.com/acme/skills");
    expect(safeSourceLink("javascript:alert(1)")).toBeNull();
    expect(safeSourceLink("")).toBeNull();
  });

  it("maps sync status with a fallback for unknown values", () => {
    const base = { last_sync_at: null, last_sync_status: "" } as Pick<HubSource, "last_sync_at" | "last_sync_status">;
    expect(syncStatusKind({ ...base, last_sync_status: "ok" })).toBe("ok");
    expect(syncStatusKind({ ...base, last_sync_status: "error" })).toBe("error");
    expect(syncStatusKind({ ...base, last_sync_status: "syncing" })).toBe("syncing");
    expect(syncStatusKind(base)).toBe("never");
    expect(syncStatusKind({ last_sync_at: "2026-08-01T00:00:00Z", last_sync_status: "paused" })).toBe("unknown");
  });

  it("recognises source types from a newer backend only when they are known", () => {
    expect(isKnownSourceType("url_zip")).toBe(true);
    expect(isKnownSourceType("s3_bucket")).toBe(false);
  });
});

describe("dropped folders", () => {
  it("gives each file its path inside the dropped folder", async () => {
    const files = await filesFromEntries([
      dirEntry("deploy", [fileEntry("SKILL.md", "# deploy"), dirEntry("scripts", [fileEntry("run.sh")])]),
    ]);
    expect(files.map((file) => file.webkitRelativePath)).toEqual([
      "deploy/SKILL.md",
      "deploy/scripts/run.sh",
    ]);
  });

  it("walks folders from the drop items and falls back to plain files", async () => {
    const folder = await filesFromDrop({
      items: [{ kind: "file", webkitGetAsEntry: () => dirEntry("pkg", [fileEntry("SKILL.md")]) }],
      files: [],
    } as unknown as Pick<DataTransfer, "items" | "files">);
    expect(folder.map((file) => file.webkitRelativePath)).toEqual(["pkg/SKILL.md"]);

    const plain = new File(["zip"], "deploy.zip");
    const dropped = await filesFromDrop({
      items: [{ kind: "file", webkitGetAsEntry: () => null }],
      files: [plain],
    } as unknown as Pick<DataTransfer, "items" | "files">);
    expect(dropped).toEqual([plain]);
  });

  it("uses a single archive as-is and packs a folder selection", async () => {
    const archive = new File(["zip"], "deploy.zip");
    const prepared = await prepareImportFiles([archive]);
    expect(prepared).toMatchObject({ ok: true, file: archive, preview: { source: "archive", skillName: "deploy" } });

    const folderFile = new File(["# deploy"], "SKILL.md");
    Object.defineProperty(folderFile, "webkitRelativePath", { value: "deploy/SKILL.md" });
    const folder = await prepareImportFiles([folderFile]);
    expect(folder).toMatchObject({ ok: true, preview: { source: "folder" } });
  });
});

describe("suggestSourceName", () => {
  it("names a GitHub source owner/repo, without a .git suffix", () => {
    expect(suggestSourceName("github", " https://github.com/acme/skills.git ")).toBe("acme/skills");
    expect(suggestSourceName("github", "https://github.com/acme/skills/tree/main/x")).toBe("acme/skills");
  });

  it("names a zip source by its host and last path segment", () => {
    expect(suggestSourceName("url_zip", "https://example.com/dist/skill.zip")).toBe("example.com/skill.zip");
    expect(suggestSourceName("url_zip", "https://example.com/")).toBe("example.com");
  });

  it("falls back to the trimmed text when it is not a URL", () => {
    expect(suggestSourceName("github", "  not a url  ")).toBe("not a url");
  });
});
