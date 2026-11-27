// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import { useStartNecessityTest } from "./mutations";

const { request } = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("../../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api")>()),
  api: { request },
}));

const createdTest = {
  id: "nt-1",
  subject_type: "skill",
  subject_ref: "sk-1",
  agent_id: "a-1",
  shadow_run_id: "sr-1",
  state: "running",
  result: {},
  created_at: "2026-11-27T01:00:00Z",
};

function setup() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const invalidate = vi.spyOn(qc, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  return { qc, invalidate, wrapper };
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

describe("useStartNecessityTest", () => {
  it("posts skill_id to the necessity route and invalidates the seat list on settle", async () => {
    request.mockResolvedValueOnce(createdTest);
    const { wrapper, invalidate } = setup();
    const { result } = renderHook(() => useStartNecessityTest("ws-1", "a-1"), { wrapper });

    let started: unknown;
    await act(async () => {
      started = await result.current.mutateAsync({ skill_id: "sk-1" });
    });

    expect(request).toHaveBeenCalledWith(
      "/api/agents/a-1/necessity",
      expect.objectContaining({ method: "POST" }),
    );
    expect(bodyOf(0)).toEqual({ skill_id: "sk-1" });
    expect(started).toMatchObject({ id: "nt-1", state: "running" });
    expect(invalidate).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["durable", "ws-1", "necessity", "a-1", "list"] }),
    );
  });

  it("still invalidates when the caller unmounts before the request settles", async () => {
    const reply = deferred<unknown>();
    request.mockReturnValueOnce(reply.promise);
    const { wrapper, invalidate } = setup();
    const { result, unmount } = renderHook(() => useStartNecessityTest("ws-1", "a-1"), { wrapper });

    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.mutateAsync({ skill_id: "sk-1" });
    });
    unmount();
    await act(async () => {
      reply.resolve(createdTest);
      await pending;
    });

    expect(invalidate).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["durable", "ws-1", "necessity", "a-1", "list"] }),
    );
  });

  it("invalidates on error and surfaces the ApiError", async () => {
    request.mockRejectedValueOnce(
      new ApiError("already running", 409, "Conflict", { code: "necessity_test_running" }),
    );
    const { wrapper, invalidate } = setup();
    const { result } = renderHook(() => useStartNecessityTest("ws-1", "a-1"), { wrapper });

    await act(async () => {
      await expect(result.current.mutateAsync({ skill_id: "sk-1" })).rejects.toBeInstanceOf(
        ApiError,
      );
    });

    expect(invalidate).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["durable", "ws-1", "necessity", "a-1", "list"] }),
    );
  });

  it("returns null on a malformed create reply", async () => {
    request.mockResolvedValueOnce("oops");
    const { wrapper } = setup();
    const { result } = renderHook(() => useStartNecessityTest("ws-1", "a-1"), { wrapper });

    let started: unknown = "unset";
    await act(async () => {
      started = await result.current.mutateAsync({ skill_id: "sk-1" });
    });
    expect(started).toBeNull();
  });
});
