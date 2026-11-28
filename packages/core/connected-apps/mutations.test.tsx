// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";
import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import { useDecideAppRequest } from "./mutations";
import { connectedAppsKeys } from "./queries";

vi.mock("../hooks", () => ({ useWorkspaceId: () => "ws-1" }));

afterEach(() => {
  cleanup();
});

function setup() {
  const approveAppRequest = vi.fn().mockResolvedValue({ id: "req-1", status: "approved" });
  const rejectAppRequest = vi.fn().mockResolvedValue({ id: "req-1", status: "rejected" });
  setApiInstance({ approveAppRequest, rejectAppRequest } as unknown as ApiClient);
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  const invalidate = vi.spyOn(queryClient, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);
  return { approveAppRequest, rejectAppRequest, invalidate, wrapper };
}

describe("useDecideAppRequest", () => {
  it("forwards openedAt on approve and invalidates app requests", async () => {
    const { approveAppRequest, invalidate, wrapper } = setup();
    const { result } = renderHook(() => useDecideAppRequest(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({
        id: "req-1",
        decision: "approve",
        arguments: { to: "c@example.com" },
        openedAt: 1_700_000_000_000,
      });
    });

    expect(approveAppRequest).toHaveBeenCalledWith("req-1", { to: "c@example.com" }, 1_700_000_000_000);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: connectedAppsKeys.requests("ws-1") });
  });

  it("passes undefined openedAt on approve when omitted", async () => {
    const { approveAppRequest, wrapper } = setup();
    const { result } = renderHook(() => useDecideAppRequest(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({ id: "req-1", decision: "approve" });
    });

    expect(approveAppRequest).toHaveBeenCalledWith("req-1", undefined, undefined);
  });

  it("forwards openedAt on reject and invalidates app requests", async () => {
    const { rejectAppRequest, invalidate, wrapper } = setup();
    const { result } = renderHook(() => useDecideAppRequest(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({
        id: "req-1",
        decision: "reject",
        reason: "Not needed",
        openedAt: 1_700_000_000_000,
      });
    });

    expect(rejectAppRequest).toHaveBeenCalledWith("req-1", "Not needed", 1_700_000_000_000);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: connectedAppsKeys.requests("ws-1") });
  });

  it("passes undefined openedAt on reject when omitted", async () => {
    const { rejectAppRequest, wrapper } = setup();
    const { result } = renderHook(() => useDecideAppRequest(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({ id: "req-1", decision: "reject" });
    });

    expect(rejectAppRequest).toHaveBeenCalledWith("req-1", undefined, undefined);
  });
});
