/**
 * @vitest-environment jsdom
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import { createQueryClient } from "../query-client";
import type { PluginPackage, Skill, SkillFile } from "../types";
import { HubFileConflictError } from "./files";
import { useDeleteHubFile, useRenameHubFile, useSaveHubFile } from "./files-mutations";
import { useSavePluginDraft } from "./plugin-draft";

const UPDATED = "2026-01-05T00:00:00Z";
const README_UPDATED = "2026-01-02T00:00:00Z";

function file(path: string, content: string, id = `f-${path}`): SkillFile {
  return { id, skill_id: "skill-1", path, content, created_at: "", updated_at: README_UPDATED };
}

function makeSkill(files: SkillFile[] = [file("README.md", "readme")]): Skill {
  return {
    id: "skill-1",
    workspace_id: "ws-1",
    name: "Writer",
    description: "",
    config: {},
    created_by: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: UPDATED,
    content: "# Writer",
    files,
  };
}

function withClient(qc: QueryClient) {
  function ClientWrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  }
  return ClientWrapper;
}

/** Runs a mutation and returns its error, if any, without failing the test. */
async function runMutation<TVars, TResult>(
  mutateAsync: (vars: TVars) => Promise<TResult>,
  vars: TVars,
): Promise<{ data?: TResult; error?: unknown }> {
  let outcome: { data?: TResult; error?: unknown } = {};
  await act(async () => {
    try {
      outcome = { data: await mutateAsync(vars) };
    } catch (error) {
      outcome = { error };
    }
  });
  return outcome;
}

function expectError(error: unknown, message: string) {
  expect(error).toBeInstanceOf(Error);
  expect((error as Error).message).toContain(message);
}

function fakeApi(overrides: Record<string, unknown>) {
  setApiInstance(overrides as unknown as ApiClient);
}

let qc: QueryClient;
afterEach(() => {
  qc?.clear();
  setApiInstance(null as unknown as ApiClient);
  vi.restoreAllMocks();
});

describe("useSaveHubFile", () => {
  it("refuses to overwrite a file that changed since it was loaded", async () => {
    qc = createQueryClient();
    const upsertSkillFile = vi.fn();
    fakeApi({ getSkill: vi.fn(async () => makeSkill([file("README.md", "someone else's edit", "f1")])), upsertSkillFile });
    const { result } = renderHook(() => useSaveHubFile(), { wrapper: withClient(qc) });

    const outcome = await runMutation(result.current.mutateAsync, {
      wsId: "ws-1", refId: "skill-1", path: "README.md", content: "mine", etag: "2025-12-01T00:00:00Z",
    });
    expect(outcome.error).toBeInstanceOf(HubFileConflictError);
    expect(upsertSkillFile).not.toHaveBeenCalled();
  });

  it("writes a non-SKILL.md file with upsertSkillFile and returns its new etag", async () => {
    qc = createQueryClient();
    const upsertSkillFile = vi.fn(async () => file("README.md", "mine", "f1"));
    fakeApi({ getSkill: vi.fn(async () => makeSkill()), upsertSkillFile, updateSkill: vi.fn() });
    const { result } = renderHook(() => useSaveHubFile(), { wrapper: withClient(qc) });

    const outcome = await runMutation(result.current.mutateAsync, {
      wsId: "ws-1", refId: "skill-1", path: "README.md", content: "mine", etag: README_UPDATED,
    });
    expect(upsertSkillFile).toHaveBeenCalledWith("skill-1", { path: "README.md", content: "mine" });
    expect(outcome.data).toEqual({ path: "README.md", content: "mine", is_binary: false, size_bytes: 4, etag: README_UPDATED });
  });

  it("saves SKILL.md through updateSkill, checked against the skill updated_at", async () => {
    qc = createQueryClient();
    const updateSkill = vi.fn(async () => ({ ...makeSkill(), updated_at: "2026-02-01T00:00:00Z" }));
    const upsertSkillFile = vi.fn();
    fakeApi({ getSkill: vi.fn(async () => makeSkill()), updateSkill, upsertSkillFile });
    const { result } = renderHook(() => useSaveHubFile(), { wrapper: withClient(qc) });

    const outcome = await runMutation(result.current.mutateAsync, {
      wsId: "ws-1", refId: "skill-1", path: "SKILL.md", content: "# New", etag: UPDATED,
    });
    expect(updateSkill).toHaveBeenCalledWith("skill-1", { content: "# New" });
    expect(upsertSkillFile).not.toHaveBeenCalled();
    expect(outcome.data?.etag).toBe("2026-02-01T00:00:00Z");
  });

  it("refuses a body over 1 MiB before any request", async () => {
    qc = createQueryClient();
    const getSkill = vi.fn();
    fakeApi({ getSkill });
    const { result } = renderHook(() => useSaveHubFile(), { wrapper: withClient(qc) });
    const outcome = await runMutation(result.current.mutateAsync, {
      wsId: "ws-1", refId: "skill-1", path: "big.txt", content: "x".repeat((1 << 20) + 1), etag: "",
    });
    expectError(outcome.error, "larger than 1 MiB");
    expect(getSkill).not.toHaveBeenCalled();
  });
});

describe("useDeleteHubFile", () => {
  it("refuses to delete SKILL.md", async () => {
    qc = createQueryClient();
    const deleteSkillFile = vi.fn();
    fakeApi({ getSkill: vi.fn(async () => makeSkill()), deleteSkillFile });
    const { result } = renderHook(() => useDeleteHubFile(), { wrapper: withClient(qc) });
    const outcome = await runMutation(result.current.mutateAsync, { wsId: "ws-1", refId: "skill-1", path: "SKILL.md" });
    expectError(outcome.error, "SKILL.md cannot be deleted");
    expect(deleteSkillFile).not.toHaveBeenCalled();
  });

  it("deletes by the file id found for the path", async () => {
    qc = createQueryClient();
    const deleteSkillFile = vi.fn(async () => undefined);
    fakeApi({ getSkill: vi.fn(async () => makeSkill([file("a.md", "a", "id-a"), file("b.md", "b", "id-b")])), deleteSkillFile });
    const { result } = renderHook(() => useDeleteHubFile(), { wrapper: withClient(qc) });
    await runMutation(result.current.mutateAsync, { wsId: "ws-1", refId: "skill-1", path: "b.md" });
    expect(deleteSkillFile).toHaveBeenCalledWith("skill-1", "id-b");
  });

  it("treats a path that is already gone as deleted", async () => {
    qc = createQueryClient();
    const deleteSkillFile = vi.fn();
    fakeApi({ getSkill: vi.fn(async () => makeSkill([])), deleteSkillFile });
    const { result } = renderHook(() => useDeleteHubFile(), { wrapper: withClient(qc) });
    const outcome = await runMutation(result.current.mutateAsync, { wsId: "ws-1", refId: "skill-1", path: "gone.md" });
    expect(outcome.error).toBeUndefined();
    expect(deleteSkillFile).not.toHaveBeenCalled();
  });
});

describe("useRenameHubFile", () => {
  it("writes the new path, then deletes the old one", async () => {
    qc = createQueryClient();
    const calls: string[] = [];
    fakeApi({
      getSkill: vi.fn(async () => makeSkill([file("old.md", "body", "id-old")])),
      upsertSkillFile: vi.fn(async (_id: string, body: { path: string }) => {
        calls.push(`upsert:${body.path}`);
        return file(body.path, "body", "id-new");
      }),
      deleteSkillFile: vi.fn(async (_skillId: string, fileId: string) => {
        calls.push(`delete:${fileId}`);
      }),
    });
    const { result } = renderHook(() => useRenameHubFile(), { wrapper: withClient(qc) });
    const outcome = await runMutation(result.current.mutateAsync, { wsId: "ws-1", refId: "skill-1", from: "old.md", to: "new.md" });
    expect(calls).toEqual(["upsert:new.md", "delete:id-old"]);
    expect(outcome.data).toMatchObject({ path: "new.md", content: "body" });
  });

  it("refuses to overwrite an existing target", async () => {
    qc = createQueryClient();
    const upsertSkillFile = vi.fn();
    fakeApi({
      getSkill: vi.fn(async () => makeSkill([file("old.md", "a"), file("new.md", "b")])),
      upsertSkillFile,
    });
    const { result } = renderHook(() => useRenameHubFile(), { wrapper: withClient(qc) });
    const outcome = await runMutation(result.current.mutateAsync, { wsId: "ws-1", refId: "skill-1", from: "old.md", to: "new.md" });
    expectError(outcome.error, "already exists at new.md");
    expect(upsertSkillFile).not.toHaveBeenCalled();
  });

  it("says the file exists at both paths when the old one cannot be removed", async () => {
    qc = createQueryClient();
    fakeApi({
      getSkill: vi.fn(async () => makeSkill([file("old.md", "a", "id-old")])),
      upsertSkillFile: vi.fn(async () => file("new.md", "a", "id-new")),
      deleteSkillFile: vi.fn(async () => { throw new Error("network down"); }),
    });
    const { result } = renderHook(() => useRenameHubFile(), { wrapper: withClient(qc) });
    const outcome = await runMutation(result.current.mutateAsync, { wsId: "ws-1", refId: "skill-1", from: "old.md", to: "new.md" });
    expectError(outcome.error, "Renamed to new.md, but old.md could not be removed");
  });

  it("refuses to rename SKILL.md", async () => {
    qc = createQueryClient();
    fakeApi({ getSkill: vi.fn(async () => makeSkill()) });
    const { result } = renderHook(() => useRenameHubFile(), { wrapper: withClient(qc) });
    const outcome = await runMutation(result.current.mutateAsync, { wsId: "ws-1", refId: "skill-1", from: "SKILL.md", to: "x.md" });
    expectError(outcome.error, "SKILL.md cannot be renamed");
  });
});

describe("useSavePluginDraft", () => {
  const manifest = {
    manifest_version: 1, key: "acme.hello", name: "Hello", version: "1.0.0",
    author: { name: "Acme" }, scopes: [], contributes: { surfaces: [{ key: "p", type: "modal", name: "P", entry: "ui.html" }] },
  };
  const pkg: PluginPackage = {
    id: "pkg-1", plugin_key: "acme.hello", name: "Hello", created_at: "",
    versions: [{ id: "v1", version: "1.0.0", digest: "", size_bytes: 0, published_at: "", installed: true }],
  };

  it("publishes a new version built from the base version", async () => {
    qc = createQueryClient();
    const publishPluginPackage = vi.fn(async (_wsId: string) => ({ ...pkg, versions: [{ ...pkg.versions[0]!, version: "1.0.1" }, ...pkg.versions] }));
    fakeApi({
      listPluginPackages: vi.fn(async () => ({ packages: [pkg] })),
      listPluginVersionFiles: vi.fn(async () => ({ files: [{ path: "ui.html", size_bytes: 3, sha256: "h" }] })),
      getPluginVersionFileContent: vi.fn(async (_ws: string, _pkg: string, _v: string, path: string) => ({
        path,
        content: path === "multica.plugin.json" ? JSON.stringify(manifest) : "<p>old</p>",
        is_binary: false,
        size_bytes: 1,
        sha256: "h",
      })),
      publishPluginPackage,
    });
    const { result } = renderHook(() => useSavePluginDraft(), { wrapper: withClient(qc) });
    const outcome = await runMutation(result.current.mutateAsync, {
      wsId: "ws-1", refId: "pkg-1", baseVersionId: "v1", changes: { "ui.html": "<p>new</p>" }, newVersion: "1.0.1",
    });
    expect(outcome.error).toBeUndefined();
    const [wsId, bundle] = publishPluginPackage.mock.calls[0] as unknown as [string, File];
    expect(wsId).toBe("ws-1");
    expect(bundle.name).toBe("acme.hello-1.0.1.zip");
  });

  it("refuses a base version that is not one of this plugin's versions", async () => {
    qc = createQueryClient();
    const publishPluginPackage = vi.fn();
    fakeApi({ listPluginPackages: vi.fn(async () => ({ packages: [pkg] })), publishPluginPackage });
    const { result } = renderHook(() => useSavePluginDraft(), { wrapper: withClient(qc) });
    const outcome = await runMutation(result.current.mutateAsync, {
      wsId: "ws-1", refId: "pkg-1", baseVersionId: "other", changes: {}, newVersion: "1.0.1",
    });
    expectError(outcome.error, "does not belong to this plugin");
    expect(publishPluginPackage).not.toHaveBeenCalled();
  });

  it("refuses to publish when a referenced base file is binary", async () => {
    qc = createQueryClient();
    const publishPluginPackage = vi.fn();
    fakeApi({
      listPluginPackages: vi.fn(async () => ({ packages: [pkg] })),
      listPluginVersionFiles: vi.fn(async () => ({ files: [{ path: "ui.html", size_bytes: 3, sha256: "h" }] })),
      getPluginVersionFileContent: vi.fn(async (_ws: string, _pkg: string, _v: string, path: string) => ({
        path,
        content: path === "multica.plugin.json" ? JSON.stringify(manifest) : null,
        is_binary: path !== "multica.plugin.json",
        size_bytes: 3,
        sha256: "h",
      })),
      publishPluginPackage,
    });
    const { result } = renderHook(() => useSavePluginDraft(), { wrapper: withClient(qc) });
    const outcome = await runMutation(result.current.mutateAsync, {
      wsId: "ws-1", refId: "pkg-1", baseVersionId: "v1", changes: {}, newVersion: "1.0.1",
    });
    expectError(outcome.error, "ui.html is binary or too large");
    expect(publishPluginPackage).not.toHaveBeenCalled();
  });
});
