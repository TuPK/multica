import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiError } from "@multica/core/api";
import { renderWithI18n } from "../test/i18n";
import {
  BillingOrder,
  BillingOrderPage,
  validateTaxCode,
} from "./order-page";

function renderWithProviders(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return renderWithI18n(
    <QueryClientProvider client={queryClient}>
      {ui}
    </QueryClientProvider>,
  );
}

const mockOrder: BillingOrder = {
  id: "order-123",
  workspaceId: "ws-456",
  kind: "subscribe",
  planKey: "pro",
  interval: "month",
  seats: 5,
  amount: 745000,
  vatAmount: 74500,
  currency: "vnd",
  status: "pending",
  provider: "manual",
  transferCode: "MUL-789-XYZ",
  expiresAt: "2030-05-15T12:00:00Z",
  createdAt: "2030-05-08T12:00:00Z",
  bankDetails: {
    bankName: "Vietcombank",
    accountName: "CONG TY TNHH MULTICA",
    accountNumber: "9876543210",
  },
  buyer: {
    name: "Acme Corp",
    taxCode: "0101234567",
    address: "123 Main Street",
    invoiceEmail: "billing@acme.com",
  },
};

const mocks = vi.hoisted(() => ({
  updateBuyer: vi.fn(),
  updateInvoiceBuyer: vi.fn(),
  getOrderUsage: vi.fn(),
  workspaceId: "ws-456",
}));

vi.mock("@multica/core/api", async () => {
  const actual = await vi.importActual<any>("@multica/core/api");
  return {
    ...actual,
    api: {
      ...actual.api,
      updateWorkspaceOrderBuyer: (...args: any[]) => mocks.updateBuyer(...args),
      updateWorkspaceOrderInvoiceBuyer: (...args: any[]) =>
        mocks.updateInvoiceBuyer(...args),
      updateOrderBuyer: (...args: any[]) => mocks.updateBuyer(...args),
      getWorkspaceOrder: vi.fn(),
      getWorkspaceOrderUsage: (...args: any[]) => mocks.getOrderUsage(...args),
    },
  };
});

vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => ({
    id: mocks.workspaceId,
    slug: "acme",
    name: "Acme",
  }),
}));

const mockSearchParams = new URLSearchParams();
vi.mock("../navigation", () => ({
  useNavigation: () => ({
    pathname: "/acme/billing/order",
    searchParams: mockSearchParams,
    replace: vi.fn(),
    push: vi.fn(),
  }),
}));

describe("validateTaxCode", () => {
  it("refuses a 9-digit tax code", () => {
    expect(validateTaxCode("123456789")).toBe(false);
  });

  it("accepts a 10-digit tax code", () => {
    expect(validateTaxCode("0101234567")).toBe(true);
  });

  it("accepts a 13-digit tax code", () => {
    expect(validateTaxCode("0101234567001")).toBe(true);
    expect(validateTaxCode("0101234567-001")).toBe(true);
  });

  it("accepts an empty tax code", () => {
    expect(validateTaxCode("")).toBe(true);
    expect(validateTaxCode("   ")).toBe(true);
  });
});

describe("BillingOrderPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSearchParams.set("order_id", "order-123");
    mocks.updateBuyer.mockResolvedValue({ success: true });
  });

  it("renders the bank details, amounts with VAT, and transfer code", () => {
    renderWithProviders(<BillingOrderPage initialOrder={mockOrder} />);

    // Bank details
    expect(screen.getByText("Vietcombank")).toBeInTheDocument();
    expect(screen.getByText("CONG TY TNHH MULTICA")).toBeInTheDocument();
    expect(screen.getByText("9876543210")).toBeInTheDocument();

    // Transfer code
    expect(screen.getByText("MUL-789-XYZ")).toBeInTheDocument();

    // Transfer note instructions
    expect(
      screen.getAllByText("Include the transfer code in the transfer note.")
        .length,
    ).toBeGreaterThan(0);

    // Status
    expect(screen.getByText("Pending")).toBeInTheDocument();

    // Amount & VAT
    expect(screen.getByText("Subtotal")).toBeInTheDocument();
    expect(screen.getByText("VAT")).toBeInTheDocument();
    expect(screen.getAllByText("Total amount").length).toBeGreaterThan(0);

    // Non-usage order does not show usage breakdown
    expect(
      screen.queryByRole("heading", { name: "Usage breakdown" }),
    ).not.toBeInTheDocument();
    expect(mocks.getOrderUsage).not.toHaveBeenCalled();
  });

  it("copies the transfer code to the clipboard when copy button is clicked", async () => {
    const user = userEvent.setup();
    const writeTextMock = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText: writeTextMock },
      configurable: true,
      writable: true,
    });

    renderWithProviders(<BillingOrderPage initialOrder={mockOrder} />);

    const copyBtn = screen.getByRole("button", { name: "Copy code" });
    await user.click(copyBtn);

    expect(writeTextMock).toHaveBeenCalledWith("MUL-789-XYZ");
    expect(await screen.findByText("Copied")).toBeInTheDocument();
  });

  it("submits the buyer form with four fields", async () => {
    const user = userEvent.setup();
    renderWithProviders(<BillingOrderPage initialOrder={mockOrder} />);

    const nameInput = screen.getByLabelText("Name");
    const taxCodeInput = screen.getByLabelText("Tax code");
    const addressInput = screen.getByLabelText("Address");
    const emailInput = screen.getByLabelText("Invoice email");

    await user.clear(nameInput);
    await user.type(nameInput, "Globex Inc");

    await user.clear(taxCodeInput);
    await user.type(taxCodeInput, "0123456789");

    await user.clear(addressInput);
    await user.type(addressInput, "456 Market St");

    await user.clear(emailInput);
    await user.type(emailInput, "invoices@globex.com");

    const submitBtn = screen.getByRole("button", { name: "Save buyer details" });
    await user.click(submitBtn);

    await waitFor(() => {
      expect(mocks.updateBuyer).toHaveBeenCalled();
    });
    const call = mocks.updateBuyer.mock.calls[0]!;
    expect(call[0]).toBe("ws-456");
    expect(call[1]).toBe("order-123");
    const buyerArg = call[2];
    expect(buyerArg.name).toBe("Globex Inc");
    expect(buyerArg.taxCode ?? buyerArg.tax_code).toBe("0123456789");
    expect(buyerArg.address).toBe("456 Market St");
    expect(buyerArg.invoiceEmail ?? buyerArg.invoice_email).toBe("invoices@globex.com");

    expect(await screen.findByText("Buyer details saved")).toBeInTheDocument();
  });

  it("refuses a 9-digit tax code and shows validation error without submitting", async () => {
    const user = userEvent.setup();
    renderWithProviders(<BillingOrderPage initialOrder={mockOrder} />);

    const taxCodeInput = screen.getByLabelText("Tax code");
    await user.clear(taxCodeInput);
    await user.type(taxCodeInput, "123456789");

    const submitBtn = screen.getByRole("button", { name: "Save buyer details" });
    await user.click(submitBtn);

    expect(
      screen.getByText("Tax code must be 10 or 13 digits or empty"),
    ).toBeInTheDocument();
    expect(mocks.updateBuyer).not.toHaveBeenCalled();
  });

  it("handles 409 order_not_pending error when updating a non-pending order", async () => {
    const user = userEvent.setup();
    mocks.updateBuyer.mockRejectedValue(
      new ApiError("order not pending", 409, "Conflict", { code: "order_not_pending" }),
    );

    renderWithProviders(<BillingOrderPage initialOrder={mockOrder} />);

    const submitBtn = screen.getByRole("button", { name: "Save buyer details" });
    await user.click(submitBtn);

    expect(
      await screen.findByText("Only pending orders can be updated"),
    ).toBeInTheDocument();
  });

  it("disables buyer form inputs when order status is not pending", () => {
    const paidOrder = { ...mockOrder, status: "paid" };
    renderWithProviders(<BillingOrderPage initialOrder={paidOrder} />);

    expect(screen.getByLabelText("Name")).toBeDisabled();
    expect(screen.getByLabelText("Tax code")).toBeDisabled();
    expect(screen.getByLabelText("Address")).toBeDisabled();
    expect(screen.getByLabelText("Invoice email")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Save buyer details" })).toBeDisabled();
  });

  it("renders per-day usage lines for a usage order and omits expiry", async () => {
    mocks.getOrderUsage.mockResolvedValue({
      orderId: "order-usage-1",
      lines: [
        {
          day: "2026-08-03",
          meter: "tasks_run",
          quantity: 120,
          unitPrice: 30000,
          per: 1000,
          discountPct: 20,
          netAmount: 2880,
          note: "",
        },
      ],
    });
    const usageOrder: BillingOrder = {
      ...mockOrder,
      id: "order-usage-1",
      kind: "usage",
      planKey: "payg",
      seats: 0,
      expiresAt: null,
    };

    renderWithProviders(<BillingOrderPage initialOrder={usageOrder} />);

    expect(screen.queryByText(/Expires at/)).not.toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Usage breakdown" }),
    ).toBeInTheDocument();
    expect(await screen.findByText("Tasks run")).toBeInTheDocument();
    expect(screen.getByText("120")).toBeInTheDocument();
    expect(mocks.getOrderUsage).toHaveBeenCalledWith(
      "ws-456",
      "order-usage-1",
    );
  });
});

describe("BillingOrderPage e-invoice", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSearchParams.set("order_id", "order-123");
    mocks.updateInvoiceBuyer.mockResolvedValue({
      ...mockOrder,
      status: "paid",
      einvoice: {
        state: "pending",
        number: "",
        lookupUrl: "",
        missingFields: [],
      },
    });
  });

  it("shows nothing when einvoice is absent", () => {
    renderWithProviders(
      <BillingOrderPage
        initialOrder={{ ...mockOrder, status: "paid", einvoice: null }}
      />,
    );

    expect(screen.queryByRole("heading", { name: "E-invoice" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /View e-invoice/ })).not.toBeInTheDocument();
  });

  it("shows an https lookup link and number when einvoice is issued", () => {
    renderWithProviders(
      <BillingOrderPage
        initialOrder={{
          ...mockOrder,
          status: "paid",
          einvoice: {
            state: "issued",
            number: "00001234",
            lookupUrl: "https://einvoice.sepay.vn/lookup/abc",
            missingFields: [],
          },
        }}
      />,
    );

    expect(screen.getByRole("heading", { name: "E-invoice" })).toBeInTheDocument();
    expect(screen.getByText("Issued")).toBeInTheDocument();
    expect(screen.getByText("00001234")).toBeInTheDocument();
    const link = screen.getByRole("link", { name: /View e-invoice/ });
    expect(link).toHaveAttribute("href", "https://einvoice.sepay.vn/lookup/abc");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", expect.stringContaining("noopener"));
  });

  it("never renders a lookup link for non-https URLs", () => {
    for (const unsafeUrl of [
      "http://insecure.example.com/inv/1",
      "http://localhost:8080/inv/1",
      "javascript:alert(1)",
      "",
    ]) {
      const { unmount } = renderWithProviders(
        <BillingOrderPage
          initialOrder={{
            ...mockOrder,
            status: "paid",
            einvoice: {
              state: "issued",
              number: "00001234",
              lookupUrl: unsafeUrl,
              missingFields: [],
            },
          }}
        />,
      );

      expect(screen.getByText("Issued")).toBeInTheDocument();
      expect(screen.queryByRole("link", { name: /View e-invoice/ })).not.toBeInTheDocument();
      unmount();
    }
  });

  it("shows an editable buyer form for needs_info and calls PUT invoice-buyer", async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <BillingOrderPage
        initialOrder={{
          ...mockOrder,
          status: "paid",
          einvoice: {
            state: "needs_info",
            number: "",
            lookupUrl: "",
            missingFields: ["tax_code", "invoice_email"],
          },
        }}
      />,
    );

    expect(screen.getByText("Needs buyer details")).toBeInTheDocument();
    expect(screen.getByText(/Tax code, Invoice email/)).toBeInTheDocument();

    const taxInput = screen.getByLabelText("Tax code");
    expect(taxInput).not.toBeDisabled();
    await user.clear(taxInput);
    await user.type(taxInput, "0309998887");
    await user.click(screen.getByRole("button", { name: "Save buyer details" }));

    await waitFor(() => {
      expect(mocks.updateInvoiceBuyer).toHaveBeenCalledWith(
        "ws-456",
        "order-123",
        {
          name: "Acme Corp",
          taxCode: "0309998887",
          address: "123 Main Street",
          invoiceEmail: "billing@acme.com",
        },
      );
    });

    expect(await screen.findByText("Buyer details saved")).toBeInTheDocument();
    expect(screen.getByText("Pending")).toBeInTheDocument();
  });

  it("shows an editable buyer form for failed and handles 409 einvoice_already_issued", async () => {
    const user = userEvent.setup();
    mocks.updateInvoiceBuyer.mockRejectedValue(
      new ApiError("already issued", 409, "Conflict", {
        code: "einvoice_already_issued",
      }),
    );

    renderWithProviders(
      <BillingOrderPage
        initialOrder={{
          ...mockOrder,
          status: "paid",
          einvoice: {
            state: "failed",
            number: "",
            lookupUrl: "",
            missingFields: [],
          },
        }}
      />,
    );

    expect(screen.getByText("Failed")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Save buyer details" }));

    expect(
      await screen.findByText("This e-invoice has already been issued"),
    ).toBeInTheDocument();
  });

  it("shows status only without link or editable form for pending or cancelled states", () => {
    const { unmount } = renderWithProviders(
      <BillingOrderPage
        initialOrder={{
          ...mockOrder,
          status: "paid",
          einvoice: {
            state: "pending",
            number: "",
            lookupUrl: "",
            missingFields: [],
          },
        }}
      />,
    );
    expect(screen.getByText("Pending")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /View e-invoice/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save buyer details" })).toBeDisabled();
    unmount();

    renderWithProviders(
      <BillingOrderPage
        initialOrder={{
          ...mockOrder,
          status: "paid",
          einvoice: {
            state: "cancelled",
            number: "",
            lookupUrl: "",
            missingFields: [],
          },
        }}
      />,
    );
    expect(screen.getByText("Cancelled")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /View e-invoice/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save buyer details" })).toBeDisabled();
  });
});
