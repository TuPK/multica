import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { WorkspacePendingPlanChange } from "@multica/core/types";
import { renderWithI18n } from "../test/i18n";
import { PendingChangeBanner } from "./pending-change-banner";

const pending: WorkspacePendingPlanChange = {
  planKey: "personal",
  interval: "year",
  seats: 2,
  requestedAt: "2030-01-15T00:00:00Z",
};

const EFFECTIVE_DATE = "2030-02-01T00:00:00Z";
const DATE_TEXT = new Intl.DateTimeFormat("en", {
  dateStyle: "medium",
}).format(new Date(EFFECTIVE_DATE));

describe("PendingChangeBanner", () => {
  it("renders nothing without a scheduled change", () => {
    const { container } = renderWithI18n(
      <PendingChangeBanner
        pendingChange={null}
        planName="Personal"
        effectiveAt={EFFECTIVE_DATE}
        canCancel
        cancelling={false}
        onCancel={vi.fn()}
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("shows the scheduled plan and date", () => {
    renderWithI18n(
      <PendingChangeBanner
        pendingChange={pending}
        planName="Personal"
        effectiveAt={EFFECTIVE_DATE}
        canCancel={false}
        cancelling={false}
        onCancel={vi.fn()}
      />,
    );
    expect(
      screen.getByText(`Your plan changes to Personal on ${DATE_TEXT}.`),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Cancel scheduled change" }),
    ).not.toBeInTheDocument();
  });

  it("cancels the scheduled change when the action is available", async () => {
    const onCancel = vi.fn();
    renderWithI18n(
      <PendingChangeBanner
        pendingChange={pending}
        planName="Personal"
        effectiveAt={EFFECTIVE_DATE}
        canCancel
        cancelling={false}
        onCancel={onCancel}
      />,
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Cancel scheduled change" }),
    );
    expect(onCancel).toHaveBeenCalledTimes(1);
  });
});
