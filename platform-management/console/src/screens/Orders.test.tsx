// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { Orders } from "./Orders";
import { PlatformApiError } from "../api/client";

const mockUseOrders = vi.fn();
const mockConfirmOrderMutateAsync = vi.fn();
const mockCancelOrderMutateAsync = vi.fn();
let mockMeRole = "billing";

vi.mock("../api/hooks", () => ({
  useOrders: (status?: string) => mockUseOrders(status),
  useConfirmOrder: () => ({
    mutateAsync: mockConfirmOrderMutateAsync,
    isPending: false,
  }),
  useCancelOrder: () => ({
    mutateAsync: mockCancelOrderMutateAsync,
    isPending: false,
  }),
  useMe: () => ({
    data: { user_id: "u-staff", email: "staff@example.com", role: mockMeRole },
  }),
}));

const mockPendingOrder = {
  id: "ord-1",
  workspace_id: "ws-1",
  workspace_name: "Acme Corp",
  kind: "subscribe" as const,
  plan_key: "pro",
  interval: "month",
  seats: 3,
  amount: 600000,
  vat_amount: 60000,
  currency: "vnd",
  status: "pending" as const,
  transfer_code: "MUL12345678",
  expires_at: "2026-11-20T10:00:00Z",
  created_at: "2026-11-13T10:00:00Z",
  paid_at: null,
  buyer: {
    name: "Acme Vietnam Ltd",
    tax_code: "0101234567",
    address: "123 Le Loi, HCMC",
    invoice_email: "billing@acme.vn",
  },
};

const mockPaidOrder = {
  ...mockPendingOrder,
  id: "ord-2",
  status: "paid" as const,
  paid_at: "2026-11-14T12:00:00Z",
};

describe("Orders screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockMeRole = "billing";
    mockUseOrders.mockReturnValue({
      data: [mockPendingOrder],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
  });

  it("lists orders with status tabs, amount including VAT, transfer code, copy button, expiry, and buyer invoice email", () => {
    render(<Orders userRole="billing" />);

    expect(screen.getByRole("heading", { name: "Orders" })).toBeInTheDocument();

    // Status tabs exist
    expect(screen.getByRole("tab", { name: /Pending/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Paid/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Expired/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Canceled/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Refunded/i })).toBeInTheDocument();

    // Order details displayed
    expect(screen.getByText("Acme Corp")).toBeInTheDocument();
    expect(screen.getByText("MUL12345678")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /copy transfer code/i })).toBeInTheDocument();

    // Total amount including VAT: 600000 + 60000 = 660,000 VND
    expect(screen.getByText(/660,000/)).toBeInTheDocument();

    // Buyer invoice email
    expect(screen.getByText("billing@acme.vn")).toBeInTheDocument();

    // Confirm and Cancel buttons for pending order
    expect(screen.getByRole("button", { name: /^confirm/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^cancel/i })).toBeInTheDocument();
  });

  it("copies transfer code to clipboard", async () => {
    const writeTextMock = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, {
      clipboard: { writeText: writeTextMock },
    });

    render(<Orders userRole="billing" />);

    const copyBtn = screen.getByRole("button", { name: /copy transfer code/i });
    fireEvent.click(copyBtn);

    expect(writeTextMock).toHaveBeenCalledWith("MUL12345678");
  });

  it("switching status tabs fetches that status", () => {
    render(<Orders userRole="billing" />);

    const paidTab = screen.getByRole("tab", { name: /Paid/i });
    fireEvent.click(paidTab);

    expect(mockUseOrders).toHaveBeenCalledWith("paid");
  });

  it("Confirm button opens ConfirmReasonDialog and requires reason before submit", async () => {
    mockConfirmOrderMutateAsync.mockResolvedValue({ order: mockPaidOrder, applied: true });
    render(<Orders userRole="billing" />);

    const confirmBtn = screen.getByRole("button", { name: /^confirm/i });
    fireEvent.click(confirmBtn);

    expect(screen.getByRole("heading", { name: /confirm payment/i })).toBeInTheDocument();

    const submitBtn = screen.getByRole("button", { name: "Confirm payment" });
    expect(submitBtn).toBeDisabled();

    const reasonInput = screen.getByLabelText(/reason/i);
    fireEvent.change(reasonInput, { target: { value: "Bank wire received at Vietcombank" } });
    expect(submitBtn).toBeEnabled();

    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(mockConfirmOrderMutateAsync).toHaveBeenCalledWith({
        orderId: "ord-1",
        reason: "Bank wire received at Vietcombank",
        transferReference: undefined,
      });
    });
  });

  it("Cancel button opens ConfirmReasonDialog and requires reason before submit", async () => {
    mockCancelOrderMutateAsync.mockResolvedValue({ order: { ...mockPendingOrder, status: "canceled" }, applied: true });
    render(<Orders userRole="billing" />);

    const cancelBtn = screen.getByRole("button", { name: /^cancel/i });
    fireEvent.click(cancelBtn);

    expect(screen.getByRole("heading", { name: /cancel order/i })).toBeInTheDocument();

    const submitBtn = screen.getByRole("button", { name: "Cancel order" });
    expect(submitBtn).toBeDisabled();

    const reasonInput = screen.getByLabelText(/reason/i);
    fireEvent.change(reasonInput, { target: { value: "Customer requested cancellation" } });
    expect(submitBtn).toBeEnabled();

    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(mockCancelOrderMutateAsync).toHaveBeenCalledWith({
        orderId: "ord-1",
        reason: "Customer requested cancellation",
      });
    });
  });

  it("a 409 conflict error shows the error code's message", async () => {
    const error409 = new PlatformApiError(409, "order_expired", "This order has expired and cannot be confirmed");
    mockConfirmOrderMutateAsync.mockRejectedValue(error409);

    render(<Orders userRole="billing" />);

    fireEvent.click(screen.getByRole("button", { name: /^confirm/i }));

    const reasonInput = screen.getByLabelText(/reason/i);
    fireEvent.change(reasonInput, { target: { value: "Manual override attempt" } });

    fireEvent.click(screen.getByRole("button", { name: "Confirm payment" }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("This order has expired and cannot be confirmed");
    });
  });

  it("support role sees the not-found state if it opens the screen directly", () => {
    mockMeRole = "support";
    render(<Orders userRole="support" />);

    expect(screen.queryByRole("heading", { name: "Orders" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Not found" })).toBeInTheDocument();
  });
});
