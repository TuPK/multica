/** @vitest-environment jsdom */
import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { WSClient } from "../api/ws-client";
import { flowDraftKeys } from "../flow-drafts/queries";
import { createQueryClient } from "../query-client";
import { useRealtimeSync, type RealtimeSyncStores } from "./use-realtime-sync";

vi.mock("../platform/workspace-storage", () => ({
  getCurrentWsId: () => "ws-1",
  getCurrentSlug: () => "test-ws",
  createWorkspaceAwareStorage: (adapter: unknown) => adapter,
  registerForWorkspaceRehydration: () => {},
}));
vi.mock("../paths", () => ({
  useHasOnboarded: () => true,
  resolvePostAuthDestination: () => "/",
}));

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

// The server emits flow_draft:updated with {squad_id, draft_id} on every draft write (spec 11).
// The client refreshes the workspace's draft lists and details, and only those.
it("invalidates the draft list and detail of the workspace on flow_draft:updated", async () => {
  const qc = createQueryClient();
  const list = flowDraftKeys.list("ws-1", "squad-1");
  const detail = flowDraftKeys.detail("ws-1", "squad-1", "draft-1");
  const capabilities = flowDraftKeys.capabilities("ws-1");
  const otherWorkspace = flowDraftKeys.list("ws-2", "squad-1");
  for (const key of [list, detail, capabilities, otherWorkspace]) qc.setQueryData(key, { seeded: true });

  let any: ((msg: { type: string; payload: unknown }) => void) | undefined;
  const ws = {
    on: () => () => {},
    onAny: (handler: (msg: { type: string; payload: unknown }) => void) => {
      any = handler;
      return () => {};
    },
    onReconnect: () => () => {},
  } as unknown as WSClient;
  const stores = { authStore: { getState: () => ({ user: { id: "u1" } }) } } as unknown as RealtimeSyncStores;
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  }
  vi.spyOn(document, "hasFocus").mockReturnValue(true);
  renderHook(() => useRealtimeSync(ws, stores), { wrapper: Wrapper });

  act(() => {
    any!({ type: "flow_draft:updated", payload: { squad_id: "squad-1", draft_id: "draft-1" } });
    vi.advanceTimersByTime(150);
  });

  expect(qc.getQueryState(list)?.isInvalidated).toBe(true);
  expect(qc.getQueryState(detail)?.isInvalidated).toBe(true);
  expect(qc.getQueryState(capabilities)?.isInvalidated).toBe(false);
  expect(qc.getQueryState(otherWorkspace)?.isInvalidated).toBe(false);
});
