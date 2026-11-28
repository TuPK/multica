// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";
import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import { proposalKeys } from "../company/proposals";
import { issueKeys } from "../issues/queries";
import { useDecideInitiativeGate } from "./mutations";
import { initiativeKeys } from "./queries";

afterEach(() => {
  cleanup();
});

function setup(decideProposal: ReturnType<typeof vi.fn>) {
  setApiInstance({ decideProposal } as unknown as ApiClient);
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  const invalidate = vi.spyOn(queryClient, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);
  return { invalidate, wrapper };
}

describe("useDecideInitiativeGate", () => {
  it("strips openedAt from the payload, passes it to api.decideProposal, and invalidates related caches", async () => {
    const decideProposal = vi.fn().mockResolvedValue({ id: "g-1", state: "approved" });
    const { invalidate, wrapper } = setup(decideProposal);
    const { result } = renderHook(() => useDecideInitiativeGate("ws-1"), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({
        id: "g-1",
        decision: "approve",
        outcome: "scale",
        openedAt: 1_700_000_000_000,
      });
    });

    expect(decideProposal).toHaveBeenCalledWith(
      "g-1",
      { decision: "approve", outcome: "scale" },
      1_700_000_000_000,
    );
    expect("openedAt" in (decideProposal.mock.calls[0]?.[1] as Record<string, unknown>)).toBe(false);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: proposalKeys.all("ws-1") });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: issueKeys.all("ws-1") });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: initiativeKeys.all("ws-1") });
  });

  it("passes undefined openedAt when omitted", async () => {
    const decideProposal = vi.fn().mockResolvedValue({ id: "g-1", state: "rejected" });
    const { wrapper } = setup(decideProposal);
    const { result } = renderHook(() => useDecideInitiativeGate("ws-1"), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({
        id: "g-1",
        decision: "reject",
        note: "Needs more evidence",
      });
    });

    expect(decideProposal).toHaveBeenCalledWith(
      "g-1",
      { decision: "reject", note: "Needs more evidence" },
      undefined,
    );
  });
});
