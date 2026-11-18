// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import { EinvoiceFailures } from "./EinvoiceFailures";
import { PlatformApiError } from "../api/client";

const mockUseEinvoices = vi.fn();
let mockMeRole = "billing";

vi.mock("../api/hooks", () => ({
  useEinvoices: (state?: string) => mockUseEinvoices(state),
  useMe: () => ({
    data: { user_id: "u-staff", email: "staff@example.com", role: mockMeRole },
  }),
}));

const ORDER_FAILED_ID = "11111111-1111-4111-8111-111111111111";
const ORDER_NEEDS_INFO_ID = "22222222-2222-4222-8222-222222222222";
const WS_ID = "33333333-3333-4333-8333-333333333333";

const mockFailedEinvoice = {
  order_id: ORDER_FAILED_ID,
  workspace_id: WS_ID,
  revision: 1,
  state: "failed" as const,
  currency: "vnd",
  amount: 600000,
  vat_amount: 60000,
  provider: "sepay",
  invoice_number: "",
  lookup_url: "",
  attempts: 8,
  last_error_code: "provider_rejected",
  missing_fields: [],
  next_attempt_at: "2026-11-18T12:00:00Z",
  updated_at: "2026-11-18T11:00:00Z",
};

const mockNeedsInfoEinvoice = {
  order_id: ORDER_NEEDS_INFO_ID,
  workspace_id: WS_ID,
  revision: 2,
  state: "needs_info" as const,
  currency: "vnd",
  amount: 300000,
  vat_amount: 30000,
  provider: "sepay",
  invoice_number: "",
  lookup_url: "",
  attempts: 0,
  last_error_code: "missing_fields",
  missing_fields: ["tax_code", "address"],
  next_attempt_at: "2026-11-18T12:00:00Z",
  updated_at: "2026-11-18T11:05:00Z",
};

describe("EinvoiceFailures screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockMeRole = "billing";
    mockUseEinvoices.mockReturnValue({
      data: [mockFailedEinvoice, mockNeedsInfoEinvoice],
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });
  });

  it("lists failed and needs_info e-invoices with state, amount, attempts, error code, and missing fields", () => {
    render(<EinvoiceFailures userRole="billing" />);

    expect(screen.getByRole("heading", { name: "E-invoices" })).toBeInTheDocument();
    expect(mockUseEinvoices).toHaveBeenCalledWith("");

    const table = within(screen.getByRole("table"));
    expect(table.getByText(ORDER_FAILED_ID)).toBeInTheDocument();
    expect(table.getByText(ORDER_NEEDS_INFO_ID)).toBeInTheDocument();
    expect(table.getByText("Failed")).toBeInTheDocument();
    expect(table.getByText("Needs buyer details")).toBeInTheDocument();
    expect(table.getByText("provider_rejected")).toBeInTheDocument();
    expect(table.getByText("tax_code, address")).toBeInTheDocument();
    expect(table.getByText(/660,000/)).toBeInTheDocument();
  });

  it("switches state filter tabs", () => {
    render(<EinvoiceFailures userRole="admin" />);

    fireEvent.click(screen.getByRole("tab", { name: "Issued" }));
    expect(mockUseEinvoices).toHaveBeenCalledWith("issued");

    fireEvent.click(screen.getByRole("tab", { name: "Pending" }));
    expect(mockUseEinvoices).toHaveBeenCalledWith("pending");
  });

  it("shows an https lookup link when issued and never renders unsafe URLs", () => {
    mockUseEinvoices.mockReturnValue({
      data: [
        {
          ...mockFailedEinvoice,
          order_id: "ord-https",
          state: "issued",
          invoice_number: "00001234",
          lookup_url: "https://einvoice.sepay.vn/lookup/abc",
          last_error_code: "",
        },
        {
          ...mockFailedEinvoice,
          order_id: "ord-http",
          state: "issued",
          invoice_number: "00001235",
          lookup_url: "http://insecure.example.com/inv/2",
          last_error_code: "",
        },
      ],
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<EinvoiceFailures userRole="billing" />);

    const links = screen.getAllByRole("link", { name: /View/i });
    expect(links).toHaveLength(1);
    expect(links[0]).toHaveAttribute("href", "https://einvoice.sepay.vn/lookup/abc");
    expect(links[0]).toHaveAttribute("target", "_blank");
    expect(links[0]).toHaveAttribute("rel", expect.stringContaining("noopener"));
  });

  it("shows loading, error with retry, and empty states", () => {
    mockUseEinvoices.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });
    const { rerender } = render(<EinvoiceFailures userRole="billing" />);
    expect(screen.getByText("Loading e-invoices...")).toBeInTheDocument();

    const refetch = vi.fn();
    mockUseEinvoices.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new PlatformApiError(500, null, "boom"),
      refetch,
    });
    rerender(<EinvoiceFailures userRole="billing" />);
    expect(screen.getByText("Failed to load e-invoices")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(refetch).toHaveBeenCalledTimes(1);

    mockUseEinvoices.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      error: null,
      refetch,
    });
    rerender(<EinvoiceFailures userRole="billing" />);
    expect(screen.getByText("No e-invoices found")).toBeInTheDocument();
  });

  it("hides the screen from support role", () => {
    mockMeRole = "support";
    render(<EinvoiceFailures />);

    expect(screen.queryByRole("heading", { name: "E-invoices" })).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Not found" })).toBeInTheDocument();
  });
});
