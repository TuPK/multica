// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useSaveEffectPolicyOverride, useUndoEffect } from "./mutations";

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

describe("useUndoEffect", () => {
  it("posts the undo to the effect's route and refreshes the effect log", async () => {
    request.mockResolvedValueOnce({ effect_id: "e-1", status: "undone", undo_event_id: "e-2" });
    const { wrapper, invalidate } = setup();
    const { result } = renderHook(() => useUndoEffect("ws-1"), { wrapper });

    let out: unknown;
    await act(async () => {
      out = await result.current.mutateAsync("e-1");
    });

    expect(request).toHaveBeenCalledWith("/api/effects/e-1/undo", expect.objectContaining({ method: "POST" }));
    expect(out).toEqual({ effect_id: "e-1", status: "undone", undo_event_id: "e-2" });
    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "effects"] }));
  });

  it("rejects the undo when the answer is not an undo", async () => {
    request.mockResolvedValueOnce({ status: 7 });
    const { wrapper } = setup();
    const { result } = renderHook(() => useUndoEffect("ws-1"), { wrapper });

    let out: unknown = "unset";
    await act(async () => {
      out = await result.current.mutateAsync("e-1");
    });
    expect(out).toBeNull();
  });
});

describe("useSaveEffectPolicyOverride", () => {
  it("saves one cell's decision and refreshes the grid", async () => {
    request.mockResolvedValueOnce({
      autonomy_level: 3,
      effect_class: "send",
      reversibility: "irreversible",
      default: "allow",
      override: "rehearse",
      options: [],
    });
    const { wrapper, invalidate } = setup();
    const { result } = renderHook(() => useSaveEffectPolicyOverride("ws-1"), { wrapper });

    await act(() =>
      result.current.mutateAsync({ autonomy_level: 3, effect_class: "send", reversibility: "irreversible", decision: "rehearse" }),
    );

    expect(request).toHaveBeenCalledWith("/api/effects/policy", expect.objectContaining({ method: "PUT" }));
    expect(bodyOf(0)).toEqual({ autonomy_level: 3, effect_class: "send", reversibility: "irreversible", decision: "rehearse" });
    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "effects", "policy"] }));
  });
});
