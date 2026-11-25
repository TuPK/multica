// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useApproveIntent, useApproveIntentBatch, useRejectIntent } from "./mutations";

const { request } = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("../../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api")>()),
  api: { request },
}));

const intentRow = {
  id: "i-1",
  created_at: "2026-11-25T08:00:00Z",
  expires_at: "2026-11-28T08:00:00Z",
  agent_id: "a-1",
  issue_id: null,
  source: "connected_app",
  target: "gmail",
  effect_class: "send",
  state: "executed",
  call: null,
  call_sha256: "h",
  preview: { kind: "email", title: "", body_markdown: "", fields: [], recipients: [], amount_micros: null },
  rationale: "",
  decision_note: "",
  batch_key: "",
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

afterEach(() => {
  cleanup();
  request.mockReset();
});

describe("useApproveIntent", () => {
  it("approves the intent with no edit when none was made", async () => {
    request.mockResolvedValueOnce(intentRow);
    const { wrapper } = setup();
    const { result } = renderHook(() => useApproveIntent("ws-1"), { wrapper });

    await act(() => result.current.mutateAsync({ id: "i-1" }));

    expect(request).toHaveBeenCalledWith("/api/intents/i-1/approve", expect.objectContaining({ method: "POST" }));
    expect(bodyOf(0)).toEqual({ note: "" });
  });

  it("sends the edited call the person made before approving", async () => {
    request.mockResolvedValueOnce(intentRow);
    const { wrapper } = setup();
    const { result } = renderHook(() => useApproveIntent("ws-1"), { wrapper });
    const editedCall = { tool: "GMAIL_SEND_EMAIL", account_id: "acc-1", args: { to: "c@example.com", body: "Bản đã sửa" } };

    await act(() => result.current.mutateAsync({ id: "i-1", editedCall, note: "sửa lời chào" }));

    expect(bodyOf(0)).toEqual({ edited_call: editedCall, note: "sửa lời chào" });
  });

  it("answers null for an approval whose body does not parse, and refreshes intents and effects", async () => {
    request.mockResolvedValueOnce("not an intent");
    const { wrapper, invalidate } = setup();
    const { result } = renderHook(() => useApproveIntent("ws-1"), { wrapper });

    let approved: unknown;
    await act(async () => {
      approved = await result.current.mutateAsync({ id: "i-1" });
    });

    expect(approved).toBeNull();
    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "intents"] }));
    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ["durable", "ws-1", "effects"] }));
  });
});

describe("useRejectIntent", () => {
  it("sends the rejection with its note", async () => {
    request.mockResolvedValueOnce({ ...intentRow, state: "rejected" });
    const { wrapper } = setup();
    const { result } = renderHook(() => useRejectIntent("ws-1"), { wrapper });

    await act(() => result.current.mutateAsync({ id: "i-1", note: "Giá chưa đúng" }));

    expect(request).toHaveBeenCalledWith("/api/intents/i-1/reject", expect.objectContaining({ method: "POST" }));
    expect(bodyOf(0)).toEqual({ note: "Giá chưa đúng" });
  });
});

describe("useApproveIntentBatch", () => {
  it("approves the listed intents in one request and reads each outcome", async () => {
    request.mockResolvedValueOnce({
      results: [
        { id: "i-1", state: "executed", status: 200 },
        { id: "i-2", error: "already decided", status: 409 },
      ],
    });
    const { wrapper } = setup();
    const { result } = renderHook(() => useApproveIntentBatch("ws-1"), { wrapper });

    let outcomes: unknown;
    await act(async () => {
      outcomes = await result.current.mutateAsync({ ids: ["i-1", "i-2"] });
    });

    expect(request).toHaveBeenCalledWith("/api/intents/approve-batch", expect.objectContaining({ method: "POST" }));
    expect(bodyOf(0)).toEqual({ ids: ["i-1", "i-2"] });
    expect(outcomes).toEqual([
      { id: "i-1", state: "executed", error: "", status: 200 },
      { id: "i-2", state: "", error: "already decided", status: 409 },
    ]);
  });
});
