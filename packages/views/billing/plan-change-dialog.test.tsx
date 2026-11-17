import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { WorkspacePlanChangePreview } from "@multica/core/types";
import { formatStripeMinorAmount } from "../settings/components/billing-format";
import { renderWithI18n } from "../test/i18n";
import { PlanChangeDialog } from "./plan-change-dialog";

const EFFECTIVE_AT = "2030-02-01T00:00:00Z";
const EFFECTIVE_DATE = new Intl.DateTimeFormat("en", {
  dateStyle: "medium",
}).format(new Date(EFFECTIVE_AT));
const AMOUNT = formatStripeMinorAmount(1200, "usd", "en");

function preview(
  mode: WorkspacePlanChangePreview["mode"],
): WorkspacePlanChangePreview {
  return {
    mode,
    planKey: "team",
    interval: "month",
    currentSeats: 3,
    resultingSeats: 5,
    currency: "usd",
    amount: 1200,
    vatAmount: 0,
    effectiveAt: EFFECTIVE_AT,
    expectedVersion: 4,
    quotedAt: "2030-01-15T00:00:00Z",
  };
}

function renderDialog(
  mode: WorkspacePlanChangePreview["mode"],
  onConfirm = vi.fn(),
) {
  renderWithI18n(
    <PlanChangeDialog
      open
      onOpenChange={vi.fn()}
      planName="Team"
      preview={preview(mode)}
      submitting={false}
      error={null}
      onConfirm={onConfirm}
    />,
  );
  return { onConfirm };
}

describe("PlanChangeDialog", () => {
  it("shows the amount due now for an immediate charge", () => {
    renderDialog("charge");
    expect(screen.getByText("Due now")).toBeInTheDocument();
    expect(screen.getByText(AMOUNT!)).toBeInTheDocument();
    expect(screen.queryByText(/Takes effect on/)).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Continue to payment" }),
    ).toBeInTheDocument();
  });

  it("shows the amount due now when a free workspace subscribes", () => {
    renderDialog("subscribe");
    expect(screen.getByText(AMOUNT!)).toBeInTheDocument();
    expect(screen.queryByText(/Takes effect on/)).not.toBeInTheDocument();
  });

  it("shows the effective date and no amount for a scheduled change", () => {
    renderDialog("schedule");
    expect(
      screen.getByText(`Takes effect on ${EFFECTIVE_DATE}`),
    ).toBeInTheDocument();
    expect(screen.queryByText("Due now")).not.toBeInTheDocument();
    expect(screen.queryByText(AMOUNT!)).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Schedule change" }),
    ).toBeInTheDocument();
  });

  it("confirms the change once", async () => {
    const { onConfirm } = renderDialog("schedule");
    await userEvent.click(
      screen.getByRole("button", { name: "Schedule change" }),
    );
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });
});
