import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { renderWithI18n } from "../test/i18n";
import { UnpaidUsageBanner } from "./unpaid-usage-banner";

describe("UnpaidUsageBanner", () => {
  it.each([null, undefined, ""])(
    "is hidden when unpaidOrderId is %s",
    (unpaidOrderId) => {
      const { container } = renderWithI18n(
        <UnpaidUsageBanner unpaidOrderId={unpaidOrderId} />,
      );
      expect(container).toBeEmptyDOMElement();
    },
  );

  it("renders warning banner when unpaidOrderId is set and triggers view invoice", async () => {
    const user = userEvent.setup();
    const onViewInvoice = vi.fn();

    renderWithI18n(
      <UnpaidUsageBanner
        unpaidOrderId="ord-unpaid-1"
        onViewInvoice={onViewInvoice}
      />,
    );

    expect(screen.getByText("Unpaid usage invoice")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Pay the outstanding usage invoice before upgrading or changing to a paid plan.",
      ),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "View invoice" }));
    expect(onViewInvoice).toHaveBeenCalledWith("ord-unpaid-1");
  });
});
