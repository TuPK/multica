import { screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { WorkspaceUsagePeriod } from "@multica/core/types";
import { renderWithI18n } from "../test/i18n";
import { UsageCard } from "./usage-card";

function makeUsagePeriod(
  overrides: Partial<WorkspaceUsagePeriod> = {},
): WorkspaceUsagePeriod {
  return {
    periodStart: "2026-08-01T00:00:00Z",
    periodEnd: "2026-09-01T00:00:00Z",
    currency: "vnd",
    accruedAmount: 1_000_000,
    creditLimit: 2_000_000,
    unpaidOrderId: null,
    meters: [
      {
        key: "tasks_run",
        quantity: 2000,
        unitPrice: 30000,
        per: 1000,
        discountPct: 0,
        amount: 60000,
      },
      {
        key: "storage_bytes",
        quantity: 2147483648,
        unitPrice: 3000,
        per: 1073741824,
        discountPct: 0,
        amount: 6000,
      },
      {
        key: "members",
        quantity: 10,
        unitPrice: 49000,
        per: 1,
        discountPct: 0,
        amount: 934000,
      },
    ],
    ...overrides,
  };
}

describe("UsageCard", () => {
  it("renders empty state when usage is null", () => {
    renderWithI18n(<UsageCard usage={null} />);

    expect(
      screen.getByRole("heading", { name: "Usage this period" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("No usage recorded for this period yet."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    expect(
      screen.queryByText("Approaching credit limit"),
    ).not.toBeInTheDocument();
  });

  it("renders empty state when usage has no meters", () => {
    renderWithI18n(
      <UsageCard usage={makeUsagePeriod({ accruedAmount: 0, meters: [] })} />,
    );

    expect(
      screen.getByText("No usage recorded for this period yet."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    expect(
      screen.queryByText("Approaching credit limit"),
    ).not.toBeInTheDocument();
  });

  it("renders meters, accrued amount, and credit limit bar under 80%", () => {
    renderWithI18n(
      <UsageCard usage={makeUsagePeriod({ accruedAmount: 1_599_999 })} />,
    );

    expect(screen.getByText("Tasks run")).toBeInTheDocument();
    expect(screen.getByText("Storage")).toBeInTheDocument();
    expect(screen.getByText("Members")).toBeInTheDocument();
    expect(screen.getByText(/2,000 ·/)).toBeInTheDocument();
    expect(screen.getByText(/2 GiB ·/)).toBeInTheDocument();
    expect(screen.getByText("Accrued this period")).toBeInTheDocument();
    expect(screen.getByText("Credit limit")).toBeInTheDocument();
    expect(screen.getByRole("progressbar")).toBeInTheDocument();
    expect(
      screen.queryByText("Approaching credit limit"),
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/Business discount/i)).not.toBeInTheDocument();
  });

  it.each([
    [1_600_000, "80%"],
    [1_800_000, "90%"],
  ])(
    "shows credit limit warning when accrued amount is at %i (%s)",
    (accruedAmount, expectedPct) => {
      renderWithI18n(
        <UsageCard usage={makeUsagePeriod({ accruedAmount })} />,
      );

      expect(
        screen.getByText("Approaching credit limit"),
      ).toBeInTheDocument();
      expect(
        screen.getByText(new RegExp(expectedPct)),
      ).toBeInTheDocument();
    },
  );

  it("renders error state with retry action", () => {
    const onRetry = vi.fn();
    renderWithI18n(<UsageCard usage={null} error onRetry={onRetry} />);

    expect(
      screen.getByText("Usage details are temporarily unavailable."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });

  it("renders discount badges when discountPct > 0 and falls back for unknown meters", () => {
    renderWithI18n(
      <UsageCard
        usage={makeUsagePeriod({
          meters: [
            {
              key: "tasks_run",
              quantity: 1000,
              unitPrice: 30000,
              per: 1000,
              discountPct: 20,
              amount: 24000,
            },
            {
              key: "custom_meter",
              quantity: 5,
              unitPrice: 1000,
              per: 1,
              discountPct: 0,
              amount: 5000,
            },
          ],
        })}
      />,
    );

    expect(screen.getByText("-20%")).toBeInTheDocument();
    expect(screen.getByText("Unknown meter")).toBeInTheDocument();
    expect(screen.queryByText(/Business discount/i)).not.toBeInTheDocument();
  });

  it("omits rates and credit limit progress when unitPrice and creditLimit are 0", () => {
    renderWithI18n(
      <UsageCard
        usage={makeUsagePeriod({
          creditLimit: 0,
          accruedAmount: 60000,
          meters: [
            {
              key: "tasks_run",
              quantity: 2000,
              unitPrice: 0,
              per: 1,
              discountPct: 0,
              amount: 60000,
            },
          ],
        })}
      />,
    );

    expect(screen.getByText("Tasks run")).toBeInTheDocument();
    expect(screen.getByText("2,000")).toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    expect(
      screen.queryByText("Approaching credit limit"),
    ).not.toBeInTheDocument();
  });
});
