// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { Orders } from "./Orders";
import { PlatformApiError } from "../api/client";

const mockUseOrders = vi.fn();
const mockUseOrderEinvoices = vi.fn();
const mockConfirmOrderMutateAsync = vi.fn();
const mockCancelOrderMutateAsync = vi.fn();
let mockMeRole = "billing";

vi.mock("../api/hooks", () => ({
  useOrders: (status?: string) => mockUseOrders(status),
  useOrderEinvoices: (orderId: string | null) => mockUseOrderEinvoices(orderId),
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
  invoice_state: null,
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
    mockUseOrderEinvoices.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      error: null,
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
    expect(screen.getByText("Subscribe")).toBeInTheDocument();
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

  it("shows the e-invoice state label for pending, issued, needs_info, failed and an em dash for null", () => {
    mockUseOrders.mockReturnValue({
      data: [
        { ...mockPaidOrder, id: "ord-pending", workspace_name: "WS Pending", invoice_state: "pending" },
        { ...mockPaidOrder, id: "ord-issued", workspace_name: "WS Issued", invoice_state: "issued" },
        { ...mockPaidOrder, id: "ord-needs-info", workspace_name: "WS Needs Info", invoice_state: "needs_info" },
        { ...mockPaidOrder, id: "ord-failed", workspace_name: "WS Failed", invoice_state: "failed" },
        { ...mockPaidOrder, id: "ord-none", workspace_name: "WS None", invoice_state: null },
      ],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<Orders userRole="billing" />);
    fireEvent.click(screen.getByRole("tab", { name: /Paid/i }));

    const rowOf = (workspace: string) => {
      const cell = screen.getByText(workspace);
      const row = cell.closest("tr");
      if (!row) throw new Error(`Missing row for ${workspace}`);
      return within(row);
    };

    expect(screen.getByRole("columnheader", { name: /E-invoice|Invoice/i })).toBeInTheDocument();
    expect(rowOf("WS Pending").getByText("Pending")).toBeInTheDocument();
    expect(rowOf("WS Issued").getByText("Issued")).toBeInTheDocument();
    expect(rowOf("WS Needs Info").getByText("Needs info")).toBeInTheDocument();
    expect(rowOf("WS Failed").getByText("Failed")).toBeInTheDocument();
    expect(rowOf("WS None").getByText("—")).toBeInTheDocument();
  });

  it("opens the e-invoice history dialog showing revisions newest first without buyer address or email", () => {
    mockUseOrders.mockReturnValue({
      data: [{ ...mockPaidOrder, id: "ord-hist", invoice_state: "issued" }],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockUseOrderEinvoices.mockImplementation((orderId: string | null) =>
      orderId === "ord-hist"
        ? {
            data: [
              {
                revision: 2,
                state: "issued",
                invoice_number: "INV-002",
                lookup_url: "https://einvoice.sepay.vn/lookup/rev2",
                buyer: {
                  name: "Acme Revised Co",
                  tax_code: "0109998888",
                  address: "99 Secret Address",
                  invoice_email: "hidden-rev2@acme.vn",
                },
                attempts: 1,
                last_error_code: "",
                created_at: "2026-11-18T07:00:00Z",
                issued_at: "2026-11-18T08:00:00Z",
                updated_at: "2026-11-18T09:05:00Z",
              },
              {
                revision: 1,
                state: "failed",
                invoice_number: "",
                lookup_url: "http://insecure.example.com/rev1",
                buyer: {
                  name: "Acme Old Co",
                  tax_code: "0101112222",
                  address: "11 Old Street",
                  invoice_email: "hidden-rev1@acme.vn",
                },
                attempts: 4,
                last_error_code: "provider_timeout",
                created_at: "2026-11-17T06:00:00Z",
                issued_at: null,
                updated_at: "2026-11-17T08:30:00Z",
              },
            ],
            isLoading: false,
            isError: false,
            error: null,
            refetch: vi.fn(),
          }
        : { data: [], isLoading: false, isError: false, error: null, refetch: vi.fn() },
    );

    render(<Orders userRole="billing" />);
    fireEvent.click(screen.getByRole("tab", { name: /Paid/i }));
    fireEvent.click(screen.getByRole("button", { name: /History/i }));

    expect(mockUseOrderEinvoices).toHaveBeenLastCalledWith("ord-hist");

    const dialog = screen.getByRole("dialog");
    const d = within(dialog);
    const rows = d.getAllByRole("row");
    // header + 2 revision rows; revision 2 must appear before revision 1
    expect(rows).toHaveLength(3);
    expect(within(rows[1]!).getByText("rev 2")).toBeInTheDocument();
    expect(within(rows[1]!).getByText("Issued")).toBeInTheDocument();
    expect(within(rows[1]!).getByText("INV-002")).toBeInTheDocument();
    expect(within(rows[1]!).getByText("Acme Revised Co")).toBeInTheDocument();
    expect(within(rows[1]!).getByText("0109998888")).toBeInTheDocument();
    expect(within(rows[1]!).getByText("1")).toBeInTheDocument();
    expect(within(rows[1]!).getByText("—")).toBeInTheDocument();
    expect(within(rows[1]!).getByText(new Date("2026-11-18T09:05:00Z").toLocaleString())).toBeInTheDocument();
    const links = within(rows[1]!).getAllByRole("link");
    expect(links).toHaveLength(1);
    expect(links[0]).toHaveAttribute("href", "https://einvoice.sepay.vn/lookup/rev2");

    expect(within(rows[2]!).getByText("rev 1")).toBeInTheDocument();
    expect(within(rows[2]!).getByText("Failed")).toBeInTheDocument();
    expect(within(rows[2]!).getByText("—")).toBeInTheDocument();
    expect(within(rows[2]!).getByText("Acme Old Co")).toBeInTheDocument();
    expect(within(rows[2]!).getByText("0101112222")).toBeInTheDocument();
    expect(within(rows[2]!).getByText("4")).toBeInTheDocument();
    expect(within(rows[2]!).getByText("provider_timeout")).toBeInTheDocument();
    expect(within(rows[2]!).getByText(new Date("2026-11-17T08:30:00Z").toLocaleString())).toBeInTheDocument();
    expect(within(rows[2]!).queryByRole("link")).not.toBeInTheDocument();

    // Buyer address, invoice email, created_at, and issued_at must never appear in the history view
    expect(d.queryByText(/99 Secret Address/)).not.toBeInTheDocument();
    expect(d.queryByText(/hidden-rev2@acme\.vn/)).not.toBeInTheDocument();
    expect(d.queryByText(/11 Old Street/)).not.toBeInTheDocument();
    expect(d.queryByText(/hidden-rev1@acme\.vn/)).not.toBeInTheDocument();
    expect(d.queryByText(new Date("2026-11-18T07:00:00Z").toLocaleString())).not.toBeInTheDocument();
    expect(d.queryByText(new Date("2026-11-18T08:00:00Z").toLocaleString())).not.toBeInTheDocument();
    expect(d.queryByText(new Date("2026-11-17T06:00:00Z").toLocaleString())).not.toBeInTheDocument();
  });

  it("shows empty, loading, and error states in the e-invoice history dialog", () => {
    mockUseOrderEinvoices.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });
    const { rerender } = render(<Orders userRole="billing" />);

    fireEvent.click(screen.getByRole("button", { name: /History/i }));
    expect(within(screen.getByRole("dialog")).getByText(/No e-invoice for this order/i)).toBeInTheDocument();

    mockUseOrderEinvoices.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });
    rerender(<Orders userRole="billing" />);
    expect(within(screen.getByRole("dialog")).getByText("Loading e-invoice history...")).toBeInTheDocument();

    const refetchHistory = vi.fn();
    mockUseOrderEinvoices.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new PlatformApiError(404, "order_not_found", "order not found"),
      refetch: refetchHistory,
    });
    rerender(<Orders userRole="billing" />);
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByRole("alert")).toHaveTextContent("order not found");
    fireEvent.click(dialog.getByRole("button", { name: "Try again" }));
    expect(refetchHistory).toHaveBeenCalledTimes(1);
  });

  it("shows Write off on pending usage orders only for admin role, and submits cancel with reason", async () => {
    const usageOrder = {
      ...mockPendingOrder,
      id: "ord-usage-9",
      kind: "usage" as const,
      transfer_code: "MULUSAGE01",
    };
    mockUseOrders.mockReturnValue({
      data: [usageOrder],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockCancelOrderMutateAsync.mockResolvedValue({
      order: { ...usageOrder, status: "canceled" },
      applied: true,
    });

    mockMeRole = "billing";
    const { rerender } = render(<Orders userRole="billing" />);
    expect(screen.getByText("Usage")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Write off" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();

    mockMeRole = "admin";
    rerender(<Orders userRole="admin" />);
    expect(screen.queryByRole("button", { name: "Confirm" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Write off" })).not.toBeInTheDocument();

    mockMeRole = "system_admin";
    rerender(<Orders userRole="system_admin" />);
    const writeOffBtn = screen.getByRole("button", { name: "Write off" });
    fireEvent.click(writeOffBtn);
    expect(screen.getByRole("heading", { name: "Write off usage order" })).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText(/Reason/i), {
      target: { value: "Approved goodwill write-off" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Write off order" }));

    await waitFor(() => {
      expect(mockCancelOrderMutateAsync).toHaveBeenCalledWith({
        orderId: "ord-usage-9",
        reason: "Approved goodwill write-off",
      });
    });
  });
});
