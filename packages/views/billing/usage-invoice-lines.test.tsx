import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithI18n } from "../test/i18n";
import { UsageInvoiceLines } from "./usage-invoice-lines";

const mocks = vi.hoisted(() => ({
  getWorkspaceOrderUsage: vi.fn(),
}));

vi.mock("@multica/core/api", async () => {
  const actual = await vi.importActual<any>("@multica/core/api");
  return {
    ...actual,
    api: {
      ...actual.api,
      getWorkspaceOrderUsage: (...args: any[]) =>
        mocks.getWorkspaceOrderUsage(...args),
    },
  };
});

function renderWithQuery(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return renderWithI18n(
    <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>,
  );
}

describe("UsageInvoiceLines", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("loads and renders per-day usage invoice lines", async () => {
    mocks.getWorkspaceOrderUsage.mockResolvedValue({
      orderId: "ord-usage-1",
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
        {
          day: "2026-08-04",
          meter: "members",
          quantity: 10,
          unitPrice: 49000,
          per: 1,
          discountPct: 0,
          netAmount: 0,
          note: "workspace_suspended",
        },
      ],
    });

    renderWithQuery(
      <UsageInvoiceLines
        wsId="ws-1"
        orderId="ord-usage-1"
        currency="vnd"
      />,
    );

    expect(
      screen.getByRole("status", { name: "Loading usage breakdown" }),
    ).toBeInTheDocument();

    expect(await screen.findByText("Tasks run")).toBeInTheDocument();
    expect(screen.getByText("Members")).toBeInTheDocument();
    expect(screen.getByText("120")).toBeInTheDocument();
    expect(screen.getByText("-20%")).toBeInTheDocument();
    expect(screen.getByText("Workspace suspended")).toBeInTheDocument();
    expect(mocks.getWorkspaceOrderUsage).toHaveBeenCalledWith(
      "ws-1",
      "ord-usage-1",
    );
  });

  it("renders empty state when invoice has no lines", async () => {
    mocks.getWorkspaceOrderUsage.mockResolvedValue({
      orderId: "ord-usage-empty",
      lines: [],
    });

    renderWithQuery(
      <UsageInvoiceLines
        wsId="ws-1"
        orderId="ord-usage-empty"
        currency="vnd"
      />,
    );

    expect(
      await screen.findByText("No usage lines for this invoice."),
    ).toBeInTheDocument();
  });

  it("renders unavailable state and retries when response is null or fails", async () => {
    const user = userEvent.setup();
    mocks.getWorkspaceOrderUsage
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce({ orderId: "ord-usage-err", lines: [] });

    renderWithQuery(
      <UsageInvoiceLines
        wsId="ws-1"
        orderId="ord-usage-err"
        currency="vnd"
      />,
    );

    expect(
      await screen.findByText("Usage breakdown could not be loaded."),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(
      await screen.findByText("No usage lines for this invoice."),
    ).toBeInTheDocument();
  });
});
