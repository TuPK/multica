// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useAddCriterion, useArchiveCriterion, useVerifyOutcome } from "./mutations";

const { request } = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("../../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api")>()),
  api: { request },
}));

const criterionRow = {
  id: "c-1",
  issue_id: "i-1",
  position: 0,
  text: "Tests pass",
  check_kind: "command",
  check_spec: { run: "pnpm test", expect_exit: 0 },
  source: "member",
  created_by_type: "member",
  created_by_id: "u-1",
  archived_at: null,
  created_at: "2026-11-18T09:00:00Z",
};

function setup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const invalidate = vi.spyOn(qc, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  return { qc, invalidate, wrapper };
}

/** The JSON body of the nth request. */
function bodyOf(n: number): unknown {
  const init = request.mock.calls[n]?.[1] as { body?: string } | undefined;
  return JSON.parse(init?.body ?? "null");
}

function headersOf(n: number): Record<string, string> | undefined {
  const init = request.mock.calls[n]?.[1] as { headers?: Record<string, string> } | undefined;
  return init?.headers;
}

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

afterEach(() => {
  cleanup();
  request.mockReset();
  vi.useRealTimers();
});

describe("useAddCriterion", () => {
  it("posts the criterion to the issue's route", async () => {
    request.mockResolvedValueOnce(criterionRow);
    const { wrapper } = setup();
    const { result } = renderHook(() => useAddCriterion("ws-1", "i-1"), { wrapper });

    let created: unknown;
    await act(async () => {
      created = await result.current.mutateAsync({ text: "Tests pass", check_kind: "command", check_spec: { run: "pnpm test", expect_exit: 0 } });
    });

    expect(request).toHaveBeenCalledWith("/api/issues/i-1/criteria", expect.objectContaining({ method: "POST" }));
    const body = bodyOf(0);
    expect(body).toEqual({ text: "Tests pass", check_kind: "command", check_spec: { run: "pnpm test", expect_exit: 0 } });
    expect(created).toMatchObject({ id: "c-1", check_kind: "command" });
  });

  it("refreshes the workspace's criteria and outcomes once it settles", async () => {
    request.mockResolvedValueOnce(criterionRow);
    const { wrapper, invalidate } = setup();
    const { result } = renderHook(() => useAddCriterion("ws-1", "i-1"), { wrapper });

    await act(() => result.current.mutateAsync({ text: "Tests pass", check_kind: "manual" }));

    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "criteria", "i-1"] }));
    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "outcomes"] }));
  });

  it("still refreshes when the caller unmounts before the request settles", async () => {
    const reply = deferred<unknown>();
    request.mockReturnValueOnce(reply.promise);
    const { wrapper, invalidate } = setup();
    const { result, unmount } = renderHook(() => useAddCriterion("ws-1", "i-1"), { wrapper });

    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.mutateAsync({ text: "Tests pass", check_kind: "manual" });
    });
    unmount();
    await act(async () => {
      reply.resolve(criterionRow);
      await pending;
    });

    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "criteria", "i-1"] }));
  });

  it("parses a malformed reply instead of casting it", async () => {
    request.mockResolvedValueOnce("oops");
    const { wrapper } = setup();
    const { result } = renderHook(() => useAddCriterion("ws-1", "i-1"), { wrapper });

    let created: unknown = "unset";
    await act(async () => {
      created = await result.current.mutateAsync({ text: "x", check_kind: "manual" });
    });
    expect(created).toBeNull();
  });
});

describe("useArchiveCriterion", () => {
  it("archives through the criterion's PATCH route", async () => {
    request.mockResolvedValueOnce({ ...criterionRow, archived_at: "2026-11-19T09:00:00Z" });
    const { wrapper } = setup();
    const { result } = renderHook(() => useArchiveCriterion("ws-1", "i-1"), { wrapper });

    await act(() => result.current.mutateAsync("c-1"));

    expect(request).toHaveBeenCalledWith("/api/issues/i-1/criteria/c-1", expect.objectContaining({ method: "PATCH" }));
    expect(bodyOf(0)).toEqual({ archived: true });
  });
});

describe("useVerifyOutcome", () => {
  it("records a check on the outcome without sending its id in the body", async () => {
    request.mockResolvedValueOnce({ id: "v-1", outcome_id: "o-1", result: "pass" });
    const { wrapper, invalidate } = setup();
    const { result } = renderHook(() => useVerifyOutcome("ws-1"), { wrapper });

    let v: unknown;
    await act(async () => {
      v = await result.current.mutateAsync({ outcomeId: "o-1", result: "pass", note: "checked" });
    });

    expect(request).toHaveBeenCalledWith("/api/outcomes/o-1/verifications", expect.objectContaining({ method: "POST" }));
    expect(bodyOf(0)).toEqual({ result: "pass", note: "checked" });
    expect(headersOf(0)?.["X-Attention-Opened-At"]).toBeUndefined();
    expect(v).toMatchObject({ id: "v-1", result: "pass" });
    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "outcomes"] }));
  });

  it("sends X-Attention-Opened-At when openedAt is given, and omits it from body", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(1_700_000_120_750);
    request.mockResolvedValueOnce({ id: "v-1", outcome_id: "o-1", result: "pass" });
    const { wrapper } = setup();
    const { result } = renderHook(() => useVerifyOutcome("ws-1"), { wrapper });

    await act(() =>
      result.current.mutateAsync({
        outcomeId: "o-1",
        result: "pass",
        note: "checked",
        openedAt: 1_700_000_000_000,
      }),
    );

    expect(headersOf(0)).toEqual({ "X-Attention-Opened-At": "120" });
    expect(bodyOf(0)).toEqual({ result: "pass", note: "checked" });
  });
});
