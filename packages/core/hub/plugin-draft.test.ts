// @vitest-environment node
import { describe, expect, it, vi } from "vitest";
import { buildPluginDraftFile, PLUGIN_MANIFEST_FILENAME } from "./plugin-draft";

const manifest = {
  manifest_version: 1,
  key: "acme.hello",
  name: "Hello",
  version: "1.0.0",
  author: { name: "Acme" },
  icon: "assets/icon.txt",
  scopes: [],
  contributes: {
    surfaces: [{ key: "panel", type: "issue_panel", name: "Panel", entry: "ui/panel.html" }],
    resources: [{ type: "skill", key: "guide", entry: "skills/guide.md" }],
  },
};

const baseFiles: Record<string, string> = {
  "ui/panel.html": "<h1>v1</h1>",
  "skills/guide.md": "guide v1",
  "assets/icon.txt": "icon",
  "unreferenced.txt": "not carried",
};

/** Reads a store-only zip written by packStoreZip: local headers in order. */
function readStoreZip(bytes: Uint8Array): Map<string, string> {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const decoder = new TextDecoder();
  const entries = new Map<string, string>();
  let offset = 0;
  while (view.getUint32(offset, true) === 0x04034b50) {
    const size = view.getUint32(offset + 18, true);
    const nameLength = view.getUint16(offset + 26, true);
    const extraLength = view.getUint16(offset + 28, true);
    const nameStart = offset + 30;
    const name = decoder.decode(bytes.subarray(nameStart, nameStart + nameLength));
    const dataStart = nameStart + nameLength + extraLength;
    entries.set(name, decoder.decode(bytes.subarray(dataStart, dataStart + size)));
    offset = dataStart + size;
  }
  return entries;
}

async function zipEntries(file: File) {
  return readStoreZip(new Uint8Array(await file.arrayBuffer()));
}

function loaderFrom(files: Record<string, string>) {
  return vi.fn(async (path: string) => {
    const text = files[path];
    if (text === undefined) throw new Error(`missing ${path}`);
    return text;
  });
}

const input = {
  pluginKey: "acme.hello",
  existingVersions: ["1.0.0"],
  newVersion: "1.1.0",
  changes: {},
};

describe("buildPluginDraftFile", () => {
  it("bumps the manifest version and carries exactly the referenced files", async () => {
    const load = loaderFrom({ ...baseFiles, [PLUGIN_MANIFEST_FILENAME]: JSON.stringify(manifest) });
    const file = await buildPluginDraftFile(input, load);

    expect(file.name).toBe("acme.hello-1.1.0.zip");
    const entries = await zipEntries(file);
    expect([...entries.keys()].sort()).toEqual([
      "assets/icon.txt",
      PLUGIN_MANIFEST_FILENAME,
      "skills/guide.md",
      "ui/panel.html",
    ]);
    const written = JSON.parse(entries.get(PLUGIN_MANIFEST_FILENAME) ?? "{}");
    expect(written.version).toBe("1.1.0");
    expect(written.key).toBe("acme.hello");
    expect(entries.get("ui/panel.html")).toBe("<h1>v1</h1>");
    expect(load).not.toHaveBeenCalledWith("unreferenced.txt");
  });

  it("applies changed files and skips loading them from the base version", async () => {
    const load = loaderFrom({ ...baseFiles, [PLUGIN_MANIFEST_FILENAME]: JSON.stringify(manifest) });
    const file = await buildPluginDraftFile(
      { ...input, changes: { "ui/panel.html": "<h1>v2</h1>", "notes.md": "new" } },
      load,
    );
    const entries = await zipEntries(file);
    expect(entries.get("ui/panel.html")).toBe("<h1>v2</h1>");
    expect(entries.get("notes.md")).toBe("new");
    expect(load).not.toHaveBeenCalledWith("ui/panel.html");
  });

  it("uses a changed manifest instead of the base one, still bumping its version", async () => {
    const load = loaderFrom(baseFiles);
    const edited = { ...manifest, name: "Hello 2", version: "9.9.9" };
    const file = await buildPluginDraftFile(
      { ...input, changes: { [PLUGIN_MANIFEST_FILENAME]: JSON.stringify(edited) } },
      load,
    );
    const written = JSON.parse((await zipEntries(file)).get(PLUGIN_MANIFEST_FILENAME) ?? "{}");
    expect(written.name).toBe("Hello 2");
    expect(written.version).toBe("1.1.0");
    expect(load).not.toHaveBeenCalledWith(PLUGIN_MANIFEST_FILENAME);
  });

  it("refuses to delete a file the manifest names", async () => {
    const load = loaderFrom({ ...baseFiles, [PLUGIN_MANIFEST_FILENAME]: JSON.stringify(manifest) });
    await expect(
      buildPluginDraftFile({ ...input, changes: { "ui/panel.html": null } }, load),
    ).rejects.toThrow("named by the manifest and cannot be deleted");
  });

  it("allows deleting an unreferenced file", async () => {
    const load = loaderFrom({ ...baseFiles, [PLUGIN_MANIFEST_FILENAME]: JSON.stringify(manifest) });
    const file = await buildPluginDraftFile({ ...input, changes: { "unreferenced.txt": null } }, load);
    expect(await zipEntries(file)).toBeDefined();
  });

  it("fails when a referenced file cannot be read", async () => {
    const load = loaderFrom({ [PLUGIN_MANIFEST_FILENAME]: JSON.stringify(manifest) });
    await expect(buildPluginDraftFile(input, load)).rejects.toThrow("missing ui/panel.html");
  });

  it("rejects a version that is not semver", async () => {
    const load = loaderFrom({ [PLUGIN_MANIFEST_FILENAME]: JSON.stringify(manifest) });
    await expect(buildPluginDraftFile({ ...input, newVersion: "1.1" }, load)).rejects.toThrow("semantic versioning");
    await expect(buildPluginDraftFile({ ...input, newVersion: "v1.1.0" }, load)).rejects.toThrow("semantic versioning");
  });

  it("rejects a version that is already published", async () => {
    const load = loaderFrom({ [PLUGIN_MANIFEST_FILENAME]: JSON.stringify(manifest) });
    await expect(buildPluginDraftFile({ ...input, newVersion: "1.0.0" }, load)).rejects.toThrow("already published");
  });

  it("rejects a manifest whose key differs from the package", async () => {
    const load = loaderFrom({ ...baseFiles, [PLUGIN_MANIFEST_FILENAME]: JSON.stringify({ ...manifest, key: "other.plugin" }) });
    await expect(buildPluginDraftFile(input, load)).rejects.toThrow('must stay "acme.hello"');
  });

  it("rejects a manifest that is not a JSON object", async () => {
    const load = loaderFrom({ [PLUGIN_MANIFEST_FILENAME]: "[1,2]" });
    await expect(buildPluginDraftFile(input, load)).rejects.toThrow("must be a JSON object");
    const broken = loaderFrom({ [PLUGIN_MANIFEST_FILENAME]: "{" });
    await expect(buildPluginDraftFile(input, broken)).rejects.toThrow("not valid JSON");
  });

  it("rejects an unsafe changed path before writing the archive", async () => {
    const load = loaderFrom({ ...baseFiles, [PLUGIN_MANIFEST_FILENAME]: JSON.stringify(manifest) });
    await expect(
      buildPluginDraftFile({ ...input, changes: { "../escape.js": "x" } }, load),
    ).rejects.toThrow("Invalid file path");
  });
});
