// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useCloseReachGap } from "./mutations";
import { reachKeys } from "./queries";

const { request } = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("../../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api")>()),
  api: { request },
}));

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

describe("useCloseReachGap", () => {
  it("posts surface to /api/reach/gaps and invalidates reachKeys.all(wsId)", async () => {
    request.mockResolvedValueOnce({ issue_id: "issue-42" });
    const { wrapper, invalidate } = setup();
    const { result } = renderHook(() => useCloseReachGap("ws-1"), { wrapper });

    let reply: unknown;
    await act(async () => {
      reply = await result.current.mutateAsync({ surface: "app:gmail" });
    });

    expect(request).toHaveBeenCalledWith(
      "/api/reach/gaps",
      expect.objectContaining({ method: "POST" }),
    );
    expect(bodyOf(0)).toEqual({ surface: "app:gmail" });
    expect(reply).toEqual({ issue_id: "issue-42" });
    expect(invalidate).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: reachKeys.all("ws-1") }),
    );
  });

  it("still invalidates when the caller unmounts before the request settles", async () => {
    const reply = deferred<unknown>();
    request.mockReturnValueOnce(reply.promise);
    const { wrapper, invalidate } = setup();
    const { result, unmount } = renderHook(() => useCloseReachGap("ws-1"), { wrapper });

    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.mutateAsync({ surface: "external:stripe.com" });
    });
    unmount();
    await act(async () => {
      reply.resolve({ issue_id: "issue-99" });
      await pending;
    });

    expect(invalidate).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["durable", "ws-1", "reach"] }),
    );
  });

  it("returns null on a malformed response", async () => {
    request.mockResolvedValueOnce({ unexpected: true });
    const { wrapper } = setup();
    const { result } = renderHook(() => useCloseReachGap("ws-1"), { wrapper });

    let reply: unknown = "unset";
    await act(async () => {
      reply = await result.current.mutateAsync({ surface: "app:gmail" });
    });

    expect(reply).toBeNull();
  });
});
