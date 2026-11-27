// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  useAddPrincipalTrait,
  useAnswerPrincipalInterview,
  useDeletePrincipalProfile,
  useDisablePrincipalDelegation,
  useEditPrincipalTrait,
  useEnablePrincipalDelegation,
  useEnablePrincipalProfile,
  useRetirePrincipalTrait,
} from "./mutations";

const { request } = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("../../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api")>()),
  api: { request },
}));

function setup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const invalidate = vi.spyOn(qc, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  return { invalidate, wrapper };
}

function bodyOf(n: number): unknown {
  const init = request.mock.calls[n]?.[1] as { body?: string } | undefined;
  return JSON.parse(init?.body ?? "null");
}

afterEach(() => {
  cleanup();
  request.mockReset();
});

describe("principal mutations", () => {
  it("enables or updates the profile via PUT /api/principal/profile and refreshes the profile", async () => {
    request.mockResolvedValueOnce({
      member_id: "u-1",
      status: "active",
      consent_at: "2026-11-26T08:00:00Z",
      share_with_owners: true,
      version: 1,
      traits: [],
    });
    const { wrapper, invalidate } = setup();
    const { result } = renderHook(() => useEnablePrincipalProfile("ws-1"), { wrapper });

    await act(() =>
      result.current.mutateAsync({ consent_text: "I consent", share_with_owners: true }),
    );

    expect(request).toHaveBeenCalledWith("/api/principal/profile", expect.objectContaining({ method: "PUT" }));
    expect(bodyOf(0)).toEqual({ consent_text: "I consent", share_with_owners: true });
    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "principal", "profile"] }));
  });

  it("deletes the profile via DELETE /api/principal/profile and refreshes profile and delegations", async () => {
    request.mockResolvedValueOnce(undefined);
    const { wrapper, invalidate } = setup();
    const { result } = renderHook(() => useDeletePrincipalProfile("ws-1"), { wrapper });

    await act(() => result.current.mutateAsync());

    expect(request).toHaveBeenCalledWith("/api/principal/profile", expect.objectContaining({ method: "DELETE" }));
    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "principal", "profile"] }));
    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "principal", "delegations"] }));
  });

  it("adds, edits and retires traits and refreshes the profile", async () => {
    const trait = {
      id: "tr-1",
      decision_kind: "general",
      statement: "Updated rule",
      origin: "edited",
      evidence: [{ type: "decision", ref: "d-1" }],
      created_at: "2026-11-26T08:00:00Z",
    };
    request.mockResolvedValueOnce(trait).mockResolvedValueOnce(trait).mockResolvedValueOnce(undefined);
    const { wrapper, invalidate } = setup();

    const add = renderHook(() => useAddPrincipalTrait("ws-1"), { wrapper });
    await act(() =>
      add.result.current.mutateAsync({
        decision_kind: "general",
        statement: "Rule",
        origin: "edited",
        evidence: [{ type: "decision", ref: "d-1" }],
      }),
    );
    expect(request).toHaveBeenNthCalledWith(1, "/api/principal/profile/traits", expect.objectContaining({ method: "POST" }));

    const edit = renderHook(() => useEditPrincipalTrait("ws-1"), { wrapper });
    await act(() => edit.result.current.mutateAsync({ id: "tr-1", statement: "Updated rule" }));
    expect(request).toHaveBeenNthCalledWith(2, "/api/principal/profile/traits/tr-1", expect.objectContaining({ method: "PATCH" }));
    expect(bodyOf(1)).toEqual({ statement: "Updated rule" });

    const retire = renderHook(() => useRetirePrincipalTrait("ws-1"), { wrapper });
    await act(() => retire.result.current.mutateAsync("tr-1"));
    expect(request).toHaveBeenNthCalledWith(3, "/api/principal/profile/traits/tr-1", expect.objectContaining({ method: "DELETE" }));
    expect(invalidate).toHaveBeenCalledTimes(3);
  });

  it("records an interview answer with { question, answer } and refreshes the profile", async () => {
    request.mockResolvedValueOnce({
      id: "tr-2",
      decision_kind: "general",
      statement: "Prefer quality",
      origin: "interview",
      evidence: [{ type: "interview", ref: "msg-1" }],
      created_at: "2026-11-26T08:00:00Z",
    });
    const { wrapper, invalidate } = setup();
    const { result } = renderHook(() => useAnswerPrincipalInterview("ws-1"), { wrapper });

    await act(() =>
      result.current.mutateAsync({ question: 0, answer: "Prefer quality" }),
    );

    expect(request).toHaveBeenCalledWith(
      "/api/principal/interview/answers",
      expect.objectContaining({ method: "POST" }),
    );
    expect(bodyOf(0)).toEqual({ question: 0, answer: "Prefer quality" });
    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "principal", "profile"] }));
  });

  it("enables and disables delegation and invalidates the delegations query", async () => {
    request
      .mockResolvedValueOnce({ decision_kind: "proposal:general", enabled: true })
      .mockResolvedValueOnce(undefined);
    const { wrapper, invalidate } = setup();

    const enable = renderHook(() => useEnablePrincipalDelegation("ws-1"), { wrapper });
    await act(() => enable.result.current.mutateAsync({ decision_kind: "proposal:general" }));
    expect(request).toHaveBeenNthCalledWith(1, "/api/principal/delegations", expect.objectContaining({ method: "PUT" }));
    expect(bodyOf(0)).toEqual({ decision_kind: "proposal:general", min_confidence: 0.9 });
    expect(invalidate).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["durable", "ws-1", "principal", "delegations"] }),
    );

    const disable = renderHook(() => useDisablePrincipalDelegation("ws-1"), { wrapper });
    await act(() => disable.result.current.mutateAsync("proposal:general"));
    expect(request).toHaveBeenNthCalledWith(
      2,
      "/api/principal/delegations?decision_kind=proposal%3Ageneral",
      expect.objectContaining({ method: "DELETE" }),
    );
    expect(invalidate).toHaveBeenCalledTimes(2);
  });
});
