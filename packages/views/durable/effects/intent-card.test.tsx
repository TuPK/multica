import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { api, ApiError } from "@multica/core/api";
import type { Intent } from "@multica/core/durable/intents";
import { renderWithI18n } from "../../test/i18n";
import { IntentCard } from "./intent-card";

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const emailIntent: Intent = {
  id: "i-1",
  created_at: "2026-11-25T08:00:00Z",
  expires_at: "2026-11-28T08:00:00Z",
  agent_id: "a-1",
  issue_id: null,
  source: "connected_app",
  target: "gmail:GMAIL_SEND_EMAIL",
  effect_class: "send",
  state: "pending",
  call: {
    tool: "GMAIL_SEND_EMAIL",
    account_id: "acc-1",
    args: { to: "customer@example.com", subject: "Báo giá", body: "Xin chào" },
  },
  call_sha256: "h1",
  preview: {
    kind: "email",
    title: "Báo giá",
    body_markdown: "Xin chào",
    fields: [{ label: "Subject", before: "", after: "Báo giá" }],
    recipients: ["customer@example.com"],
    amount_micros: null,
  },
  rationale: "Khách hỏi giá",
  decision_note: "",
  batch_key: "",
};

const recordIntent: Intent = {
  ...emailIntent,
  id: "i-2",
  effect_class: "write",
  call: { tool: "CRM_UPDATE", account_id: "acc-2", args: { properties: { stage: "won" } } },
  preview: {
    kind: "record",
    title: "",
    body_markdown: "",
    fields: [{ label: "stage", before: "lead", after: "won" }],
    recipients: [],
    amount_micros: null,
  },
};

function renderCard(intent: Intent, onDecided?: () => void) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <IntentCard intent={intent} onDecided={onDecided} />
    </QueryClientProvider>,
  );
}

const request = vi.mocked(api.request);

function bodyOfCall(n: number): unknown {
  const init = request.mock.calls[n]?.[1] as { body?: string } | undefined;
  return JSON.parse(init?.body ?? "null");
}

beforeEach(() => {
  vi.clearAllMocks();
});

afterEach(() => {
  request.mockReset();
});

describe("IntentCard", () => {
  it("shows the preview the platform built from the call, and the agent's reason apart from it", () => {
    renderCard(emailIntent);

    expect(screen.getByRole("heading", { name: "Báo giá" })).toBeInTheDocument();
    expect(screen.getByText("customer@example.com")).toBeInTheDocument();
    expect(screen.getByText("Xin chào")).toBeInTheDocument();
    expect(screen.getByText("Khách hỏi giá")).toBeInTheDocument();
  });

  it("shows each field as it changes from before to after", () => {
    renderCard(recordIntent);
    expect(screen.getByText("stage")).toBeInTheDocument();
    expect(screen.getByText("lead")).toBeInTheDocument();
    expect(screen.getByText("won")).toBeInTheDocument();
  });

  it("offers an edit only for a message whose text the server lets a person change", () => {
    const { unmount } = renderCard(emailIntent);
    expect(screen.getByRole("button", { name: "Edit and approve" })).toBeInTheDocument();
    unmount();

    renderCard(recordIntent);
    expect(screen.queryByRole("button", { name: "Edit and approve" })).not.toBeInTheDocument();
  });

  it("sends the edited call when the person approves an edited message", async () => {
    request.mockResolvedValueOnce({ ...emailIntent, state: "executed" });
    renderCard(emailIntent);

    fireEvent.click(screen.getByRole("button", { name: "Edit and approve" }));
    fireEvent.change(screen.getByLabelText("Message text"), { target: { value: "Xin chào, đây là bản sửa" } });
    fireEvent.click(screen.getByRole("button", { name: "Approve edited version" }));

    await waitFor(() => expect(request).toHaveBeenCalledTimes(1));
    expect(request).toHaveBeenCalledWith("/api/intents/i-1/approve", expect.objectContaining({ method: "POST" }));
    expect(bodyOfCall(0)).toEqual({
      edited_call: {
        tool: "GMAIL_SEND_EMAIL",
        account_id: "acc-1",
        args: { to: "customer@example.com", subject: "Báo giá", body: "Xin chào, đây là bản sửa" },
      },
      note: "",
    });
  });

  it("approves as proposed, with no edited call", async () => {
    request.mockResolvedValueOnce({ ...emailIntent, state: "executed" });
    renderCard(emailIntent);

    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    await waitFor(() => expect(toast.success).toHaveBeenCalled());
    expect(bodyOfCall(0)).toEqual({ note: "" });
  });

  it("keeps the rejection disabled until the person writes why", async () => {
    request.mockResolvedValueOnce({ ...emailIntent, state: "rejected" });
    renderCard(emailIntent);

    fireEvent.click(screen.getByRole("button", { name: "Reject" }));
    expect(screen.getByRole("button", { name: "Send rejection" })).toBeDisabled();

    fireEvent.change(screen.getByLabelText("Reason (the agent reads it)"), { target: { value: "Giá chưa đúng" } });
    fireEvent.click(screen.getByRole("button", { name: "Send rejection" }));

    await waitFor(() => expect(request).toHaveBeenCalledTimes(1));
    expect(request).toHaveBeenCalledWith("/api/intents/i-1/reject", expect.objectContaining({ method: "POST" }));
    expect(bodyOfCall(0)).toEqual({ note: "Giá chưa đúng" });
  });

  it("still shows the approval toast after the card unmounts before the request settles", async () => {
    let finish!: (value: unknown) => void;
    request.mockImplementationOnce(() => new Promise((resolve) => (finish = resolve)));
    const { unmount } = renderCard(emailIntent);

    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    await waitFor(() => expect(request).toHaveBeenCalled());
    unmount();
    await act(async () => finish({ ...emailIntent, state: "executed" }));

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Approved. The action runs once."));
  });

  it("still reports a failed approval after the card unmounts", async () => {
    let fail!: (reason: unknown) => void;
    request.mockImplementationOnce(() => new Promise((_, reject) => (fail = reject)));
    const { unmount } = renderCard(emailIntent);

    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    await waitFor(() => expect(request).toHaveBeenCalled());
    unmount();
    await act(async () => fail(new Error("boom")));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Could not approve this action."));
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("explains a decision that was already made by someone else", async () => {
    request.mockRejectedValueOnce(
      new ApiError("conflict", 409, "Conflict", { code: "intent_not_pending", error: "conflict" }),
    );
    renderCard(emailIntent);

    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("This action was already decided."));
  });

  it("calls back once the decision is recorded", async () => {
    request.mockResolvedValueOnce({ ...emailIntent, state: "executed" });
    const onDecided = vi.fn();
    renderCard(emailIntent, onDecided);

    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    await waitFor(() => expect(onDecided).toHaveBeenCalledTimes(1));
  });
});
