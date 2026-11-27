// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useStartShadowRun } from "./mutations";

const { request } = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("../../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api")>()),
  api: { request },
}));

const runRow = {
  id: "sr-1",
  agent_id: "a-1",
  candidate_runtime_id: "rt-2",
  candidate_model: "claude-sonnet",
  sample_rate: 0.3,
  target_pairs: 20,
  auto_promote: false,
  state: "running",
};

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

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

afterEach(() => {
  cleanup();
  request.mockReset();
});

describe("useStartShadowRun", () => {
  it("posts the shadow run request and invalidates both shadow estimate and dossier queries", async () => {
    request.mockResolvedValueOnce(runRow);
    const { wrapper, invalidate } = setup();
    const { result } = renderHook(() => useStartShadowRun("ws-1", "a-1"), { wrapper });

    const payload = {
      candidate_runtime_id: "rt-2",
      candidate_model: "claude-sonnet",
      sample_rate: 0.3,
      target_pairs: 20,
      work_class_prefix: "flow:",
      auto_promote: false,
      confirm_cost: false,
    };

    let created: unknown;
    await act(async () => {
      created = await result.current.mutateAsync(payload);
    });

    expect(request).toHaveBeenCalledWith("/api/agents/a-1/shadow", expect.objectContaining({ method: "POST" }));
    expect(bodyOf(0)).toEqual(payload);
    expect(created).toMatchObject({ id: "sr-1", state: "running", auto_promote: false });
    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "shadow", "a-1"] }));
    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "dossier", "a-1"] }));
  });

  it("still invalidates when the caller unmounts before the request settles", async () => {
    const reply = deferred<unknown>();
    request.mockReturnValueOnce(reply.promise);
    const { wrapper, invalidate } = setup();
    const { result, unmount } = renderHook(() => useStartShadowRun("ws-1", "a-1"), { wrapper });

    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.mutateAsync({
        candidate_runtime_id: "rt-2",
        candidate_model: "",
        sample_rate: 0.3,
        target_pairs: 20,
        work_class_prefix: "",
        auto_promote: false,
        confirm_cost: true,
      });
    });
    unmount();
    await act(async () => {
      reply.resolve(runRow);
      await pending;
    });

    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "shadow", "a-1"] }));
    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "dossier", "a-1"] }));
  });
});
