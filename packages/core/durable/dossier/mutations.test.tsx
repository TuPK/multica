// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useAddSeatDossierEntries, useRetireSeatDossierEntry } from "./mutations";

const { request } = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("../../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api")>()),
  api: { request },
}));

const entryRow = {
  id: "d-1",
  agent_id: "a-1",
  kind: "lesson",
  text: "Always verify test fixtures",
  work_class: "general",
  evidence: [{ type: "document", ref: "doc-1" }],
  origin: "member",
  status: "active",
  created_at: "2026-11-26T09:00:00Z",
  retired_at: null,
};

function setup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
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

describe("useAddSeatDossierEntries", () => {
  it("posts entries to the seat dossier route and invalidates the seat cache", async () => {
    request.mockResolvedValueOnce({ entries: [entryRow] });
    const { wrapper, invalidate } = setup();
    const { result } = renderHook(() => useAddSeatDossierEntries("ws-1", "a-1"), { wrapper });

    let added: unknown;
    await act(async () => {
      added = await result.current.mutateAsync({
        entries: [
          {
            kind: "lesson",
            text: "Always verify test fixtures",
            work_class: "general",
            evidence: [{ type: "document", ref: "doc-1" }],
          },
        ],
      });
    });

    expect(request).toHaveBeenCalledWith("/api/agents/a-1/dossier", expect.objectContaining({ method: "POST" }));
    expect(bodyOf(0)).toEqual({
      entries: [
        {
          kind: "lesson",
          text: "Always verify test fixtures",
          work_class: "general",
          evidence: [{ type: "document", ref: "doc-1" }],
        },
      ],
    });
    expect(added).toHaveLength(1);
    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "dossier", "a-1"] }));
  });

  it("still invalidates when the caller unmounts before the request settles", async () => {
    const reply = deferred<unknown>();
    request.mockReturnValueOnce(reply.promise);
    const { wrapper, invalidate } = setup();
    const { result, unmount } = renderHook(() => useAddSeatDossierEntries("ws-1", "a-1"), { wrapper });

    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.mutateAsync({
        entries: [{ kind: "lesson", text: "x", work_class: "", evidence: [{ type: "comment", ref: "c-1" }] }],
      });
    });
    unmount();
    await act(async () => {
      reply.resolve({ entries: [entryRow] });
      await pending;
    });

    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "dossier", "a-1"] }));
  });
});

describe("useRetireSeatDossierEntry", () => {
  it("posts the reason to the retire route and invalidates the seat cache", async () => {
    request.mockResolvedValueOnce({ ...entryRow, status: "retired", retired_at: "2026-11-27T01:00:00Z" });
    const { wrapper, invalidate } = setup();
    const { result } = renderHook(() => useRetireSeatDossierEntry("ws-1", "a-1"), { wrapper });

    let retired: unknown;
    await act(async () => {
      retired = await result.current.mutateAsync({ entryId: "d-1", reason: "Superseded by new workflow" });
    });

    expect(request).toHaveBeenCalledWith(
      "/api/agents/a-1/dossier/d-1/retire",
      expect.objectContaining({ method: "POST" }),
    );
    expect(bodyOf(0)).toEqual({ reason: "Superseded by new workflow" });
    expect(retired).toMatchObject({ id: "d-1", status: "retired" });
    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "dossier", "a-1"] }));
  });

  it("returns null on a malformed retire reply", async () => {
    request.mockResolvedValueOnce("oops");
    const { wrapper } = setup();
    const { result } = renderHook(() => useRetireSeatDossierEntry("ws-1", "a-1"), { wrapper });

    let retired: unknown = "unset";
    await act(async () => {
      retired = await result.current.mutateAsync({ entryId: "d-1", reason: "Old" });
    });
    expect(retired).toBeNull();
  });
});
