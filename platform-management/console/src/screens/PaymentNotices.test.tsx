// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { PaymentNotices } from "./PaymentNotices";
import { PlatformApiError } from "../api/client";

const mockUsePaymentNotices = vi.fn();
const mockUsePaymentNotice = vi.fn();
const mockResolveMutateAsync = vi.fn();
const mockDismissMutateAsync = vi.fn();
let mockMeRole = "billing";

vi.mock("../api/hooks", () => ({
  usePaymentNotices: () => mockUsePaymentNotices(),
  usePaymentNotice: (id: string | null) => mockUsePaymentNotice(id),
  useResolvePaymentNotice: () => ({ mutateAsync: mockResolveMutateAsync, isPending: false }),
  useDismissPaymentNotice: () => ({ mutateAsync: mockDismissMutateAsync, isPending: false }),
  useMe: () => ({
    data: { user_id: "u-staff", email: "staff@example.com", role: mockMeRole },
  }),
}));

const NOTICE_ID = "11111111-1111-4111-8111-111111111111";
const ORDER_ID = "22222222-2222-4222-8222-222222222222";

const mockNotice = {
  id: NOTICE_ID,
  provider: "sepay",
  provider_event_id: "evt-123",
  state: "needs_review" as const,
  transfer_code: "MUL12345678",
  amount: 660000,
  currency: "vnd",
  review_reason: "ambiguous_code",
  order_id: "",
  received_at: "2026-11-17T10:00:00Z",
  decided_at: null,
};

const mockCandidate = {
  order_id: ORDER_ID,
  workspace_id: "33333333-3333-4333-8333-333333333333",
  state: "pending",
  transfer_code: "MUL12345678",
  amount: 600000,
  vat_amount: 60000,
  currency: "vnd",
  created_at: "2026-11-13T10:00:00Z",
  match: "code",
};

const mockDetail = { ...mockNotice, candidates: [mockCandidate] };

function detailResult(data: unknown) {
  return { data, isLoading: false, isError: false, error: null, refetch: vi.fn() };
}

describe("PaymentNotices screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockMeRole = "billing";
    mockUsePaymentNotices.mockReturnValue({
      data: [mockNotice],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockUsePaymentNotice.mockImplementation((id: string | null) =>
      id === NOTICE_ID ? detailResult(mockDetail) : detailResult(undefined),
    );
  });

  it("lists notices that need review with transfer code, amount and a readable reason", () => {
    render(<PaymentNotices userRole="billing" />);

    expect(screen.getByRole("heading", { name: "Payment notices" })).toBeInTheDocument();
    expect(screen.getByText("MUL12345678")).toBeInTheDocument();
    expect(screen.getByText(/660,000/)).toBeInTheDocument();
    expect(screen.getByText("Ambiguous transfer code")).toBeInTheDocument();
    expect(screen.getByText("No notice selected")).toBeInTheDocument();
  });

  it("shows the loading state while the queue loads", () => {
    mockUsePaymentNotices.mockReturnValue({ data: undefined, isLoading: true, isError: false, refetch: vi.fn() });
    render(<PaymentNotices userRole="billing" />);

    expect(screen.getByText("Loading payment notices...")).toBeInTheDocument();
  });

  it("shows an error state and retries when the queue fails", () => {
    const refetch = vi.fn();
    mockUsePaymentNotices.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new PlatformApiError(500, null, "boom"),
      refetch,
    });
    render(<PaymentNotices userRole="billing" />);

    expect(screen.getByText("Failed to load payment notices")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(refetch).toHaveBeenCalledTimes(1);
  });

  it("shows an empty state with refresh action when no notice needs review", () => {
    const refetch = vi.fn();
    mockUsePaymentNotices.mockReturnValue({ data: [], isLoading: false, isError: false, refetch });
    render(<PaymentNotices userRole="billing" />);

    expect(screen.getByText("No payment notices need review")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(refetch).toHaveBeenCalledTimes(1);
  });

  it("opens the detail of one notice with its event id and candidate orders", () => {
    render(<PaymentNotices userRole="billing" />);

    fireEvent.click(screen.getAllByText("MUL12345678")[0]!.closest("tr")!);

    expect(mockUsePaymentNotice).toHaveBeenLastCalledWith(NOTICE_ID);
    expect(screen.getByText("evt-123")).toBeInTheDocument();
    expect(screen.getByText("33333333-3333-4333-8333-333333333333")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Match" })).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Dismiss" }).length).toBeGreaterThan(0);
  });

  it("says when no order matches the notice", () => {
    mockUsePaymentNotice.mockImplementation(() => detailResult({ ...mockDetail, candidates: [] }));
    render(<PaymentNotices userRole="billing" />);

    fireEvent.click(screen.getAllByText("MUL12345678")[0]!.closest("tr")!);

    expect(screen.getByText("No matching orders")).toBeInTheDocument();
  });

  it("asks for a reason before a manual match and sends the order id with it", async () => {
    mockResolveMutateAsync.mockResolvedValue(mockNotice);
    render(<PaymentNotices userRole="billing" />);
    fireEvent.click(screen.getAllByText("MUL12345678")[0]!.closest("tr")!);
    fireEvent.click(screen.getByRole("button", { name: "Match" }));

    const confirm = screen.getByRole("button", { name: "Match payment" });
    expect(confirm).toBeDisabledControl();

    fireEvent.change(within(screen.getByRole("dialog")).getByLabelText(/Reason/), { target: { value: "Customer sent the wrong code" } });
    expect(confirm).not.toBeDisabledControl();
    fireEvent.click(confirm);

    await waitFor(() =>
      expect(mockResolveMutateAsync).toHaveBeenCalledWith({
        noticeId: NOTICE_ID,
        orderId: ORDER_ID,
        reason: "Customer sent the wrong code",
      }),
    );
  });

  it("asks for a reason before dismissing and sends it with the notice id", async () => {
    mockDismissMutateAsync.mockResolvedValue({ ...mockNotice, state: "dismissed" });
    render(<PaymentNotices userRole="billing" />);
    fireEvent.click(screen.getAllByText("MUL12345678")[0]!.closest("tr")!);
    fireEvent.click(screen.getAllByRole("button", { name: "Dismiss" }).at(-1)!);

    const confirm = screen.getByRole("button", { name: "Dismiss notice" });
    expect(confirm).toBeDisabledControl();

    fireEvent.change(within(screen.getByRole("dialog")).getByLabelText(/Reason/), { target: { value: "Test transfer from the bank" } });
    fireEvent.click(confirm);

    await waitFor(() =>
      expect(mockDismissMutateAsync).toHaveBeenCalledWith({
        noticeId: NOTICE_ID,
        reason: "Test transfer from the bank",
      }),
    );
  });

  it("shows a closed notice conflict in the dialog instead of a generic failure", async () => {
    mockResolveMutateAsync.mockRejectedValue(
      new PlatformApiError(409, "payment_notice_closed", "the payment notice is no longer waiting for review"),
    );
    render(<PaymentNotices userRole="billing" />);
    fireEvent.click(screen.getAllByText("MUL12345678")[0]!.closest("tr")!);
    fireEvent.click(screen.getByRole("button", { name: "Match" }));
    fireEvent.change(within(screen.getByRole("dialog")).getByLabelText(/Reason/), { target: { value: "Matched by hand" } });
    fireEvent.click(screen.getByRole("button", { name: "Match payment" }));

    const dialog = screen.getByRole("dialog");
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("no longer waiting for review");
  });

  it("hides the queue and its actions from support", () => {
    mockMeRole = "support";
    render(<PaymentNotices />);

    expect(screen.queryByRole("heading", { name: "Payment notices" })).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Not found" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Match" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Dismiss" })).not.toBeInTheDocument();
  });
});
