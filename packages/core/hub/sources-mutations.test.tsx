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
import { pluginKeys } from "../plugins/queries";
import { workspaceKeys } from "../workspace/queries";
import { HubSourceInUseError } from "./errors";
import { hubSourceKeys } from "./sources";
import { useCreateHubSource, useDeleteHubSource, useUpdateHubSource } from "./sources";
import { useCommitHubImport, usePreviewHubImport } from "./import";

function fakeApi(overrides: Record<string, unknown>) {
  setApiInstance(overrides as unknown as ApiClient);
}

function withClient(qc: QueryClient) {
  function ClientWrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  }
  return ClientWrapper;
}

let qc: QueryClient;
afterEach(() => {
  qc?.clear();
  setApiInstance(null as unknown as ApiClient);
  vi.restoreAllMocks();
});

function invalidatedKeys(spy: { mock: { calls: unknown[][] } }) {
  return spy.mock.calls.map((call) => (call[0] as { queryKey: readonly unknown[] }).queryKey);
}

const INVALIDATED_BY_CATALOG_CHANGE = [
  hubSourceKeys.all("ws-1"),
  workspaceKeys.skills("ws-1"),
  pluginKeys.packages("ws-1"),
];

describe("hub source mutations", () => {
  it("creates a source through the API and then refreshes sources, skills and plugins", async () => {
    qc = createQueryClient();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const createHubSource = vi.fn().mockResolvedValue({ id: "src-1" });
    fakeApi({ createHubSource });
    const { result } = renderHook(() => useCreateHubSource(), { wrapper: withClient(qc) });

    await act(async () => {
      await result.current.mutateAsync({ wsId: "ws-1", name: "Team", type: "github", url: "https://github.com/a/b" });
    });

    expect(createHubSource).toHaveBeenCalledWith("ws-1", { name: "Team", type: "github", url: "https://github.com/a/b" });
    expect(invalidatedKeys(spy)).toEqual(expect.arrayContaining(INVALIDATED_BY_CATALOG_CHANGE));
  });

  it("sends the auto-sync interval chosen at creation with the new source", async () => {
    qc = createQueryClient();
    const createHubSource = vi.fn().mockResolvedValue({ id: "src-2" });
    fakeApi({ createHubSource });
    const { result } = renderHook(() => useCreateHubSource(), { wrapper: withClient(qc) });

    await act(async () => {
      await result.current.mutateAsync({
        wsId: "ws-1",
        name: "Team",
        type: "github",
        url: "https://github.com/a/b",
        auto_sync_interval_sec: 3600,
      });
    });

    expect(createHubSource).toHaveBeenCalledWith("ws-1", {
      name: "Team",
      type: "github",
      url: "https://github.com/a/b",
      auto_sync_interval_sec: 3600,
    });
  });

  it("does not invalidate anything when the server rejects the update", async () => {
    qc = createQueryClient();
    const spy = vi.spyOn(qc, "invalidateQueries");
    fakeApi({ updateHubSource: vi.fn().mockRejectedValue(new Error("nope")) });
    const { result } = renderHook(() => useUpdateHubSource(), { wrapper: withClient(qc) });

    await act(async () => {
      await expect(result.current.mutateAsync({ wsId: "ws-1", id: "src-1", enabled: false })).rejects.toThrow("nope");
    });

    expect(spy).not.toHaveBeenCalled();
  });

  it("sends the delete mode and keeps the in-use list on a blocked delete", async () => {
    qc = createQueryClient();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const inUse = new HubSourceInUseError([{ kind: "agent", id: "a1", name: "Writer" }]);
    const deleteHubSource = vi.fn().mockRejectedValueOnce(inUse).mockResolvedValueOnce(undefined);
    fakeApi({ deleteHubSource });
    const { result } = renderHook(() => useDeleteHubSource(), { wrapper: withClient(qc) });

    await act(async () => {
      await expect(result.current.mutateAsync({ wsId: "ws-1", id: "src-1", items: "delete" })).rejects.toBe(inUse);
    });
    expect(spy).not.toHaveBeenCalled();

    await act(async () => {
      await result.current.mutateAsync({ wsId: "ws-1", id: "src-1", items: "keep" });
    });
    expect(deleteHubSource).toHaveBeenLastCalledWith("ws-1", "src-1", "keep");
    expect(invalidatedKeys(spy)).toEqual(expect.arrayContaining(INVALIDATED_BY_CATALOG_CHANGE));
  });
});

describe("hub import mutations", () => {
  it("previews from a source with previewHubImport and from a file with previewHubImportUpload", async () => {
    qc = createQueryClient();
    const previewHubImport = vi.fn().mockResolvedValue({ preview_id: "pv-1" });
    const previewHubImportUpload = vi.fn().mockResolvedValue({ preview_id: "pv-2" });
    fakeApi({ previewHubImport, previewHubImportUpload });
    const { result } = renderHook(() => usePreviewHubImport(), { wrapper: withClient(qc) });

    await act(async () => {
      await result.current.mutateAsync({ wsId: "ws-1", source: { type: "clawhub", url: "slug" } });
    });
    expect(previewHubImport).toHaveBeenCalledWith("ws-1", { type: "clawhub", url: "slug" });

    const file = new File(["zip"], "pack.zip");
    await act(async () => {
      await result.current.mutateAsync({ wsId: "ws-1", file });
    });
    expect(previewHubImportUpload).toHaveBeenCalledWith("ws-1", file);
    expect(previewHubImport).toHaveBeenCalledTimes(1);
  });

  it("commits with snake_case request fields and refreshes the catalog on success", async () => {
    qc = createQueryClient();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const commitHubImport = vi.fn().mockResolvedValue({ kind: "skill", id: "sk-1", hub_item_id: "hi-1", skipped: false });
    fakeApi({ commitHubImport });
    const { result } = renderHook(() => useCommitHubImport(), { wrapper: withClient(qc) });

    await act(async () => {
      await result.current.mutateAsync({
        wsId: "ws-1",
        previewId: "pv-1",
        onConflict: "replace",
        confirmWarnings: true,
        sourceId: "src-1",
      });
    });

    expect(commitHubImport).toHaveBeenCalledWith("ws-1", {
      preview_id: "pv-1",
      on_conflict: "replace",
      name: undefined,
      confirm_warnings: true,
      source_id: "src-1",
    });
    expect(invalidatedKeys(spy)).toEqual(expect.arrayContaining(INVALIDATED_BY_CATALOG_CHANGE));
  });
});
