/**
 * @vitest-environment jsdom
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import { createQueryClient } from "../query-client";
import { pluginKeys } from "../plugins/queries";
import { workspaceKeys } from "../workspace/queries";
import { hubKeys } from "./files";
import { hubItemKeys } from "./item-keys";
import { useHubItemMeta } from "./item-meta";
import { useApplyHubUpdate, useForkHubItem } from "./update";
import { useSyncHubSource } from "./sync";
import { hubSourceKeys, useCreateHubSource } from "./sources";
import { HubLocalChangesError } from "./errors";

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

describe("hub item mutations", () => {
  it("sync refreshes sources, update checks and item state, and calls the source sync endpoint", async () => {
    qc = createQueryClient();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const syncHubSource = vi.fn().mockResolvedValue({ status: "ok", checked: 0, updates_available: 0, unchanged: 0, items: [] });
    fakeApi({ syncHubSource });
    const { result } = renderHook(() => useSyncHubSource(), { wrapper: withClient(qc) });

    await act(async () => {
      await result.current.mutateAsync({ wsId: "ws-1", id: "src-1" });
    });

    expect(syncHubSource).toHaveBeenCalledWith("ws-1", "src-1");
    expect(invalidatedKeys(spy)).toEqual(
      expect.arrayContaining([
        hubSourceKeys.all("ws-1"),
        hubItemKeys.updates("ws-1"),
        hubItemKeys.items("ws-1"),
        hubItemKeys.quota("ws-1"),
        hubItemKeys.activityAll("ws-1"),
      ]),
    );
  });

  it("refreshes the sync caches even when the sync call fails", async () => {
    qc = createQueryClient();
    const spy = vi.spyOn(qc, "invalidateQueries");
    fakeApi({ syncHubSource: vi.fn().mockRejectedValue(new Error("network")) });
    const { result } = renderHook(() => useSyncHubSource(), { wrapper: withClient(qc) });

    await act(async () => {
      await result.current.mutateAsync({ wsId: "ws-1", id: "src-1" }).catch(() => undefined);
    });

    expect(invalidatedKeys(spy)).toEqual(expect.arrayContaining([hubItemKeys.items("ws-1")]));
  });

  it("apply sends the chosen policy and refreshes files, skill and plugin lists, versions, updates and item state", async () => {
    qc = createQueryClient();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const applyHubUpdate = vi.fn().mockResolvedValue({ kind: "plugin", id: "p1", version: "2.0.0", tracking: "tracking" });
    fakeApi({ applyHubUpdate });
    const { result } = renderHook(() => useApplyHubUpdate(), { wrapper: withClient(qc) });

    await act(async () => {
      await result.current.mutateAsync({ wsId: "ws-1", kind: "plugin", refId: "p1", onLocalChanges: "overwrite" });
    });

    expect(applyHubUpdate).toHaveBeenCalledWith("ws-1", "plugin", "p1", "overwrite");
    expect(invalidatedKeys(spy)).toEqual(
      expect.arrayContaining([
        hubKeys.files("ws-1", "plugin", "p1"),
        workspaceKeys.skills("ws-1"),
        pluginKeys.packages("ws-1"),
        hubItemKeys.update("ws-1", "plugin", "p1"),
        hubItemKeys.items("ws-1"),
        hubItemKeys.activityAll("ws-1"),
      ]),
    );
  });

  it("a local-changes refusal rejects the mutation and still refreshes the item", async () => {
    qc = createQueryClient();
    const spy = vi.spyOn(qc, "invalidateQueries");
    fakeApi({ applyHubUpdate: vi.fn().mockRejectedValue(new HubLocalChangesError()) });
    const { result } = renderHook(() => useApplyHubUpdate(), { wrapper: withClient(qc) });

    await act(async () => {
      await expect(
        result.current.mutateAsync({ wsId: "ws-1", kind: "skill", refId: "sk-1", onLocalChanges: "fork" }),
      ).rejects.toBeInstanceOf(HubLocalChangesError);
    });

    expect(invalidatedKeys(spy)).toEqual(expect.arrayContaining([hubItemKeys.update("ws-1", "skill", "sk-1")]));
  });

  it("fork refreshes the item's update check, the item list and activity", async () => {
    qc = createQueryClient();
    const spy = vi.spyOn(qc, "invalidateQueries");
    const forkHubItem = vi.fn().mockResolvedValue({ kind: "skill", id: "sk-1", tracking: "forked" });
    fakeApi({ forkHubItem });
    const { result } = renderHook(() => useForkHubItem(), { wrapper: withClient(qc) });

    await act(async () => {
      await result.current.mutateAsync({ wsId: "ws-1", kind: "skill", refId: "sk-1" });
    });

    expect(forkHubItem).toHaveBeenCalledWith("ws-1", "skill", "sk-1");
    expect(invalidatedKeys(spy)).toEqual(
      expect.arrayContaining([hubItemKeys.update("ws-1", "skill", "sk-1"), hubItemKeys.items("ws-1"), hubItemKeys.activityAll("ws-1")]),
    );
  });

  it("creating a source refreshes the item list, quota and activity along with the catalog", async () => {
    qc = createQueryClient();
    const spy = vi.spyOn(qc, "invalidateQueries");
    fakeApi({ createHubSource: vi.fn().mockResolvedValue({ id: "src-1" }) });
    const { result } = renderHook(() => useCreateHubSource(), { wrapper: withClient(qc) });

    await act(async () => {
      await result.current.mutateAsync({ wsId: "ws-1", name: "Team", type: "github", url: "https://github.com/a/b" });
    });

    expect(invalidatedKeys(spy)).toEqual(
      expect.arrayContaining([hubItemKeys.items("ws-1"), hubItemKeys.quota("ws-1"), hubItemKeys.activityAll("ws-1")]),
    );
  });
});

describe("useHubItemMeta", () => {
  it("returns the matching row from the workspace list once it loads", async () => {
    qc = createQueryClient();
    fakeApi({
      listHubItems: vi.fn().mockResolvedValue([
        { id: "hi-1", kind: "skill", ref_id: "sk-1", tracking: "tracking", update_available: true },
        { id: "hi-2", kind: "plugin", ref_id: "p1", tracking: "manual" },
      ]),
    });
    const { result } = renderHook(() => useHubItemMeta("ws-1", "plugin", "p1"), { wrapper: withClient(qc) });

    expect(result.current.isLoading).toBe(true);
    await waitFor(() => expect(result.current.meta?.id).toBe("hi-2"));
    expect(result.current.meta).toMatchObject({ kind: "plugin", tracking: "manual" });
    expect(result.current.isLoading).toBe(false);
  });

  it("reports no meta and no loading when the workspace is not set", () => {
    qc = createQueryClient();
    const listHubItems = vi.fn();
    fakeApi({ listHubItems });
    const { result } = renderHook(() => useHubItemMeta("", "skill", "sk-1"), { wrapper: withClient(qc) });
    expect(result.current).toEqual({ meta: undefined, isLoading: false });
    expect(listHubItems).not.toHaveBeenCalled();
  });
});
